// Package ws é o adaptador de WebSocket: transforma o evento do domínio em
// frame JSON e o entrega a quem está com o quadro aberto.
//
// O handshake é um GET HTTP comum que troca de protocolo no meio
// (101 Switching Protocols). É por ser HTTP que o cookie de sessão viaja junto
// — e é por isso que a autenticação aqui é a mesma do resto da API, sem token
// separado nem query string com segredo.
package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/coder/websocket"

	"stacktrack/internal/adapter/realtime/hub"
	"stacktrack/internal/domain/evento"
	"stacktrack/internal/pkg/limite"

	"github.com/go-chi/httprate"
	"github.com/google/uuid"
)

const (
	// intervaloPing é de quanto em quanto tempo o servidor cutuca o cliente.
	//
	// Existe porque uma conexão TCP morta não avisa: um notebook que fecha a
	// tampa deixa o socket aberto do lado de cá para sempre, e o hub ficaria
	// entregando eventos para ninguém. O ping é o que transforma isso em erro.
	intervaloPing = 30 * time.Second
	// tempoDePing é quanto se espera pelo pong antes de considerar morto.
	tempoDePing = 10 * time.Second
	// tempoDeEscrita limita cada envio. Sem ele, um cliente que aceita a
	// conexão e para de ler travaria a goroutine de escrita indefinidamente.
	tempoDeEscrita = 10 * time.Second
	// intervaloRevalidacao é de quanto em quanto tempo se reconfere que quem
	// está na ponta AINDA pode estar ali.
	//
	// O handshake autoriza uma vez; a conexão dura horas. Sem reconferir, quem
	// é removido do quadro — ou desloga — continua recebendo o quadro ao vivo
	// até fechar a aba, porque nada no caminho de escrita toca no banco de novo.
	//
	// Separado do ping de propósito: detectar cliente morto e reconferir
	// permissão são preocupações diferentes, e uma não deve reger a cadência da
	// outra.
	intervaloRevalidacao = 30 * time.Second
)

// Autorizador responde se o usuário pode acompanhar o quadro. É a mesma
// pergunta que o usecase faz antes de qualquer leitura — declarada aqui como
// interface para o adaptador não depender do usecase concreto.
type Autorizador interface {
	PodeVer(ctx context.Context, boardID, usuarioID string) bool
}

// Identificador extrai a identidade do contexto, já validada pelo middleware
// de sessão.
type Identificador func(ctx context.Context) (string, bool)

// Nomeador resolve o nome de exibição de quem conectou. É o que a presença
// mostra: um avatar com id cru não diz nada a ninguém.
type Nomeador func(ctx context.Context, usuarioID string) string

// SessaoValida informa se o token de sessão continua valendo.
//
// Existe porque o middleware de autenticação roda uma vez, no handshake, e a
// conexão sobrevive a ele por horas: sem reconferir, o logout apaga a sessão no
// banco e o socket segue transmitindo.
type SessaoValida func(ctx context.Context, token string) bool

// Historico é o log do quadro, consultado no reconnect.
//
// Os dois pares convivem durante a transição: `Desde`/`UltimoSeq` atendem o
// cliente antigo, que ainda manda `?desde={seq}`, e `DesdeRevisao`/
// `RevisaoAtual` atendem o protocolo novo. O cliente novo prefere revisão, e é
// ela que resolve o buraco silencioso descrito em evento.Evento.Seq.
type Historico interface {
	Desde(ctx context.Context, boardID string, seq int64, limite int) ([]evento.Evento, error)
	UltimoSeq(ctx context.Context, boardID string) (int64, error)
	DesdeRevisao(ctx context.Context, boardID string, revisao int64, limite int) ([]evento.Evento, error)
	RevisaoAtual(ctx context.Context, boardID string) (int64, error)
}

// maxBacklog é o teto de eventos entregues num reconnect.
//
// Uma aba que ficou uma semana fechada pediria a história inteira do quadro, e
// montá-la em memória para entregá-la seria uma negação de serviço com pedido
// educado. Passando do teto, o cliente é mandado recarregar tudo — mais barato
// e sempre correto.
const maxBacklog = 200

