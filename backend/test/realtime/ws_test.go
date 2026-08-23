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
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	httpmiddleware "stacktrack/internal/adapter/http/middleware"
	"stacktrack/internal/adapter/http/ws"
	"stacktrack/internal/adapter/realtime/hub"
	"stacktrack/internal/domain/evento"
)

// --- dublês -----------------------------------------------------------------

// historicoFalso é o log do quadro em memória.
// quadroUm é o id usado por toda a suíte.
//
// É um UUID de verdade, e não um rótulo legível, porque o handshake passou a
// recusar id malformado com a mesma régua da borda HTTP — antes, `?board=lixo`
// chegava ao Postgres e voltava como erro de sintaxe de UUID.
const quadroUm = "11111111-1111-4111-8111-111111111111"

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

// DesdeRevisao e RevisaoAtual são o caminho do protocolo novo. O falso as
// implementa com a mesma régua do banco: ordem por (revisão, índice), e só
// eventos que TÊM revisão — os legados, sem ela, não entram no replay novo.
func (h *historicoFalso) DesdeRevisao(_ context.Context, boardID string, revisao int64, limite int) ([]evento.Evento, error) {
	if h.erro != nil {
		return nil, h.erro
	}
	var fora []evento.Evento
	for _, e := range h.eventos {
		if e.BoardID == boardID && e.Revisao > revisao {
			fora = append(fora, e)
			if len(fora) == limite {
				break
			}
		}
	}
	sort.SliceStable(fora, func(i, j int) bool {
		if fora[i].Revisao != fora[j].Revisao {
			return fora[i].Revisao < fora[j].Revisao
		}
		return fora[i].Indice < fora[j].Indice
	})
	return fora, nil
}

func (h *historicoFalso) RevisaoAtual(_ context.Context, boardID string) (int64, error) {
	if h.erro != nil {
		return 0, h.erro
	}
	var maior int64
	for _, e := range h.eventos {
		if e.BoardID == boardID && e.Revisao > maior {
			maior = e.Revisao
		}
	}
	return maior, nil
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
	Versao     int         `json:"versao"`
	Seq        int64       `json:"seq"`
	Revisao    int64       `json:"revisao"`
	Indice     int         `json:"indice"`
	Quantidade int         `json:"quantidade"`
	Tipo       evento.Tipo `json:"tipo"`
	BoardID    string      `json:"boardId"`
	CardID     string      `json:"cardId"`
	AutorID    string      `json:"autorId"`
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
	sessaoOk      atomic.Bool
	validarSessao ws.SessaoValida
	// prazoHTTP envolve o handler com o middleware real quando positivo.
	prazoHTTP       time.Duration
	prazoPreparacao time.Duration
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
	c.validarSessao = func(context.Context, string) bool { return c.sessaoOk.Load() }
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
		c.validarSessao,
		"sessao",
		[]string{"exemplo.test"},
		mudo,
	).ComIntervaloDeRevalidacao(50 * time.Millisecond)
	if c.prazoPreparacao > 0 {
		h.ComPrazoDePreparacao(c.prazoPreparacao)
	}

	var endpoint http.Handler = http.HandlerFunc(h.Acompanhar)
	if c.prazoHTTP > 0 {
		endpoint = httpmiddleware.Prazo(c.prazoHTTP, c.prazoHTTP)(endpoint)
	}
	c.servidor = httptest.NewServer(endpoint)
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

	resposta := chamarHTTP(t, c, "/?board="+quadroUm)
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
	c := montarWS(t, func(c *cenarioWS) { c.autorizador.revogar(quadroUm, "ana") })

	if resposta := chamarHTTP(t, c, "/?board="+quadroUm); resposta != http.StatusNotFound {
		t.Errorf("status = %d, esperado 404", resposta)
	}
}

