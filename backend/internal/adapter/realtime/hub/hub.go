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
	"sort"
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

// Pessoa é quem está com o quadro aberto. O hub guarda o mínimo para a tela
// desenhar um avatar — nada de sessão, email ou papel.
type Pessoa struct {
	ID   string `json:"id"`
	Nome string `json:"nome"`
}

// Assinante é uma conexão interessada num quadro.
type Assinante struct {
	// Eventos é por onde o hub entrega. Quem assina lê deste canal até ele
	// fechar — o fechamento é o sinal de que o hub desistiu desta conexão.
	Eventos chan evento.Evento
	boardID string
	pessoa  Pessoa
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
func (h *Hub) Assinar(boardID string, quem Pessoa) *Assinante {
	h.mu.Lock()

	if h.fechado {
		h.mu.Unlock()
		return nil
	}

	a := &Assinante{Eventos: make(chan evento.Evento, tamanhoDaFila), boardID: boardID, pessoa: quem}
	if h.salas[boardID] == nil {
		h.salas[boardID] = make(map[*Assinante]struct{})
	}
	h.salas[boardID][a] = struct{}{}
	presentes := h.presentesBloqueado(boardID)
	h.mu.Unlock()

	// O anúncio sai FORA do lock: Publicar também o pega, e chamá-lo com o
	// mutex na mão travaria o processo em si mesmo.
	h.anunciarPresenca(boardID, presentes)
	return a
}

// presentesBloqueado monta a lista de quem está na sala. Exige o mutex.
//
// Deduplica por pessoa: duas abas da mesma conta são duas conexões e UM avatar.
// Sem isso, quem abrisse o quadro em dois monitores apareceria como duas
// pessoas — e a contagem de "quem está aqui" deixaria de significar algo.
func (h *Hub) presentesBloqueado(boardID string) []Pessoa {
	vistos := make(map[string]struct{})
	lista := make([]Pessoa, 0, len(h.salas[boardID]))
	for a := range h.salas[boardID] {
		if a.pessoa.ID == "" {
			continue
		}
		if _, repetido := vistos[a.pessoa.ID]; repetido {
			continue
		}
		vistos[a.pessoa.ID] = struct{}{}
		lista = append(lista, a.pessoa)
	}
	// Ordem estável: sem ela a lista chega embaralhada a cada evento, e a tela
	// reordenaria os avatares sozinha a cada entrada e saída.
	sort.Slice(lista, func(i, j int) bool { return lista[i].ID < lista[j].ID })
	return lista
}

// Presentes informa quem está com o quadro aberto.
func (h *Hub) Presentes(boardID string) []Pessoa {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.presentesBloqueado(boardID)
}

// anunciarPresenca avisa a sala de quem está nela.
//
// AutorID vazio de propósito: presença não tem autor, e o filtro de eco do
// adaptador ignora quem agiu. Com um autor, justamente quem entrou não receberia
// a lista — e ficaria sem saber quem já estava lá.
func (h *Hub) anunciarPresenca(boardID string, presentes []Pessoa) {
	h.Publicar(evento.Novo(evento.PresencaAlterada, boardID, "", presentes))
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
	saiu := h.removerBloqueado(a)
	presentes := h.presentesBloqueado(a.boardID)
	h.mu.Unlock()

	if saiu {
		h.anunciarPresenca(a.boardID, presentes)
	}
}

// removerBloqueado exige que o chamador já tenha o mutex de escrita. Devolve
// se realmente removeu — chamar duas vezes é normal, e só a primeira anuncia.
func (h *Hub) removerBloqueado(a *Assinante) bool {
	sala, existe := h.salas[a.boardID]
	if !existe {
		return false
	}
	if _, inscrito := sala[a]; !inscrito {
		return false
	}
	delete(sala, a)
	close(a.Eventos)
	// Sala vazia é removida: sem isso o mapa cresceria para sempre, guardando
	// uma entrada por quadro que alguém abriu uma vez.
	if len(sala) == 0 {
		delete(h.salas, a.boardID)
	}
	return true
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
