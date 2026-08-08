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
	"log/slog"
	"net/http"
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
)

// Autorizador responde se o usuário pode acompanhar o quadro. É a mesma
// pergunta que o usecase faz antes de qualquer leitura — declarada aqui como
// interface para o adaptador não depender do usecase concreto.
type Autorizador interface {
	PodeVer(boardID, usuarioID string) bool
}

// Identificador extrai a identidade do contexto, já validada pelo middleware
// de sessão.
type Identificador func(ctx context.Context) (string, bool)

// Nomeador resolve o nome de exibição de quem conectou. É o que a presença
// mostra: um avatar com id cru não diz nada a ninguém.
type Nomeador func(usuarioID string) string

// Handler serve o endpoint de tempo real.
type Handler struct {
	hub            *hub.Hub
	autorizador    Autorizador
	identidade     Identificador
	nome           Nomeador
	origensAceitas []string
	log            *slog.Logger
}

// NovoHandler cria o handler do WebSocket.
//
// origensAceitas alimenta o OriginPatterns do coder/websocket, e não é
// opcional: **WebSocket não obedece CORS**. Sem checar a origem, qualquer site
// que a vítima visite abre uma conexão autenticada com o cookie dela e passa a
// ler o quadro inteiro em tempo real — o Cross-Site WebSocket Hijacking. O
// SameSite=Lax do cookie é a segunda camada, não a primeira.
func NovoHandler(
	h *hub.Hub,
	a Autorizador,
	id Identificador,
	nome Nomeador,
	origensAceitas []string,
	log *slog.Logger,
) *Handler {
	return &Handler{hub: h, autorizador: a, identidade: id, nome: nome, origensAceitas: origensAceitas, log: log}
}

// mensagem é o formato que viaja no fio. É separado de evento.Evento de
// propósito: o domínio não deve ganhar tags de JSON por causa do transporte.
type mensagem struct {
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
	if !h.autorizador.PodeVer(boardID, usuarioID) {
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

	assinante := h.hub.Assinar(boardID, hub.Pessoa{ID: usuarioID, Nome: h.nome(usuarioID)})
	if assinante == nil {
		// O hub está desligando.
		_ = conexao.Close(websocket.StatusGoingAway, "servidor encerrando")
		return
	}
	defer h.hub.Cancelar(assinante)

	// Um contexto por conexão: qualquer um dos lados que termine cancela o
	// outro. É o que faz a goroutine de leitura e a de escrita morrerem juntas
	// em vez de deixar uma pendurada.
	ctx, encerrar := context.WithCancel(r.Context())
	defer encerrar()

	// UMA goroutine de leitura e UMA de escrita, nunca mais.
	//
	// Escrever de dois lugares ao mesmo tempo corrompe o frame — a biblioteca
	// não serializa isso por você, e o sintoma é a conexão morrer com erro de
	// protocolo sob carga. Por isso todo envio sai daqui, do laço abaixo, e a
	// única fonte é o canal do assinante.
	go h.lerAteMorrer(ctx, encerrar, conexao)

	h.escreverAteMorrer(ctx, conexao, assinante, usuarioID, boardID)
}

// lerAteMorrer existe mesmo sem o cliente mandar nada.
//
// Sem ler, os pongs de resposta ao ping nunca seriam processados — a biblioteca
// os entrega pelo mesmo caminho das mensagens — e a conexão morreria sozinha em
// 30 segundos. Ler também é o que detecta o fechamento pelo lado do cliente.
func (h *Handler) lerAteMorrer(ctx context.Context, encerrar context.CancelFunc, c *websocket.Conn) {
	defer encerrar()
	for {
		if _, _, err := c.Read(ctx); err != nil {
			return
		}
		// O cliente não manda nada hoje. Quando mandar (a presença da fase 6),
		// é aqui que entra o tratamento.
	}
}

func (h *Handler) escreverAteMorrer(
	ctx context.Context,
	c *websocket.Conn,
	a *hub.Assinante,
	usuarioID, boardID string,
) {
	ticker := time.NewTicker(intervaloPing)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

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
	})
	if err != nil {
		return err
	}
	return c.Write(prazo, websocket.MessageText, corpo)
}
