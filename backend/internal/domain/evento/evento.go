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
	// Estrutura do quadro. Os payloads já permitem aplicação direta, mas hoje a
	// tela ainda junta os eventos numa rajada e recarrega o quadro; a aplicação
	// incremental ficou adiada para a fase 12.
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
	//
	// ⚠️ São MUITOS tipos, e por um motivo que não é a tela: para ela, todos
	// significam a mesma coisa — "recarregue o quadro". Quem precisa
	// distingui-los é a AUDITORIA.
	//
	// Antes existia um `quadro.alterado` só, usado por doze ações diferentes
	// (etiqueta, anexo, checklist, responsável, renomear, fundo) e sempre com
	// payload vazio. Funcionava perfeitamente como aviso de recarga e era inútil
	// como histórico: o log registrava que ALGO mudou, e nada mais. O tipo do
	// evento é a identidade do que aconteceu, e um tipo genérico apaga essa
	// identidade na hora de gravar — depois não há como recuperá-la.
	QuadroRenomeado Tipo = "quadro.renomeado"
	QuadroFundo     Tipo = "quadro.fundo"

	EtiquetaCriada   Tipo = "etiqueta.criada"
	EtiquetaAlterada Tipo = "etiqueta.alterada"
	EtiquetaApagada  Tipo = "etiqueta.apagada"
	EtiquetaAplicada Tipo = "etiqueta.aplicada"
	EtiquetaRetirada Tipo = "etiqueta.retirada"

	ChecklistCriada   Tipo = "checklist.criada"
	ChecklistAlterada Tipo = "checklist.alterada"
	ChecklistApagada  Tipo = "checklist.apagada"
	ItemCriado        Tipo = "item.criado"
	ItemAlterado      Tipo = "item.alterado"
	ItemApagado       Tipo = "item.apagado"

	AnexoAdicionado Tipo = "anexo.adicionado"
	AnexoRemovido   Tipo = "anexo.removido"

	ResponsavelAtribuido Tipo = "responsavel.atribuido"
	ResponsavelRemovido  Tipo = "responsavel.removido"

	ComentarioCriado  Tipo = "comentario.criado"
	ComentarioEditado Tipo = "comentario.editado"
	ComentarioApagado Tipo = "comentario.apagado"

	// Dois tipos, e não um: quem ADICIONA alguém e quem ACEITA um convite são
	// pessoas diferentes, e o autor do evento é diferente em cada caso. Com um
	// tipo só, o de adicionar sairia sem autor — e a auditoria mostrava
	// "conta removida entrou no quadro", que é uma frase falsa sobre um fato
	// verdadeiro.
	MembroAdicionado    Tipo = "membro.adicionado"
	MembroEntrou        Tipo = "membro.entrou"
	MembroPapelAlterado Tipo = "membro.papel"
	MembroRemovido      Tipo = "membro.removido"

	// QuadroAlterado e os dois abaixo continuam existindo porque o log é
	// append-only: linhas gravadas antes desta separação carregam estes tipos, e
	// a tela precisa saber lê-las. Nenhum caminho novo os produz.
	QuadroAlterado     Tipo = "quadro.alterado"
	ComentarioAlterado Tipo = "comentario.alterado"
	MembrosAlterados   Tipo = "membros.alterados"

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
	Seq     int64
	Tipo    Tipo
	BoardID string
	// CardID diz a que card o evento pertence, quando pertence a algum.
	//
	// Existe para o histórico de um card ser lido por índice, e não varrendo o
	// payload de todos os eventos do quadro. Vazio nos eventos que são do quadro
	// como um todo — coluna criada, membros alterados.
	CardID     string
	AutorID    string
	OcorridoEm time.Time
	// Dados carrega o retrato útil da mudança — o card movido, a coluna criada ou
	// os nomes históricos usados pela auditoria. Pode ser nil nos tipos em que o
	// evento serve apenas como aviso de recarga.
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

// NoCard marca o evento como pertencente a um card, e devolve a cópia marcada.
//
// Separado de Novo de propósito: a maioria dos eventos não é de card nenhum, e
// um parâmetro a mais em todos eles só seria ruído.
func (e Evento) NoCard(cardID string) Evento {
	e.CardID = cardID
	return e
}
