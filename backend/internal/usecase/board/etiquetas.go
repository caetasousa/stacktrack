package board

import (
	"context"
	detiqueta "stacktrack/internal/domain/etiqueta"
	"stacktrack/internal/domain/evento"
	"stacktrack/internal/domain/ordem"

	"github.com/google/uuid"
)

// EtiquetaUseCase reúne as etiquetas do quadro e a aplicação delas nos cards.
type EtiquetaUseCase struct {
	eventos
	membros   RepositorioMembro
	colunas   RepositorioColuna
	cards     RepositorioCard
	etiquetas repositorioEtiqueta
}

// NovoEtiquetaUseCase cria uma instância de EtiquetaUseCase com as dependências injetadas.
func NovoEtiquetaUseCase(
	membros RepositorioMembro,
	colunas RepositorioColuna,
	cards RepositorioCard,
	etiquetas repositorioEtiqueta,
) *EtiquetaUseCase {
	return &EtiquetaUseCase{membros: membros, colunas: colunas, cards: cards, etiquetas: etiquetas}
}

// Listar devolve as etiquetas do quadro. Qualquer membro pode ver — elas
// aparecem nos cards que ele já enxerga.
func (uc *EtiquetaUseCase) Listar(ctx context.Context, boardID, usuarioID string) ([]detiqueta.Etiqueta, error) {
	if _, err := acesso(ctx, uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}
	return uc.etiquetas.ListarDoBoard(ctx, boardID)
}

// Criar acrescenta uma etiqueta ao quadro. Exige papel de edição.
func (uc *EtiquetaUseCase) Criar(ctx context.Context, boardID, usuarioID, nome string, cor detiqueta.Cor) (*detiqueta.Etiqueta, error) {
	if _, err := acessoDeEdicao(ctx, uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}

	ultima, err := uc.etiquetas.UltimaPosicao(ctx, boardID)
	if err != nil {
		return nil, err
	}

	e, err := detiqueta.Nova(uuid.NewString(), boardID, nome, cor, ordem.NoFim(ultima))
	if err != nil {
		return nil, err
	}
	if err := uc.etiquetas.Salvar(ctx, e); err != nil {
		return nil, err
	}
	uc.publicar(ctx, evento.QuadroAlterado, boardID, usuarioID, nil)
	return e, nil
}

// Editar troca nome e cor da etiqueta, valendo para todos os cards que a usam.
func (uc *EtiquetaUseCase) Editar(ctx context.Context, etiquetaID, usuarioID, nome string, cor detiqueta.Cor) (*detiqueta.Etiqueta, error) {
	e, err := uc.carregarComAcessoDeEdicao(ctx, etiquetaID, usuarioID)
	if err != nil {
		return nil, err
	}
	if err := e.Editar(nome, cor); err != nil {
		return nil, err
	}
	if err := uc.etiquetas.Atualizar(ctx, e); err != nil {
		return nil, err
	}
	uc.publicar(ctx, evento.QuadroAlterado, e.BoardID, usuarioID, nil)
	return e, nil
}

// Apagar remove a etiqueta do quadro. Ela some de todos os cards junto, pelo
// ON DELETE CASCADE — é o comportamento esperado: a marcação deixou de existir.
func (uc *EtiquetaUseCase) Apagar(ctx context.Context, etiquetaID, usuarioID string) error {
	e, err := uc.carregarComAcessoDeEdicao(ctx, etiquetaID, usuarioID)
	if err != nil {
		return err
	}
	if err := uc.etiquetas.Apagar(ctx, etiquetaID); err != nil {
		return err
	}
	uc.publicar(ctx, evento.QuadroAlterado, e.BoardID, usuarioID, nil)
	return nil
}

// Aplicar pendura a etiqueta no card. Aplicar duas vezes não é erro: a chave
// primária composta já garante uma linha só, e o resultado pretendido — card
// com aquela etiqueta — já vale.
func (uc *EtiquetaUseCase) Aplicar(ctx context.Context, cardID, etiquetaID, usuarioID string) error {
	if err := uc.conferirMesmoQuadro(ctx, cardID, etiquetaID, usuarioID); err != nil {
		return err
	}
	if err := uc.etiquetas.Aplicar(ctx, cardID, etiquetaID); err != nil {
		return err
	}
	uc.publicarDoCard(ctx, cardID, usuarioID)
	return nil
}

// Remover tira a etiqueta do card, sem apagá-la do quadro.
func (uc *EtiquetaUseCase) Remover(ctx context.Context, cardID, etiquetaID, usuarioID string) error {
	if err := uc.conferirMesmoQuadro(ctx, cardID, etiquetaID, usuarioID); err != nil {
		return err
	}
	if err := uc.etiquetas.Remover(ctx, cardID, etiquetaID); err != nil {
		return err
	}
	uc.publicarDoCard(ctx, cardID, usuarioID)
	return nil
}

// publicarDoCard resolve o quadro a partir do card e avisa a sala. Falha aqui
// não desfaz a escrita nem vira erro para quem chamou: o dado já mudou, e o
// pior que acontece é a outra aba levar um F5 para ver.
func (uc *EtiquetaUseCase) publicarDoCard(ctx context.Context, cardID, usuarioID string) {
	boardID, err := uc.boardDoCard(ctx, cardID)
	if err != nil {
		return
	}
	uc.publicar(ctx, evento.QuadroAlterado, boardID, usuarioID, nil)
}

// conferirMesmoQuadro é a checagem que impede pendurar num card a etiqueta de
// OUTRO quadro. Sem ela, quem participa de dois quadros poderia usar o id de
// uma etiqueta do quadro A num card do quadro B — e a etiqueta apareceria com
// nome e cor que ninguém do quadro B consegue editar.
func (uc *EtiquetaUseCase) conferirMesmoQuadro(ctx context.Context, cardID, etiquetaID, usuarioID string) error {
	boardDoCard, err := uc.boardDoCard(ctx, cardID)
	if err != nil {
		return err
	}
	if _, err := acessoDeEdicao(ctx, uc.membros, boardDoCard, usuarioID); err != nil {
		return traduzirParaCard(err)
	}

	e, err := uc.etiquetas.BuscarPorID(ctx, etiquetaID)
	if err != nil {
		return err
	}
	if e == nil || e.BoardID != boardDoCard {
		return detiqueta.ErrNaoEncontrada
	}
	return nil
}

// boardDoCard percorre card → coluna → quadro.
func (uc *EtiquetaUseCase) boardDoCard(ctx context.Context, cardID string) (string, error) {
	c, err := uc.cards.BuscarPorID(ctx, cardID)
	if err != nil {
		return "", err
	}
	if c == nil {
		return "", dcardNaoEncontrado
	}
	col, err := uc.colunas.BuscarPorID(ctx, c.ColunaID)
	if err != nil {
		return "", err
	}
	if col == nil {
		return "", dcardNaoEncontrado
	}
	return col.BoardID, nil
}

func (uc *EtiquetaUseCase) carregarComAcessoDeEdicao(ctx context.Context, etiquetaID, usuarioID string) (*detiqueta.Etiqueta, error) {
	e, err := uc.etiquetas.BuscarPorID(ctx, etiquetaID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, detiqueta.ErrNaoEncontrada
	}
	if _, err := acessoDeEdicao(ctx, uc.membros, e.BoardID, usuarioID); err != nil {
		return nil, traduzirParaEtiqueta(err)
	}
	return e, nil
}
