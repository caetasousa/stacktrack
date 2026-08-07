package repository

import (
	"context"
	"errors"

	"kanbango/internal/domain/card"
	"kanbango/internal/domain/cor"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CardPostgres persiste cards no PostgreSQL.
type CardPostgres struct {
	pool *pgxpool.Pool
}

// NovoCardPostgres cria o repositório de cards sobre o pool informado.
func NovoCardPostgres(pool *pgxpool.Pool) *CardPostgres {
	return &CardPostgres{pool: pool}
}

// Salvar persiste um card novo.
func (r *CardPostgres) Salvar(c *card.Card) error {
	_, err := r.pool.Exec(context.Background(),
		`INSERT INTO cards (id, coluna_id, titulo, descricao, cor, posicao, version, prazo, criado_em, atualizado_em)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		c.ID, c.ColunaID, c.Titulo, c.Descricao, vazioParaNulo(string(c.Cor)),
		c.Posicao, c.Version, c.Prazo, c.CriadoEm, c.AtualizadoEm,
	)
	return err
}

// Atualizar grava as alterações de um card existente.
//
// O UPDATE ainda não filtra por version — a versão é gravada, não conferida.
// O `WHERE ... AND version = $x` com 409 na volta é o bloqueio otimista da
// fase 6; até lá, a última escrita vence.
func (r *CardPostgres) Atualizar(c *card.Card) error {
	_, err := r.pool.Exec(context.Background(),
		`UPDATE cards SET coluna_id = $2, titulo = $3, descricao = $4, cor = $5, posicao = $6,
		        version = $7, prazo = $8, atualizado_em = $9
		 WHERE id = $1`,
		c.ID, c.ColunaID, c.Titulo, c.Descricao, vazioParaNulo(string(c.Cor)),
		c.Posicao, c.Version, c.Prazo, c.AtualizadoEm,
	)
	return err
}

// BuscarPorID retorna (card, nil) quando encontra e (nil, nil) quando não existe.
func (r *CardPostgres) BuscarPorID(id string) (*card.Card, error) {
	var c card.Card
	var corLida *string
	err := r.pool.QueryRow(context.Background(),
		`SELECT id, coluna_id, titulo, descricao, cor, posicao, version, prazo, criado_em, atualizado_em
		 FROM cards WHERE id = $1`, id,
	).Scan(&c.ID, &c.ColunaID, &c.Titulo, &c.Descricao, &corLida, &c.Posicao, &c.Version, &c.Prazo, &c.CriadoEm, &c.AtualizadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Cor = cor.Cor(valorOuVazio(corLida))
	return &c, nil
}

// ListarDoBoard devolve todos os cards do quadro em ordem de posição, numa
// consulta só — uma por coluna seria um N+1 que piora conforme o quadro cresce.
func (r *CardPostgres) ListarDoBoard(boardID string) ([]card.Card, error) {
	linhas, err := r.pool.Query(context.Background(),
		`SELECT c.id, c.coluna_id, c.titulo, c.descricao, c.cor, c.posicao, c.version, c.prazo, c.criado_em, c.atualizado_em
		 FROM cards c
		 JOIN colunas col ON col.id = c.coluna_id
		 WHERE col.board_id = $1
		 ORDER BY c.posicao, c.id`, boardID,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	cards := make([]card.Card, 0)
	for linhas.Next() {
		var c card.Card
		var corLida *string
		if err := linhas.Scan(&c.ID, &c.ColunaID, &c.Titulo, &c.Descricao, &corLida, &c.Posicao, &c.Version, &c.Prazo, &c.CriadoEm, &c.AtualizadoEm); err != nil {
			return nil, err
		}
		c.Cor = cor.Cor(valorOuVazio(corLida))
		cards = append(cards, c)
	}
	return cards, linhas.Err()
}

// Apagar remove o card.
func (r *CardPostgres) Apagar(id string) error {
	_, err := r.pool.Exec(context.Background(), `DELETE FROM cards WHERE id = $1`, id)
	return err
}

// UltimaPosicao devolve a maior posição em uso na coluna, ou 0 quando ela está
// vazia.
func (r *CardPostgres) UltimaPosicao(colunaID string) (float64, error) {
	var ultima float64
	err := r.pool.QueryRow(context.Background(),
		`SELECT COALESCE(MAX(posicao), 0) FROM cards WHERE coluna_id = $1`, colunaID,
	).Scan(&ultima)
	return ultima, err
}
