package board

import (
	"context"

	"stacktrack/internal/domain/evento"
)

// Publicador é a porta de saída de eventos. Quem a implementa é o hub, em
// adapter/realtime — e é justamente por ser uma interface declarada AQUI, no
// pacote que a consome, que nenhum usecase precisa saber que WebSocket existe.
//
// Trocar WebSocket por SSE, ou por LISTEN/NOTIFY do Postgres quando houver mais
// de uma instância da API, é escrever outro adaptador. Nada deste pacote muda.
type Publicador interface {
	Publicar(evento.Evento)
}

// RegistroDeEventos é o log do quadro: grava o que aconteceu e devolve a
// posição na história.
//
// O seq é o que permite a um cliente que caiu perguntar "o que houve desde o
// 41?" ao voltar — sem ele, reconectar seria recomeçar do zero sem saber o que
// se perdeu.
type RegistroDeEventos interface {
	Registrar(ctx context.Context, e evento.Evento) (int64, error)
}

// Escrita são os repositórios ligados a uma transação em curso.
//
// Só traz os três que participam de escrita com evento estrutural. Etiqueta,
// checklist e anexo ficam de fora porque os eventos deles são um AVISO de
// "recarregue o quadro", e perder um não deixa buraco perceptível.
type Escrita struct {
	Cards   RepositorioCard
	Colunas RepositorioColuna
	Boards  RepositorioBoard
}

// EscritaAtomica grava a mudança e o evento que a descreve na MESMA transação.
//
// É a porta do outbox transacional. Sem ela, o dado é gravado numa transação e
// o evento noutra, logo depois — e um processo que morra entre as duas deixa a
// mudança sem evento. O cliente que reconecta pede "o que houve desde o 41?",
// recebe do 42 em diante e nunca fica sabendo que houve algo no meio: a tela
// dele passa a discordar do banco em silêncio.
//
// Quem a implementa é adapter/repository.UnidadeDeTrabalho. Nada aqui sabe o
// que é uma transação de Postgres — só que existe um jeito de as duas escritas
// caírem ou valerem juntas.
type EscritaAtomica interface {
	Escrever(ctx context.Context, e evento.Evento, mudanca func(Escrita) error) (int64, error)
}

// eventos é embutido em cada usecase de escrita. Fica separado do usecase para
// que ligar a publicação seja uma linha por usecase, e não um parâmetro novo em
// sete construtores — e em todos os testes que os chamam.
type eventos struct {
	pub     Publicador
	log     RegistroDeEventos
	atomica EscritaAtomica
}

// ComPublicador liga a saída de eventos deste usecase. Sem ela o usecase segue
// funcionando e não publica nada, que é exatamente o que os testes querem: eles
// exercitam regra de negócio, não entrega.
func (e *eventos) ComPublicador(p Publicador) {
	e.pub = p
}

// ComRegistro liga o log de eventos. Sem ele o usecase continua publicando ao
// vivo, só que sem história — que é o que os testes querem.
func (e *eventos) ComRegistro(r RegistroDeEventos) {
	e.log = r
}

// escrita monta os repositórios do caminho NÃO transacional — os mesmos de
// sempre, ligados direto ao pool. É o que escreverEPublicar usa quando não há
// EscritaAtomica ligada.
func (uc *CardUseCase) escrita() Escrita {
	return Escrita{Cards: uc.cards, Colunas: uc.colunas}
}

func (uc *ColunaUseCase) escrita() Escrita {
	return Escrita{Colunas: uc.colunas}
}

func (uc *QuadroUseCase) escrita() Escrita {
	return Escrita{Boards: uc.boards, Colunas: uc.colunas, Cards: uc.cards}
}

// ComEscritaAtomica liga o outbox transacional. Sem ele, o usecase continua
// funcionando com a escrita e o registro em transações separadas — que é o que
// os testes de regra de negócio querem, já que não há banco nenhum ali.
func (e *eventos) ComEscritaAtomica(a EscritaAtomica) {
	e.atomica = a
}

// publicar entrega um evento cujo dado JÁ foi gravado.
//
// ⚠️ Só deve ser chamado DEPOIS da escrita ter dado certo. Publicar antes
// anunciaria uma mudança que pode não acontecer — e o cliente que recebesse o
// evento e recarregasse veria o estado velho, concluindo que o servidor está
// mentindo.
//
// Este caminho NÃO é atômico, e é o certo para os eventos que são apenas um
// aviso de "recarregue o quadro" (etiqueta, checklist, anexo): perder um deles
// não deixa buraco perceptível, porque qualquer evento seguinte — ou a própria
// reconexão — manda a tela buscar tudo de novo. Para as mudanças estruturais,
// onde um buraco é invisível, use escreverEPublicar.
func (e *eventos) publicar(ctx context.Context, tipo evento.Tipo, boardID, autorID string, dados any) {
	ev := evento.Novo(tipo, boardID, autorID, dados)

	// O log vem ANTES da entrega ao vivo, e a ordem importa: é ele que atribui
	// o seq, e um evento entregue sem seq não pode ser retomado por quem
	// reconecta — o cliente não teria como dizer até onde já aplicou.
	//
	// Falhar ao registrar não impede a entrega: o dado já mudou, e deixar de
	// avisar quem está olhando agora seria trocar um problema de histórico por
	// um problema de presente. Quem reconectar depois cai no caminho de
	// recarga completa, que é sempre correto.
	if e.log != nil {
		if seq, err := e.log.Registrar(ctx, ev); err == nil {
			ev.Seq = seq
		}
	}
	if e.pub == nil {
		return
	}
	e.pub.Publicar(ev)
}

// escreverEPublicar grava a mudança e o evento na MESMA transação e, só depois
// do commit, entrega ao vivo.
//
// É o caminho das mudanças estruturais — card e coluna criados, movidos,
// alterados, apagados. A atomicidade importa porque um evento faltando aqui é
// INVISÍVEL para quem reconecta: ele pede o intervalo a partir do último seq
// que aplicou, recebe o que existe, e não tem como saber que houve uma mudança
// que nunca virou evento.
//
// A publicação fica FORA da transação de propósito. Publicar antes do commit
// anunciaria uma mudança que o rollback ainda pode desfazer, e quem recebesse o
// evento recarregaria o quadro para encontrar o estado anterior.
//
// Sem EscritaAtomica ligada (o caso dos testes de regra), cai no caminho não
// transacional: a mudança roda contra os repositórios de sempre e o evento é
// registrado em seguida.
func (e *eventos) escreverEPublicar(
	ctx context.Context,
	tipo evento.Tipo, boardID, autorID string, dados any,
	padrao Escrita,
	mudanca func(Escrita) error,
) error {
	ev := evento.Novo(tipo, boardID, autorID, dados)

	if e.atomica == nil {
		if err := mudanca(padrao); err != nil {
			return err
		}
		if e.log != nil {
			if seq, err := e.log.Registrar(ctx, ev); err == nil {
				ev.Seq = seq
			}
		}
	} else {
		seq, err := e.atomica.Escrever(ctx, ev, mudanca)
		if err != nil {
			return err
		}
		ev.Seq = seq
	}

	if e.pub != nil {
		e.pub.Publicar(ev)
	}
	return nil
}
