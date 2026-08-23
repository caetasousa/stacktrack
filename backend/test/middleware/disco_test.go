// O porteiro do disco: recusa mutação, mantém leitura.
package middleware_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"stacktrack/internal/adapter/armazem"
	"stacktrack/internal/adapter/http/middleware"
)

func mudo() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// guardaSemMargem monta um porteiro que nunca aceita escrita: o piso
// percentual em 100% torna qualquer disco "sem margem", sem precisar encher
// disco nenhum no teste.
func guardaSemMargem(t *testing.T) *armazem.Guarda {
	t.Helper()
	return armazem.NovaGuarda(t.TempDir(), 0, 100, time.Millisecond)
}

// guardaComMargem monta um porteiro que sempre aceita.
func guardaComMargem(t *testing.T) *armazem.Guarda {
	t.Helper()
	return armazem.NovaGuarda(t.TempDir(), 0, 0, time.Millisecond)
}

func chamarRotaComPorteiro(guarda *armazem.Guarda, metodo, rota string) int {
	rec := httptest.NewRecorder()
	middleware.PorteiroDeDisco(guarda, mudo())(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	).ServeHTTP(rec, httptest.NewRequest(metodo, rota, nil))
	return rec.Code
}

func chamarComPorteiro(guarda *armazem.Guarda, metodo string) int {
	return chamarRotaComPorteiro(guarda, metodo, "/boards")
}

// 507, e não 503: o servidor está no ar e o que falta é ESPAÇO. Um 503 diria
// "tente de novo já", e tentar de novo não resolve até alguém liberar disco.
func TestSemMargemDeDiscoAMutacaoRecebe507(t *testing.T) {
	guarda := guardaSemMargem(t)
	for _, metodo := range []string{http.MethodPost, http.MethodPatch, http.MethodPut} {
		if codigo := chamarComPorteiro(guarda, metodo); codigo != http.StatusInsufficientStorage {
			t.Errorf("%s: status = %d, esperado 507", metodo, codigo)
		}
	}
}

// DELETE precisa continuar justamente na emergencia: anexos, cards e quadros
// sao os meios que a aplicacao oferece para devolver espaco ao volume.
func TestSemMargemDeDiscoPermiteExclusaoParaRecuperar(t *testing.T) {
	if codigo := chamarComPorteiro(guardaSemMargem(t), http.MethodDelete); codigo != http.StatusOK {
		t.Errorf("DELETE: status = %d, esperado 200", codigo)
	}
}

func TestSemMargemPermiteLoginELogoutMasBloqueiaCadastro(t *testing.T) {
	guarda := guardaSemMargem(t)
	for _, rota := range []string{"/auth/login", "/auth/logout"} {
		if codigo := chamarRotaComPorteiro(guarda, http.MethodPost, rota); codigo != http.StatusOK {
			t.Errorf("POST %s: status = %d, esperado 200", rota, codigo)
		}
	}
	if codigo := chamarRotaComPorteiro(guarda, http.MethodPost, "/auth/cadastro"); codigo != http.StatusInsufficientStorage {
		t.Errorf("cadastro: status = %d, esperado 507", codigo)
	}
}

// A LEITURA continua. É a razão de o porteiro filtrar por método em vez de
// derrubar a instância inteira: quem precisa decidir o que apagar tem de
// conseguir ver o que está lá.
func TestSemMargemDeDiscoALeituraContinua(t *testing.T) {
	guarda := guardaSemMargem(t)
	for _, metodo := range []string{http.MethodGet, http.MethodHead} {
		if codigo := chamarComPorteiro(guarda, metodo); codigo != http.StatusOK {
			t.Errorf("%s: status = %d, esperado 200 — a leitura não pode parar", metodo, codigo)
		}
	}
}

// OPTIONS passa: é o preflight do CORS, e recusá-lo faria o navegador esconder
// o 507 atrás de um erro genérico de rede.
func TestPreflightDoCORSNaoEhBarradoPeloDisco(t *testing.T) {
	if codigo := chamarComPorteiro(guardaSemMargem(t), http.MethodOptions); codigo != http.StatusOK {
		t.Errorf("OPTIONS: status = %d, esperado 200", codigo)
	}
}

func TestComMargemDeDiscoTudoPassa(t *testing.T) {
	guarda := guardaComMargem(t)
	for _, metodo := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		if codigo := chamarComPorteiro(guarda, metodo); codigo != http.StatusOK {
			t.Errorf("%s: status = %d, esperado 200", metodo, codigo)
		}
	}
}

// Guarda nil desliga o porteiro — é o que os testes e o desenvolvimento querem.
func TestPorteiroDesligadoDeixaTudoPassar(t *testing.T) {
	rec := httptest.NewRecorder()
	middleware.PorteiroDeDisco(nil, mudo())(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
	).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/boards", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, esperado 200", rec.Code)
	}
}

// Falha ao MEDIR não bloqueia: é problema do instrumento, não do espaço.
// Transformar "não consegui olhar" em "recuso tudo" derrubaria o produto sem
// que faltasse disco nenhum.
func TestFalhaAoMedirODiscoNaoBloqueiaAEscrita(t *testing.T) {
	// Caminho que não existe: a medição falha.
	guarda := armazem.NovaGuarda("/nao/existe/em/lugar/nenhum", 1<<40, 99, time.Millisecond)

	if codigo := chamarComPorteiro(guarda, http.MethodPost); codigo != http.StatusOK {
		t.Errorf("status = %d, esperado 200: a medição falhou e isso não é falta de espaço", codigo)
	}
}

// O piso ABSOLUTO e o PERCENTUAL valem juntos, e prevalece o mais conservador.
// Só o percentual falharia num volume de 2 TB; só o absoluto falharia num
// volume pequeno, onde 2 GiB podem ser metade do disco.
func TestPisoAbsolutoBloqueiaMesmoComPercentualFolgado(t *testing.T) {
	// Piso absurdo em bytes, percentual desligado.
	guarda := armazem.NovaGuarda(t.TempDir(), 1<<62, 0, time.Millisecond)

	if codigo := chamarComPorteiro(guarda, http.MethodPost); codigo != http.StatusInsufficientStorage {
		t.Errorf("status = %d, esperado 507", codigo)
	}
}
