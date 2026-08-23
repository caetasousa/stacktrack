// Package hub mantém as conexões abertas e entrega os eventos a quem está
// olhando o mesmo quadro.
//
// É a peça concorrente do projeto, e o desenho segue o padrão do exemplo de
// chat do gorilla/websocket: o estado compartilhado é um mapa de salas, e todo
// acesso a ele passa por um mutex.
//
// ⚠️ O mutex é RWMutex, mas NÃO espere dele leituras concorrentes no caminho
// quente. `Publicar` toma o lock EXCLUSIVO, e não o de leitura, porque ele
// pode remover da sala quem não acompanha o ritmo — remoção é escrita. Só
// `Presentes` e `Inscritos` usam RLock de verdade.
//
// A consequência a conhecer: como o mutex é do hub inteiro, e não de cada sala,
// publicar no quadro A serializa com publicar no quadro B. Enquanto o gargalo
// for a rede, e não o mapa, isso não aparece; o dia em que aparecer, o caminho
// é um mutex por sala (ou sync.Map de salas), não trocar este por RLock — que
// seria uma corrida, já que a remoção do lento escreve no mapa.
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
	// EditandoColunaID é a coluna que esta pessoa está editando AGORA, quando
	// está editando alguma. Vazio é o caso normal.
	//
	// Viaja junto da presença, e não num evento próprio, porque é a mesma
	// pergunta: "quem está aqui, e fazendo o quê". Um segundo canal para isto
	// teria de repetir a entrada, a saída e a deduplicação por pessoa — e
	// divergir do primeiro no dia em que alguém mexesse só num.
	//
	// É estado EFÊMERO, como a presença: vive no mapa de conexões e morre com o
	// processo. Não há migration para ele, e é isso que o torna correto — um
	// "está editando" persistido num banco é um cadeado que ninguém abre quando
	// o navegador fecha sem avisar.
	EditandoColunaID string `json:"editandoColunaId,omitempty"`
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
	mu sync.RWMutex
	// derrubadosPorLentidao conta as conexões desligadas por não acompanharem o
	// ritmo. É o sinal de degradação do tempo real — ver Publicar.
	derrubadosPorLentidao int64
	salas                 map[string]map[*Assinante]struct{}
	fechado               bool
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
	// Duas abas da mesma pessoa viram um avatar só, e a dedup acima fica com a
	// PRIMEIRA que aparecer no mapa — cuja ordem o Go não garante. Se ela
	// estiver sem foco e a outra estiver editando, o "está editando" some por
	// sorteio. Esta passada devolve o foco de qualquer aba daquela pessoa.
	for a := range h.salas[boardID] {
		if a.pessoa.ID == "" || a.pessoa.EditandoColunaID == "" {
			continue
		}
		for i := range lista {
			if lista[i].ID == a.pessoa.ID && lista[i].EditandoColunaID == "" {
				lista[i].EditandoColunaID = a.pessoa.EditandoColunaID
			}
		}
	}
	// Ordem estável: sem ela a lista chega embaralhada a cada evento, e a tela
	// reordenaria os avatares sozinha a cada entrada e saída.
	sort.Slice(lista, func(i, j int) bool { return lista[i].ID < lista[j].ID })
	return lista
}

// DefinirFoco registra que esta conexão está editando uma coluna — ou que
// parou, quando colunaID vem vazio — e reanuncia a presença da sala.
//
// Reanunciar é o ponto: sem isso o servidor saberia quem está editando e
// ninguém mais ficaria sabendo.
//
// Não valida se a coluna pertence ao quadro, e não precisa: o anúncio só sai
// para a sala DAQUELE quadro, e um id que não existe lá não casa com coluna
// nenhuma na tela — vira silêncio, não vazamento. O que precisa de limite é o
// TAMANHO, e esse é aplicado na borda, antes de chegar aqui.
func (h *Hub) DefinirFoco(a *Assinante, colunaID string) {
	if a == nil {
		return
	}
	h.mu.Lock()
	if h.fechado {
		h.mu.Unlock()
		return
	}
	if a.pessoa.EditandoColunaID == colunaID {
		// Nada mudou: reanunciar geraria uma rajada de eventos idênticos a cada
		// tecla digitada, que é exatamente o que o cliente não deve conseguir
		// provocar.
		h.mu.Unlock()
		return
	}
	a.pessoa.EditandoColunaID = colunaID
	presentes := h.presentesBloqueado(a.boardID)
	h.mu.Unlock()

	h.anunciarPresenca(a.boardID, presentes)
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
//
// Derrubar alguém MUDA quem está no quadro, e por isso a sala é avisada em
// seguida. Sem esse aviso, o avatar de quem caiu por lentidão ficava preso na
// tela dos outros: o `defer Cancelar` do handler roda depois e encontra o
// assinante já fora da sala, então não anuncia nada — e ninguém mais o faria.
func (h *Hub) Publicar(e evento.Evento) {
	h.mu.Lock()

	sala := h.salas[e.BoardID]
	if len(sala) == 0 {
		h.mu.Unlock()
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
	// Consumidor lento derrubado é DEGRADAÇÃO, e degradação precisa ser
	// contável. Sem isto ela era silenciosa: o socket fechava com um motivo que
	// só o cliente via, e do lado do servidor não sobrava nada — nem para
	// investigar depois, nem para alertar antes. O contador é o que A8 publica
	// como métrica.
	h.derrubadosPorLentidao += int64(len(lentos))

	var presentes []Pessoa
	if len(lentos) > 0 {
		presentes = h.presentesBloqueado(e.BoardID)
	}
	h.mu.Unlock()

	// Fora do lock, como em Assinar e Cancelar: anunciarPresenca chama Publicar,
	// e com o mutex na mão isso travaria o processo em si mesmo.
	//
	// A recursão termina: cada volta só acontece quando alguém foi removido, e
	// a sala é finita — no limite ela esvazia e o `len(sala) == 0` acima corta.
	if len(lentos) > 0 {
		h.anunciarPresenca(e.BoardID, presentes)
	}
}

// DerrubadosPorLentidao informa quantas conexões foram desligadas por não
// acompanharem o ritmo desde que o processo subiu.
//
// Crescendo, significa que alguém está recebendo mais eventos do que consegue
// aplicar — rede ruim, aba em segundo plano, ou um quadro produzindo eventos
// rápido demais. É a métrica que A8 transforma em alerta.
func (h *Hub) DerrubadosPorLentidao() int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.derrubadosPorLentidao
}

// Inscritos informa quantas CONEXÕES acompanham o quadro — não quantas pessoas.
// Duas abas da mesma conta contam duas vezes aqui e uma só em Presentes.
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
