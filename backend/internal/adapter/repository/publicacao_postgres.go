package repository

import (
	"context"
	"errors"

	"stacktrack/internal/domain/publicacao"

	"github.com/jackc/pgx/v5"
)

// PublicacaoPostgres persiste os links públicos de quadro no PostgreSQL.
type PublicacaoPostgres struct {
	db consultante
}

// NovoPublicacaoPostgres cria o repositório de publicações sobre o pool informado.
func NovoPublicacaoPostgres(pool Fonte) *PublicacaoPostgres {
	return &PublicacaoPostgres{db: pool}
}

// Salvar persiste a publicação de um quadro.
func (r *PublicacaoPostgres) Salvar(ctx context.Context, p *publicacao.Publicacao) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO board_publicacoes (board_id, token, criado_por, criado_em)
		 VALUES ($1, $2, $3, $4)`,
		p.BoardID, p.Token, p.CriadoPor, p.CriadoEm,
	)
	return err
}

// BuscarPorBoard retorna (publicação, nil) quando o quadro está publicado e
// (nil, nil) quando não está.
func (r *PublicacaoPostgres) BuscarPorBoard(ctx context.Context, boardID string) (*publicacao.Publicacao, error) {
	return r.buscar(ctx,
		`SELECT board_id, token, criado_por, criado_em
		 FROM board_publicacoes WHERE board_id = $1`, boardID)
}

// BuscarPorToken retorna (publicação, nil) quando o token corresponde a um link
// vivo e (nil, nil) quando não corresponde a nenhum.
//
// A busca é por igualdade num índice único: revogar apaga a linha, então não há
// filtro de "ativo" a esquecer aqui — link revogado simplesmente não é achado.
func (r *PublicacaoPostgres) BuscarPorToken(ctx context.Context, token string) (*publicacao.Publicacao, error) {
	return r.buscar(ctx,
		`SELECT board_id, token, criado_por, criado_em
		 FROM board_publicacoes WHERE token = $1`, token)
}

// Remover revoga a publicação do quadro. Remover o que não existe não é erro.
func (r *PublicacaoPostgres) Remover(ctx context.Context, boardID string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM board_publicacoes WHERE board_id = $1`, boardID)
	return err
}

func (r *PublicacaoPostgres) buscar(ctx context.Context, sql string, arg any) (*publicacao.Publicacao, error) {
	var p publicacao.Publicacao
	err := r.db.QueryRow(ctx, sql, arg).Scan(&p.BoardID, &p.Token, &p.CriadoPor, &p.CriadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}
