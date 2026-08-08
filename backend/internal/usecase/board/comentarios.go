package board

import (
	"context"

	dcard "stacktrack/internal/domain/card"
	dcomentario "stacktrack/internal/domain/comentario"
	"stacktrack/internal/domain/evento"

	"github.com/google/uuid"
)

// ComentarioUseCase reúne a conversa dos cards.
type ComentarioUseCase struct {
	eventos
	membros     RepositorioMembro
	colunas     RepositorioColuna
	cards       RepositorioCard
	comentarios RepositorioComentario
}

// NovoComentarioUseCase cria uma instância de ComentarioUseCase com as dependências injetadas.
func NovoComentarioUseCase(
	membros RepositorioMembro,
	colunas RepositorioColuna,
	cards RepositorioCard,
	comentarios RepositorioComentario,
) *ComentarioUseCase {
	return &ComentarioUseCase{membros: membros, colunas: colunas, cards: cards, comentarios: comentarios}
}

// Listar devolve a conversa do card. Qualquer membro pode ler — inclusive o
// leitor: acompanhar a discussão é ver, não editar.
func (uc *ComentarioUseCase) Listar(ctx context.Context, cardID, usuarioID string) ([]ComentarioComAutor, error) {
	if _, err := uc.boardComAcesso(ctx, cardID, usuarioID); err != nil {
		return nil, err
	}
	return uc.comentarios.ListarDoCard(ctx, cardID)
}

// Criar acrescenta um comentário ao card.
//
// Exige participação, e não papel de edição: comentar é conversar sobre o
// trabalho, não mexer nele. Um leitor que enxerga o quadro precisa poder
// responder — senão a revisão de quem só acompanha não tem para onde ir.
func (uc *ComentarioUseCase) Criar(ctx context.Context, cardID, usuarioID, texto string) (*dcomentario.Comentario, error) {
	boardID, err := uc.boardComAcesso(ctx, cardID, usuarioID)
	if err != nil {
		return nil, err
	}

	c, err := dcomentario.Novo(uuid.NewString(), cardID, usuarioID, texto)
	if err != nil {
		return nil, err
	}
	if err := uc.comentarios.Salvar(ctx, c); err != nil {
		return nil, err
	}
	uc.publicar(ctx, evento.ComentarioAlterado, boardID, usuarioID, map[string]string{"cardId": cardID})
	return c, nil
}

// Editar troca o texto de um comentário. SÓ o autor.
//
// Nem o dono do quadro pode: apagar o que não serve é responsabilidade de quem
// administra, mas reescrever é pôr palavras na boca de outra pessoa.
func (uc *ComentarioUseCase) Editar(ctx context.Context, comentarioID, usuarioID, texto string) (*dcomentario.Comentario, error) {
	c, boardID, err := uc.carregar(ctx, comentarioID, usuarioID)
	if err != nil {
		return nil, err
	}
	if !c.EhAutor(usuarioID) {
		return nil, dcomentario.ErrNaoEhAutor
	}
	if err := c.Editar(texto); err != nil {
		return nil, err
	}
	if err := uc.comentarios.Atualizar(ctx, c); err != nil {
		return nil, err
	}
	uc.publicar(ctx, evento.ComentarioAlterado, boardID, usuarioID, map[string]string{"cardId": c.CardID})
	return c, nil
}

// Apagar remove o comentário. O autor apaga o próprio; quem administra o quadro
// apaga o de qualquer um, porque é dele a responsabilidade pelo que fica ali.
func (uc *ComentarioUseCase) Apagar(ctx context.Context, comentarioID, usuarioID string) error {
	c, boardID, err := uc.carregar(ctx, comentarioID, usuarioID)
	if err != nil {
		return err
	}

	if !c.EhAutor(usuarioID) {
		if _, err := acessoDeAdministracao(ctx, uc.membros, boardID, usuarioID); err != nil {
			return err
		}
	}
	if err := uc.comentarios.Apagar(ctx, comentarioID); err != nil {
		return err
	}
	uc.publicar(ctx, evento.ComentarioAlterado, boardID, usuarioID, map[string]string{"cardId": c.CardID})
	return nil
}

// carregar busca o comentário e confere que quem pede participa do quadro dele.
func (uc *ComentarioUseCase) carregar(ctx context.Context, comentarioID, usuarioID string) (*dcomentario.Comentario, string, error) {
	c, err := uc.comentarios.BuscarPorID(ctx, comentarioID)
	if err != nil {
		return nil, "", err
	}
	if c == nil {
		return nil, "", dcomentario.ErrNaoEncontrado
	}
	boardID, err := uc.boardComAcesso(ctx, c.CardID, usuarioID)
	if err != nil {
		// Quem não participa recebe "comentário não encontrado", e não "card não
		// encontrado": a resposta não deve revelar em que ponto da cadeia parou.
		return nil, "", dcomentario.ErrNaoEncontrado
	}
	return c, boardID, nil
}

// boardComAcesso percorre card → coluna → quadro e confere a participação.
func (uc *ComentarioUseCase) boardComAcesso(ctx context.Context, cardID, usuarioID string) (string, error) {
	c, err := uc.cards.BuscarPorID(ctx, cardID)
	if err != nil {
		return "", err
	}
	if c == nil {
		return "", dcard.ErrNaoEncontrado
	}
	col, err := uc.colunas.BuscarPorID(ctx, c.ColunaID)
	if err != nil {
		return "", err
	}
	if col == nil {
		return "", dcard.ErrNaoEncontrado
	}
	if _, err := acesso(ctx, uc.membros, col.BoardID, usuarioID); err != nil {
		return "", traduzirParaCard(err)
	}
	return col.BoardID, nil
}
