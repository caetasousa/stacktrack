// Testes do hub. Rodam com -race de propósito: a peça inteira existe para ser
// usada por muitas goroutines ao mesmo tempo, e um teste sequencial provaria
// pouco sobre ela.
package realtime_test

import (
	"sync"
	"testing"
	"time"

	"stacktrack/internal/adapter/realtime/hub"
	"stacktrack/internal/domain/evento"
)

func evt(boardID, autorID string) evento.Evento {
	return evento.Novo(evento.CardMovido, boardID, autorID, nil)
}

func alguem(t *testing.T) hub.Pessoa {
	t.Helper()
	return hub.Pessoa{ID: "u-" + t.Name(), Nome: "alguém"}
}

// esperar lê até achar um evento do tipo pedido.
//
// Existe porque assinar uma sala JÁ entrega um evento: a lista de presentes.
// Sem filtrar, todo teste leria a presença achando que era o evento dele.
func esperar(t *testing.T, a *hub.Assinante, tipo evento.Tipo) evento.Evento {
	t.Helper()
	prazo := time.After(time.Second)
	for {
		select {
		case e, aberto := <-a.Eventos:
			if !aberto {
				t.Fatalf("o canal fechou antes de chegar %s", tipo)
			}
			if e.Tipo == tipo {
				return e
			}
		case <-prazo:
			t.Fatalf("o evento %s não chegou", tipo)
		}
	}
}

func TestQuemAssinaRecebeOEventoDoSeuQuadro(t *testing.T) {
	h := hub.Novo()
	a := h.Assinar("quadro-1", alguem(t))

	h.Publicar(evt("quadro-1", "ana"))

	if e := esperar(t, a, evento.CardMovido); e.BoardID != "quadro-1" {
		t.Errorf("recebeu evento de %q", e.BoardID)
	}
}

// A sala é a fronteira de acesso: quem está no quadro A não pode receber nada
// do quadro B, nem por engano de roteamento.
func TestEventoNaoVazaEntreQuadros(t *testing.T) {
	h := hub.Novo()
	a := h.Assinar("quadro-1", alguem(t))
	b := h.Assinar("quadro-2", alguem(t))

	h.Publicar(evt("quadro-1", "ana"))
	esperar(t, a, evento.CardMovido)

	// b só pode ter recebido a própria presença, nunca o card do outro quadro.
	for {
		select {
		case e := <-b.Eventos:
			if e.Tipo != evento.PresencaAlterada {
				t.Fatalf("vazou para outro quadro: %+v", e)
			}
		case <-time.After(100 * time.Millisecond):
			return
		}
	}
}

func TestTodosDaSalaRecebem(t *testing.T) {
	h := hub.Novo()
	assinantes := make([]*hub.Assinante, 5)
	for i := range assinantes {
		assinantes[i] = h.Assinar("quadro-1", alguem(t))
	}

	h.Publicar(evt("quadro-1", "ana"))

	for _, a := range assinantes {
		esperar(t, a, evento.CardMovido)
	}
}

// O comportamento que define o hub: publicar não pode esperar por ninguém.
//
// Um assinante que parou de ler enche a fila e é DESCONECTADO — o canal dele
// fecha. Sem isso, a requisição HTTP de quem moveu o card ficaria bloqueada até
// o cliente lento voltar, que pode ser nunca.
func TestAssinanteQueNaoLeEDesconectado(t *testing.T) {
	h := hub.Novo()
	lento := h.Assinar("quadro-1", alguem(t))
	atento := h.Assinar("quadro-1", alguem(t))

	// Uma a mais que o buffer: a que estoura é a que derruba.
	const demais = 200
	pronto := make(chan struct{})
	go func() {
		defer close(pronto)
		for i := 0; i < demais; i++ {
			h.Publicar(evt("quadro-1", "ana"))
		}
	}()

	select {
	case <-pronto:
	case <-time.After(2 * time.Second):
		t.Fatal("Publicar bloqueou por causa de um assinante que não lê")
	}

	// O canal do lento tem de estar fechado: drena e confirma o fechamento.
	prazo := time.After(time.Second)
	for {
		select {
		case _, aberto := <-lento.Eventos:
			if !aberto {
				goto conferirAtento
			}
		case <-prazo:
			t.Fatal("o assinante lento não foi desconectado")
		}
	}

conferirAtento:
	// E o atento também caiu, porque ele igualmente não estava lendo — o que
	// importa é que o publicador seguiu em frente. Só a sala precisa ter sido
	// esvaziada.
	if h.Inscritos("quadro-1") != 0 {
		t.Errorf("sobrou assinante lento na sala: %d", h.Inscritos("quadro-1"))
	}
	_ = atento
}

