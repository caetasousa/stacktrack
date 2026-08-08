package board

import "stacktrack/internal/domain/evento"

// Publicador é a porta de saída de eventos. Quem a implementa é o hub, em
// adapter/realtime — e é justamente por ser uma interface declarada AQUI, no
// pacote que a consome, que nenhum usecase precisa saber que WebSocket existe.
//
// Trocar WebSocket por SSE, ou por LISTEN/NOTIFY do Postgres quando houver mais
// de uma instância da API, é escrever outro adaptador. Nada deste pacote muda.
type Publicador interface {
	Publicar(evento.Evento)
}

// eventos é embutido em cada usecase de escrita. Fica separado do usecase para
// que ligar a publicação seja uma linha por usecase, e não um parâmetro novo em
// sete construtores — e em todos os testes que os chamam.
type eventos struct {
	pub Publicador
}

// ComPublicador liga a saída de eventos deste usecase. Sem ela o usecase segue
// funcionando e não publica nada, que é exatamente o que os testes querem: eles
// exercitam regra de negócio, não entrega.
func (e *eventos) ComPublicador(p Publicador) {
	e.pub = p
}

// publicar entrega o evento, se houver quem receba.
//
// ⚠️ Só deve ser chamado DEPOIS da escrita ter dado certo. Publicar antes
// anunciaria uma mudança que pode não acontecer — e o cliente que recebesse o
// evento e recarregasse veria o estado velho, concluindo que o servidor está
// mentindo. Enquanto não há transação explícita, "depois do commit" é depois do
// repositório retornar sem erro.
func (e *eventos) publicar(tipo evento.Tipo, boardID, autorID string, dados any) {
	if e.pub == nil {
		return
	}
	e.pub.Publicar(evento.Novo(tipo, boardID, autorID, dados))
}
