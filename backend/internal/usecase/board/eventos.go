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

// eventos é embutido em cada usecase de escrita. Fica separado do usecase para
// que ligar a publicação seja uma linha por usecase, e não um parâmetro novo em
// sete construtores — e em todos os testes que os chamam.
type eventos struct {
	pub Publicador
	log RegistroDeEventos
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

// publicar entrega o evento, se houver quem receba.
//
// ⚠️ Só deve ser chamado DEPOIS da escrita ter dado certo. Publicar antes
// anunciaria uma mudança que pode não acontecer — e o cliente que recebesse o
// evento e recarregasse veria o estado velho, concluindo que o servidor está
// mentindo.
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
