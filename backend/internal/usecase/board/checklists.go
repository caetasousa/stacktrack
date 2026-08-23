package board

import (
	"context"

	dcard "stacktrack/internal/domain/card"
	dchecklist "stacktrack/internal/domain/checklist"
	"stacktrack/internal/domain/evento"

	"github.com/google/uuid"
)

// ChecklistUseCase reúne as listas de verificação dos cards e os itens delas.
type ChecklistUseCase struct {
	eventos
	membros    RepositorioMembro
	colunas    RepositorioColuna
	cards      RepositorioCard
	checklists RepositorioChecklist
}

// NovoChecklistUseCase cria uma instância de ChecklistUseCase com as dependências injetadas.
func NovoChecklistUseCase(
	membros RepositorioMembro,
	colunas RepositorioColuna,
	cards RepositorioCard,
	checklists RepositorioChecklist,
) *ChecklistUseCase {
	return &ChecklistUseCase{membros: membros, colunas: colunas, cards: cards, checklists: checklists}
}

// Criar acrescenta uma checklist no fim do card. Exige papel de edição.
func (uc *ChecklistUseCase) Criar(ctx context.Context, cardID, usuarioID, titulo string) (*dchecklist.Checklist, error) {
	if err := uc.exigirEdicaoNoCard(ctx, cardID, usuarioID); err != nil {
		return nil, err
	}

	ultima, err := uc.checklists.UltimaPosicao(ctx, cardID)
	if err != nil {
		return nil, err
	}

	c, err := dchecklist.Nova(uuid.NewString(), cardID, titulo, dchecklist.PosicaoNoFim(ultima))
	if err != nil {
		return nil, err
	}
	if err := uc.escreverDoCard(ctx, evento.ChecklistCriada, cardID, usuarioID, c.Titulo,
		func(e Escrita) error { return e.Checklists.Salvar(ctx, c) }); err != nil {
		return nil, err
	}
	return c, nil
}

// Renomear troca o título da checklist.
func (uc *ChecklistUseCase) Renomear(ctx context.Context, checklistID, usuarioID, titulo string) (*dchecklist.Checklist, error) {
	c, err := uc.carregarComAcessoDeEdicao(ctx, checklistID, usuarioID)
	if err != nil {
		return nil, err
	}
	if err := c.Renomear(titulo); err != nil {
		return nil, err
	}
	if err := uc.escreverDoCard(ctx, evento.ChecklistAlterada, c.CardID, usuarioID, c.Titulo,
		func(e Escrita) error { return e.Checklists.Atualizar(ctx, c) }); err != nil {
		return nil, err
	}
	return c, nil
}

// Apagar remove a checklist e, por cascata, os itens dela.
func (uc *ChecklistUseCase) Apagar(ctx context.Context, checklistID, usuarioID string) error {
	c, err := uc.carregarComAcessoDeEdicao(ctx, checklistID, usuarioID)
	if err != nil {
		return err
	}
	return uc.escreverDoCard(ctx, evento.ChecklistApagada, c.CardID, usuarioID, c.Titulo,
		func(e Escrita) error { return e.Checklists.Apagar(ctx, checklistID) })
}

// CriarItem acrescenta uma linha no fim da checklist.
func (uc *ChecklistUseCase) CriarItem(ctx context.Context, checklistID, usuarioID, texto string) (*dchecklist.Item, error) {
	if _, err := uc.carregarComAcessoDeEdicao(ctx, checklistID, usuarioID); err != nil {
		return nil, err
	}

	ultima, err := uc.checklists.UltimaPosicaoItem(ctx, checklistID)
	if err != nil {
		return nil, err
	}

	item, err := dchecklist.NovoItem(uuid.NewString(), checklistID, texto, dchecklist.PosicaoNoFim(ultima))
	if err != nil {
		return nil, err
	}
	if err := uc.escreverDoChecklist(ctx, evento.ItemCriado, checklistID, usuarioID, item.Texto,
		func(e Escrita) error { return e.Checklists.SalvarItem(ctx, item) }); err != nil {
		return nil, err
	}
	return item, nil
}

// EditarItem troca o texto e/ou o estado de conclusão da linha.
//
// texto nil significa "não mexer no texto", e concluido nil, "não mexer na
// marcação": marcar uma caixa não pode exigir reenviar o texto, e renomear não
// pode desmarcar sem querer.
func (uc *ChecklistUseCase) EditarItem(ctx context.Context, itemID, usuarioID string, texto *string, concluido *bool) (*dchecklist.Item, error) {
	item, err := uc.checklists.BuscarItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, dchecklist.ErrItemNaoEncontrado
	}
	if _, err := uc.carregarComAcessoDeEdicao(ctx, item.ChecklistID, usuarioID); err != nil {
		return nil, dchecklist.ErrItemNaoEncontrado
	}

	if texto != nil {
		if err := item.Editar(*texto); err != nil {
			return nil, err
		}
	}
	if concluido != nil {
		item.Marcar(*concluido)
	}
	// Só os campos que a chamada pediu para mudar chegam ao banco. `texto nil`
	// e `concluido nil` já significavam "não mexer" na entrada; agora eles
	// significam a mesma coisa no UPDATE, e é isso que impede uma renomeação de
	// desmarcar a caixa que outra pessoa acabou de marcar.
	if err := uc.escreverDoChecklist(ctx, evento.ItemAlterado, item.ChecklistID, usuarioID, item.Texto,
		func(e Escrita) error {
			if texto != nil {
				if err := e.Checklists.EditarItem(ctx, item.ID, item.Texto, item.AtualizadoEm); err != nil {
					return err
				}
			}
			if concluido != nil {
				return e.Checklists.MarcarItem(ctx, item.ID, item.Concluido, item.AtualizadoEm)
			}
			return nil
		}); err != nil {
		return nil, err
	}
	return item, nil
}