func TestCancelarDuasVezesNaoEntraEmPanico(t *testing.T) {
	h := hub.Novo()
	a := h.Assinar("quadro-1", alguem(t))

	h.Cancelar(a)
	h.Cancelar(a) // o handler tem dois caminhos de saída; os dois chamam isto
	h.Cancelar(nil)

	if h.Inscritos("quadro-1") != 0 {
		t.Errorf("assinante continuou na sala")
	}
}

// Sala vazia tem de sumir do mapa: sem isso ele cresceria para sempre,
// guardando uma entrada por quadro que alguém abriu uma vez.
func TestSalaVaziaSaiDoMapa(t *testing.T) {
	h := hub.Novo()
	a := h.Assinar("quadro-1", alguem(t))
	if h.Inscritos("quadro-1") != 1 {
		t.Fatal("não assinou")
	}
	h.Cancelar(a)
	if h.Inscritos("quadro-1") != 0 {
		t.Error("a sala não foi esvaziada")
	}
}

func TestFecharDerrubaTodosEImpedeNovasAssinaturas(t *testing.T) {
	h := hub.Novo()
	a := h.Assinar("quadro-1", alguem(t))
	b := h.Assinar("quadro-2", alguem(t))

	h.Fechar()

	// Drena o que já estava na fila (a própria presença) e confirma que o canal
	// FECHA — é o fechamento que avisa a goroutine de escrita para encerrar.
	for nome, canal := range map[string]chan evento.Evento{"a": a.Eventos, "b": b.Eventos} {
		prazo := time.After(time.Second)
		fechou := false
		for !fechou {
			select {
			case _, aberto := <-canal:
				fechou = !aberto
			case <-prazo:
				t.Errorf("o canal de %s não foi fechado", nome)
				fechou = true
			}
		}
	}

	if h.Assinar("quadro-3", alguem(t)) != nil {
		t.Error("aceitou assinatura durante o desligamento")
	}
}

// O teste que justifica o -race: entrar, sair e publicar ao mesmo tempo, de
// muitas goroutines. Sem o mutex protegendo o mapa de salas, isto acusa.
func TestUsoConcorrenteNaoTemCorrida(t *testing.T) {
	h := hub.Novo()
	defer h.Fechar()

	var wg sync.WaitGroup
	const goroutines = 20

	// Publicadores, como se fossem requisições HTTP simultâneas.
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				h.Publicar(evt("quadro-1", "ana"))
			}
		}()
	}

	// Conexões entrando e saindo o tempo todo.
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				a := h.Assinar("quadro-1", alguem(t))
				if a == nil {
					return
				}
				// Lê o que der, como a goroutine de escrita faria.
				select {
				case <-a.Eventos:
				default:
				}
				h.Cancelar(a)
			}
		}()
	}

	// E alguém perguntando o tamanho da sala, que é o caminho de leitura.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 500; j++ {
			_ = h.Inscritos("quadro-1")
		}
	}()

	wg.Wait()
}

// --- presença ---------------------------------------------------------------

func TestQuemEntraRecebeAListaDePresentes(t *testing.T) {
	h := hub.Novo()
	ana := h.Assinar("quadro-1", hub.Pessoa{ID: "ana", Nome: "Ana"})

	e := esperar(t, ana, evento.PresencaAlterada)
	presentes, ok := e.Dados.([]hub.Pessoa)
	if !ok || len(presentes) != 1 || presentes[0].ID != "ana" {
		t.Fatalf("presentes = %#v", e.Dados)
	}

	// E quem já estava é avisado quando alguém chega.
	h.Assinar("quadro-1", hub.Pessoa{ID: "bob", Nome: "Bob"})
	e = esperar(t, ana, evento.PresencaAlterada)
	if presentes, _ := e.Dados.([]hub.Pessoa); len(presentes) != 2 {
		t.Errorf("depois da entrada do bob, presentes = %#v", e.Dados)
	}
}

// Duas abas da mesma conta são duas conexões e UM avatar. Sem deduplicar, quem
// abrisse o quadro em dois monitores apareceria como duas pessoas — e "quem
// está aqui" deixaria de significar algo.
func TestDuasAbasDaMesmaPessoaContamComoUma(t *testing.T) {
	h := hub.Novo()
	h.Assinar("quadro-1", hub.Pessoa{ID: "ana", Nome: "Ana"})
	h.Assinar("quadro-1", hub.Pessoa{ID: "ana", Nome: "Ana"})

	if presentes := h.Presentes("quadro-1"); len(presentes) != 1 {
		t.Errorf("presentes = %#v, esperado só a ana", presentes)
	}
	if h.Inscritos("quadro-1") != 2 {
		t.Errorf("conexões = %d, esperado 2", h.Inscritos("quadro-1"))
	}
}