// Observador é avisado de quais quadros têm alguém olhando.
//
// É o despachante: ele só entrega eventos de quadro observado, porque sem
// assinante não há a quem entregar — e manter cursor de todo quadro que já teve
// uma conexão faria o mapa crescer para sempre.
type Observador interface {
	Observar(ctx context.Context, boardID string)
	Esquecer(boardID string)
}

// Handler serve o endpoint de tempo real.
type Handler struct {
	observador      Observador
	hub             *hub.Hub
	limites         *hub.Limites
	handshakes      *limite.PorChave
	historico       Historico
	autorizador     Autorizador
	identidade      Identificador
	nome            Nomeador
	sessaoValida    SessaoValida
	nomeCookie      string
	origensAceitas  []string
	revalidarACada  time.Duration
	prazoPreparacao time.Duration
	log             *slog.Logger
}

// NovoHandler cria o handler do WebSocket.
//
// origensAceitas alimenta o OriginPatterns do coder/websocket, e não é
// opcional: **WebSocket não obedece CORS**. Sem checar a origem, qualquer site
// que a vítima visite abre uma conexão autenticada com o cookie dela e passa a
// ler o quadro inteiro em tempo real — o Cross-Site WebSocket Hijacking. O
// SameSite=Lax do cookie é a segunda camada, não a primeira.
//
// sessaoValida e nomeCookie são o que permite reconferir a sessão enquanto a
// conexão vive; nil desliga a reconferência da sessão (a da participação no
// quadro continua valendo).
func NovoHandler(
	h *hub.Hub,
	hist Historico,
	a Autorizador,
	id Identificador,
	nome Nomeador,
	sessaoValida SessaoValida,
	nomeCookie string,
	origensAceitas []string,
	log *slog.Logger,
) *Handler {
	return &Handler{
		hub: h, historico: hist, autorizador: a, identidade: id, nome: nome,
		sessaoValida: sessaoValida, nomeCookie: nomeCookie,
		origensAceitas: origensAceitas, log: log,
		revalidarACada:  intervaloRevalidacao,
		prazoPreparacao: 10 * time.Second,
		// Sem limites ligados nada é contado — o que os testes querem. Produção
		// os liga por ComLimites; config.Validar recusa subir sem eles.
		limites: hub.NovosLimites(0, 0),
	}
}

// ComPrazoDePreparacao limita consultas feitas logo depois do 101 (nome e
// replay). A conexao longa fica sem deadline global; cada operacao curta nao.
func (h *Handler) ComPrazoDePreparacao(d time.Duration) *Handler {
	if d > 0 {
		h.prazoPreparacao = d
	}
	return h
}

// ComLimites liga o teto de conexões simultâneas (por conta e global) e o de
// tentativas de handshake por IP.
//
// São três tetos porque são três abusos diferentes, e nenhum cobre o outro:
//
//   - por CONTA impede uma pessoa de consumir sozinha a capacidade de tempo
//     real de todo mundo, abrindo abas ou rodando um script com a própria
//     sessão;
//   - GLOBAL protege a memória do processo, que paga um buffer de eventos por
//     conexão;
//   - por IP, no HANDSHAKE, cobre o que os dois primeiros não veem: quem abre e
//     fecha em laço nunca ocupa vaga, e mesmo assim faz o servidor pagar
//     autorização, consulta de nome e replay a cada tentativa.
func (h *Handler) ComLimites(porConta, global, handshakesPorMinuto int, janela time.Duration) *Handler {
	h.limites = hub.NovosLimites(porConta, global)
	h.handshakes = limite.NovoPorChave(handshakesPorMinuto, janela)
	return h
}

// ComObservador liga o despachante que entrega os eventos lendo o log.
//
// Sem ele, o handler continua funcionando — a entrega ao vivo passa a depender
// de quem publica diretamente no hub, que é o que os testes de protocolo
// querem.
func (h *Handler) ComObservador(o Observador) *Handler {
	h.observador = o
	return h
}

// ComIntervaloDeRevalidacao ajusta de quanto em quanto tempo a permissão é
// reconferida. Valor não positivo restaura o padrão.
//
// É o compromisso entre quanto tempo alguém que perdeu o acesso continua
// recebendo o quadro e quantas consultas por conexão o servidor paga para
// evitá-lo.
func (h *Handler) ComIntervaloDeRevalidacao(d time.Duration) *Handler {
	if d <= 0 {
		d = intervaloRevalidacao
	}
	h.revalidarACada = d
	return h
}

