// Testes do adaptador de WebSocket — o protocolo, sem navegador e sem stack no ar.
//
// Ficam na suíte padrão de propósito. O teste de duas abas (duas_abas_test.go)
// cobre o caminho completo, mas exige a API rodando, então fica atrás de build
// tag e não roda no dia a dia. O resultado era que a peça mais delicada do
// projeto — reposição do backlog, filtro de eco, revalidação de acesso — só era
// exercitada por quem lembrasse de subir a stack.
//
// Aqui o handler roda sobre httptest e o cliente é o mesmo coder/websocket do
// outro lado, então o que se prova é o protocolo de verdade.
package realtime_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"stacktrack/internal/adapter/http/ws"
	"stacktrack/internal/adapter/realtime/hub"
	"stacktrack/internal/domain/evento"
)

// --- dublês -----------------------------------------------------------------

// historicoFalso é o log do quadro em memória.
type historicoFalso struct {
	eventos []evento.Evento
	erro    error
}

func (h *historicoFalso) Desde(_ context.Context, boardID string, seq int64, limite int) ([]evento.Evento, error) {
	if h.erro != nil {
		return nil, h.erro
	}
	var fora []evento.Evento
	for _, e := range h.eventos {
		if e.BoardID == boardID && e.Seq > seq {
			fora = append(fora, e)
			if len(fora) == limite {
				break
			}
		}
	}
	return fora, nil
}

func (h *historicoFalso) UltimoSeq(_ context.Context, boardID string) (int64, error) {
	if h.erro != nil {
		return 0, h.erro
	}
	var maior int64
	for _, e := range h.eventos {
		if e.BoardID == boardID && e.Seq > maior {
			maior = e.Seq
		}
	}
	return maior, nil
}

// autorizadorFalso decide quem participa de qual quadro, e permite revogar o
// acesso no meio do teste — que é o que a revalidação precisa enxergar.
//
// O mutex não é zelo excessivo: o teste revoga a partir da goroutine dele
// enquanto o handler consulta da goroutine da conexão, e sem sincronizar isso
// o -race reprova (com razão).
type autorizadorFalso struct {
	mu        sync.RWMutex
	proibidos map[string]bool
}

func (a *autorizadorFalso) PodeVer(_ context.Context, boardID, usuarioID string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return !a.proibidos[boardID+"|"+usuarioID]
}

func (a *autorizadorFalso) revogar(boardID, usuarioID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.proibidos == nil {
		a.proibidos = map[string]bool{}
	}
	a.proibidos[boardID+"|"+usuarioID] = true
}

// mensagemRecebida espelha o formato que viaja no fio (ver adapter/http/ws).
type mensagemRecebida struct {
	Seq     int64       `json:"seq"`
	Tipo    evento.Tipo `json:"tipo"`
	BoardID string      `json:"boardId"`
	AutorID string      `json:"autorId"`
}

// --- montagem ---------------------------------------------------------------

type cenarioWS struct {
	servidor    *httptest.Server
	hub         *hub.Hub
	historico   *historicoFalso
	autorizador *autorizadorFalso
	// usuario é quem o Identificador devolve; trocar aqui troca quem conecta.
	usuario string
	// autenticado em false faz o Identificador falhar, como sessão ausente.
	autenticado bool
	// sessaoOk alimenta a reconferência de sessão do handler. Atômico porque o
	// teste do logout o desliga com a conexão já aberta, de outra goroutine.
	sessaoOk atomic.Bool
}

