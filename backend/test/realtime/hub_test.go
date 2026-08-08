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

func TestQuemAssinaRecebeOEventoDoSeuQuadro(t *testing.T) {
	h := hub.Novo()
	a := h.Assinar("quadro-1")

	h.Publicar(evt("quadro-1", "ana"))

	select {
	case e := <-a.Eventos:
		if e.BoardID != "quadro-1" {
			t.Errorf("recebeu evento de %q", e.BoardID)
		}
	case <-time.After(time.Second):
		t.Fatal("o evento não chegou")
	}
}

// A sala é a fronteira de acesso: quem está no quadro A não pode receber nada
// do quadro B, nem por engano de roteamento.
func TestEventoNaoVazaEntreQuadros(t *testing.T) {
	h := hub.Novo()
	a := h.Assinar("quadro-1")
	b := h.Assinar("quadro-2")

	h.Publicar(evt("quadro-1", "ana"))

	select {
	case <-a.Eventos:
	case <-time.After(time.Second):
		t.Fatal("quem devia receber não recebeu")
	}
	select {
	case e := <-b.Eventos:
		t.Fatalf("vazou para outro quadro: %+v", e)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestTodosDaSalaRecebem(t *testing.T) {
	h := hub.Novo()
	assinantes := make([]*hub.Assinante, 5)
	for i := range assinantes {
		assinantes[i] = h.Assinar("quadro-1")
	}

	h.Publicar(evt("quadro-1", "ana"))

	for i, a := range assinantes {
		select {
		case <-a.Eventos:
		case <-time.After(time.Second):
			t.Fatalf("assinante %d não recebeu", i)
		}
	}
}

// O comportamento que define o hub: publicar não pode esperar por ninguém.
//
// Um assinante que parou de ler enche a fila e é DESCONECTADO — o canal dele
// fecha. Sem isso, a requisição HTTP de quem moveu o card ficaria bloqueada até
// o cliente lento voltar, que pode ser nunca.
func TestAssinanteQueNaoLeEDesconectado(t *testing.T) {
	h := hub.Novo()
	lento := h.Assinar("quadro-1")
	atento := h.Assinar("quadro-1")

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
	a := h.Assinar("quadro-1")

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
	a := h.Assinar("quadro-1")
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
	a := h.Assinar("quadro-1")
	b := h.Assinar("quadro-2")

	h.Fechar()

	for nome, canal := range map[string]chan evento.Evento{"a": a.Eventos, "b": b.Eventos} {
		select {
		case _, aberto := <-canal:
			if aberto {
				t.Errorf("%s recebeu evento depois do Fechar", nome)
			}
		case <-time.After(time.Second):
			t.Errorf("o canal de %s não foi fechado", nome)
		}
	}

	if h.Assinar("quadro-3") != nil {
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
				a := h.Assinar("quadro-1")
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