// versaoDoEnvelope identifica o formato da mensagem no fio.
//
// Viaja em TODA mensagem para que o cliente possa reconhecer um formato que ele
// não entende e reagir buscando snapshot, em vez de aplicar pela metade um
// evento cujo significado mudou. Campo desconhecido de versão futura não quebra
// o transporte; versão desconhecida, sim, e é de propósito.
const versaoDoEnvelope = 1

// mensagem é o formato que viaja no fio. É separado de evento.Evento de
// propósito: o domínio não deve ganhar tags de JSON por causa do transporte.
type mensagem struct {
	Versao int `json:"versao"`
	// Seq é a identidade global e imutável do evento. Continua no envelope por
	// compatibilidade com o cliente anterior, que ainda o usa como cursor.
	//
	// ⚠️ Não é cursor para o cliente novo: sendo BIGSERIAL, ele registra a
	// ordem de ALOCAÇÃO e não a de COMMIT (ver evento.Evento.Seq).
	Seq int64 `json:"seq,omitempty"`
	// Revisao é o cursor de verdade: contígua por quadro e atribuída na ordem
	// de commit, sob o lock.
	Revisao *int64 `json:"revisao,omitempty"`
	// Indice e Quantidade descrevem o grupo. O cliente só confirma a revisão
	// depois de aplicar os `quantidade` eventos dela.
	Indice     int         `json:"indice"`
	Quantidade int         `json:"quantidade"`
	Tipo       evento.Tipo `json:"tipo"`
	BoardID    string      `json:"boardId"`
	// CardID permite que consumidores com projeção por card decidam se o
	// evento lhes interessa sem interpretar o payload. Vazio nos eventos do
	// quadro como um todo.
	CardID  string    `json:"cardId,omitempty"`
	AutorID string    `json:"autorId"`
	Em      time.Time `json:"em"`
	Dados   any       `json:"dados,omitempty"`
}