func TestSairAvisaQuemFicou(t *testing.T) {
	h := hub.Novo()
	ana := h.Assinar("quadro-1", hub.Pessoa{ID: "ana", Nome: "Ana"})
	bob := h.Assinar("quadro-1", hub.Pessoa{ID: "bob", Nome: "Bob"})

	esperar(t, ana, evento.PresencaAlterada) // a própria entrada
	esperar(t, ana, evento.PresencaAlterada) // a entrada do bob

	h.Cancelar(bob)

	e := esperar(t, ana, evento.PresencaAlterada)
	presentes, _ := e.Dados.([]hub.Pessoa)
	if len(presentes) != 1 || presentes[0].ID != "ana" {
		t.Errorf("depois da saída do bob, presentes = %#v", e.Dados)
	}
}

// Cair por lentidão também é sair, e quem fica precisa saber.
//
// É o defeito que este teste tranca: a remoção por lentidão acontece dentro de
// Publicar, e o `defer Cancelar` do handler roda depois — encontrando o
// assinante já fora da sala, ele não anuncia nada. Sem o aviso de dentro do
// próprio Publicar, o avatar de quem caiu ficava preso na tela dos outros até
// o próximo evento de presença, que pode nunca vir.
func TestQuedaPorLentidaoAvisaQuemFicou(t *testing.T) {
	h := hub.Novo()
	ana := h.Assinar("quadro-1", hub.Pessoa{ID: "ana", Nome: "Ana"})

	// A ana lê o tempo todo; sem isso ela encheria junto e cairia também.
	recebidos := make(chan evento.Evento, 4096)
	go func() {
		for e := range ana.Eventos {
			recebidos <- e
		}
	}()

	h.Assinar("quadro-1", hub.Pessoa{ID: "bob", Nome: "Bob"}) // o bob nunca lê

	// Consome as presenças até ver a sala com os dois: só a partir daí uma
	// presença com uma pessoa significa "o bob saiu".
	esperarPresencaCom(t, recebidos, 2)

	for i := 0; i < 100; i++ {
		h.Publicar(evt("quadro-1", "ana"))
		// A pausa dá à goroutine da ana a chance de drenar. Sem ela os dois
		// enchem, os dois caem, e o teste deixaria de falar sobre a presença.
		time.Sleep(time.Millisecond)
	}

	if p := h.Presentes("quadro-1"); len(p) != 1 {
		t.Fatalf("o bob não foi derrubado; presentes = %#v", p)
	}
	esperarPresencaCom(t, recebidos, 1)
}

// esperarPresencaCom lê até achar um evento de presença com o tamanho pedido.
func esperarPresencaCom(t *testing.T, recebidos chan evento.Evento, quantos int) {
	t.Helper()
	prazo := time.After(time.Second)
	for {
		select {
		case e := <-recebidos:
			if e.Tipo != evento.PresencaAlterada {
				continue
			}
			if p, _ := e.Dados.([]hub.Pessoa); len(p) == quantos {
				return
			}
		case <-prazo:
			t.Fatalf("não chegou presenca.alterada com %d pessoa(s)", quantos)
		}
	}
}

// A presença não vaza entre quadros pelo mesmo motivo que os eventos não vazam:
// a sala é a fronteira.
func TestPresencaNaoVazaEntreQuadros(t *testing.T) {
	h := hub.Novo()
	h.Assinar("quadro-1", hub.Pessoa{ID: "ana", Nome: "Ana"})

	if presentes := h.Presentes("quadro-2"); len(presentes) != 0 {
		t.Errorf("quadro-2 = %#v, esperado vazio", presentes)
	}
}

// --- quem está editando uma coluna -----------------------------------------

// O foco viaja JUNTO da presença, e não num evento próprio: é a mesma pergunta
// — "quem está aqui, e fazendo o quê". Este teste tranca o caminho inteiro: um
// avisa, o outro fica sabendo.
func TestQuemEditaUmaColunaApareceParaOsOutros(t *testing.T) {
	h := hub.Novo()
	ana := h.Assinar("quadro-1", hub.Pessoa{ID: "ana", Nome: "Ana"})
	bob := h.Assinar("quadro-1", hub.Pessoa{ID: "bob", Nome: "Bob"})
	esperar(t, ana, evento.PresencaAlterada)
	esperar(t, ana, evento.PresencaAlterada)

	h.DefinirFoco(bob, "coluna-7")

	e := esperar(t, ana, evento.PresencaAlterada)
	presentes, _ := e.Dados.([]hub.Pessoa)
	if focoDe(presentes, "bob") != "coluna-7" {
		t.Errorf("presentes = %#v, esperado o bob editando coluna-7", presentes)
	}
	if focoDe(presentes, "ana") != "" {
		t.Errorf("a ana não estava editando nada: %#v", presentes)
	}
}

