// O despachante: entrega lendo o LOG, não a chamada em processo.
//
// O que estes testes trancam são os dois furos da publicação direta, que eram
// invisíveis de dentro: a ORDEM não garantida entre goroutines que comitam ao
// mesmo tempo, e a PERDA quando a entrega não acontece — processo que morre
// entre o commit e a chamada, ou entrega que falha com o processo vivo.
package despachante_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"stacktrack/internal/adapter/realtime/despachante"
	"stacktrack/internal/domain/evento"
)

const quadro = "11111111-1111-4111-8111-111111111111"

// logFalso é o board_events em memória, já ordenado por revisão.
type logFalso struct {
	mu       sync.Mutex
	eventos  []evento.Evento
	erro     error
	consulta int
}

func (l *logFalso) gravar(revisoes ...int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range revisoes {
		l.eventos = append(l.eventos, evento.Evento{
			Revisao: r, Indice: 0, Quantidade: 1,
			Tipo: evento.CardMovido, BoardID: quadro, Seq: r,
		})
	}
}

func (l *logFalso) gravarGrupo(revisao int64, quantidade int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := 0; i < quantidade; i++ {
		l.eventos = append(l.eventos, evento.Evento{
			Revisao: revisao, Indice: i, Quantidade: quantidade,
			Tipo: evento.CardMovido, BoardID: quadro,
		})
	}
}

func (l *logFalso) DesdeRevisao(_ context.Context, boardID string, revisao int64, limite int) ([]evento.Evento, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.consulta++
	if l.erro != nil {
		return nil, l.erro
	}
	var fora []evento.Evento
	for _, e := range l.eventos {
		if e.BoardID == boardID && e.Revisao > revisao {
			fora = append(fora, e)
			if len(fora) == limite {
				break
			}
		}
	}
	return fora, nil
}

func (l *logFalso) RevisaoAtual(_ context.Context, boardID string) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.erro != nil {
		return 0, l.erro
	}
	var maior int64
	for _, e := range l.eventos {
		if e.BoardID == boardID && e.Revisao > maior {
			maior = e.Revisao
		}
	}
	return maior, nil
}

// entregaEspia guarda a ordem em que os eventos chegaram ao hub.
type entregaEspia struct {
	mu        sync.Mutex
	entregues []evento.Evento
}

func (e *entregaEspia) Publicar(ev evento.Evento) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.entregues = append(e.entregues, ev)
}

func (e *entregaEspia) revisoes() []int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	fora := make([]int64, 0, len(e.entregues))
	for _, ev := range e.entregues {
		fora = append(fora, ev.Revisao)
	}
	return fora
}

func mudo() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func montar(t *testing.T) (*despachante.Despachante, *logFalso, *entregaEspia) {
	t.Helper()
	registro, espia := &logFalso{}, &entregaEspia{}
	d := despachante.Novo(registro, espia, 20*time.Millisecond, mudo())
	return d, registro, espia
}

// A ENTREGA SEGUE A ORDEM DO LOG, e não a ordem em que os avisos chegaram.
//
// É o primeiro furo da publicação direta: duas mutações que comitam nas
// revisões 5 e 6 podiam chamar o hub na ordem inversa, porque cada uma corre na
// própria goroutine e nada as coordena depois do commit.
func TestEntregaNaOrdemDoLogAindaQueOsAvisosChegemTrocados(t *testing.T) {
	d, registro, espia := montar(t)
	ctx := context.Background()

	registro.gravar(1, 2, 3)
	d.Observar(ctx, quadro) // cursor = 3, nada a entregar

	registro.gravar(4, 5, 6)
	// Os avisos chegam fora de ordem, como aconteceria de verdade.
	d.Publicar(evento.Evento{BoardID: quadro, Revisao: 6})
	d.Publicar(evento.Evento{BoardID: quadro, Revisao: 4})
	d.EntregarAgora(ctx, quadro)

	esperado := []int64{4, 5, 6}
	recebido := espia.revisoes()
	if len(recebido) != len(esperado) {
		t.Fatalf("entregues = %v, esperado %v", recebido, esperado)
	}
	for i := range esperado {
		if recebido[i] != esperado[i] {
			t.Fatalf("entregues = %v, esperado %v", recebido, esperado)
		}
	}
}

