package board

import (
	"context"

	dcard "stacktrack/internal/domain/card"
	"stacktrack/internal/domain/evento"
)

// LimiteDaAtividade é quantas entradas o histórico de um card devolve.
//
// O histórico não pagina: um card com mais de cinquenta acontecimentos é raro,
// e quem precisa auditar o quadro inteiro tem o log completo no banco. Um teto
// simples evita que um card muito antigo devolva uma resposta gigante.
const LimiteDaAtividade = 50

// DadosDoCard é o payload dos eventos de card.
//
// ⚠️ Guarda NOMES, e não só ids, e isso é decisão e não descuido. Um log de
// eventos registra o que era verdade NO MOMENTO em que a coisa aconteceu.
// Resolver o id na hora de ler mostraria o título de hoje numa frase sobre
// ontem — "moveu Migração para Pronto" viraria "moveu Deploy para Arquivo" só
// porque alguém renomeou depois. E não mostraria nada quando a coluna já
// tivesse sido apagada.
//
// É por isso que o histórico é barato de ler: ele não precisa de JOIN nenhum
// além do autor.
type DadosDoCard struct {
	CardID string `json:"cardId,omitempty"`
	// Titulo é o título DEPOIS da mudança.
	Titulo string `json:"titulo,omitempty"`
	// TituloAnterior vem preenchido só quando o título mudou de verdade.
	TituloAnterior string `json:"tituloAnterior,omitempty"`
	// Coluna é onde o card está depois — e, em card.movido, DeColuna é de onde
	// ele saiu. Sem ela o histórico só saberia dizer "moveu este card", que é a
	// metade inútil da informação.
	Coluna   string `json:"coluna,omitempty"`
	DeColuna string `json:"deColuna,omitempty"`
	ColunaID string `json:"colunaId,omitempty"`
	Version  int    `json:"version,omitempty"`
}

// DadosDaColuna é o payload dos eventos de coluna.
type DadosDaColuna struct {
	ColunaID string `json:"colunaId,omitempty"`
	Titulo   string `json:"titulo,omitempty"`
	// Cor é o nome da cor na paleta, vazio quando a coluna não tem uma.
	//
	// O payload existe para o cliente aplicar a mudança SEM perguntar de novo, e
	// uma coluna criada verde que chegasse cinza do outro lado é exatamente o
	// tipo de divergência silenciosa que o evento deveria evitar. Hoje a tela
	// recarrega o quadro a cada evento e o defeito não aparecia — o teste de
	// protocolo cobrava o contrato que o código não cumpria.
	Cor            string `json:"cor,omitempty"`
	TituloAnterior string `json:"tituloAnterior,omitempty"`
}

// Atividade é uma linha do histórico, já com o nome de quem agiu.
type Atividade struct {
	Seq       int64
	Tipo      evento.Tipo
	AutorID   string
	AutorNome string
	// AutorEmail existe porque NOME NÃO IDENTIFICA NINGUÉM. Dois "Ana Silva" no
	// mesmo quadro tornam a auditoria inútil justamente quando ela é necessária
	// — e é impossível saber, olhando, se são duas pessoas ou a mesma.
	//
	// Não é exposição nova: qualquer membro já lê o email de todos na tela de
	// membros (MembroUseCase.Listar exige `acesso`, não administração). A
	// auditoria mostra a mesma informação para a mesma plateia.
	//
	// Vazio quando a conta foi removida — o LEFT JOIN preserva a linha do
	// histórico, que é o ponto dele.
	AutorEmail string
	Dados      any
	OcorridoEm string
}

// Movimentacao é a última vez que um card mudou de lugar, e por quem.
//
// Guarda o NOME de quem moveu e os nomes das colunas, e não ids: é a mesma
// razão de DadosDoCard guardá-los. O selo do card responde "quem mexeu nisto
// por último", e a resposta precisa continuar verdadeira depois de a coluna ser
// renomeada ou apagada.
type Movimentacao struct {
	AutorID   string
	AutorNome string
	// DeColuna e ParaColuna são iguais quando o card só foi reordenado dentro da
	// própria coluna — é o que deixa a tela dizer "reordenou" em vez de inventar
	// um movimento que não houve.
	DeColuna   string
	ParaColuna string
	OcorridoEm string
}

// FiltroDeAtividade escolhe o recorte do histórico do quadro.
type FiltroDeAtividade struct {
	// SoMovimentacoes restringe a card.movido. É o padrão da tela de auditoria:
	// a pergunta que ela responde é "quem mexeu na ordem do quadro", e misturar
	// comentário e renomeação afogaria a resposta.
	SoMovimentacoes bool
	// AutorID vazio significa "de todo mundo".
	AutorID string
	// AntesDe é o cursor de paginação: devolve o que vier ANTES deste seq.
	// Zero começa do mais recente.
	//
	// Cursor por seq, e não OFFSET: o log recebe escrita o tempo todo, e um
	// OFFSET faria a segunda página pular linhas que entraram entre um pedido e
	// o outro. O seq é ordem total, então o cursor não escorrega.
	AntesDe int64
	Limite  int
}

