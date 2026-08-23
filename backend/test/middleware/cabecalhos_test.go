package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"stacktrack/internal/adapter/http/middleware"
)

type writerComPrazoDeLeitura struct {
	*httptest.ResponseRecorder
	prazosDeLeitura []time.Time
	prazosDeEscrita []time.Time
}

func (w *writerComPrazoDeLeitura) SetReadDeadline(prazo time.Time) error {
	w.prazosDeLeitura = append(w.prazosDeLeitura, prazo)
	return nil
}

func (w *writerComPrazoDeLeitura) SetWriteDeadline(prazo time.Time) error {
	w.prazosDeEscrita = append(w.prazosDeEscrita, prazo)
	return nil
}

// respostaCom devolve os cabeçalhos que o middleware deixou na resposta.
func respostaCom(mw func(http.Handler) http.Handler) http.Header {
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boards", nil))
	return rec.Header()
}

// no-store, e não no-cache: o no-cache permite GRAVAR a resposta e só obriga a
// revalidar, o que já deixa o conteúdo privado em disco no caminho.
func TestSemCacheMarcaARespostaComoNaoArmazenavel(t *testing.T) {
	if got := respostaCom(middleware.SemCache).Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, esperado no-store", got)
	}
}

// O cabeçalho precisa ser escrito ANTES do handler: depois de o corpo começar a
// sair, um Set não vai para o fio e o teste passaria sem o cabeçalho existir.
func TestSemCacheValeMesmoComHandlerQueEscreveOCorpo(t *testing.T) {
	rec := httptest.NewRecorder()
	middleware.SemCache(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("corpo primeiro"))
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q: o cabeçalho saiu depois do corpo", got)
	}
}

// A URL do convite carrega o segredo. Sem esta política, qualquer recurso
// externo naquela página manda o token inteiro no Referer para um domínio de
// fora — e o convite vaza pelo log de acesso de outra pessoa.
func TestSemReferrerImpedeVazamentoDeTokenPeloReferer(t *testing.T) {
	if got := respostaCom(middleware.SemReferrer).Get("Referrer-Policy"); got != "no-referrer" {
		t.Errorf("Referrer-Policy = %q, esperado no-referrer", got)
	}
}

// O prazo da requisição vive no CONTEXTO, e não num timer que responde por
// fora. A diferença é o que faz o pgx abortar a query e devolver a conexão ao
// pool: um timeout que só escrevesse a resposta deixaria o trabalho rodando
// atrás dela, segurando exatamente o recurso que o teto protege.
func TestPrazoPoeDeadlineNoContextoDaRequisicao(t *testing.T) {
	var prazo time.Time
	var tinha bool
	mw := middleware.Prazo(50*time.Millisecond, time.Minute)
	mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		prazo, tinha = r.Context().Deadline()
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/boards", nil))

	if !tinha {
		t.Fatal("a requisição chegou ao handler sem deadline")
	}
	if restante := time.Until(prazo); restante > 60*time.Millisecond {
		t.Errorf("deadline em %v, esperado ~50ms", restante)
	}
}

// O upload tem orçamento PRÓPRIO. Herdar o teto comum recusaria envios
// legítimos: dez megabytes numa conexão móvel ruim passam de dez segundos sem
// nada de errado acontecendo.
func TestUploadRecebeUmPrazoMaior(t *testing.T) {
	var prazo time.Time
	mw := middleware.Prazo(50*time.Millisecond, 10*time.Second)
	req := httptest.NewRequest(http.MethodPost, "/cards/x/anexos/arquivo", nil)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=abc")
	mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		prazo, _ = r.Context().Deadline()
	})).ServeHTTP(httptest.NewRecorder(), req)

	if restante := time.Until(prazo); restante < time.Second {
		t.Errorf("deadline em %v: o upload herdou o teto comum", restante)
	}
}

// O handshake do WebSocket recebe deadline: antes do 101 ele ainda paga
// autenticacao, autorizacao e reserva de vaga. O handler destaca o contexto da
// conexao longa somente depois de aceitar o upgrade.
func TestWebSocketRecebeDeadlineDuranteOHandshake(t *testing.T) {
	var tinha bool
	req := httptest.NewRequest(http.MethodGet, "/ws?board=x", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	middleware.Prazo(50*time.Millisecond, time.Minute)(
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			_, tinha = r.Context().Deadline()
		}),
	).ServeHTTP(httptest.NewRecorder(), req)

	if !tinha {
		t.Error("o handshake do WebSocket chegou sem deadline")
	}
}

// Cabeçalhos de upgrade em outra rota nao podem desligar a protecao. Antes,
// qualquer POST podia se declarar WebSocket e ficar sem teto de leitura.
func TestCabecalhoDeUpgradeNaoContornaOPrazo(t *testing.T) {
	var tinha bool
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	middleware.Prazo(50*time.Millisecond, time.Minute)(
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			_, tinha = r.Context().Deadline()
		}),
	).ServeHTTP(httptest.NewRecorder(), req)
	if !tinha {
		t.Error("cabecalhos de upgrade desligaram o prazo fora do endpoint WebSocket")
	}
}

// Context cancellation alone does not interrupt a goroutine blocked reading
// the body. The socket read deadline is the part that cuts a slow upload.
func TestPrazoTambemLimitaLeituraDoCorpo(t *testing.T) {
	w := &writerComPrazoDeLeitura{ResponseRecorder: httptest.NewRecorder()}
	middleware.Prazo(50*time.Millisecond, time.Minute)(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/boards", nil))

	if len(w.prazosDeLeitura) < 2 {
		t.Fatalf("SetReadDeadline chamado %d vez(es), esperado aplicar e limpar", len(w.prazosDeLeitura))
	}
	if w.prazosDeLeitura[0].IsZero() {
		t.Error("o prazo inicial de leitura ficou zerado")
	}
	if !w.prazosDeLeitura[len(w.prazosDeLeitura)-1].IsZero() {
		t.Error("o prazo de leitura nao foi limpo ao terminar a requisicao")
	}
	if len(w.prazosDeEscrita) < 2 || w.prazosDeEscrita[0].IsZero() ||
		!w.prazosDeEscrita[len(w.prazosDeEscrita)-1].IsZero() {
		t.Errorf("prazos de escrita = %v, esperado aplicar e limpar", w.prazosDeEscrita)
	}
}

// Prazo estourado vira 503 com Retry-After — não 500. O servidor não falhou,
// ele desistiu de esperar, e repetir é a ação certa.
func TestTempoEsgotadoResponde503(t *testing.T) {
	rec := httptest.NewRecorder()
	middleware.Prazo(time.Millisecond, time.Minute)(
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done() // o handler respeita o cancelamento e sai
		}),
	).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boards", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, esperado 503", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("503 de prazo sem Retry-After")
	}
}

// Handler que JÁ respondeu não é sobrescrito: trocar o status depois de o corpo
// começar a sair produziria uma resposta corrompida no lugar de uma truncada.
func TestTempoEsgotadoNaoSobrescreveRespostaJaEscrita(t *testing.T) {
	rec := httptest.NewRecorder()
	middleware.Prazo(time.Millisecond, time.Minute)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"ok":true}`))
			<-r.Context().Done()
		}),
	).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boards", nil))

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, esperado 201: a resposta já escrita foi sobrescrita", rec.Code)
	}
}