// WebSocket NÃO obedece CORS: sem a checagem de origem, qualquer site que a
// vítima visitasse abriria uma conexão autenticada com o cookie dela.
func TestWSRecusaOrigemDesconhecida(t *testing.T) {
	c := montarWS(t, nil)
	endereco := "ws" + strings.TrimPrefix(c.servidor.URL, "http") + "/?board=" + quadroUm

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

// Sem cursor nenhum, o cliente recebe só a posição atual — e é essa mensagem
// que o OBRIGA a baixar o snapshot antes de aplicar qualquer evento: ela informa
// a revisão sem entregar história nenhuma.
func TestWSPrimeiraConexaoRecebeAPosicaoAtual(t *testing.T) {
	c := montarWS(t, func(c *cenarioWS) {
		c.historico.eventos = []evento.Evento{
			{Seq: 7, Revisao: 7, Quantidade: 1, Tipo: evento.CardMovido, BoardID: quadroUm, AutorID: "bob"},
		}
	})
	conexao := c.conectar(t, "/?board="+quadroUm)

	m := receberPulandoPresenca(t, conexao)
	if m.Tipo != evento.Sincronizado || m.Revisao != 7 {
		t.Errorf("recebeu %s com revisão %d, esperado sincronizado com 7", m.Tipo, m.Revisao)
	}
}

// Zero é uma revisão válida, especialmente depois de restaurar um banco
// anterior à primeira mutação revisionada. O campo precisa viajar no JSON:
// omiti-lo tornaria o servidor novo indistinguível do protocolo legado e o
// cliente não conseguiria recuar um cursor que estava no futuro.
func TestWSSincronizadoExplicitaRevisaoZero(t *testing.T) {
	c := montarWS(t, nil)
	conexao := c.conectar(t, "/?board="+quadroUm+"&revisao=0")

	prazo, cancelar := context.WithTimeout(context.Background(), time.Second)
	defer cancelar()
	_, corpo, err := conexao.Read(prazo)
	if err != nil {
		t.Fatalf("ler sincronizado: %v", err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(corpo, &envelope); err != nil {
		t.Fatalf("decodificar envelope: %v", err)
	}
	if string(envelope["revisao"]) != "0" {
		t.Fatalf("revisao no fio = %s, esperado campo explícito com zero", envelope["revisao"])
	}
}

// O handshake herda o prazo HTTP, mas a conexao aceita precisa sobreviver a
// ele. Este teste pega tambem o Hijack do writer vigiado e a limpeza do
// read-deadline no net.Conn.
func TestWSContinuaAbertoDepoisDoPrazoDoHandshake(t *testing.T) {
	c := montarWS(t, func(c *cenarioWS) { c.prazoHTTP = 20 * time.Millisecond })
	conexao := c.conectar(t, "/?board="+quadroUm)
	receberPulandoPresenca(t, conexao)

	time.Sleep(50 * time.Millisecond)
	const cardID = "22222222-2222-4222-8222-222222222222"
	c.hub.Publicar(evento.Novo(evento.CardMovido, quadroUm, "bob", nil).NoCard(cardID))
	if m := receberPulandoPresenca(t, conexao); m.Tipo != evento.CardMovido {
		t.Errorf("recebeu %s depois do prazo, esperado card.movido", m.Tipo)
	}
}

func TestWSRevalidacaoPeriodicaRecebePrazoProprio(t *testing.T) {
	viuPrazo := make(chan time.Duration, 1)
	c := montarWS(t, func(c *cenarioWS) {
		c.prazoPreparacao = 30 * time.Millisecond
		c.validarSessao = func(ctx context.Context, _ string) bool {
			prazo, ok := ctx.Deadline()
			if ok {
				select {
				case viuPrazo <- time.Until(prazo):
				default:
				}
			}
			return true
		}
	})
	conexao := c.conectar(t, "/?board="+quadroUm)
	receberPulandoPresenca(t, conexao)

	select {
	case restante := <-viuPrazo:
		if restante <= 0 || restante > 40*time.Millisecond {
			t.Errorf("prazo da revalidacao = %v, esperado cerca de 30ms", restante)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("a revalidacao nao recebeu deadline")
	}
}

// O envelope carrega a versão do formato. Sem ela, um cliente antigo aplicaria
// pela metade um evento cujo significado mudou, em vez de reconhecer que não
// entende e buscar snapshot.
func TestWSEnvelopeCarregaVersaoEGrupo(t *testing.T) {
	c := montarWS(t, nil)
	conexao := c.conectar(t, "/?board="+quadroUm)
	receberPulandoPresenca(t, conexao)

	const cardID = "22222222-2222-4222-8222-222222222222"
	c.hub.Publicar(evento.Novo(evento.CardMovido, quadroUm, "bob", nil).NoCard(cardID))

	m := receberPulandoPresenca(t, conexao)
	if m.Versao != 1 {
		t.Errorf("versao = %d, esperado 1", m.Versao)
	}
	if m.Quantidade != 1 || m.Indice != 0 {
		t.Errorf("grupo = (indice %d, quantidade %d), esperado (0, 1)", m.Indice, m.Quantidade)
	}
	if m.CardID != cardID {
		t.Errorf("cardId = %q, esperado %q", m.CardID, cardID)
	}
}

func TestWSEntregaEventoAoVivo(t *testing.T) {
	c := montarWS(t, nil)
	conexao := c.conectar(t, "/?board="+quadroUm)
	receberPulandoPresenca(t, conexao) // o sincronizado inicial

	c.hub.Publicar(evento.Novo(evento.CardMovido, quadroUm, "bob", nil))

	if m := receberPulandoPresenca(t, conexao); m.Tipo != evento.CardMovido {
		t.Errorf("recebeu %s, esperado card.movido", m.Tipo)
	}
}

// O AUTOR TAMBÉM RECEBE. Havia aqui o teste oposto, e ele guardava um defeito.
//
// O filtro por autorId supunha "quem agiu já viu o resultado na própria tela".
// Isso é verdade para a aba que disparou a ação e FALSO para todas as outras da
// mesma conta: em dois monitores, ou no celular e no computador, a segunda tela
// parava de receber o que a primeira fazia. Como o servidor não distingue
// conexões da mesma pessoa, filtrar por autor filtra as duas.
//
// O eco duplicado que o filtro evitava é problema do cliente, e lá se resolve
// de forma correta — aplicar o mesmo evento duas vezes é idempotente.
func TestWSEntregaOEventoTambemAoProprioAutor(t *testing.T) {
	c := montarWS(t, nil)
	conexao := c.conectar(t, "/?board="+quadroUm)
	receberPulandoPresenca(t, conexao)

	c.hub.Publicar(evento.Novo(evento.CardMovido, quadroUm, "ana", nil)) // a própria ana

	m := receberPulandoPresenca(t, conexao)
	if m.Tipo != evento.CardMovido {
		t.Fatalf("recebeu %s, esperado card.movido — o evento da própria ana não chegou", m.Tipo)
	}
	if m.AutorID != "ana" {
		t.Errorf("autorId = %q, esperado ana", m.AutorID)
	}
}

// Duas conexões da MESMA conta, que é o caso que o filtro por autor quebrava.
func TestWSDuasAbasDaMesmaContaRecebemAsDuas(t *testing.T) {
	c := montarWS(t, nil)
	primeira := c.conectar(t, "/?board="+quadroUm)
	receberPulandoPresenca(t, primeira)
	segunda := c.conectar(t, "/?board="+quadroUm)
	receberPulandoPresenca(t, segunda)

	c.hub.Publicar(evento.Novo(evento.CardMovido, quadroUm, "ana", nil))

	for i, conexao := range []*websocket.Conn{primeira, segunda} {
		if m := receberPulandoPresenca(t, conexao); m.Tipo != evento.CardMovido {
			t.Errorf("aba %d recebeu %s, esperado card.movido", i+1, m.Tipo)
		}
	}
}

// --- reposição do backlog ---------------------------------------------------

func TestWSReponOQueSePerdeu(t *testing.T) {
	c := montarWS(t, func(c *cenarioWS) {
		c.historico.eventos = []evento.Evento{
			{Seq: 1, Tipo: evento.CardCriado, BoardID: quadroUm, AutorID: "bob"},
			{Seq: 2, Tipo: evento.CardMovido, BoardID: quadroUm, AutorID: "bob"},
			{Seq: 3, Tipo: evento.ColunaCriada, BoardID: quadroUm, AutorID: "bob"},
		}
	})
	conexao := c.conectar(t, "/?board="+quadroUm+"&desde=1")

	// Pede a partir do 1: recebe o 2 e o 3, nessa ordem, e nunca o 1.
	if m := receberPulandoPresenca(t, conexao); m.Seq != 2 {
		t.Fatalf("primeiro reposto tem seq %d, esperado 2", m.Seq)
	}
	if m := receberPulandoPresenca(t, conexao); m.Seq != 3 {
		t.Fatalf("segundo reposto tem seq %d, esperado 3", m.Seq)
	}
}

// O replay TAMBÉM devolve o que a própria pessoa fez enquanto esteve fora.
//
// Aqui estava o defeito mais silencioso do filtro por autor: a aba offline
// pulava os eventos feitos no outro dispositivo da mesma conta e ainda assim
// avançava o cursor por cima deles, como se os tivesse aplicado. O estado dela
// passava a discordar do banco sem nenhum sinal.
func TestWSReplayDevolveTambemOsEventosDoProprioAutor(t *testing.T) {
	c := montarWS(t, func(c *cenarioWS) {
		c.historico.eventos = []evento.Evento{
			{Seq: 1, Revisao: 1, Quantidade: 1, Tipo: evento.CardCriado, BoardID: quadroUm, AutorID: "ana"},
			{Seq: 2, Revisao: 2, Quantidade: 1, Tipo: evento.CardMovido, BoardID: quadroUm, AutorID: "bob"},
		}
	})
	conexao := c.conectar(t, "/?board="+quadroUm+"&revisao=0")

	m := receberPulandoPresenca(t, conexao)
	if m.AutorID != "ana" || m.Revisao != 1 {
		t.Fatalf("primeiro reposto = autor %q revisão %d, esperado ana na revisão 1", m.AutorID, m.Revisao)
	}
	if m := receberPulandoPresenca(t, conexao); m.Revisao != 2 {
		t.Errorf("segundo reposto tem revisão %d, esperado 2", m.Revisao)
	}
}

// O intervalo é fechado com a revisão atual mesmo quando o cliente já está em
// dia — sem isso ele não teria confirmação de que não perdeu nada.
func TestWSReplayFechaOIntervaloComARevisaoAtual(t *testing.T) {
	c := montarWS(t, func(c *cenarioWS) {
		c.historico.eventos = []evento.Evento{
			{Seq: 1, Revisao: 1, Quantidade: 1, Tipo: evento.CardCriado, BoardID: quadroUm, AutorID: "ana"},
			{Seq: 2, Revisao: 2, Quantidade: 1, Tipo: evento.CardMovido, BoardID: quadroUm, AutorID: "ana"},
		}
	})
	conexao := c.conectar(t, "/?board="+quadroUm+"&revisao=2")

	m := receberPulandoPresenca(t, conexao)
	if m.Tipo != evento.Sincronizado || m.Revisao != 2 {
		t.Errorf("recebeu %s com revisão %d, esperado sincronizado com 2", m.Tipo, m.Revisao)
	}
}

// Um grupo INCOMPLETO no fim do intervalo é descartado inteiro.
//
// Entregar meia revisão faria o cliente confirmar um estado que ele aplicou pela
// metade — e o que faltou nunca mais seria entregue, porque está abaixo do
// cursor. É o mesmo buraco silencioso que a revisão existe para eliminar.
func TestWSReplayNaoCortaUmGrupoNoMeio(t *testing.T) {
	c := montarWS(t, func(c *cenarioWS) {
		// A revisão 2 declara dois eventos e só um cabe no intervalo pedido.
		c.historico.eventos = []evento.Evento{
			{Seq: 1, Revisao: 1, Indice: 0, Quantidade: 1, Tipo: evento.CardCriado, BoardID: quadroUm, AutorID: "bob"},
			{Seq: 2, Revisao: 2, Indice: 0, Quantidade: 2, Tipo: evento.CardMovido, BoardID: quadroUm, AutorID: "bob"},
		}
	})
	conexao := c.conectar(t, "/?board="+quadroUm+"&revisao=0")

	// Chega a revisão 1 inteira...
	if m := receberPulandoPresenca(t, conexao); m.Revisao != 1 {
		t.Fatalf("primeiro reposto tem revisão %d, esperado 1", m.Revisao)
	}
	// ...e o grupo incompleto da 2 NÃO chega: vem o sincronizado.
	m := receberPulandoPresenca(t, conexao)
	if m.Tipo != evento.Sincronizado {
		t.Errorf("recebeu %s (revisão %d, índice %d): o grupo incompleto foi entregue",
			m.Tipo, m.Revisao, m.Indice)
	}
}

// Intervalo grande demais não é reproduzido: mandar recarregar tudo é mais
// barato que montar centenas de eventos em memória, e é sempre correto.
func TestWSBacklogGrandeDemaisMandaRecarregarTudo(t *testing.T) {
	c := montarWS(t, func(c *cenarioWS) {
		for i := 1; i <= 300; i++ {
			c.historico.eventos = append(c.historico.eventos, evento.Evento{
				Seq: int64(i), Tipo: evento.CardMovido, BoardID: quadroUm, AutorID: "bob",
			})
		}
	})
	conexao := c.conectar(t, "/?board="+quadroUm+"&desde=0")

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
	conexao := c.conectar(t, "/?board="+quadroUm)
	receberPulandoPresenca(t, conexao)

	c.autorizador.revogar(quadroUm, "ana")

	esperarConexaoCair(t, conexao, "a remoção do membro")
}

// Mesma coisa para o logout: a sessão morre no banco e o socket não pode
// continuar transmitindo.
func TestWSDerrubaQuemEncerrouASessao(t *testing.T) {
	c := montarWS(t, nil)
	conexao := c.conectar(t, "/?board="+quadroUm)
	receberPulandoPresenca(t, conexao)

	c.sessaoOk.Store(false)

	esperarConexaoCair(t, conexao, "o logout")
}

func esperarConexaoCair(t *testing.T, c *websocket.Conn, porque string) {
	t.Helper()
	ctx, cancelar := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelar()

	for {
		_, _, err := c.Read(ctx)
		if err == nil {
			continue // chegou mensagem; segue esperando o fechamento
		}
		// ⚠️ A ordem importa, e é onde este helper já esteve errado: quando o
		// PRAZO DAQUI expira, o Read também devolve erro. Tratar todo erro como
		// "caiu" fazia o teste passar mesmo com a conexão intacta — ele não
		// tinha como falhar, e deixou passar uma revalidação desligada.
		//
		// Por isso o ctx.Err() é conferido ANTES: se o prazo estourou, o erro é
		// nosso, não do servidor.
		if ctx.Err() != nil {
			t.Fatalf("a conexão continuou de pé depois de %s", porque)
		}
		return // fechamento de verdade, vindo do servidor
	}
}