// RepositorioAtividade lê o histórico do log de eventos.
//
// É uma porta de LEITURA sobre a mesma tabela que RegistroDeEventos escreve —
// o histórico é um read model, e não uma segunda fonte da verdade. Foi essa
// separação que permitiu a fase 11 entregar o histórico sem tabela nova, e é a
// mesma que agora entrega a auditoria do quadro.
type RepositorioAtividade interface {
	// DoCard devolve o que aconteceu com o card, do mais recente para o mais
	// antigo — é a ordem em que se lê histórico.
	DoCard(ctx context.Context, cardID string, limite int) ([]Atividade, error)
	// DoBoard devolve o que aconteceu no quadro inteiro, do mais recente para o
	// mais antigo, aplicando o filtro.
	DoBoard(ctx context.Context, boardID string, filtro FiltroDeAtividade) ([]Atividade, error)
	// UltimaMovimentacaoPorCard devolve, para cada card do quadro que já foi
	// movido, a última movimentação dele — numa consulta só.
	//
	// É a mesma razão de EtiquetasDoBoardPorCard e ProgressoDoBoard existirem: a
	// tela do quadro mostra isto em TODO card, e uma consulta por card seria um
	// N+1 que piora justamente nos quadros grandes, que são os que precisam de
	// auditoria.
	UltimaMovimentacaoPorCard(ctx context.Context, boardID string) (map[string]Movimentacao, error)
}

// AtividadeUseCase responde "o que aconteceu com este card".
type AtividadeUseCase struct {
	membros    RepositorioMembro
	colunas    RepositorioColuna
	cards      RepositorioCard
	atividades RepositorioAtividade
}

// NovoAtividadeUseCase cria uma instância de AtividadeUseCase com as dependências injetadas.
func NovoAtividadeUseCase(
	membros RepositorioMembro,
	colunas RepositorioColuna,
	cards RepositorioCard,
	atividades RepositorioAtividade,
) *AtividadeUseCase {
	return &AtividadeUseCase{membros: membros, colunas: colunas, cards: cards, atividades: atividades}
}

// DoCard devolve o histórico do card. Qualquer membro pode ler: acompanhar o
// que aconteceu é ver, não mexer.
func (uc *AtividadeUseCase) DoCard(ctx context.Context, cardID, usuarioID string) ([]Atividade, error) {
	c, err := uc.cards.BuscarPorID(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, dcard.ErrNaoEncontrado
	}
	col, err := uc.colunas.BuscarPorID(ctx, c.ColunaID)
	if err != nil {
		return nil, err
	}
	if col == nil {
		return nil, dcard.ErrNaoEncontrado
	}
	if _, err := acesso(ctx, uc.membros, col.BoardID, usuarioID); err != nil {
		return nil, traduzirParaCard(err)
	}
	return uc.atividades.DoCard(ctx, cardID, LimiteDaAtividade)
}

// LimiteDaAuditoria é quantas linhas uma página da auditoria do quadro devolve.
//
// Menor que um quadro inteiro de propósito: a tela pagina por cursor, e uma
// primeira resposta curta chega antes. Quem precisa de mais pede a próxima.
const LimiteDaAuditoria = 60

// DoBoard devolve o histórico do quadro — quem mexeu no quê, do mais recente
// para o mais antigo.
//
// Qualquer membro pode ler, pela mesma razão do histórico de um card: ver o que
// aconteceu é ver, não mexer. E a informação já era acessível card a card — o
// que muda aqui é só não precisar abrir cinquenta cards para juntá-la.
//
// Retorna dboard.ErrNaoEncontrado para quadro inexistente e para quem não
// participa, os dois iguais.
func (uc *AtividadeUseCase) DoBoard(ctx context.Context, boardID, usuarioID string, filtro FiltroDeAtividade) (PaginaDeAtividade, error) {
	if _, err := acesso(ctx, uc.membros, boardID, usuarioID); err != nil {
		return PaginaDeAtividade{}, err
	}
	if filtro.Limite <= 0 || filtro.Limite > LimiteDaAuditoria {
		// Teto aplicado aqui, e não confiado a quem chama: sem ele, um `limite`
		// vindo da URL montaria a história inteira do quadro em memória.
		filtro.Limite = LimiteDaAuditoria
	}

	// Pede UMA linha a mais do que vai devolver. É o que permite responder "há
	// mais?" sem uma segunda consulta e sem contar a tabela inteira — e é o que
	// evita a tela oferecer um "carregar mais" que não carrega nada, que é um
	// botão mentindo.
	pedido := filtro
	pedido.Limite = filtro.Limite + 1
	lista, err := uc.atividades.DoBoard(ctx, boardID, pedido)
	if err != nil {
		return PaginaDeAtividade{}, err
	}

	if len(lista) > filtro.Limite {
		return PaginaDeAtividade{Linhas: lista[:filtro.Limite], TemMais: true}, nil
	}
	return PaginaDeAtividade{Linhas: lista}, nil
}

// PaginaDeAtividade é um lote do histórico e a informação de se existe o
// próximo. Quem chama não tem como deduzir isso sozinho: o teto é do servidor,
// e uma página cheia não significa que acabou.
type PaginaDeAtividade struct {
	Linhas []Atividade
	// TemMais é falso na última página. A tela usa isso para esconder o botão
	// em vez de oferecer um clique que devolve lista vazia.
	TemMais bool
}