// Acompanhar autentica, confere o acesso ao quadro e mantém a conexão aberta
// entregando os eventos daquele quadro.
func (h *Handler) Acompanhar(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.identidade(r.Context())
	if !ok {
		http.Error(w, "não autenticado", http.StatusUnauthorized)
		return
	}

	// O teto de HANDSHAKES vem antes de qualquer trabalho: autorizar, resolver
	// nome e repor história custam consultas, e quem abre e fecha em laço as
	// paga todas sem nunca ocupar uma vaga de conexão.
	if h.handshakes != nil {
		chave, err := httprate.KeyByIP(r)
		if err != nil {
			chave = r.RemoteAddr
		}
		reserva, permitido := h.handshakes.Reservar(chave)
		if !permitido {
			w.Header().Set("Retry-After", strconv.Itoa(h.handshakes.SegundosDeEspera()))
			http.Error(w, "muitas tentativas de conexão", http.StatusTooManyRequests)
			return
		}
		// Todo handshake conta, tenha ele sucesso ou nao. Confirmar agora mantem
		// essa semantica e a reserva torna a verificacao atomica sob rajada.
		reserva.Confirmar(w)
	}

	boardID := r.URL.Query().Get("board")
	if boardID == "" {
		http.Error(w, "informe o quadro", http.StatusBadRequest)
		return
	}
	if _, err := uuid.Parse(boardID); err != nil {
		// Mesma régua da borda HTTP: id malformado é recusado antes de virar
		// consulta. Sem isto, `?board=lixo` chegava ao Postgres e voltava como
		// erro de sintaxe de UUID.
		http.Error(w, "quadro não encontrado", http.StatusNotFound)
		return
	}
	// Mesma regra do resto da API: quem não participa recebe 404, nunca 403 —
	// um 403 confirmaria que o quadro existe.
	if !h.autorizador.PodeVer(r.Context(), boardID, usuarioID) {
		if errors.Is(r.Context().Err(), context.DeadlineExceeded) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "a requisicao demorou demais; tente de novo", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "quadro não encontrado", http.StatusNotFound)
		return
	}
	if errors.Is(r.Context().Err(), context.DeadlineExceeded) {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "a requisicao demorou demais; tente de novo", http.StatusServiceUnavailable)
		return
	}

	// A vaga é reservada ANTES do Accept: aceitar e depois recusar deixaria o
	// cliente com uma conexão que abre e morre, sem status HTTP que explique o
	// motivo. Recusando aqui, ele recebe 503 e sabe o que aconteceu.
	liberar, motivo := h.limites.Reservar(usuarioID)
	if motivo != "" {
		h.log.Info("conexão de tempo real recusada",
			"motivo", string(motivo), "usuario", usuarioID, "board", boardID)
		w.Header().Set("Retry-After", "5")
		http.Error(w, string(motivo), http.StatusServiceUnavailable)
		return
	}
	defer liberar()

	conexao, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: h.origensAceitas,
	})
	if err != nil {
		// O Accept já respondeu ao cliente; aqui só resta registrar.
		h.log.Debug("handshake recusado", "erro", err, "board", boardID)
		return
	}
	defer conexao.CloseNow()

	// O contexto HTTP tem prazo inclusive durante autenticacao, autorizacao e
	// Accept. Depois do 101 ele nao pode reger uma conexao que dura horas. Os
	// prazos do protocolo passam a ser os de ping, escrita e revalidacao abaixo.
	ctxConexao, encerrarConexao := context.WithCancel(context.WithoutCancel(r.Context()))
	defer encerrarConexao()
	r = r.WithContext(ctxConexao)

	ctxPreparacao, cancelarPreparacao := context.WithTimeout(ctxConexao, h.prazoPreparacao)
	nome := h.nome(ctxPreparacao, usuarioID)

	// O despachante passa a acompanhar este quadro ANTES da assinatura.
	//
	// Antes da assinatura porque, se ele começasse a observar depois, uma
	// mutação que comitasse no meio não seria entregue por ele — e também não
	// entraria no replay, que já teria sido montado a partir de uma revisão
	// anterior. Observando primeiro, o pior caso é uma entrega repetida, que o
	// cliente descarta pela revisão.
	//
	// ⚠️ Usa ctxPreparacao, e NÃO r.Context(). Depois do 101 o contexto da
	// requisição HTTP original está cancelado — a conexão foi sequestrada —, e
	// uma leitura feita com ele falha na hora. Foi exatamente esse o defeito da
	// primeira versão desta linha: `Observar` desistia em silêncio, o quadro
	// nunca era acompanhado, e nenhum evento chegava ao vivo.
	if h.observador != nil {
		h.observador.Observar(ctxPreparacao, boardID)
	}

	assinante := h.hub.Assinar(boardID, hub.Pessoa{ID: usuarioID, Nome: nome})
	if assinante == nil {
		cancelarPreparacao()
		// O hub está desligando.
		_ = conexao.Close(websocket.StatusGoingAway, "servidor encerrando")
		return
	}
	defer func() {
		h.hub.Cancelar(assinante)
		// A sala esvaziou: ninguém mais precisa dos eventos deste quadro, e
		// manter o cursor vivo seria vazamento de memória por quadro visitado.
		if h.observador != nil && h.hub.Inscritos(boardID) == 0 {
			h.observador.Esquecer(boardID)
		}
	}()

	// O que o cliente perdeu enquanto esteve fora, ANTES de qualquer evento ao
	// vivo. A ordem é o ponto: entregar o backlog depois faria o cliente aplicar
	// o passado por cima do presente.
	//
	// A assinatura já aconteceu, então os eventos que chegarem durante a
	// reposição ficam na fila do canal e são entregues em seguida — sem buraco
	// entre o fim da história e o começo do ao vivo, que é exatamente onde um
	// desenho ingênuo perde eventos.
	// O corte devolvido é a última posição JÁ ENTREGUE pelo replay. Ele existe
	// porque a assinatura acontece ANTES da reposição — de propósito, para não
	// haver janela entre o fim da história e o começo do ao vivo —, e a
	// consequência é que um evento que comitou durante a reposição chega pelos
	// DOIS caminhos. Sem o corte, o cliente receberia esse evento duas vezes.
	corte, err := h.repor(ctxPreparacao, conexao, boardID, r.URL.Query())
	cancelarPreparacao()
	if err != nil {
		h.log.Debug("falha ao repor o histórico", "erro", err, "board", boardID)
		return
	}

	// Um contexto por conexão: qualquer um dos lados que termine cancela o
	// outro. É o que faz a goroutine de leitura e a de escrita morrerem juntas
	// em vez de deixar uma pendurada.
	ctx, encerrar := context.WithCancel(r.Context())
	defer encerrar()

	// O token da sessão, guardado para ser reconferido enquanto a conexão vive.
	// Lido aqui porque só o handshake é uma requisição HTTP com cookie; daqui
	// para a frente não há mais requisição nenhuma.
	var tokenDaSessao string
	if c, err := r.Cookie(h.nomeCookie); err == nil {
		tokenDaSessao = c.Value
	}

	// UMA goroutine de leitura e UMA de escrita, nunca mais.
	//
	// Escrever de dois lugares ao mesmo tempo corrompe o frame — a biblioteca
	// não serializa isso por você, e o sintoma é a conexão morrer com erro de
	// protocolo sob carga. Por isso todo envio sai daqui, do laço abaixo, e a
	// única fonte é o canal do assinante.
	go h.lerAteMorrer(ctx, encerrar, conexao, assinante)

	h.escreverAteMorrer(ctx, conexao, assinante, usuarioID, boardID, tokenDaSessao, corte)
}

