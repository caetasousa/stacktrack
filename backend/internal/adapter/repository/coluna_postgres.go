package repository

import (
	"context"
	"errors"

	"stacktrack/internal/domain/coluna"
	"stacktrack/internal/domain/cor"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ColunaPostgres persiste colunas no PostgreSQL.
type ColunaPostgres struct {
	db consultante
}

// NovoColunaPostgres cria o repositório de colunas sobre o pool informado.
func NovoColunaPostgres(pool *pgxpool.Pool) *ColunaPostgres {
	return &ColunaPostgres{db: pool}
}

// Salvar persiste uma coluna nova.
func (r *ColunaPostgres) Salvar(ctx context.Context, c *coluna.Coluna) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO colunas (id, board_id, titulo, cor, chave, arquivado_em, criado_em, atualizado_em)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		c.ID, c.BoardID, c.Titulo, vazioParaNulo(string(c.Cor)), c.Chave,
		c.ArquivadoEm, c.CriadoEm, c.AtualizadoEm,
	)
	return err
}

// Atualizar grava as alterações de uma coluna existente.
func (r *ColunaPostgres) Atualizar(ctx context.Context, c *coluna.Coluna) error {
	_, err := r.db.Exec(ctx,
		`UPDATE colunas SET titulo = $2, cor = $3, chave = $4, arquivado_em = $5, atualizado_em = $6 WHERE id = $1`,
		c.ID, c.Titulo, vazioParaNulo(string(c.Cor)), c.Chave, c.ArquivadoEm, c.AtualizadoEm,
	)
	return err
}

// BuscarPorID retorna (coluna, nil) quando encontra e (nil, nil) quando não existe.
func (r *ColunaPostgres) BuscarPorID(ctx context.Context, id string) (*coluna.Coluna, error) {
	var c coluna.Coluna
	var corLida *string
	err := r.db.QueryRow(ctx,
		`SELECT id, board_id, titulo, cor, chave, arquivado_em, criado_em, atualizado_em FROM colunas WHERE id = $1`, id,
	).Scan(&c.ID, &c.BoardID, &c.Titulo, &corLida, &c.Chave, &c.ArquivadoEm, &c.CriadoEm, &c.AtualizadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Cor = cor.Cor(valorOuVazio(corLida))
	return &c, nil
}

// ListarDoBoard devolve as colunas ATIVAS do quadro em ordem de chave.
//
// O desempate por id não é decoração: com duas chaves iguais — que duas
// inserções simultâneas no mesmo ponto podem produzir — a ordem sem desempate
// varia entre consultas, e a tela reordenaria sozinha a cada F5.
func (r *ColunaPostgres) ListarDoBoard(ctx context.Context, boardID string) ([]coluna.Coluna, error) {
	linhas, err := r.db.Query(ctx,
		`SELECT id, board_id, titulo, cor, chave, arquivado_em, criado_em, atualizado_em
		 FROM colunas WHERE board_id = $1 AND arquivado_em IS NULL
		 ORDER BY chave COLLATE "C", id`, boardID,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	colunas := make([]coluna.Coluna, 0)
	for linhas.Next() {
		var c coluna.Coluna
		var corLida *string
		if err := linhas.Scan(&c.ID, &c.BoardID, &c.Titulo, &corLida, &c.Chave, &c.ArquivadoEm, &c.CriadoEm, &c.AtualizadoEm); err != nil {
			return nil, err
		}
		c.Cor = cor.Cor(valorOuVazio(corLida))
		colunas = append(colunas, c)
	}
	return colunas, linhas.Err()
}

// ListarArquivadasDoBoard devolve as colunas arquivadas do quadro, da mais
// recente para a mais antiga.
func (r *ColunaPostgres) ListarArquivadasDoBoard(ctx context.Context, boardID string) ([]coluna.Coluna, error) {
	linhas, err := r.db.Query(ctx,
		`SELECT id, board_id, titulo, cor, chave, arquivado_em, criado_em, atualizado_em
		 FROM colunas WHERE board_id = $1 AND arquivado_em IS NOT NULL
		 ORDER BY arquivado_em DESC, id`, boardID,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	colunas := make([]coluna.Coluna, 0)
	for linhas.Next() {
		var c coluna.Coluna
		var corLida *string
		if err := linhas.Scan(&c.ID, &c.BoardID, &c.Titulo, &corLida, &c.Chave, &c.ArquivadoEm, &c.CriadoEm, &c.AtualizadoEm); err != nil {
			return nil, err
		}
		c.Cor = cor.Cor(valorOuVazio(corLida))
		colunas = append(colunas, c)
	}
	return colunas, linhas.Err()
}

// Apagar remove a coluna. Os cards dela vão junto pelo ON DELETE CASCADE.
func (r *ColunaPostgres) Apagar(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM colunas WHERE id = $1`, id)
	return err
}

// UltimaChave devolve a maior chave em uso no quadro, ou vazio.
func (r *ColunaPostgres) UltimaChave(ctx context.Context, boardID string) (string, error) {
	var chave *string
	err := r.db.QueryRow(ctx,
		`SELECT max(chave COLLATE "C") FROM colunas WHERE board_id = $1`, boardID,
	).Scan(&chave)
	if err != nil {
		return "", err
	}
	return valorOuVazio(chave), nil
}
