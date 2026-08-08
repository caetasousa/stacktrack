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
	Coluna   string  `json:"coluna,omitempty"`
	DeColuna string  `json:"deColuna,omitempty"`
	ColunaID string  `json:"colunaId,omitempty"`
	Posicao  float64 `json:"posicao,omitempty"`
	Version  int     `json:"version,omitempty"`
}

// DadosDaColuna é o payload dos eventos de coluna.
type DadosDaColuna struct {
	ColunaID       string `json:"colunaId,omitempty"`
	Titulo         string `json:"titulo,omitempty"`
	TituloAnterior string `json:"tituloAnterior,omitempty"`
}

// Atividade é uma linha do histórico, já com o nome de quem agiu.
type Atividade struct {
	Seq        int64
	Tipo       evento.Tipo
	AutorID    string
	AutorNome  string
	Dados      any
	OcorridoEm string
}

// RepositorioAtividade lê o histórico do log de eventos.
//
// É uma porta de LEITURA sobre a mesma tabela que RegistroDeEventos escreve —
// o histórico é um read model, e não uma segunda fonte da verdade. Foi essa
// separação que permitiu a fase 11 entregar o histórico sem tabela nova.
type RepositorioAtividade interface {
	// DoCard devolve o que aconteceu com o card, do mais recente para o mais
	// antigo — é a ordem em que se lê histórico.
	DoCard(ctx context.Context, cardID string, limite int) ([]Atividade, error)
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
