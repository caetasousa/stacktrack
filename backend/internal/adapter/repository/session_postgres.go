package repository

import (
	"context"
	"errors"
	"time"

	"stacktrack/internal/domain/session"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionPostgres persiste sessões autenticadas no PostgreSQL.
type SessionPostgres struct {
	pool *pgxpool.Pool
}

// NovoSessionPostgres cria o repositório de sessões sobre o pool informado.
func NovoSessionPostgres(pool *pgxpool.Pool) *SessionPostgres {
	return &SessionPostgres{pool: pool}
}

// Salvar persiste uma nova sessão.
func (r *SessionPostgres) Salvar(s *session.Session) error {
	_, err := r.pool.Exec(context.Background(),
		`INSERT INTO sessions (token_hash, usuario_id, criado_em, expira_em)
		 VALUES ($1, $2, $3, $4)`,
		s.TokenHash, s.UsuarioID, s.CriadoEm, s.ExpiraEm,
	)
	return err
}

// BuscarPorTokenHash retorna (sessão, nil) quando encontra, (nil, nil) quando
// não existe sessão com o hash informado, e (nil, err) em falha real de
// infraestrutura.
func (r *SessionPostgres) BuscarPorTokenHash(hash string) (*session.Session, error) {
	var s session.Session
	err := r.pool.QueryRow(context.Background(),
		`SELECT token_hash, usuario_id, criado_em, expira_em
		 FROM sessions WHERE token_hash = $1`, hash,
	).Scan(&s.TokenHash, &s.UsuarioID, &s.CriadoEm, &s.ExpiraEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Remover apaga a sessão com o hash informado. Não é erro remover uma sessão inexistente.
func (r *SessionPostgres) Remover(hash string) error {
	_, err := r.pool.Exec(context.Background(),
		`DELETE FROM sessions WHERE token_hash = $1`, hash,
	)
	return err
}

// RemoverExpiradas apaga todas as sessões cuja expira_em já passou.
func (r *SessionPostgres) RemoverExpiradas() error {
	_, err := r.pool.Exec(context.Background(),
		`DELETE FROM sessions WHERE expira_em < $1`, time.Now(),
	)
	return err
}
