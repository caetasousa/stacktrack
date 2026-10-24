package repository

import (
	"context"
	"errors"
	"time"

	"stacktrack/internal/domain/confirmacao"

	"github.com/jackc/pgx/v5"
)

// ConfirmacaoPostgres guarda os links de confirmação de email. Serve à
// transação da confirmação (pela UnidadeDeAutenticacao) e à faxina (pelo pool).
type ConfirmacaoPostgres struct {
	db consultante
}

// NovoConfirmacaoPostgres cria o repositório sobre o pool — o da faxina.
func NovoConfirmacaoPostgres(pool Fonte) *ConfirmacaoPostgres {
	return &ConfirmacaoPostgres{db: pool}
}

// Salvar grava um link novo.
func (r *ConfirmacaoPostgres) Salvar(ctx context.Context, c *confirmacao.Confirmacao) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO confirmacoes_email (id, usuario_id, token_hash, criado_em, expira_em, usada_em)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		c.ID, c.UsuarioID, c.TokenHash, c.CriadoEm, c.ExpiraEm, c.UsadaEm,
	)
	return err
}

// BuscarPorTokenHash devolve o link do hash, travando a linha (FOR UPDATE).
// Devolve (nil, nil) quando não encontra.
func (r *ConfirmacaoPostgres) BuscarPorTokenHash(ctx context.Context, hash string) (*confirmacao.Confirmacao, error) {
	var c confirmacao.Confirmacao
	err := r.db.QueryRow(ctx,
		`SELECT id, usuario_id, token_hash, criado_em, expira_em, usada_em
		   FROM confirmacoes_email WHERE token_hash = $1
		    FOR UPDATE`, hash,
	).Scan(&c.ID, &c.UsuarioID, &c.TokenHash, &c.CriadoEm, &c.ExpiraEm, &c.UsadaEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ContarDoUsuarioDesde conta os links que a conta pediu a partir de `desde`.
func (r *ConfirmacaoPostgres) ContarDoUsuarioDesde(ctx context.Context, usuarioID string, desde time.Time) (int, error) {
	var n int
	err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM confirmacoes_email WHERE usuario_id = $1 AND criado_em >= $2`,
		usuarioID, desde,
	).Scan(&n)
	return n, err
}

// EncerrarDoUsuario marca como usados os links ainda abertos da conta.
func (r *ConfirmacaoPostgres) EncerrarDoUsuario(ctx context.Context, usuarioID string, em time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE confirmacoes_email SET usada_em = $2
		  WHERE usuario_id = $1 AND usada_em IS NULL`, usuarioID, em,
	)
	return err
}

// Nome identifica a limpeza no log da faxina.
func (r *ConfirmacaoPostgres) Nome() string { return "confirmacoes_vencidas" }

// LimparLote apaga até `limite` links vencidos.
func (r *ConfirmacaoPostgres) LimparLote(ctx context.Context, limite int) (int64, error) {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM confirmacoes_email
		  WHERE ctid IN (
		    SELECT ctid FROM confirmacoes_email WHERE expira_em < $1 LIMIT $2
		  )`, time.Now(), limite,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