// ApagarItem remove a linha da checklist.
func (uc *ChecklistUseCase) ApagarItem(ctx context.Context, itemID, usuarioID string) error {
	item, err := uc.checklists.BuscarItem(ctx, itemID)
	if err != nil {
		return err
	}
	if item == nil {
		return dchecklist.ErrItemNaoEncontrado
	}
	if _, err := uc.carregarComAcessoDeEdicao(ctx, item.ChecklistID, usuarioID); err != nil {
		return dchecklist.ErrItemNaoEncontrado
	}
	return uc.escreverDoChecklist(ctx, evento.ItemApagado, item.ChecklistID, usuarioID, item.Texto,
		func(e Escrita) error { return e.Checklists.ApagarItem(ctx, itemID) })
}

// escreverDoChecklist e escreverDoCard resolvem o quadro — para achar a sala e
// para o lock da unidade de trabalho — e gravam mudança e evento no mesmo
// commit.
//
// A resolução agora PROPAGA erro, e acontece ANTES da escrita. Antes ela era um
// `return` mudo depois de o dado já ter sido gravado: o item mudava, nada ia
// para o log, ninguém era avisado, e não havia como distinguir isso de um item
// que nunca foi criado.
func (uc *ChecklistUseCase) escreverDoChecklist(
	ctx context.Context, tipo evento.Tipo, checklistID, usuarioID, alvo string,
	mudanca func(Escrita) error,
) error {
	c, err := uc.checklists.BuscarPorID(ctx, checklistID)
	if err != nil {
		return err
	}
	if c == nil {
		return dchecklist.ErrNaoEncontrada
	}
	return uc.escreverDoCard(ctx, tipo, c.CardID, usuarioID, alvo, mudanca)
}

func (uc *ChecklistUseCase) escreverDoCard(
	ctx context.Context, tipo evento.Tipo, cardID, usuarioID, alvo string,
	mudanca func(Escrita) error,
) error {
	card, err := uc.cards.BuscarPorID(ctx, cardID)
	if err != nil {
		return err
	}
	if card == nil {
		return dcard.ErrNaoEncontrado
	}
	col, err := uc.colunas.BuscarPorID(ctx, card.ColunaID)
	if err != nil {
		return err
	}
	if col == nil {
		return dcard.ErrNaoEncontrado
	}
	return uc.escreverEPublicarNoCard(ctx, tipo, col.BoardID, cardID, usuarioID,
		DadosDoCard{CardID: cardID, Titulo: card.Titulo, Alvo: alvo},
		uc.escrita(), func(e Escrita) error {
			if err := revalidarEdicao(ctx, e, col.BoardID, usuarioID); err != nil {
				return err
			}
			return mudanca(e)
		})
}

// carregarComAcessoDeEdicao percorre checklist → card → coluna → quadro. É o
// caminho mais longo da aplicação, e existe porque nem checklist nem card
// guardam o quadro a que pertencem.
func (uc *ChecklistUseCase) carregarComAcessoDeEdicao(ctx context.Context, checklistID, usuarioID string) (*dchecklist.Checklist, error) {
	c, err := uc.checklists.BuscarPorID(ctx, checklistID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, dchecklist.ErrNaoEncontrada
	}
	if err := uc.exigirEdicaoNoCard(ctx, c.CardID, usuarioID); err != nil {
		return nil, traduzirChecklist(err)
	}
	return c, nil
}

// exigirEdicaoNoCard resolve o quadro do card e confere o papel.
func (uc *ChecklistUseCase) exigirEdicaoNoCard(ctx context.Context, cardID, usuarioID string) error {
	c, err := uc.cards.BuscarPorID(ctx, cardID)
	if err != nil {
		return err
	}
	if c == nil {
		return dcard.ErrNaoEncontrado
	}
	col, err := uc.colunas.BuscarPorID(ctx, c.ColunaID)
	if err != nil {
		return err
	}
	if col == nil {
		return dcard.ErrNaoEncontrado
	}
	if _, err := acessoDeEdicao(ctx, uc.membros, col.BoardID, usuarioID); err != nil {
		return traduzirParaCard(err)
	}
	return nil
}

// traduzirChecklist converte "card não encontrado" em "checklist não
// encontrada": quem pediu uma checklist não deve descobrir por qual etapa da
// cadeia a busca parou.
func traduzirChecklist(err error) error {
	if err == dcard.ErrNaoEncontrado {
		return dchecklist.ErrNaoEncontrada
	}
	return traduzirParaChecklist(err)
}
