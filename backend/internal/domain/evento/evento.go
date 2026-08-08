// Package evento descreve o que aconteceu num quadro, sem saber por onde isso
// será entregue.
//
// É de propósito que nada aqui mencione WebSocket, JSON ou conexão: o domínio
// diz "um card mudou de coluna", e quem transporta isso é o adaptador. Trocar
// WebSocket por SSE amanhã não toca neste arquivo.
package evento

import "time"

// Tipo identifica o que aconteceu. O valor viaja para o cliente e é o que ele
// usa para decidir como reagir, então mudar uma destas strings é mudar
// contrato.
type Tipo string

const (
	// Estrutura do quadro — são estes que a tela aplica direto.
	ColunaCriada   Tipo = "coluna.criada"
	ColunaAlterada Tipo = "coluna.alterada"
	ColunaApagada  Tipo = "coluna.apagada"
	ColunaMovida   Tipo = "coluna.movida"
	CardCriado     Tipo = "card.criado"
	CardAlterado   Tipo = "card.alterado"
	CardApagado    Tipo = "card.apagado"
	CardMovido     Tipo = "card.movido"

	// O que pende de um card ou do quadro. A tela recarrega em vez de aplicar
	// diferença: são mudanças menos frequentes, e o custo de manter trinta
	// caminhos de aplicação não se paga.
	QuadroAlterado   Tipo = "quadro.alterado"
	MembrosAlterados Tipo = "membros.alterados"

	// Presença: quem está com o quadro aberto AGORA. É estado efêmero — não
	// existe no banco, vive só no mapa de conexões do hub e morre com o
	// processo. Justamente por isso não há migration para ele.
	PresencaAlterada Tipo = "presenca.alterada"

	// Controle da reconexão. Não descrevem mudança no quadro: dizem ao cliente
	// em que ponto da história ele está.
	//
	// Sincronizado é a primeira conexão — "você está em dia, a posição é esta".
	// RecarregueTudo é a desistência honesta: o intervalo perdido é grande
	// demais para reproduzir, então busque o quadro inteiro.
	Sincronizado   Tipo = "sincronizado"
	RecarregueTudo Tipo = "recarregue.tudo"
)

// Evento é uma coisa que aconteceu num quadro.
//
// AutorID existe para o cliente poder ignorar o próprio eco: quem arrastou o
// card já moveu a tela na hora, e reaplicar o evento causaria um solavanco.
type Evento struct {
	// Seq é a posição do evento na história do quadro, atribuída pelo banco.
	//
	// É o que torna a reconexão possível: o cliente guarda o último aplicado e
	// pergunta "o que houve desde ele?". E é o que torna a reaplicação
	// inofensiva — evento com seq menor ou igual ao último já visto é
	// descartado, então receber o mesmo duas vezes não duplica nada.
	//
	// Zero significa "não registrado": presença não vai para o log, porque
	// descreve o agora e não faz sentido reproduzir depois.
	Seq        int64
	Tipo       Tipo
	BoardID    string
	AutorID    string
	OcorridoEm time.Time
	// Dados carrega o que a tela precisa para aplicar a mudança sem perguntar
	// de novo — o card movido, a coluna criada. Pode ser nil: nos tipos que
	// pedem recarga, não há o que carregar.
	Dados any
}

// Novo monta um evento carimbado com a hora em que aconteceu.
func Novo(tipo Tipo, boardID, autorID string, dados any) Evento {
	return Evento{
		Tipo:       tipo,
		BoardID:    boardID,
		AutorID:    autorID,
		OcorridoEm: time.Now(),
		Dados:      dados,
	}
}
