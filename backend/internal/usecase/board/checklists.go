package board

import (
	dcard "stacktrack/internal/domain/card"
	dchecklist "stacktrack/internal/domain/checklist"
	"stacktrack/internal/domain/evento"

	"github.com/google/uuid"
)

// ChecklistUseCase reúne as listas de verificação dos cards e os itens delas.
type ChecklistUseCase struct {
	eventos
	membros    repositorioMembro
	colunas    repositorioColuna
	cards      repositorioCard
	checklists repositorioChecklist
}

// NovoChecklistUseCase cria uma instância de ChecklistUseCase com as dependências injetadas.
func NovoChecklistUseCase(
	membros repositorioMembro,
	colunas repositorioColuna,
	cards repositorioCard,
	checklists repositorioChecklist,
) *ChecklistUseCase {
	return &ChecklistUseCase{membros: membros, colunas: colunas, cards: cards, checklists: checklists}
}

// Criar acrescenta uma checklist no fim do card. Exige papel de edição.
func (uc *ChecklistUseCase) Criar(cardID, usuarioID, titulo string) (*dchecklist.Checklist, error) {
	if err := uc.exigirEdicaoNoCard(cardID, usuarioID); err != nil {
		return nil, err
	}

	ultima, err := uc.checklists.UltimaPosicao(cardID)
	if err != nil {
		return nil, err
	}

	c, err := dchecklist.Nova(uuid.NewString(), cardID, titulo, dchecklist.PosicaoNoFim(ultima))
	if err != nil {
		return nil, err
	}
	if err := uc.checklists.Salvar(c); err != nil {
		return nil, err
	}
	uc.publicarDoCard(cardID, usuarioID)
	return c, nil
}

// Renomear troca o título da checklist.
func (uc *ChecklistUseCase) Renomear(checklistID, usuarioID, titulo string) (*dchecklist.Checklist, error) {
	c, err := uc.carregarComAcessoDeEdicao(checklistID, usuarioID)
	if err != nil {
		return nil, err
	}
	if err := c.Renomear(titulo); err != nil {
		return nil, err
	}
	if err := uc.checklists.Atualizar(c); err != nil {
		return nil, err
	}
	uc.publicarDoCard(c.CardID, usuarioID)
	return c, nil
}

// Apagar remove a checklist e, por cascata, os itens dela.
func (uc *ChecklistUseCase) Apagar(checklistID, usuarioID string) error {
	c, err := uc.carregarComAcessoDeEdicao(checklistID, usuarioID)
	if err != nil {
		return err
	}
	if err := uc.checklists.Apagar(checklistID); err != nil {
		return err
	}
	uc.publicarDoCard(c.CardID, usuarioID)
	return nil
}

// CriarItem acrescenta uma linha no fim da checklist.
func (uc *ChecklistUseCase) CriarItem(checklistID, usuarioID, texto string) (*dchecklist.Item, error) {
	if _, err := uc.carregarComAcessoDeEdicao(checklistID, usuarioID); err != nil {
		return nil, err
	}

	ultima, err := uc.checklists.UltimaPosicaoItem(checklistID)
	if err != nil {
		return nil, err
	}

	item, err := dchecklist.NovoItem(uuid.NewString(), checklistID, texto, dchecklist.PosicaoNoFim(ultima))
	if err != nil {
		return nil, err
	}
	if err := uc.checklists.SalvarItem(item); err != nil {
		return nil, err
	}
	uc.publicarDoChecklist(checklistID, usuarioID)
	return item, nil
}

// EditarItem troca o texto e/ou o estado de conclusão da linha.
//
// texto nil significa "não mexer no texto", e concluido nil, "não mexer na
// marcação": marcar uma caixa não pode exigir reenviar o texto, e renomear não
// pode desmarcar sem querer.
func (uc *ChecklistUseCase) EditarItem(itemID, usuarioID string, texto *string, concluido *bool) (*dchecklist.Item, error) {
	item, err := uc.checklists.BuscarItem(itemID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, dchecklist.ErrItemNaoEncontrado
	}
	if _, err := uc.carregarComAcessoDeEdicao(item.ChecklistID, usuarioID); err != nil {
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
	if err := uc.checklists.AtualizarItem(item); err != nil {
		return nil, err
	}
	uc.publicarDoChecklist(item.ChecklistID, usuarioID)
	return item, nil
}

// ApagarItem remove a linha da checklist.
func (uc *ChecklistUseCase) ApagarItem(itemID, usuarioID string) error {
	item, err := uc.checklists.BuscarItem(itemID)
	if err != nil {
		return err
	}
	if item == nil {
		return dchecklist.ErrItemNaoEncontrado
	}
	if _, err := uc.carregarComAcessoDeEdicao(item.ChecklistID, usuarioID); err != nil {
		return dchecklist.ErrItemNaoEncontrado
	}
	if err := uc.checklists.ApagarItem(itemID); err != nil {
		return err
	}
	uc.publicarDoChecklist(item.ChecklistID, usuarioID)
	return nil
}

// publicarDoChecklist e publicarDoCard resolvem o quadro para achar a sala.
// Falha na resolução não desfaz a escrita nem vira erro: o dado já mudou, e o
// pior que acontece é a outra aba precisar de um F5.
func (uc *ChecklistUseCase) publicarDoChecklist(checklistID, usuarioID string) {
	c, err := uc.checklists.BuscarPorID(checklistID)
	if err != nil || c == nil {
		return
	}
	uc.publicarDoCard(c.CardID, usuarioID)
}

func (uc *ChecklistUseCase) publicarDoCard(cardID, usuarioID string) {
	card, err := uc.cards.BuscarPorID(cardID)
	if err != nil || card == nil {
		return
	}
	col, err := uc.colunas.BuscarPorID(card.ColunaID)
	if err != nil || col == nil {
		return
	}
	uc.publicar(evento.QuadroAlterado, col.BoardID, usuarioID, nil)
}

// carregarComAcessoDeEdicao percorre checklist → card → coluna → quadro. É o
// caminho mais longo da aplicação, e existe porque nem checklist nem card
// guardam o quadro a que pertencem.
func (uc *ChecklistUseCase) carregarComAcessoDeEdicao(checklistID, usuarioID string) (*dchecklist.Checklist, error) {
	c, err := uc.checklists.BuscarPorID(checklistID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, dchecklist.ErrNaoEncontrada
	}
	if err := uc.exigirEdicaoNoCard(c.CardID, usuarioID); err != nil {
		return nil, traduzirChecklist(err)
	}
	return c, nil
}

// exigirEdicaoNoCard resolve o quadro do card e confere o papel.
func (uc *ChecklistUseCase) exigirEdicaoNoCard(cardID, usuarioID string) error {
	c, err := uc.cards.BuscarPorID(cardID)
	if err != nil {
		return err
	}
	if c == nil {
		return dcard.ErrNaoEncontrado
	}
	col, err := uc.colunas.BuscarPorID(c.ColunaID)
	if err != nil {
		return err
	}
	if col == nil {
		return dcard.ErrNaoEncontrado
	}
	if _, err := acessoDeEdicao(uc.membros, col.BoardID, usuarioID); err != nil {
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
