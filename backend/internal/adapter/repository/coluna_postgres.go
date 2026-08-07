package repository

import (
	"context"
	"errors"

	"kanbango/internal/domain/coluna"
	"kanbango/internal/domain/cor"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ColunaPostgres persiste colunas no PostgreSQL.
type ColunaPostgres struct {
	pool *pgxpool.Pool
}

// NovoColunaPostgres cria o repositório de colunas sobre o pool informado.
func NovoColunaPostgres(pool *pgxpool.Pool) *ColunaPostgres {
	return &ColunaPostgres{pool: pool}
}

// Salvar persiste uma coluna nova.
func (r *ColunaPostgres) Salvar(c *coluna.Coluna) error {
	_, err := r.pool.Exec(context.Background(),
		`INSERT INTO colunas (id, board_id, titulo, cor, posicao, criado_em, atualizado_em)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		c.ID, c.BoardID, c.Titulo, vazioParaNulo(string(c.Cor)), c.Posicao, c.CriadoEm, c.AtualizadoEm,
	)
	return err
}

// Atualizar grava as alterações de uma coluna existente.
func (r *ColunaPostgres) Atualizar(c *coluna.Coluna) error {
	_, err := r.pool.Exec(context.Background(),
		`UPDATE colunas SET titulo = $2, cor = $3, posicao = $4, atualizado_em = $5 WHERE id = $1`,
		c.ID, c.Titulo, vazioParaNulo(string(c.Cor)), c.Posicao, c.AtualizadoEm,
	)
	return err
}

// BuscarPorID retorna (coluna, nil) quando encontra e (nil, nil) quando não existe.
func (r *ColunaPostgres) BuscarPorID(id string) (*coluna.Coluna, error) {
	var c coluna.Coluna
	var corLida *string
	err := r.pool.QueryRow(context.Background(),
		`SELECT id, board_id, titulo, cor, posicao, criado_em, atualizado_em FROM colunas WHERE id = $1`, id,
	).Scan(&c.ID, &c.BoardID, &c.Titulo, &corLida, &c.Posicao, &c.CriadoEm, &c.AtualizadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Cor = cor.Cor(valorOuVazio(corLida))
	return &c, nil
}

// ListarDoBoard devolve as colunas do quadro em ordem de posição.
//
// O desempate por id não é decoração: com duas posições iguais — que a fase 4
// pode produzir ao esgotar a precisão do float — a ordem sem desempate varia
// entre consultas, e a tela reordenaria sozinha a cada F5.
func (r *ColunaPostgres) ListarDoBoard(boardID string) ([]coluna.Coluna, error) {
	linhas, err := r.pool.Query(context.Background(),
		`SELECT id, board_id, titulo, cor, posicao, criado_em, atualizado_em
		 FROM colunas WHERE board_id = $1 ORDER BY posicao, id`, boardID,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	colunas := make([]coluna.Coluna, 0)
	for linhas.Next() {
		var c coluna.Coluna
		var corLida *string
		if err := linhas.Scan(&c.ID, &c.BoardID, &c.Titulo, &corLida, &c.Posicao, &c.CriadoEm, &c.AtualizadoEm); err != nil {
			return nil, err
		}
		c.Cor = cor.Cor(valorOuVazio(corLida))
		colunas = append(colunas, c)
	}
	return colunas, linhas.Err()
}

// Apagar remove a coluna. Os cards dela vão junto pelo ON DELETE CASCADE.
func (r *ColunaPostgres) Apagar(id string) error {
	_, err := r.pool.Exec(context.Background(), `DELETE FROM colunas WHERE id = $1`, id)
	return err
}

// UltimaPosicao devolve a maior posição em uso no quadro, ou 0 quando ele não
// tem coluna nenhuma. O COALESCE evita ter de tratar o NULL do MAX sobre
// conjunto vazio no Go.
func (r *ColunaPostgres) UltimaPosicao(boardID string) (float64, error) {
	var ultima float64
	err := r.pool.QueryRow(context.Background(),
		`SELECT COALESCE(MAX(posicao), 0) FROM colunas WHERE board_id = $1`, boardID,
	).Scan(&ultima)
	return ultima, err
}