func montarWS(t *testing.T, ajustar func(*cenarioWS)) *cenarioWS {
	t.Helper()

	c := &cenarioWS{
		hub:         hub.Novo(),
		historico:   &historicoFalso{},
		autorizador: &autorizadorFalso{},
		usuario:     "ana",
		autenticado: true,
	}
	c.sessaoOk.Store(true)
	if ajustar != nil {
		ajustar(c)
	}

	// Silencia o log do handler: o que ele registra não é o objeto do teste, e
	// o ruído esconderia a falha de verdade.
	mudo := slog.New(slog.NewTextHandler(io.Discard, nil))

	h := ws.NovoHandler(
		c.hub, c.historico, c.autorizador,
		func(context.Context) (string, bool) { return c.usuario, c.autenticado },
		func(context.Context, string) string { return "Alguém" },
		func(context.Context, string) bool { return c.sessaoOk.Load() },
		"sessao",
		[]string{"exemplo.test"},
		mudo,
	).ComIntervaloDeRevalidacao(50 * time.Millisecond)

	c.servidor = httptest.NewServer(http.HandlerFunc(h.Acompanhar))
	t.Cleanup(func() {
		c.hub.Fechar()
		c.servidor.Close()
	})
	return c
}

// conectar abre o WebSocket e devolve a conexão já pronta para leitura.
func (c *cenarioWS) conectar(t *testing.T, consulta string) *websocket.Conn {
	t.Helper()
	endereco := "ws" + strings.TrimPrefix(c.servidor.URL, "http") + consulta

	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()

	conexao, _, err := websocket.Dial(ctx, endereco, nil)
	if err != nil {
		t.Fatalf("não conectou em %s: %v", consulta, err)
	}
	t.Cleanup(func() { conexao.CloseNow() })
	return conexao
}

// receber lê a próxima mensagem, falhando se ela não vier a tempo.
func receber(t *testing.T, c *websocket.Conn) mensagemRecebida {
	t.Helper()
	ctx, cancelar := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelar()

	_, dados, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("não recebeu mensagem: %v", err)
	}
	var m mensagemRecebida
	if err := json.Unmarshal(dados, &m); err != nil {
		t.Fatalf("mensagem ilegível (%s): %v", dados, err)
	}
	return m
}

// receberPulandoPresenca ignora os avisos de presença, que chegam sozinhos a
// cada entrada e saída e não são o objeto destes testes.
func receberPulandoPresenca(t *testing.T, c *websocket.Conn) mensagemRecebida {
	t.Helper()
	for {
		if m := receber(t, c); m.Tipo != evento.PresencaAlterada {
			return m
		}
	}
}

// --- handshake --------------------------------------------------------------

func TestWSRecusaQuemNaoEstaAutenticado(t *testing.T) {
	c := montarWS(t, func(c *cenarioWS) { c.autenticado = false })

	resposta := chamarHTTP(t, c, "/?board=quadro-1")
	if resposta != http.StatusUnauthorized {
		t.Errorf("status = %d, esperado 401", resposta)
	}
}

func TestWSExigeOQuadro(t *testing.T) {
	c := montarWS(t, nil)

	if resposta := chamarHTTP(t, c, "/"); resposta != http.StatusBadRequest {
		t.Errorf("status = %d, esperado 400", resposta)
	}
}

// Quem não participa recebe 404, e não 403: um 403 confirmaria que o quadro
// existe, que já é informação sobre dado alheio.
func TestWSResponde404AQuemNaoParticipa(t *testing.T) {
	c := montarWS(t, func(c *cenarioWS) { c.autorizador.revogar("quadro-1", "ana") })

	if resposta := chamarHTTP(t, c, "/?board=quadro-1"); resposta != http.StatusNotFound {
		t.Errorf("status = %d, esperado 404", resposta)
	}
}

// WebSocket NÃO obedece CORS: sem a checagem de origem, qualquer site que a
// vítima visitasse abriria uma conexão autenticada com o cookie dela.
func TestWSRecusaOrigemDesconhecida(t *testing.T) {
	c := montarWS(t, nil)
	endereco := "ws" + strings.TrimPrefix(c.servidor.URL, "http") + "/?board=quadro-1"

	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()

	conexao, _, err := websocket.Dial(ctx, endereco, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"https://site-do-atacante.test"}},
	})
	if err == nil {
		conexao.CloseNow()
		t.Fatal("aceitou conexão de uma origem que não está na lista")
	}
}

