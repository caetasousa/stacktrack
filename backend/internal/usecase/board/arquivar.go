// Arquivar e desarquivar cards e colunas — a exclusão reversível da fase 13.
//
// Apagar continua existindo, e é o que NÃO tem volta: o DELETE de um card leva
// por cascata comentários, checklists, anexos, responsáveis e etiquetas
// aplicadas. Arquivar tira do quadro sem tirar do banco.
package board

import (
	"context"

	dcard "stacktrack/internal/domain/card"
	dcoluna "stacktrack/internal/domain/coluna"
	"stacktrack/internal/domain/evento"
)

// Arquivados é o conteúdo da tela de arquivados de um quadro.
type Arquivados struct {
	Cards   []dcard.Card
	Colunas []dcoluna.Coluna
	// ColunaDe diz de que coluna cada card arquivado veio, pelo id do card.
	//
	// Vem resolvido aqui porque a tela precisa do NOME: "Migração, de A fazer"
	// responde onde o card vai cair ao voltar, e o cliente não tem como saber o
	// título de uma coluna que pode, ela própria, estar arquivada.
	ColunaDe map[string]string
}

// ArquivarCard tira o card do quadro sem apagá-lo. Exige papel de edição.
//
// Erra com dcard.ErrJaArquivado quando ele já está fora — o que acontece quando
// duas pessoas arquivam o mesmo card, e dizer isso é melhor que fingir que a
// segunda também arquivou.
func (uc *CardUseCase) ArquivarCard(ctx context.Context, cardID, usuarioID string) (*dcard.Card, error) {
	return uc.mudarArquivamentoDoCard(ctx, cardID, usuarioID, evento.CardArquivado,
		func(c *dcard.Card) error { return c.Arquivar() })
}

// DesarquivarCard devolve o card ao quadro, na coluna e na posição em que
// estava. Exige papel de edição.
func (uc *CardUseCase) DesarquivarCard(ctx context.Context, cardID, usuarioID string) (*dcard.Card, error) {
	return uc.mudarArquivamentoDoCard(ctx, cardID, usuarioID, evento.CardDesarquivado,
		func(c *dcard.Card) error { return c.Desarquivar() })
}

// mudarArquivamentoDoCard é o caminho comum das duas operações: elas diferem
// só no método do domínio e no tipo do evento.
func (uc *CardUseCase) mudarArquivamentoDoCard(
	ctx context.Context,
	cardID, usuarioID string,
	tipo evento.Tipo,
	mudar func(*dcard.Card) error,
) (*dcard.Card, error) {
	c, boardID, err := uc.carregarComAcessoDeEdicao(ctx, cardID, usuarioID)
	if err != nil {
		return nil, err
	}
	if err := mudar(c); err != nil {
		return nil, err
	}
	// O título vai no payload como em card.apagado: o histórico precisa dizer
	// "arquivou Migração", e quem lê o log meses depois não tem o card na tela.
	dados := DadosDoCard{CardID: c.ID, Titulo: c.Titulo, ColunaID: c.ColunaID, Version: c.Version}
	if err := uc.escreverEPublicarNoCard(ctx, tipo, boardID, c.ID, usuarioID, dados,
		uc.escrita(), func(e Escrita) error { return e.Cards.Atualizar(ctx, c) }); err != nil {
		return nil, err
	}
	return c, nil
}

// ArquivarColuna tira a coluna do quadro. Os cards dela saem junto da tela, mas
// NÃO são arquivados: eles voltam com ela. Exige papel de edição.
func (uc *ColunaUseCase) ArquivarColuna(ctx context.Context, colunaID, usuarioID string) (*dcoluna.Coluna, error) {
	return uc.mudarArquivamentoDaColuna(ctx, colunaID, usuarioID, evento.ColunaArquivada,
		func(c *dcoluna.Coluna) error { return c.Arquivar() })
}

// DesarquivarColuna devolve a coluna ao quadro, na posição em que estava, com
// os cards que ela tinha. Exige papel de edição.
func (uc *ColunaUseCase) DesarquivarColuna(ctx context.Context, colunaID, usuarioID string) (*dcoluna.Coluna, error) {
	return uc.mudarArquivamentoDaColuna(ctx, colunaID, usuarioID, evento.ColunaDesarquivada,
		func(c *dcoluna.Coluna) error { return c.Desarquivar() })
}

func (uc *ColunaUseCase) mudarArquivamentoDaColuna(
	ctx context.Context,
	colunaID, usuarioID string,
	tipo evento.Tipo,
	mudar func(*dcoluna.Coluna) error,
) (*dcoluna.Coluna, error) {
	c, err := uc.carregarComAcessoDeEdicao(ctx, colunaID, usuarioID)
	if err != nil {
		return nil, err
	}
	if err := mudar(c); err != nil {
		return nil, err
	}
	if err := uc.escreverEPublicar(ctx, tipo, c.BoardID, usuarioID,
		DadosDaColuna{ColunaID: c.ID, Titulo: c.Titulo},
		uc.escrita(), func(e Escrita) error { return e.Colunas.Atualizar(ctx, c) }); err != nil {
		return nil, err
	}
	return c, nil
}

// ListarArquivados devolve o que saiu do quadro. Basta PARTICIPAR: ver o que
// foi arquivado é leitura, e quem lê o quadro pode ler o arquivo dele.
func (uc *QuadroUseCase) ListarArquivados(ctx context.Context, boardID, usuarioID string) (*Arquivados, error) {
	if _, err := acesso(ctx, uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}

	cards, err := uc.cards.ListarArquivadosDoBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	colunasArquivadas, err := uc.colunas.ListarArquivadasDoBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}

	// O nome da coluna de origem sai de TODAS as colunas do quadro, ativas e
	// arquivadas: um card arquivado pode ter vindo de uma coluna que depois foi
	// arquivada também, e resolver só pelas ativas deixaria a origem em branco
	// justamente no caso mais confuso.
	ativas, err := uc.colunas.ListarDoBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	nomeDaColuna := make(map[string]string, len(ativas)+len(colunasArquivadas))
	for _, col := range ativas {
		nomeDaColuna[col.ID] = col.Titulo
	}
	for _, col := range colunasArquivadas {
		nomeDaColuna[col.ID] = col.Titulo
	}

	origem := make(map[string]string, len(cards))
	for _, c := range cards {
		origem[c.ID] = nomeDaColuna[c.ColunaID]
	}

	return &Arquivados{Cards: cards, Colunas: colunasArquivadas, ColunaDe: origem}, nil
}