// aindaPodeAcompanhar reconfere, com a conexão já aberta, que quem está na
// ponta continua tendo direito ao que recebe.
//
// São duas perguntas independentes, e as duas mudam sem que a conexão saiba:
// a sessão pode ter sido encerrada (logout) e a participação no quadro pode ter
// sido revogada (o dono removeu o membro).
func (h *Handler) aindaPodeAcompanhar(ctx context.Context, usuarioID, boardID, token string) bool {
	// O contexto da conexao dura horas e por isso nao tem deadline. Cada ida ao
	// banco durante a revalidacao precisa do proprio teto, senao pool ou banco
	// travado prenderia esta goroutine (e a vaga do socket) indefinidamente.
	prazo, cancelar := context.WithTimeout(ctx, h.prazoPreparacao)
	defer cancelar()
	if h.sessaoValida != nil && !h.sessaoValida(prazo, token) {
		return false
	}
	return h.autorizador.PodeVer(prazo, boardID, usuarioID)
}

// mensagemDoCliente é o único formato que o cliente pode mandar.
//
// UM tipo, e não um mapa aberto: o que chega aqui é entrada de fora, e um
// formato fechado é o que impede o campo de amanhã virar caminho de ontem.
type mensagemDoCliente struct {
	Tipo string `json:"tipo"`
	// ColunaID vazio significa "parei de editar".
	ColunaID string `json:"colunaId"`
}

// tamanhoMaximoDeIdDeColuna limita o que é repassado para a sala inteira.
//
// Um UUID tem 36 caracteres. O teto existe porque este valor é REDISTRIBUÍDO a
// todo mundo que está no quadro: sem ele, uma conexão mandaria um id de dez mil
// caracteres e o servidor o multiplicaria por cada pessoa conectada.
const tamanhoMaximoDeIdDeColuna = 64

// limiteDeLeitura é o teto de bytes por mensagem vinda do cliente.
//
// A biblioteca traz um padrão de 32 KB, pensado para quem trafega dados. Aqui o
// cliente só manda `{"tipo":"foco","colunaId":"<uuid>"}` — menos de 60 bytes.
// Apertar o limite é grátis e transforma "mandar lixo grande" em desconexão
// imediata, antes de qualquer alocação.
const limiteDeLeitura = 512

