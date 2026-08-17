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
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"

	"stacktrack/internal/adapter/realtime/hub"
	"stacktrack/internal/domain/evento"
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
type Historico interface {
	Desde(ctx context.Context, boardID string, seq int64, limite int) ([]evento.Evento, error)
	UltimoSeq(ctx context.Context, boardID string) (int64, error)
}

// maxBacklog é o teto de eventos entregues num reconnect.
//
// Uma aba que ficou uma semana fechada pediria a história inteira do quadro, e
// montá-la em memória para entregá-la seria uma negação de serviço com pedido
// educado. Passando do teto, o cliente é mandado recarregar tudo — mais barato
// e sempre correto.
const maxBacklog = 200

// Handler serve o endpoint de tempo real.
type Handler struct {
	hub            *hub.Hub
	historico      Historico
	autorizador    Autorizador
	identidade     Identificador
	nome           Nomeador
	sessaoValida   SessaoValida
	nomeCookie     string
	origensAceitas []string
	revalidarACada time.Duration
	log            *slog.Logger
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
		revalidarACada: intervaloRevalidacao,
	}
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

// mensagem é o formato que viaja no fio. É separado de evento.Evento de
// propósito: o domínio não deve ganhar tags de JSON por causa do transporte.
type mensagem struct {
	// Seq é a posição na história do quadro. O cliente guarda a maior que
	// aplicou e a devolve no reconnect, em ?desde=N.
	Seq     int64       `json:"seq,omitempty"`
	Tipo    evento.Tipo `json:"tipo"`
	BoardID string      `json:"boardId"`
	AutorID string      `json:"autorId"`
	Em      time.Time   `json:"em"`
	Dados   any         `json:"dados,omitempty"`
}

// Acompanhar autentica, confere o acesso ao quadro e mantém a conexão aberta
// entregando os eventos daquele quadro.
func (h *Handler) Acompanhar(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.identidade(r.Context())
	if !ok {
		http.Error(w, "não autenticado", http.StatusUnauthorized)
		return
	}

	boardID := r.URL.Query().Get("board")
	if boardID == "" {
		http.Error(w, "informe o quadro", http.StatusBadRequest)
		return
	}
	// Mesma regra do resto da API: quem não participa recebe 404, nunca 403 —
	// um 403 confirmaria que o quadro existe.
	if !h.autorizador.PodeVer(r.Context(), boardID, usuarioID) {
		http.Error(w, "quadro não encontrado", http.StatusNotFound)
		return
	}

	conexao, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: h.origensAceitas,
	})
	if err != nil {
		// O Accept já respondeu ao cliente; aqui só resta registrar.
		h.log.Debug("handshake recusado", "erro", err, "board", boardID)
		return
	}
	defer conexao.CloseNow()

	assinante := h.hub.Assinar(boardID, hub.Pessoa{ID: usuarioID, Nome: h.nome(r.Context(), usuarioID)})
	if assinante == nil {
		// O hub está desligando.
		_ = conexao.Close(websocket.StatusGoingAway, "servidor encerrando")
		return
	}
	defer h.hub.Cancelar(assinante)

	// O que o cliente perdeu enquanto esteve fora, ANTES de qualquer evento ao
	// vivo. A ordem é o ponto: entregar o backlog depois faria o cliente aplicar
	// o passado por cima do presente.
	//
	// A assinatura já aconteceu, então os eventos que chegarem durante a
	// reposição ficam na fila do canal e são entregues em seguida — sem buraco
	// entre o fim da história e o começo do ao vivo, que é exatamente onde um
	// desenho ingênuo perde eventos.
	if err := h.repor(r.Context(), conexao, boardID, usuarioID, r.URL.Query().Get("desde")); err != nil {
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

	h.escreverAteMorrer(ctx, conexao, assinante, usuarioID, boardID, tokenDaSessao)
}

// aindaPodeAcompanhar reconfere, com a conexão já aberta, que quem está na
// ponta continua tendo direito ao que recebe.
//
// São duas perguntas independentes, e as duas mudam sem que a conexão saiba:
// a sessão pode ter sido encerrada (logout) e a participação no quadro pode ter
// sido revogada (o dono removeu o membro).
func (h *Handler) aindaPodeAcompanhar(ctx context.Context, usuarioID, boardID, token string) bool {
	if h.sessaoValida != nil && !h.sessaoValida(ctx, token) {
		return false
	}
	return h.autorizador.PodeVer(ctx, boardID, usuarioID)
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
		h.hub.DefinirFoco(a, msg.ColunaID)
	}
}