func chamarHTTP(t *testing.T, c *cenarioWS, caminho string) int {
	t.Helper()
	resposta, err := http.Get(c.servidor.URL + caminho)
	if err != nil {
		t.Fatalf("erro na requisição: %v", err)
	}
	defer resposta.Body.Close()
	return resposta.StatusCode
}

// --- primeira conexão e ao vivo ---------------------------------------------

// Sem `desde`, o cliente recebe só a posição atual da história — é dela que
// parte a primeira reconexão.
func TestWSPrimeiraConexaoRecebeAPosicaoAtual(t *testing.T) {
	c := montarWS(t, func(c *cenarioWS) {
		c.historico.eventos = []evento.Evento{
			{Seq: 7, Tipo: evento.CardMovido, BoardID: "quadro-1", AutorID: "bob"},
		}
	})
	conexao := c.conectar(t, "/?board=quadro-1")

	m := receberPulandoPresenca(t, conexao)
	if m.Tipo != evento.Sincronizado || m.Seq != 7 {
		t.Errorf("recebeu %s com seq %d, esperado sincronizado com 7", m.Tipo, m.Seq)
	}
}

func TestWSEntregaEventoAoVivo(t *testing.T) {
	c := montarWS(t, nil)
	conexao := c.conectar(t, "/?board=quadro-1")
	receberPulandoPresenca(t, conexao) // o sincronizado inicial

	c.hub.Publicar(evento.Novo(evento.CardMovido, "quadro-1", "bob", nil))

	if m := receberPulandoPresenca(t, conexao); m.Tipo != evento.CardMovido {
		t.Errorf("recebeu %s, esperado card.movido", m.Tipo)
	}
}

// O próprio autor não recebe o eco: ele já mexeu na tela quando agiu, e
// reaplicar daria um solavanco no que acabou de fazer.
func TestWSNaoDevolveOEcoAoProprioAutor(t *testing.T) {
	c := montarWS(t, nil)
	conexao := c.conectar(t, "/?board=quadro-1")
	receberPulandoPresenca(t, conexao)

	c.hub.Publicar(evento.Novo(evento.CardMovido, "quadro-1", "ana", nil)) // a própria ana
	c.hub.Publicar(evento.Novo(evento.ColunaMovida, "quadro-1", "bob", nil))

	// O primeiro a chegar tem de ser o do bob: o da ana foi filtrado.
	if m := receberPulandoPresenca(t, conexao); m.Tipo != evento.ColunaMovida {
		t.Errorf("recebeu %s, esperado coluna.movida — o eco da própria ana vazou", m.Tipo)
	}
}

// --- reposição do backlog ---------------------------------------------------

func TestWSReponOQueSePerdeu(t *testing.T) {
	c := montarWS(t, func(c *cenarioWS) {
		c.historico.eventos = []evento.Evento{
			{Seq: 1, Tipo: evento.CardCriado, BoardID: "quadro-1", AutorID: "bob"},
			{Seq: 2, Tipo: evento.CardMovido, BoardID: "quadro-1", AutorID: "bob"},
			{Seq: 3, Tipo: evento.ColunaCriada, BoardID: "quadro-1", AutorID: "bob"},
		}
	})
	conexao := c.conectar(t, "/?board=quadro-1&desde=1")

	// Pede a partir do 1: recebe o 2 e o 3, nessa ordem, e nunca o 1.
	if m := receberPulandoPresenca(t, conexao); m.Seq != 2 {
		t.Fatalf("primeiro reposto tem seq %d, esperado 2", m.Seq)
	}
	if m := receberPulandoPresenca(t, conexao); m.Seq != 3 {
		t.Fatalf("segundo reposto tem seq %d, esperado 3", m.Seq)
	}
}