// lerAteMorrer processa o que o cliente manda, e existiria mesmo se ele não
// mandasse nada.
//
// Sem ler, os pongs de resposta ao ping nunca seriam processados — a biblioteca
// os entrega pelo mesmo caminho das mensagens — e a conexão morreria sozinha em
// 30 segundos. Ler também é o que detecta o fechamento pelo lado do cliente.
func (h *Handler) lerAteMorrer(ctx context.Context, encerrar context.CancelFunc, c *websocket.Conn, a *hub.Assinante) {
	defer encerrar()
	c.SetReadLimit(limiteDeLeitura)
	for {
		_, dados, err := c.Read(ctx)
		if err != nil {
			return
		}

		// Mensagem malformada é IGNORADA, e não derruba a conexão: o canal
		// carrega o quadro ao vivo, e perder isso por causa de um JSON torto
		// seria trocar o essencial pelo acessório. Um cliente que só mande lixo
		// não consegue nada além de gastar o próprio socket.
		var msg mensagemDoCliente
		if json.Unmarshal(dados, &msg) != nil {
			continue
		}
		if msg.Tipo != "foco" {
			continue
		}
		if len(msg.ColunaID) > tamanhoMaximoDeIdDeColuna {
			continue
		}
		if msg.ColunaID != "" {
			if _, err := uuid.Parse(msg.ColunaID); err != nil {
				continue
			}
		}
		h.hub.DefinirFoco(a, msg.ColunaID)
	}
}

func (h *Handler) escreverAteMorrer(
	ctx context.Context,
	c *websocket.Conn,
	a *hub.Assinante,
	usuarioID, boardID, tokenDaSessao string,
	corte posicao,
) {
	ticker := time.NewTicker(intervaloPing)
	defer ticker.Stop()

	revalidar := time.NewTicker(h.revalidarACada)
	defer revalidar.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-revalidar.C:
			if !h.aindaPodeAcompanhar(ctx, usuarioID, boardID, tokenDaSessao) {
				h.log.Info("conexão encerrada: o acesso ao quadro deixou de valer",
					"board", boardID, "usuario", usuarioID)
				_ = c.Close(websocket.StatusPolicyViolation, "acesso revogado")
				return
			}

		case e, aberto := <-a.Eventos:
			if !aberto {
				// O hub fechou o canal: ou está desligando, ou desistiu desta
				// conexão por ela não acompanhar o ritmo. O motivo diz ao
				// cliente para buscar snapshot em vez de tentar retomar de onde
				// parou — ele não tem como saber o que perdeu.
				_ = c.Close(websocket.StatusPolicyViolation, "fila cheia: busque um snapshot novo")
				return
			}
			// O EVENTO VAI PARA TODA CONEXÃO AUTORIZADA, inclusive as do próprio
			// autor. Havia aqui um `if e.AutorID == usuarioID { continue }`, e
			// ele quebrava duas abas da mesma conta: a suposição era "quem agiu
			// já viu o resultado na própria tela", verdadeira para a aba que
			// disparou a ação e falsa para todas as outras da mesma pessoa. Em
			// dois monitores, ou no celular e no computador, a segunda tela
			// simplesmente parava de receber o que a primeira fazia — e ainda
			// avançava o cursor por cima desses eventos no replay, como se os
			// tivesse aplicado.
			//
			// O eco duplicado que o filtro evitava é problema do CLIENTE, e lá
			// ele se resolve de forma correta: aplicar o mesmo evento duas vezes
			// é idempotente, e a revisão confirmada diz o que já foi aplicado.
			if corte.jaEntregue(e) {
				// Comitou durante a reposição e já foi entregue por ela.
				continue
			}
			if err := h.enviar(ctx, c, e); err != nil {
				h.log.Debug("falha ao enviar evento", "erro", err, "board", boardID)
				return
			}

		case <-ticker.C:
			prazo, cancelar := context.WithTimeout(ctx, tempoDePing)
			err := c.Ping(prazo)
			cancelar()
			if err != nil {
				return
			}
		}
	}
}

func (h *Handler) enviar(ctx context.Context, c *websocket.Conn, e evento.Evento) error {
	prazo, cancelar := context.WithTimeout(ctx, tempoDeEscrita)
	defer cancelar()

	corpo, err := json.Marshal(mensagem{
		Versao:     versaoDoEnvelope,
		Tipo:       e.Tipo,
		BoardID:    e.BoardID,
		CardID:     e.CardID,
		AutorID:    e.AutorID,
		Em:         e.OcorridoEm,
		Dados:      e.Dados,
		Seq:        e.Seq,
		Revisao:    revisaoNoFio(e),
		Indice:     e.Indice,
		Quantidade: e.Quantidade,
	})
	if err != nil {
		return err
	}
	return c.Write(prazo, websocket.MessageText, corpo)
}

