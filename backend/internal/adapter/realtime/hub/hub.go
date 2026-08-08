// Package hub mantém as conexões abertas e entrega os eventos a quem está
// olhando o mesmo quadro.
//
// É a peça concorrente do projeto, e o desenho segue o padrão do exemplo de
// chat do gorilla/websocket: o estado compartilhado é um mapa de salas, e todo
// acesso a ele passa por um RWMutex — leitura (publicar) é muito mais frequente
// que escrita (entrar e sair), e o RWMutex deixa as leituras correrem juntas.
//
// A regra que sustenta tudo: **publicar nunca bloqueia**. Cada assinante tem um
// canal com buffer, e quem não consegue acompanhar é DESCONECTADO em vez de
// segurar o publicador. Um `select` com `default` é o que transforma "cliente
// lento" num problema do cliente, e não do servidor inteiro.
package hub

import (
	"sync"

	"stacktrack/internal/domain/evento"
)

// tamanhoDaFila é quantos eventos um assinante pode ficar devendo antes de ser
// considerado lento demais.
//
// Não é 0 (que derrubaria qualquer um numa rajada legítima, como arrastar
// rápido) nem grande (que guardaria memória por conexão morta e ainda entregaria
// um passado longo quando ela voltasse). 32 cobre uma rajada de interação humana
// com folga.
const tamanhoDaFila = 32

// Assinante é uma conexão interessada num quadro.
type Assinante struct {
	// Eventos é por onde o hub entrega. Quem assina lê deste canal até ele
	// fechar — o fechamento é o sinal de que o hub desistiu desta conexão.
	Eventos chan evento.Evento
	boardID string
}

// Hub distribui eventos por sala. A sala é o quadro: quem não participa dele
// nunca é registrado, e por isso nunca recebe nada.
type Hub struct {
	mu      sync.RWMutex
	salas   map[string]map[*Assinante]struct{}
	fechado bool
}

// Novo cria um hub vazio.
func Novo() *Hub {
	return &Hub{salas: make(map[string]map[*Assinante]struct{})}
}

// Assinar registra uma conexão na sala do quadro e devolve o assinante.
//
// Devolve nil se o hub já estiver desligando: quem chega durante o shutdown não
// entra numa sala que ninguém mais vai esvaziar.
func (h *Hub) Assinar(boardID string) *Assinante {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.fechado {
		return nil
	}

	a := &Assinante{Eventos: make(chan evento.Evento, tamanhoDaFila), boardID: boardID}
	if h.salas[boardID] == nil {
		h.salas[boardID] = make(map[*Assinante]struct{})
	}
	h.salas[boardID][a] = struct{}{}
	return a
}

// Cancelar tira o assinante da sala e fecha o canal dele.
//
// É idempotente porque a conexão pode sair por dois caminhos ao mesmo tempo: o
// `defer` do handler e a desconexão por lentidão. Chamar duas vezes fecharia um
// canal já fechado — que é panic, não erro.
func (h *Hub) Cancelar(a *Assinante) {
	if a == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.removerBloqueado(a)
}

// removerBloqueado exige que o chamador já tenha o mutex de escrita.
func (h *Hub) removerBloqueado(a *Assinante) {
	sala, existe := h.salas[a.boardID]
	if !existe {
		return
	}
	if _, inscrito := sala[a]; !inscrito {
		return
	}
	delete(sala, a)
	close(a.Eventos)
	// Sala vazia é removida: sem isso o mapa cresceria para sempre, guardando
	// uma entrada por quadro que alguém abriu uma vez.
	if len(sala) == 0 {
		delete(h.salas, a.boardID)
	}
}

// Publicar entrega o evento a todo mundo na sala do quadro, SEM bloquear.
//
// Quem não tem espaço na fila é desconectado ali mesmo: o canal fecha, e a
// goroutine de escrita daquela conexão encerra sozinha ao ver o fechamento. É
// a decisão que impede um cliente travado de segurar o publicador — que, no
// caminho de escrita, é a requisição HTTP de outra pessoa.
func (h *Hub) Publicar(e evento.Evento) {
	h.mu.Lock()
	defer h.mu.Unlock()

	sala := h.salas[e.BoardID]
	if len(sala) == 0 {
		return
	}

	var lentos []*Assinante
	for a := range sala {
		select {
		case a.Eventos <- e:
		default:
			lentos = append(lentos, a)
		}
	}
	// A remoção sai do laço porque mexer no mapa durante o range sobre ele é
	// pedir problema.
	for _, a := range lentos {
		h.removerBloqueado(a)
	}
}

// Inscritos informa quantas conexões acompanham o quadro. Serve para o teste e,
// na fase 6, para a presença.
func (h *Hub) Inscritos(boardID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.salas[boardID])
}

// Fechar encerra todas as conexões e impede novas assinaturas.
//
// Chamado no desligamento gracioso: sem isto, o processo esperaria pelo
// timeout do shutdown com conexões que nunca vão terminar sozinhas — uma
// conexão de tempo real não tem fim natural.
func (h *Hub) Fechar() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.fechado = true
	for _, sala := range h.salas {
		for a := range sala {
			close(a.Eventos)
		}
	}
	h.salas = make(map[string]map[*Assinante]struct{})
}