// Parar de editar precisa avisar tanto quanto começar. Sem isto a coluna ficaria
// com um cadeado eterno na tela dos outros.
func TestPararDeEditarTambemAvisa(t *testing.T) {
	h := hub.Novo()
	ana := h.Assinar("quadro-1", hub.Pessoa{ID: "ana", Nome: "Ana"})
	bob := h.Assinar("quadro-1", hub.Pessoa{ID: "bob", Nome: "Bob"})
	esperar(t, ana, evento.PresencaAlterada)
	esperar(t, ana, evento.PresencaAlterada)

	h.DefinirFoco(bob, "coluna-7")
	esperar(t, ana, evento.PresencaAlterada)

	h.DefinirFoco(bob, "")

	e := esperar(t, ana, evento.PresencaAlterada)
	presentes, _ := e.Dados.([]hub.Pessoa)
	if focoDe(presentes, "bob") != "" {
		t.Errorf("o bob parou de editar e continuou marcado: %#v", presentes)
	}
}

// Fechar a aba solta o foco junto. É a razão de o "está editando" ser efêmero:
// persistido, ficaria travado por quem fechou o navegador sem avisar.
func TestSairSoltaAColunaQueEstavaSendoEditada(t *testing.T) {
	h := hub.Novo()
	ana := h.Assinar("quadro-1", hub.Pessoa{ID: "ana", Nome: "Ana"})
	bob := h.Assinar("quadro-1", hub.Pessoa{ID: "bob", Nome: "Bob"})
	esperar(t, ana, evento.PresencaAlterada)
	esperar(t, ana, evento.PresencaAlterada)
	h.DefinirFoco(bob, "coluna-7")
	esperar(t, ana, evento.PresencaAlterada)

	h.Cancelar(bob)

	e := esperar(t, ana, evento.PresencaAlterada)
	presentes, _ := e.Dados.([]hub.Pessoa)
	if len(presentes) != 1 || focoDe(presentes, "bob") != "" {
		t.Errorf("depois da saída do bob: %#v", presentes)
	}
}

// Repetir o mesmo foco NÃO pode gerar evento: o cliente reanuncia a cada
// mudança de estado, e sem esta guarda uma tela com defeito viraria uma rajada
// de presença para a sala inteira.
func TestFocoRepetidoNaoGeraEvento(t *testing.T) {
	h := hub.Novo()
	ana := h.Assinar("quadro-1", hub.Pessoa{ID: "ana", Nome: "Ana"})
	bob := h.Assinar("quadro-1", hub.Pessoa{ID: "bob", Nome: "Bob"})
	esperar(t, ana, evento.PresencaAlterada)
	esperar(t, ana, evento.PresencaAlterada)

	h.DefinirFoco(bob, "coluna-7")
	esperar(t, ana, evento.PresencaAlterada)

	h.DefinirFoco(bob, "coluna-7")

	select {
	case e := <-ana.Eventos:
		t.Errorf("o foco repetido gerou %q", e.Tipo)
	case <-time.After(80 * time.Millisecond):
	}
}

// Duas abas da mesma pessoa viram UM avatar, e a deduplicação fica com a
// primeira que o mapa devolver — cuja ordem o Go não garante. Se ela estiver
// sem foco e a outra estiver editando, o "está editando" sumiria por sorteio:
// o teste passaria na maioria das execuções e falharia sem explicação nas
// outras, que é o pior defeito que um teste pode esconder.
func TestFocoDeUmaAbaSobreviveAOutraAbaDaMesmaPessoa(t *testing.T) {
	for i := 0; i < 40; i++ {
		h := hub.Novo()
		h.Assinar("quadro-1", hub.Pessoa{ID: "ana", Nome: "Ana"})
		segunda := h.Assinar("quadro-1", hub.Pessoa{ID: "ana", Nome: "Ana"})
		h.DefinirFoco(segunda, "coluna-9")

		presentes := h.Presentes("quadro-1")
		if len(presentes) != 1 {
			t.Fatalf("presentes = %#v, esperada uma ana só", presentes)
		}
		if focoDe(presentes, "ana") != "coluna-9" {
			t.Fatalf("execução %d: o foco da segunda aba se perdeu: %#v", i, presentes)
		}
	}
}

// focoDe devolve a coluna que a pessoa está editando na lista de presentes.
func focoDe(presentes []hub.Pessoa, id string) string {
	for _, p := range presentes {
		if p.ID == id {
			return p.EditandoColunaID
		}
	}
	return ""
}
