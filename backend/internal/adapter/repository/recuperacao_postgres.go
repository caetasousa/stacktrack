package repository

import (
	"context"
	"errors"
	"time"

	"stacktrack/internal/domain/recuperacao"

	"github.com/jackc/pgx/v5"
)

// RecuperacaoPostgres guarda os pedidos de recuperação de senha. Serve à
// transação da recuperação (ligado a ela pela UnidadeDeAutenticacao) e à faxina
// (ligado ao pool).
type RecuperacaoPostgres struct {
	db consultante
}

// NovoRecuperacaoPostgres cria o repositório sobre o pool informado — o da
// faxina. O caso de uso usa o que a unidade de autenticação liga à transação.
func NovoRecuperacaoPostgres(pool Fonte) *RecuperacaoPostgres {
	return &RecuperacaoPostgres{db: pool}
}

// Salvar grava um pedido novo.
func (r *RecuperacaoPostgres) Salvar(ctx context.Context, rec *recuperacao.Recuperacao) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO recuperacoes_senha (id, usuario_id, token_hash, criado_em, expira_em, encerrada_em)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		rec.ID, rec.UsuarioID, rec.TokenHash, rec.CriadoEm, rec.ExpiraEm, rec.EncerradaEm,
	)
	return err
}

// BuscarPorTokenHash devolve o pedido do hash, travando a linha (FOR UPDATE):
// numa transação, a segunda redefinição com o mesmo link espera a primeira
// terminar e relê a linha já encerrada. Fora de transação o lock dura só o
// comando, e não faz mal. Devolve (nil, nil) quando não encontra.
func (r *RecuperacaoPostgres) BuscarPorTokenHash(ctx context.Context, hash string) (*recuperacao.Recuperacao, error) {
	var rec recuperacao.Recuperacao
	err := r.db.QueryRow(ctx,
		`SELECT id, usuario_id, token_hash, criado_em, expira_em, encerrada_em
		   FROM recuperacoes_senha WHERE token_hash = $1
		    FOR UPDATE`, hash,
	).Scan(&rec.ID, &rec.UsuarioID, &rec.TokenHash, &rec.CriadoEm, &rec.ExpiraEm, &rec.EncerradaEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// TravarConta segura a linha da conta até o fim da transação.
func (r *RecuperacaoPostgres) TravarConta(ctx context.Context, usuarioID string) error {
	var id string
	err := r.db.QueryRow(ctx, `SELECT id FROM usuarios WHERE id = $1 FOR UPDATE`, usuarioID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

// ContarDoUsuarioDesde conta os pedidos que a conta fez a partir de `desde`.
func (r *RecuperacaoPostgres) ContarDoUsuarioDesde(ctx context.Context, usuarioID string, desde time.Time) (int, error) {
	var n int
	err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM recuperacoes_senha WHERE usuario_id = $1 AND criado_em >= $2`,
		usuarioID, desde,
	).Scan(&n)
	return n, err
}

// EncerrarDoUsuario encerra os links ainda abertos da conta.
func (r *RecuperacaoPostgres) EncerrarDoUsuario(ctx context.Context, usuarioID string, em time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE recuperacoes_senha SET encerrada_em = $2
		  WHERE usuario_id = $1 AND encerrada_em IS NULL`, usuarioID, em,
	)
	return err
}

// Nome identifica a limpeza no log da faxina.
func (r *RecuperacaoPostgres) Nome() string { return "recuperacoes_vencidas" }

// LimparLote apaga até `limite` pedidos vencidos. Um link vencido não serve a
// ninguém — nem como registro, porque o teto só olha a última hora —, e o
// hash dele não precisa ficar no banco.
func (r *RecuperacaoPostgres) LimparLote(ctx context.Context, limite int) (int64, error) {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM recuperacoes_senha
		  WHERE ctid IN (
		    SELECT ctid FROM recuperacoes_senha WHERE expira_em < $1 LIMIT $2
		  )`, time.Now(), limite,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
