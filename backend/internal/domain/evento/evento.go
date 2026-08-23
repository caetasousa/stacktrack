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
	QuadroCriado    Tipo = "quadro.criado"
	QuadroRenomeado Tipo = "quadro.renomeado"
	QuadroFundo     Tipo = "quadro.fundo"
	// QuadroApagado é o único evento terminal e efêmero: o DELETE leva o log
	// persistido por cascata, então ele só pode ser emitido depois do commit.
	QuadroApagado Tipo = "quadro.apagado"
	// Publicação é estado do quadro e participa da mesma revisão/outbox. O
	// payload destes eventos é deliberadamente vazio: o token é uma credencial
	// e nunca pode parar no log, no replay ou no WebSocket.
	QuadroPublicado          Tipo = "quadro.publicado"
	QuadroPublicacaoRevogada Tipo = "quadro.publicacao_revogada"

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

	// MembroAdicionado descrevia o caminho em que o dono punha no quadro, na
	// hora, alguém que já tinha conta. Esse caminho FOI REMOVIDO: conhecer um
	// email não concede participação, e todo acesso passa por um convite que a
	// pessoa aceita. Nenhum caminho novo produz este tipo — ele fica porque o
	// log é append-only e as linhas antigas precisam continuar legíveis.
	MembroAdicionado Tipo = "membro.adicionado"
	MembroEntrou     Tipo = "membro.entrou"
	// ConviteCriado e ConviteRevogado descrevem o CONVITE, não a participação:
	// nesses dois instantes ninguém entrou nem saiu do quadro.
	//
	// Existem porque convidar e revogar viraram mutações com evento no mesmo
	// commit (A2), e um evento precisa dizer o que aconteceu. Reaproveitar
	// membro.entrou para "foi convidado" faria a auditoria afirmar que alguém
	// entrou num quadro em que ela ainda não pôs os pés.
	//
	// O payload leva o email MASCARADO: o evento é lido por todo mundo que
	// participa do quadro, e o endereço completo de um convidado que talvez
	// nunca aceite não é informação que a auditoria precise carregar.
	ConviteCriado   Tipo = "convite.criado"
	ConviteRevogado Tipo = "convite.revogado"

	// OrdenacaoReparada é emitido pelo comando de manutenção que redistribui
	// chaves de ordenação duplicadas (ver usecase/board/reparo.go).
	//
	// É evento de mutação como qualquer outro, e não um aviso interno: o reparo
	// muda a ordem do quadro, então quem está com ele aberto precisa
	// reconciliar em vez de continuar vendo a ordem antiga. Fica no log com
	// autor, que é o que permite explicar depois por que a ordem de um quadro
	// mudou sozinha numa madrugada de terça-feira.
	OrdenacaoReparada   Tipo = "ordenacao.reparada"
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
// AutorID identifica quem causou a mudança. Ele alimenta a auditoria e permite
// ao cliente coalescer a confirmação da própria mutação sem ocultar o evento
// das outras abas e dispositivos da mesma conta.
type Evento struct {
	// Seq é a posição do evento na história do quadro, atribuída pelo banco.
	//
	// Continua no protocolo para identificar eventos e manter compatibilidade
	// com clientes antigos. O cliente novo retoma e deduplica por Revisao.
	//
	// Zero significa "não registrado": presença não vai para o log, porque
	// descreve o agora e não faz sentido reproduzir depois.
	//
	// ⚠️ Seq NÃO é cursor de reconexão, embora tenha sido usado como um. Ele é
	// BIGSERIAL, então registra a ordem de ALOCAÇÃO do número e não a de
	// COMMIT: duas transações concorrentes pegam 42 e 43 nessa ordem e podem
	// comitar na inversa. Quem usa o seq como cursor recebe o 43, avança para
	// 43, e nunca mais vê o 42 — buraco silencioso e permanente. O cursor é
	// Revisao. Seq permanece como identidade imutável e compatibilidade.
	Seq int64
	// Revisao é a posição do evento na história DAQUELE QUADRO.
	//
	// Diferente do seq, ela é atribuída sob o lock do quadro: só uma transação
	// por vez a incrementa, então a ordem de numeração é a ordem de commit, e a
	// sequência é contígua dentro do quadro. É por isso que ela pode ser cursor
	// e o seq não.
	//
	// Zero significa "sem revisão" — evento efêmero (presença) ou linha legada,
	// gravada antes de a revisão existir.
	Revisao int64
	// Indice é a posição do evento dentro da revisão, começando em zero.
	// Quantidade é quantos eventos formam aquela revisão.
	//
	// Hoje toda mutação produz exatamente um evento, então o par é sempre
	// (0, 1). Eles existem assim mesmo porque é o que permite ao cliente saber
	// QUANDO o grupo está completo: confirmar uma revisão pela metade deixaria
	// o cursor à frente do que foi realmente aplicado, e o que faltou nunca
	// mais seria entregue.
	Indice     int
	Quantidade int
	Tipo       Tipo
	BoardID    string
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
		Tipo:    tipo,
		BoardID: boardID,
		AutorID: autorID,
		// Um evento por mutação é o desenho vigente, então o grupo nasce
		// completo com um item só. Quando alguma mutação passar a produzir
		// vários, quem os monta ajusta índice e quantidade — e o cliente já
		// sabe esperar o grupo inteiro antes de confirmar a revisão.
		Indice:     0,
		Quantidade: 1,
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
