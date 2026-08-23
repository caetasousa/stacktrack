package repository

import (
	"context"
	"errors"

	"stacktrack/internal/domain/comentario"
	ucboard "stacktrack/internal/usecase/board"

	"github.com/jackc/pgx/v5"
)

// ComentarioPostgres persiste a conversa dos cards.
type ComentarioPostgres struct {
	db consultante
}

// NovoComentarioPostgres cria o repositório de comentários sobre o pool informado.
func NovoComentarioPostgres(pool Fonte) *ComentarioPostgres {
	return &ComentarioPostgres{db: pool}
}

// Salvar persiste um comentário novo.
func (r *ComentarioPostgres) Salvar(ctx context.Context, c *comentario.Comentario) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO comentarios (id, card_id, autor_id, texto, criado_em, editado_em)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		c.ID, c.CardID, c.AutorID, c.Texto, c.CriadoEm, c.EditadoEm,
	)
	return err
}

// Atualizar grava o texto editado.
//
// O autor NÃO entra no UPDATE: quem pode editar é decidido no domínio, e
// repetir a regra aqui criaria uma segunda fonte da verdade que se descobre
// desatualizada em produção.
func (r *ComentarioPostgres) Atualizar(ctx context.Context, c *comentario.Comentario) error {
	_, err := r.db.Exec(ctx,
		`UPDATE comentarios SET texto = $2, editado_em = $3 WHERE id = $1`,
		c.ID, c.Texto, c.EditadoEm,
	)
	return err
}

// BuscarPorID retorna (comentário, nil) quando encontra e (nil, nil) quando não existe.
func (r *ComentarioPostgres) BuscarPorID(ctx context.Context, id string) (*comentario.Comentario, error) {
	var c comentario.Comentario
	err := r.db.QueryRow(ctx,
		`SELECT id, card_id, autor_id, texto, criado_em, editado_em FROM comentarios WHERE id = $1`, id,
	).Scan(&c.ID, &c.CardID, &c.AutorID, &c.Texto, &c.CriadoEm, &c.EditadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Apagar remove o comentário.
func (r *ComentarioPostgres) Apagar(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM comentarios WHERE id = $1`, id)
	return err
}

// ListarDoCard devolve a conversa do card em ordem de tempo, já com o nome de
// quem escreveu.
func (r *ComentarioPostgres) ListarDoCard(ctx context.Context, cardID string) ([]ucboard.ComentarioComAutor, error) {
	linhas, err := r.db.Query(ctx,
		`SELECT c.id, c.card_id, c.autor_id, c.texto, c.criado_em, c.editado_em, u.nome
		   FROM comentarios c
		   JOIN usuarios u ON u.id = c.autor_id
		  WHERE c.card_id = $1
		  ORDER BY c.criado_em, c.id`, cardID,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	lista := make([]ucboard.ComentarioComAutor, 0)
	for linhas.Next() {
		var item ucboard.ComentarioComAutor
		c := &item.Comentario
		if err := linhas.Scan(&c.ID, &c.CardID, &c.AutorID, &c.Texto, &c.CriadoEm, &c.EditadoEm, &item.AutorNome); err != nil {
			return nil, err
		}
		lista = append(lista, item)
	}
	return lista, linhas.Err()
}

// ContarPorCardDoBoard devolve quantos comentários cada card do quadro tem —
// numa consulta só, como os outros selos do card.
func (r *ComentarioPostgres) ContarPorCardDoBoard(ctx context.Context, boardID string) (map[string]int, error) {
	linhas, err := r.db.Query(ctx,
		`SELECT cm.card_id, count(*)
		   FROM comentarios cm
		   JOIN cards c   ON c.id = cm.card_id
		   JOIN colunas l ON l.id = c.coluna_id
		  WHERE l.board_id = $1
		  GROUP BY cm.card_id`, boardID,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	porCard := make(map[string]int)
	for linhas.Next() {
		var cardID string
		var quantos int
		if err := linhas.Scan(&cardID, &quantos); err != nil {
			return nil, err
		}
		porCard[cardID] = quantos
	}
	return porCard, linhas.Err()
}
