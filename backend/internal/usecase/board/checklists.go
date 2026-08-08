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
	membros    repositorioMembro
	colunas    RepositorioColuna
	cards      RepositorioCard
	checklists repositorioChecklist
}

// NovoChecklistUseCase cria uma instância de ChecklistUseCase com as dependências injetadas.
func NovoChecklistUseCase(
	membros repositorioMembro,
	colunas RepositorioColuna,
	cards RepositorioCard,
	checklists repositorioChecklist,
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
	if err := uc.checklists.Salvar(ctx, c); err != nil {
		return nil, err
	}
	uc.publicarDoCard(ctx, cardID, usuarioID)
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
	if err := uc.checklists.Atualizar(ctx, c); err != nil {
		return nil, err
	}
	uc.publicarDoCard(ctx, c.CardID, usuarioID)
	return c, nil
}

// Apagar remove a checklist e, por cascata, os itens dela.
func (uc *ChecklistUseCase) Apagar(ctx context.Context, checklistID, usuarioID string) error {
	c, err := uc.carregarComAcessoDeEdicao(ctx, checklistID, usuarioID)
	if err != nil {
		return err
	}
	if err := uc.checklists.Apagar(ctx, checklistID); err != nil {
		return err
	}
	uc.publicarDoCard(ctx, c.CardID, usuarioID)
	return nil
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
	if err := uc.checklists.SalvarItem(ctx, item); err != nil {
		return nil, err
	}
	uc.publicarDoChecklist(ctx, checklistID, usuarioID)
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
	if err := uc.checklists.AtualizarItem(ctx, item); err != nil {
		return nil, err
	}
	uc.publicarDoChecklist(ctx, item.ChecklistID, usuarioID)
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
	if err := uc.checklists.ApagarItem(ctx, itemID); err != nil {
		return err
	}
	uc.publicarDoChecklist(ctx, item.ChecklistID, usuarioID)
	return nil
}

// publicarDoChecklist e publicarDoCard resolvem o quadro para achar a sala.
// Falha na resolução não desfaz a escrita nem vira erro: o dado já mudou, e o
// pior que acontece é a outra aba precisar de um F5.
func (uc *ChecklistUseCase) publicarDoChecklist(ctx context.Context, checklistID, usuarioID string) {
	c, err := uc.checklists.BuscarPorID(ctx, checklistID)
	if err != nil || c == nil {
		return
	}
	uc.publicarDoCard(ctx, c.CardID, usuarioID)
}

func (uc *ChecklistUseCase) publicarDoCard(ctx context.Context, cardID, usuarioID string) {
	card, err := uc.cards.BuscarPorID(ctx, cardID)
	if err != nil || card == nil {
		return
	}
	col, err := uc.colunas.BuscarPorID(ctx, card.ColunaID)
	if err != nil || col == nil {
		return
	}
	uc.publicar(ctx, evento.QuadroAlterado, col.BoardID, usuarioID, nil)
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
