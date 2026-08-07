package board

import (
	"errors"
	"time"

	danexo "kanbango/internal/domain/anexo"
	dboard "kanbango/internal/domain/board"
	dcard "kanbango/internal/domain/card"
	dchecklist "kanbango/internal/domain/checklist"
	dcoluna "kanbango/internal/domain/coluna"
	detiqueta "kanbango/internal/domain/etiqueta"

	"github.com/google/uuid"
)

// CardUseCase reúne as operações sobre cards.
type CardUseCase struct {
	membros    repositorioMembro
	colunas    repositorioColuna
	cards      repositorioCard
	etiquetas  repositorioEtiqueta
	checklists repositorioChecklist
	anexos     repositorioAnexo
}

// NovoCardUseCase cria uma instância de CardUseCase com as dependências injetadas.
func NovoCardUseCase(
	membros repositorioMembro,
	colunas repositorioColuna,
	cards repositorioCard,
	etiquetas repositorioEtiqueta,
	checklists repositorioChecklist,
	anexos repositorioAnexo,
) *CardUseCase {
	return &CardUseCase{
		membros: membros, colunas: colunas, cards: cards,
		etiquetas: etiquetas, checklists: checklists, anexos: anexos,
	}
}

// Detalhar devolve o card com tudo que pende dele — é o que o modal mostra.
// Qualquer membro pode ver; editar é que exige papel.
func (uc *CardUseCase) Detalhar(cardID, usuarioID string) (*CardDetalhado, error) {
	c, err := uc.cards.BuscarPorID(cardID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, dcard.ErrNaoEncontrado
	}
	col, err := uc.colunas.BuscarPorID(c.ColunaID)
	if err != nil {
		return nil, err
	}
	if col == nil {
		return nil, dcard.ErrNaoEncontrado
	}
	if _, err := acesso(uc.membros, col.BoardID, usuarioID); err != nil {
		return nil, traduzirParaCard(err)
	}

	etiquetas, err := uc.etiquetas.EtiquetasDoCard(cardID)
	if err != nil {
		return nil, err
	}
	listas, err := uc.checklists.ListarDoCard(cardID)
	if err != nil {
		return nil, err
	}
	anexos, err := uc.anexos.ListarDoCard(cardID)
	if err != nil {
		return nil, err
	}

	comItens := make([]ChecklistComItens, 0, len(listas))
	for _, lista := range listas {
		itens, err := uc.checklists.ListarItens(lista.ID)
		if err != nil {
			return nil, err
		}
		comItens = append(comItens, ChecklistComItens{Checklist: lista, Itens: itens})
	}

	return &CardDetalhado{
		Card: *c, BoardID: col.BoardID,
		Etiquetas: etiquetas, Checklists: comItens, Anexos: anexos,
	}, nil
}

// DefinirPrazo marca ou limpa a data de entrega do card. Exige papel de edição.
func (uc *CardUseCase) DefinirPrazo(cardID, usuarioID string, prazo *time.Time) (*dcard.Card, error) {
	c, err := uc.carregarComAcessoDeEdicao(cardID, usuarioID)
	if err != nil {
		return nil, err
	}
	c.DefinirPrazo(prazo)
	if err := uc.cards.Atualizar(c); err != nil {
		return nil, err
	}
	return c, nil
}

// Criar acrescenta um card no fim da coluna. Exige papel de edição no quadro
// da coluna.
func (uc *CardUseCase) Criar(colunaID, usuarioID, titulo, descricao string) (*dcard.Card, error) {
	col, err := uc.colunas.BuscarPorID(colunaID)
	if err != nil {
		return nil, err
	}
	if col == nil {
		return nil, dcoluna.ErrNaoEncontrada
	}
	if _, err := acessoDeEdicao(uc.membros, col.BoardID, usuarioID); err != nil {
		return nil, traduzirParaColuna(err)
	}

	ultima, err := uc.cards.UltimaPosicao(colunaID)
	if err != nil {
		return nil, err
	}

	c, err := dcard.Novo(uuid.NewString(), colunaID, titulo, descricao, dcoluna.PosicaoNoFim(ultima))
	if err != nil {
		return nil, err
	}
	if err := uc.cards.Salvar(c); err != nil {
		return nil, err
	}
	return c, nil
}