func (h *Handler) escreverAteMorrer(
	ctx context.Context,
	c *websocket.Conn,
	a *hub.Assinante,
	usuarioID, boardID, tokenDaSessao string,
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
				// conexão por ela não acompanhar o ritmo.
				_ = c.Close(websocket.StatusPolicyViolation, "conexão lenta demais")
				return
			}
			// O próprio autor não recebe o eco: ele já mexeu na tela quando
			// agiu, e reaplicar causaria um solavanco no que ele acabou de
			// fazer. O filtro é aqui, e não no hub, porque o hub não sabe quem
			// é cada conexão — e essa ignorância é o que o mantém simples.
			if e.AutorID == usuarioID {
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
		Tipo:    e.Tipo,
		BoardID: e.BoardID,
		AutorID: e.AutorID,
		Em:      e.OcorridoEm,
		Dados:   e.Dados,
		Seq:     e.Seq,
	})
	if err != nil {
		return err
	}
	return c.Write(prazo, websocket.MessageText, corpo)
}

// repor entrega o que aconteceu desde o seq informado.
//
// Sem `desde`, é a primeira conexão: nada de história, só a posição atual —
// para a próxima reconexão ter de onde partir. Com `desde` grande demais, ou
// com backlog além do teto, manda recarregar tudo em vez de reproduzir.
//
// O eco do próprio autor é filtrado aqui pelo mesmo motivo que no ao vivo: o
// que a pessoa fez ela já viu acontecer na própria tela, e devolver isso na
// reconexão faria a tela dela dar um solavanco reaplicando o próprio passado.
func (h *Handler) repor(ctx context.Context, c *websocket.Conn, boardID, usuarioID, desde string) error {
	if h.historico == nil {
		return nil
	}

	if desde == "" {
		atual, err := h.historico.UltimoSeq(ctx, boardID)
		if err != nil {
			return err
		}
		return h.enviar(ctx, c, evento.Evento{
			Tipo: evento.Sincronizado, BoardID: boardID, Seq: atual,
			OcorridoEm: time.Now(),
		})
	}

	ultimoAplicado, err := strconv.ParseInt(desde, 10, 64)
	if err != nil || ultimoAplicado < 0 {
		return fmt.Errorf("desde inválido: %q", desde)
	}

	perdidos, err := h.historico.Desde(ctx, boardID, ultimoAplicado, maxBacklog+1)
	if err != nil {
		return err
	}

	// Backlog além do teto: em vez de reproduzir centenas de eventos, manda a
	// tela buscar o quadro inteiro. Uma requisição resolve, e o resultado é o
	// mesmo — com a vantagem de ser sempre correto.
	if len(perdidos) > maxBacklog {
		atual, err := h.historico.UltimoSeq(ctx, boardID)
		if err != nil {
			return err
		}
		return h.enviar(ctx, c, evento.Evento{
			Tipo: evento.RecarregueTudo, BoardID: boardID, Seq: atual,
			OcorridoEm: time.Now(),
		})
	}

	var ultimoDoIntervalo int64
	for _, e := range perdidos {
		ultimoDoIntervalo = e.Seq
		if e.AutorID == usuarioID {
			continue
		}
		if err := h.enviar(ctx, c, e); err != nil {
			return err
		}
	}

	// Fecha o intervalo dizendo até onde ele ia, mesmo que nada tenha sido
	// entregue. Sem isto, um backlog inteiro de eventos do próprio autor não
	// faria o cliente avançar o seq: ele pediria o MESMO intervalo na próxima
	// reconexão, e o intervalo só cresceria — até estourar o teto e provocar
	// uma recarga completa que nada justificava.
	if ultimoDoIntervalo > 0 {
		return h.enviar(ctx, c, evento.Evento{
			Tipo: evento.Sincronizado, BoardID: boardID, Seq: ultimoDoIntervalo,
			OcorridoEm: time.Now(),
		})
	}
	return nil
}