// O evento que ninguém avisou É ENTREGUE ASSIM MESMO, pelo polling.
//
// É o segundo furo: um processo que morre entre o commit e a chamada ao hub, ou
// uma fila de avisos cheia, deixava o evento sem entrega. Aqui NENHUM aviso é
// dado — só o laço de polling roda.
func TestEventoSemAvisoEhEntreguePeloPolling(t *testing.T) {
	d, registro, espia := montar(t)
	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()

	registro.gravar(1)
	d.Observar(ctx, quadro)

	go d.Rodar(ctx)

	// Grava SEM avisar: é o commit cuja publicação se perdeu.
	registro.gravar(2, 3)

	prazo := time.After(2 * time.Second)
	for {
		if len(espia.revisoes()) == 2 {
			return
		}
		select {
		case <-prazo:
			t.Fatalf("o polling não entregou o que ninguém avisou: %v", espia.revisoes())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// O cursor NÃO avança quando a leitura falha: a próxima passada tenta o mesmo
// intervalo. Avançar ali transformaria uma falha temporária em buraco
// permanente.
func TestFalhaDeLeituraNaoAvancaOCursor(t *testing.T) {
	d, registro, espia := montar(t)
	ctx := context.Background()

	registro.gravar(1)
	d.Observar(ctx, quadro)
	registro.gravar(2, 3)

	registro.mu.Lock()
	registro.erro = errors.New("banco fora do ar")
	registro.mu.Unlock()
	d.EntregarAgora(ctx, quadro)

	if n := len(espia.revisoes()); n != 0 {
		t.Fatalf("entregou %d eventos com a leitura falhando", n)
	}

	// O banco volta: o intervalo inteiro é entregue.
	registro.mu.Lock()
	registro.erro = nil
	registro.mu.Unlock()
	d.EntregarAgora(ctx, quadro)

	if n := len(espia.revisoes()); n != 2 {
		t.Errorf("entregues = %v, esperado as revisões 2 e 3", espia.revisoes())
	}
}

// Quadro NÃO observado não recebe entrega: sem assinante não há a quem
// entregar, e manter cursor de todo quadro visitado vazaria memória.
func TestQuadroSemAssinanteNaoRecebeEntrega(t *testing.T) {
	d, registro, espia := montar(t)
	ctx := context.Background()

	registro.gravar(1, 2)
	d.EntregarAgora(ctx, quadro)

	if n := len(espia.revisoes()); n != 0 {
		t.Errorf("entregou %d eventos para um quadro que ninguém está olhando", n)
	}
}

// Esquecer libera o cursor e para a entrega.
func TestEsquecerParaAEntregaEliberaOCursor(t *testing.T) {
	d, registro, espia := montar(t)
	ctx := context.Background()

	registro.gravar(1)
	d.Observar(ctx, quadro)
	d.Esquecer(quadro)

	registro.gravar(2)
	d.EntregarAgora(ctx, quadro)

	if n := len(espia.revisoes()); n != 0 {
		t.Errorf("entregou depois de esquecer o quadro")
	}
	if estado := d.Estado(); estado.QuadrosObservados != 0 {
		t.Errorf("quadros observados = %d, esperado 0", estado.QuadrosObservados)
	}
}

// Observar começa da revisão ATUAL: a história anterior é assunto do replay do
// handshake. Republicá-la ao vivo mandaria o quadro inteiro para quem acabou de
// baixá-lo.
func TestObservarComecaDaRevisaoAtual(t *testing.T) {
	d, registro, espia := montar(t)
	ctx := context.Background()

	registro.gravar(1, 2, 3, 4, 5)
	d.Observar(ctx, quadro)
	d.EntregarAgora(ctx, quadro)

	if n := len(espia.revisoes()); n != 0 {
		t.Errorf("republicou %d eventos antigos ao começar a observar", n)
	}
}

// GRUPO INCOMPLETO não é entregue: o cliente confirmaria uma revisão que
// aplicou pela metade, e o que faltou ficaria abaixo do cursor dele para
// sempre.
func TestGrupoIncompletoNaoEhEntregue(t *testing.T) {
	d, registro, espia := montar(t)
	ctx := context.Background()

	registro.gravar(1)
	d.Observar(ctx, quadro)

	// A revisão 2 declara três eventos e só dois estão gravados até agora.
	registro.mu.Lock()
	registro.eventos = append(registro.eventos,
		evento.Evento{Revisao: 2, Indice: 0, Quantidade: 3, BoardID: quadro, Tipo: evento.CardMovido},
		evento.Evento{Revisao: 2, Indice: 1, Quantidade: 3, BoardID: quadro, Tipo: evento.CardMovido},
	)
	registro.mu.Unlock()
	d.EntregarAgora(ctx, quadro)

	if n := len(espia.revisoes()); n != 0 {
		t.Fatalf("entregou %d eventos de um grupo incompleto", n)
	}

	// O grupo se completa: agora vai inteiro.
	registro.mu.Lock()
	registro.eventos = append(registro.eventos,
		evento.Evento{Revisao: 2, Indice: 2, Quantidade: 3, BoardID: quadro, Tipo: evento.CardMovido})
	registro.mu.Unlock()
	d.EntregarAgora(ctx, quadro)

	if n := len(espia.revisoes()); n != 3 {
		t.Errorf("entregues = %d, esperado o grupo completo de 3", n)
	}
}

// Entregar duas vezes não duplica: o cursor avançou.
func TestEntregarDuasVezesNaoDuplica(t *testing.T) {
	d, registro, espia := montar(t)
	ctx := context.Background()

	registro.gravar(1)
	d.Observar(ctx, quadro)
	registro.gravar(2)

	d.EntregarAgora(ctx, quadro)
	d.EntregarAgora(ctx, quadro)

	if n := len(espia.revisoes()); n != 1 {
		t.Errorf("entregues = %v, esperado uma vez só", espia.revisoes())
	}
}

// Acordar NUNCA pode bloquear quem acabou de comitar: a fila tem buffer e o
// envio é não bloqueante. Com a fila cheia, o polling pega o quadro depois.
func TestPublicarNaoBloqueiaComAFilaCheia(t *testing.T) {
	d, _, _ := montar(t)

	pronto := make(chan struct{})
	go func() {
		// Muito mais avisos do que a fila comporta.
		for i := 0; i < 10_000; i++ {
			d.Publicar(evento.Evento{BoardID: quadro, Revisao: int64(i)})
		}
		close(pronto)
	}()

	select {
	case <-pronto:
	case <-time.After(2 * time.Second):
		t.Fatal("Publicar bloqueou: acordar o despachante travou quem comitou")
	}
}

// Evento sem quadro é ignorado — não há sala a acordar.
func TestPublicarSemQuadroEhIgnorado(t *testing.T) {
	d, _, _ := montar(t)
	d.Publicar(evento.Evento{Revisao: 1})
	if estado := d.Estado(); estado.QuadrosObservados != 0 {
		t.Error("um evento sem quadro criou observação")
	}
}