// O eco vale também no passado: reconectar não pode devolver à pessoa o que ela
// mesma fez enquanto estava fora.
func TestWSBacklogNaoDevolveOEcoDoProprioAutor(t *testing.T) {
	c := montarWS(t, func(c *cenarioWS) {
		c.historico.eventos = []evento.Evento{
			{Seq: 1, Tipo: evento.CardCriado, BoardID: "quadro-1", AutorID: "ana"},
			{Seq: 2, Tipo: evento.CardMovido, BoardID: "quadro-1", AutorID: "bob"},
		}
	})
	conexao := c.conectar(t, "/?board=quadro-1&desde=0")

	m := receberPulandoPresenca(t, conexao)
	if m.AutorID == "ana" {
		t.Fatalf("repôs o próprio evento da ana (seq %d)", m.Seq)
	}
	if m.Seq != 2 {
		t.Errorf("primeiro reposto tem seq %d, esperado 2", m.Seq)
	}
}

// Mesmo quando TUDO no intervalo é da própria pessoa, o cliente precisa saber
// até onde a história andou — senão ele pediria o mesmo intervalo para sempre,
// e ele só cresceria até estourar o teto e forçar uma recarga completa.
func TestWSBacklogSoDoAutorAindaFechaOIntervalo(t *testing.T) {
	c := montarWS(t, func(c *cenarioWS) {
		c.historico.eventos = []evento.Evento{
			{Seq: 1, Tipo: evento.CardCriado, BoardID: "quadro-1", AutorID: "ana"},
			{Seq: 2, Tipo: evento.CardMovido, BoardID: "quadro-1", AutorID: "ana"},
		}
	})
	conexao := c.conectar(t, "/?board=quadro-1&desde=0")

	m := receberPulandoPresenca(t, conexao)
	if m.Tipo != evento.Sincronizado || m.Seq != 2 {
		t.Errorf("recebeu %s com seq %d, esperado sincronizado com 2", m.Tipo, m.Seq)
	}
}

// Intervalo grande demais não é reproduzido: mandar recarregar tudo é mais
// barato que montar centenas de eventos em memória, e é sempre correto.
func TestWSBacklogGrandeDemaisMandaRecarregarTudo(t *testing.T) {
	c := montarWS(t, func(c *cenarioWS) {
		for i := 1; i <= 300; i++ {
			c.historico.eventos = append(c.historico.eventos, evento.Evento{
				Seq: int64(i), Tipo: evento.CardMovido, BoardID: "quadro-1", AutorID: "bob",
			})
		}
	})
	conexao := c.conectar(t, "/?board=quadro-1&desde=0")

	m := receberPulandoPresenca(t, conexao)
	if m.Tipo != evento.RecarregueTudo {
		t.Errorf("recebeu %s, esperado recarregue.tudo", m.Tipo)
	}
}

// --- revalidação de acesso --------------------------------------------------

// O handshake autoriza uma vez; a conexão dura horas. Quem é removido do quadro
// precisa parar de receber sem depender de fechar a aba.
func TestWSDerrubaQuemPerdeuOAcessoAoQuadro(t *testing.T) {
	c := montarWS(t, nil)
	conexao := c.conectar(t, "/?board=quadro-1")
	receberPulandoPresenca(t, conexao)

	c.autorizador.revogar("quadro-1", "ana")

	esperarConexaoCair(t, conexao, "a remoção do membro")
}

// Mesma coisa para o logout: a sessão morre no banco e o socket não pode
// continuar transmitindo.
func TestWSDerrubaQuemEncerrouASessao(t *testing.T) {
	c := montarWS(t, nil)
	conexao := c.conectar(t, "/?board=quadro-1")
	receberPulandoPresenca(t, conexao)

	c.sessaoOk.Store(false)

	esperarConexaoCair(t, conexao, "o logout")
}

func esperarConexaoCair(t *testing.T, c *websocket.Conn, porque string) {
	t.Helper()
	ctx, cancelar := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelar()

	for {
		if _, _, err := c.Read(ctx); err != nil {
			return // caiu, que é o esperado
		}
		if ctx.Err() != nil {
			t.Fatalf("a conexão continuou de pé depois de %s", porque)
		}
	}
}