// revisaoNoFio distingue revisão zero válida de evento legado sem revisão.
//
// `omitempty` sobre um int64 apagava o zero. Isso parecia apenas economia de
// bytes, mas tornava dois estados diferentes iguais no protocolo: um quadro
// legado/restaurado cuja revisão atual é realmente zero e um evento gravado
// antes de a revisão existir. Mensagens de controle do servidor novo precisam
// carregar `revisao: 0` explicitamente para o cliente poder recuar o cursor
// depois de uma restauração; eventos legados continuam omitindo o campo.
func revisaoNoFio(e evento.Evento) *int64 {
	if e.Revisao == 0 && e.Tipo != evento.Sincronizado && e.Tipo != evento.RecarregueTudo {
		return nil
	}
	revisao := e.Revisao
	return &revisao
}

// posicao é um ponto do log dentro de um quadro: (revisão, índice).
//
// Serve à deduplicação entre o replay e o fluxo ao vivo. O par, e não só a
// revisão: uma revisão pode ter vários eventos, e cortar por revisão inteira
// descartaria os índices seguintes de um grupo que a reposição entregou pela
// metade.
type posicao struct {
	revisao int64
	indice  int
	// porSeq marca a posição de um cliente antigo, que ainda usa `?desde={seq}`.
	// Nesse modo a comparação é por seq, porque os eventos que ele acabou de
	// receber podem ser legados, sem revisão nenhuma.
	porSeq bool
	seq    int64
}

// jaEntregue informa se o evento já saiu pela reposição.
func (p posicao) jaEntregue(e evento.Evento) bool {
	if p.porSeq {
		return e.Seq != 0 && e.Seq <= p.seq
	}
	if p.revisao == 0 || e.Revisao == 0 {
		// Sem revisão dos dois lados não há como comparar. Deixar passar é a
		// escolha certa: entregar duas vezes é inofensivo (o cliente aplica de
		// forma idempotente), e não entregar seria um buraco.
		return false
	}
	if e.Revisao != p.revisao {
		return e.Revisao < p.revisao
	}
	return e.Indice <= p.indice
}

// repor entrega o que o cliente perdeu e devolve até onde a entrega foi.
//
// Três caminhos, decididos pela query string:
//
//   - `?revisao=N` — o protocolo novo. Replay por (revisão, índice), que é
//     contígua e na ordem de commit.
//   - `?desde=N` — o cliente anterior, que usa o seq como cursor. Continua
//     atendido durante o deploy de transição, e só durante ele.
//   - nenhum dos dois — primeira conexão. Nada de história: só a posição
//     atual, para a tela saber de onde partir depois de baixar o snapshot.
func (h *Handler) repor(ctx context.Context, c *websocket.Conn, boardID string, parametros url.Values) (posicao, error) {
	if h.historico == nil {
		return posicao{}, nil
	}

	if bruta := parametros.Get("revisao"); bruta != "" {
		return h.reporPorRevisao(ctx, c, boardID, bruta)
	}
	if bruta := parametros.Get("desde"); bruta != "" {
		return h.reporPorSeq(ctx, c, boardID, bruta)
	}

	// Primeira conexão. A mensagem que sai daqui é o que OBRIGA o snapshot: ela
	// informa a revisão atual sem entregar evento nenhum, e o cliente sabe que
	// precisa baixar o estado antes de aplicar qualquer coisa que chegue depois.
	atual, err := h.historico.RevisaoAtual(ctx, boardID)
	if err != nil {
		return posicao{}, err
	}
	return posicao{}, h.enviar(ctx, c, evento.Evento{
		Tipo: evento.Sincronizado, BoardID: boardID, Revisao: atual,
		Quantidade: 1, OcorridoEm: time.Now(),
	})
}

