package repository

import (
	"context"
	"errors"
	"time"

	"stacktrack/internal/domain/usuario"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// codigoViolacaoUnique é o SQLSTATE do Postgres para violação de UNIQUE.
const codigoViolacaoUnique = "23505"

// UsuarioPostgres persiste contas no PostgreSQL.
type UsuarioPostgres struct {
	db consultante
}

// NovoUsuarioPostgres cria o repositório de usuários sobre o pool informado.
func NovoUsuarioPostgres(pool Fonte) *UsuarioPostgres {
	return &UsuarioPostgres{db: pool}
}

// Salvar persiste um usuário novo. Traduz a violação do UNIQUE de email em
// usuario.ErrEmailEmUso: é o banco quem descobre a colisão quando dois
// cadastros do mesmo email chegam ao mesmo tempo e ambos passaram pela
// checagem prévia do usecase.
func (r *UsuarioPostgres) Salvar(ctx context.Context, u *usuario.Usuario) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO usuarios (id, nome, email, senha_hash, criado_em, atualizado_em, email_confirmado_em)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		u.ID, u.Nome, u.Email, u.SenhaHash, u.CriadoEm, u.AtualizadoEm, u.EmailConfirmadoEm,
	)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == codigoViolacaoUnique {
		return usuario.ErrEmailEmUso
	}
	return err
}

// BuscarPorID retorna (usuário, nil) quando encontra, (nil, nil) quando não
// existe conta com o id informado, e (nil, err) em falha real de infraestrutura.
func (r *UsuarioPostgres) BuscarPorID(ctx context.Context, id string) (*usuario.Usuario, error) {
	return r.buscar(ctx, `WHERE id = $1`, id)
}

// BuscarPorEmail retorna (usuário, nil) quando encontra, (nil, nil) quando não
// existe conta com o email informado, e (nil, err) em falha real de infraestrutura.
// O email é normalizado antes da consulta, pelo mesmo motivo que na escrita.
func (r *UsuarioPostgres) BuscarPorEmail(ctx context.Context, email string) (*usuario.Usuario, error) {
	return r.buscar(ctx, `WHERE email = $1`, usuario.NormalizarEmail(email))
}

func (r *UsuarioPostgres) buscar(ctx context.Context, filtro string, arg any) (*usuario.Usuario, error) {
	var u usuario.Usuario
	err := r.db.QueryRow(ctx,
		`SELECT id, nome, email, senha_hash, criado_em, atualizado_em, email_confirmado_em
		 FROM usuarios `+filtro, arg,
	).Scan(&u.ID, &u.Nome, &u.Email, &u.SenhaHash, &u.CriadoEm, &u.AtualizadoEm, &u.EmailConfirmadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// DefinirSenha troca o hash da senha da conta. É o fim da recuperação de
// senha, e roda dentro da transação dela.
func (r *UsuarioPostgres) DefinirSenha(ctx context.Context, usuarioID, hash string, em time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE usuarios SET senha_hash = $2, atualizado_em = $3 WHERE id = $1`, usuarioID, hash, em,
	)
	return err
}

// ConfirmarEmail marca a conta como dona do email. Confirmar de novo não muda a
// data: a primeira prova é a que vale.
func (r *UsuarioPostgres) ConfirmarEmail(ctx context.Context, usuarioID string, em time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE usuarios SET email_confirmado_em = $2, atualizado_em = $2
		  WHERE id = $1 AND email_confirmado_em IS NULL`, usuarioID, em,
	)
	return err
}
