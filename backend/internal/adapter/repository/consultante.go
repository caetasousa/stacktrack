package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// consultante é o que *pgxpool.Pool e pgx.Tx têm em comum.
//
// Os repositórios dependem desta interface, e não do pool concreto, para que
// um mesmo repositório possa trabalhar indiferentemente por conta própria (o
// caso normal, cada consulta na sua conexão) ou dentro de uma transação já
// aberta — sem duplicar uma linha de SQL para isso.
type consultante interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}