// reporPorRevisao entrega os eventos posteriores à revisão informada.
func (h *Handler) reporPorRevisao(ctx context.Context, c *websocket.Conn, boardID, bruta string) (posicao, error) {
	confirmada, err := strconv.ParseInt(bruta, 10, 64)
	if err != nil || confirmada < 0 {
		return posicao{}, fmt.Errorf("revisao inválida: %q", bruta)
	}

	// maxBacklog+1 para DETECTAR o excesso: com o teto exato não daria para
	// distinguir "cabe justo" de "tem mais".
	perdidos, err := h.historico.DesdeRevisao(ctx, boardID, confirmada, maxBacklog+1)
	if err != nil {
		return posicao{}, err
	}

	if len(perdidos) > maxBacklog {
		return posicao{}, h.mandarRecarregar(ctx, c, boardID)
	}

	// O corte respeita o GRUPO. Se o último evento da lista for parte de uma
	// revisão que continua além do limite, ele é descartado junto com os irmãos:
	// entregar meia revisão faria o cliente confirmar um estado que ele aplicou
	// pela metade, e o resto nunca mais seria entregue — está abaixo do cursor.
	perdidos = ateOFimDoGrupo(perdidos)

	var ultima posicao
	for _, e := range perdidos {
		if err := h.enviar(ctx, c, e); err != nil {
			return posicao{}, err
		}
		ultima = posicao{revisao: e.Revisao, indice: e.Indice}
	}

	// Fecha o intervalo dizendo até onde ele ia, mesmo sem nada entregue: sem
	// isto, um cliente cuja revisão confirmada já é a atual não teria
	// confirmação de que está em dia.
	atual, err := h.historico.RevisaoAtual(ctx, boardID)
	if err != nil {
		return ultima, err
	}
	return ultima, h.enviar(ctx, c, evento.Evento{
		Tipo: evento.Sincronizado, BoardID: boardID, Revisao: atual,
		Quantidade: 1, OcorridoEm: time.Now(),
	})
}

// ateOFimDoGrupo descarta o último grupo quando ele está incompleto.
//
// Um grupo está completo quando o número de eventos daquela revisão presentes
// na lista é igual à `quantidade` que eles declaram.
func ateOFimDoGrupo(eventos []evento.Evento) []evento.Evento {
	if len(eventos) == 0 {
		return eventos
	}
	ultimo := eventos[len(eventos)-1]
	if ultimo.Quantidade <= 1 || ultimo.Indice == ultimo.Quantidade-1 {
		return eventos
	}
	corte := len(eventos)
	for corte > 0 && eventos[corte-1].Revisao == ultimo.Revisao {
		corte--
	}
	return eventos[:corte]
}

// reporPorSeq é o caminho do cliente ANTERIOR, que usa o seq como cursor.
//
// Mantido só durante o deploy de transição. Ele tem o defeito conhecido — o seq
// registra a ordem de alocação e não a de commit —, e é por isso que ele sai
// junto com o último cliente antigo.
func (h *Handler) reporPorSeq(ctx context.Context, c *websocket.Conn, boardID, bruta string) (posicao, error) {
	ultimoAplicado, err := strconv.ParseInt(bruta, 10, 64)
	if err != nil || ultimoAplicado < 0 {
		return posicao{}, fmt.Errorf("desde inválido: %q", bruta)
	}

	perdidos, err := h.historico.Desde(ctx, boardID, ultimoAplicado, maxBacklog+1)
	if err != nil {
		return posicao{}, err
	}
	if len(perdidos) > maxBacklog {
		return posicao{}, h.mandarRecarregar(ctx, c, boardID)
	}

	corte := posicao{porSeq: true, seq: ultimoAplicado}
	for _, e := range perdidos {
		if err := h.enviar(ctx, c, e); err != nil {
			return corte, err
		}
		corte.seq = e.Seq
	}
	if corte.seq > ultimoAplicado {
		return corte, h.enviar(ctx, c, evento.Evento{
			Tipo: evento.Sincronizado, BoardID: boardID, Seq: corte.seq,
			Quantidade: 1, OcorridoEm: time.Now(),
		})
	}
	return corte, nil
}

// mandarRecarregar diz ao cliente para buscar o quadro inteiro.
//
// É a resposta a um intervalo grande demais — uma aba que ficou uma semana
// fechada —, e não uma falha: uma requisição resolve, e o resultado é sempre
// correto, ao contrário de reproduzir centenas de eventos em memória.
func (h *Handler) mandarRecarregar(ctx context.Context, c *websocket.Conn, boardID string) error {
	atual, err := h.historico.RevisaoAtual(ctx, boardID)
	if err != nil {
		return err
	}
	return h.enviar(ctx, c, evento.Evento{
		Tipo: evento.RecarregueTudo, BoardID: boardID, Revisao: atual,
		Quantidade: 1, OcorridoEm: time.Now(),
	})
}
