package board

import (
	dcoluna "stacktrack/internal/domain/coluna"
	detiqueta "stacktrack/internal/domain/etiqueta"

	"github.com/google/uuid"
)

// EtiquetaUseCase reúne as etiquetas do quadro e a aplicação delas nos cards.
type EtiquetaUseCase struct {
	membros   repositorioMembro
	colunas   repositorioColuna
	cards     repositorioCard
	etiquetas repositorioEtiqueta
}

// NovoEtiquetaUseCase cria uma instância de EtiquetaUseCase com as dependências injetadas.
func NovoEtiquetaUseCase(
	membros repositorioMembro,
	colunas repositorioColuna,
	cards repositorioCard,
	etiquetas repositorioEtiqueta,
) *EtiquetaUseCase {
	return &EtiquetaUseCase{membros: membros, colunas: colunas, cards: cards, etiquetas: etiquetas}
}

// Listar devolve as etiquetas do quadro. Qualquer membro pode ver — elas
// aparecem nos cards que ele já enxerga.
func (uc *EtiquetaUseCase) Listar(boardID, usuarioID string) ([]detiqueta.Etiqueta, error) {
	if _, err := acesso(uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}
	return uc.etiquetas.ListarDoBoard(boardID)
}

// Criar acrescenta uma etiqueta ao quadro. Exige papel de edição.
func (uc *EtiquetaUseCase) Criar(boardID, usuarioID, nome string, cor detiqueta.Cor) (*detiqueta.Etiqueta, error) {
	if _, err := acessoDeEdicao(uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}

	ultima, err := uc.etiquetas.UltimaPosicao(boardID)
	if err != nil {
		return nil, err
	}

	e, err := detiqueta.Nova(uuid.NewString(), boardID, nome, cor, dcoluna.PosicaoNoFim(ultima))
	if err != nil {
		return nil, err
	}
	if err := uc.etiquetas.Salvar(e); err != nil {
		return nil, err
	}
	return e, nil
}

// Editar troca nome e cor da etiqueta, valendo para todos os cards que a usam.
func (uc *EtiquetaUseCase) Editar(etiquetaID, usuarioID, nome string, cor detiqueta.Cor) (*detiqueta.Etiqueta, error) {
	e, err := uc.carregarComAcessoDeEdicao(etiquetaID, usuarioID)
	if err != nil {
		return nil, err
	}
	if err := e.Editar(nome, cor); err != nil {
		return nil, err
	}
	if err := uc.etiquetas.Atualizar(e); err != nil {
		return nil, err
	}
	return e, nil
}

// Apagar remove a etiqueta do quadro. Ela some de todos os cards junto, pelo
// ON DELETE CASCADE — é o comportamento esperado: a marcação deixou de existir.
func (uc *EtiquetaUseCase) Apagar(etiquetaID, usuarioID string) error {
	if _, err := uc.carregarComAcessoDeEdicao(etiquetaID, usuarioID); err != nil {
		return err
	}
	return uc.etiquetas.Apagar(etiquetaID)
}

// Aplicar pendura a etiqueta no card. Aplicar duas vezes não é erro: a chave
// primária composta já garante uma linha só, e o resultado pretendido — card
// com aquela etiqueta — já vale.
func (uc *EtiquetaUseCase) Aplicar(cardID, etiquetaID, usuarioID string) error {
	if err := uc.conferirMesmoQuadro(cardID, etiquetaID, usuarioID); err != nil {
		return err
	}
	return uc.etiquetas.Aplicar(cardID, etiquetaID)
}

// Remover tira a etiqueta do card, sem apagá-la do quadro.
func (uc *EtiquetaUseCase) Remover(cardID, etiquetaID, usuarioID string) error {
	if err := uc.conferirMesmoQuadro(cardID, etiquetaID, usuarioID); err != nil {
		return err
	}
	return uc.etiquetas.Remover(cardID, etiquetaID)
}

// conferirMesmoQuadro é a checagem que impede pendurar num card a etiqueta de
// OUTRO quadro. Sem ela, quem participa de dois quadros poderia usar o id de
// uma etiqueta do quadro A num card do quadro B — e a etiqueta apareceria com
// nome e cor que ninguém do quadro B consegue editar.
func (uc *EtiquetaUseCase) conferirMesmoQuadro(cardID, etiquetaID, usuarioID string) error {
	boardDoCard, err := uc.boardDoCard(cardID)
	if err != nil {
		return err
	}
	if _, err := acessoDeEdicao(uc.membros, boardDoCard, usuarioID); err != nil {
		return traduzirParaCard(err)
	}

	e, err := uc.etiquetas.BuscarPorID(etiquetaID)
	if err != nil {
		return err
	}
	if e == nil || e.BoardID != boardDoCard {
		return detiqueta.ErrNaoEncontrada
	}
	return nil
}

// boardDoCard percorre card → coluna → quadro.
func (uc *EtiquetaUseCase) boardDoCard(cardID string) (string, error) {
	c, err := uc.cards.BuscarPorID(cardID)
	if err != nil {
		return "", err
	}
	if c == nil {
		return "", dcardNaoEncontrado
	}
	col, err := uc.colunas.BuscarPorID(c.ColunaID)
	if err != nil {
		return "", err
	}
	if col == nil {
		return "", dcardNaoEncontrado
	}
	return col.BoardID, nil
}

func (uc *EtiquetaUseCase) carregarComAcessoDeEdicao(etiquetaID, usuarioID string) (*detiqueta.Etiqueta, error) {
	e, err := uc.etiquetas.BuscarPorID(etiquetaID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, detiqueta.ErrNaoEncontrada
	}
	if _, err := acessoDeEdicao(uc.membros, e.BoardID, usuarioID); err != nil {
		return nil, traduzirParaEtiqueta(err)
	}
	return e, nil
}