// Editar troca título e descrição do card, incrementando a versão. Exige papel
// de edição.
func (uc *CardUseCase) Editar(cardID, usuarioID, titulo, descricao string) (*dcard.Card, error) {
	c, err := uc.carregarComAcessoDeEdicao(cardID, usuarioID)
	if err != nil {
		return nil, err
	}
	if err := c.Editar(titulo, descricao); err != nil {
		return nil, err
	}
	if err := uc.cards.Atualizar(c); err != nil {
		return nil, err
	}
	return c, nil
}

// Apagar remove o card. Exige papel de edição.
func (uc *CardUseCase) Apagar(cardID, usuarioID string) error {
	if _, err := uc.carregarComAcessoDeEdicao(cardID, usuarioID); err != nil {
		return err
	}
	return uc.cards.Apagar(cardID)
}

// carregarComAcessoDeEdicao percorre card → coluna → quadro para descobrir a
// quem pedir permissão. É o caminho que faz a autorização valer também para o
// card, que não guarda o quadro a que pertence.
func (uc *CardUseCase) carregarComAcessoDeEdicao(cardID, usuarioID string) (*dcard.Card, error) {
	c, err := uc.cards.BuscarPorID(cardID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, dcard.ErrNaoEncontrado
	}
	col, err := uc.colunas.BuscarPorID(c.ColunaID)
	if err != nil {
		return nil, err
	}
	if col == nil {
		// Card sem coluna é inconsistência de dados, não "não encontrado":
		// o ON DELETE CASCADE deveria ter levado o card junto.
		return nil, dcard.ErrNaoEncontrado
	}
	if _, err := acessoDeEdicao(uc.membros, col.BoardID, usuarioID); err != nil {
		return nil, traduzirParaCard(err)
	}
	return c, nil
}

// dcardNaoEncontrado é um atalho para os usecases que percorrem
// card → coluna → quadro e precisam parar no meio do caminho.
var dcardNaoEncontrado = dcard.ErrNaoEncontrado

// traduzirParaColuna converte "quadro não encontrado" em "coluna não
// encontrada": quem chamou pediu uma coluna, e a resposta não deve revelar que
// existe um quadro por trás dela.
func traduzirParaColuna(err error) error {
	if errors.Is(err, dboard.ErrNaoEncontrado) {
		return dcoluna.ErrNaoEncontrada
	}
	return err
}

// traduzirParaCard faz o mesmo para as rotas de card.
func traduzirParaCard(err error) error {
	if errors.Is(err, dboard.ErrNaoEncontrado) {
		return dcard.ErrNaoEncontrado
	}
	return err
}

// traduzirParaEtiqueta faz o mesmo para as rotas de etiqueta.
func traduzirParaEtiqueta(err error) error {
	if errors.Is(err, dboard.ErrNaoEncontrado) {
		return detiqueta.ErrNaoEncontrada
	}
	return err
}

// traduzirParaChecklist faz o mesmo para as rotas de checklist.
func traduzirParaChecklist(err error) error {
	if errors.Is(err, dboard.ErrNaoEncontrado) {
		return dchecklist.ErrNaoEncontrada
	}
	return err
}

// traduzirParaAnexo faz o mesmo para as rotas de anexo.
//
// Cobre também "card não encontrado" porque o caminho até o anexo passa pelo
// card: sem isso, quem pede um anexo de quadro alheio recebe de volta em que
// etapa da cadeia a busca parou, o que é informação sobre dado que ele não
// pode ver.
func traduzirParaAnexo(err error) error {
	if errors.Is(err, dboard.ErrNaoEncontrado) || errors.Is(err, dcard.ErrNaoEncontrado) {
		return danexo.ErrNaoEncontrado
	}
	return err
}
