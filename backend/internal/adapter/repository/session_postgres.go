package repository

import (
	"context"
	"errors"
	"time"

	"stacktrack/internal/domain/session"

	"github.com/jackc/pgx/v5"
)

// SessionPostgres persiste sessões autenticadas no PostgreSQL.
type SessionPostgres struct {
	db consultante
}

// NovoSessionPostgres cria o repositório de sessões sobre o pool informado.
func NovoSessionPostgres(pool Fonte) *SessionPostgres {
	return &SessionPostgres{db: pool}
}

// Salvar persiste uma nova sessão.
func (r *SessionPostgres) Salvar(ctx context.Context, s *session.Session) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO sessions (token_hash, usuario_id, criado_em, expira_em)
		 VALUES ($1, $2, $3, $4)`,
		s.TokenHash, s.UsuarioID, s.CriadoEm, s.ExpiraEm,
	)
	return err
}

// BuscarPorTokenHash retorna (sessão, nil) quando encontra, (nil, nil) quando
// não existe sessão com o hash informado, e (nil, err) em falha real de
// infraestrutura.
func (r *SessionPostgres) BuscarPorTokenHash(ctx context.Context, hash string) (*session.Session, error) {
	var s session.Session
	err := r.db.QueryRow(ctx,
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
func (r *SessionPostgres) Remover(ctx context.Context, hash string) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM sessions WHERE token_hash = $1`, hash,
	)
	return err
}

// RemoverExpiradas apaga todas as sessões cuja expira_em já passou.
//
// ⚠️ Sem limite e sem lote: apaga tudo numa transação só. Continua existindo
// para os testes e para uma limpeza manual, mas NÃO está no caminho de nenhuma
// requisição — quem limpa em produção é o job horário, por LimparLote.
func (r *SessionPostgres) RemoverExpiradas(ctx context.Context) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM sessions WHERE expira_em < $1`, time.Now(),
	)
	return err
}

// Nome identifica esta limpeza no log e na métrica.
func (r *SessionPostgres) Nome() string { return "sessoes_expiradas" }

// LimparLote apaga até `limite` sessões vencidas e devolve quantas apagou.
//
// O subselect com LIMIT é o que torna o DELETE curto e retomável: sem ele, o
// comando varre e apaga a tabela inteira numa transação só — lock longo, WAL
// inflado e autovacuum para trás. Em lotes, cada transação dura milissegundos e
// uma falha no meio não desfaz o que já foi limpo.
//
// `ctid` no lugar da chave primária porque ele é o endereço físico da linha: o
// planejador vai direto nela, sem passar de novo pelo índice que o subselect já
// percorreu.
func (r *SessionPostgres) LimparLote(ctx context.Context, limite int) (int64, error) {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM sessions
		  WHERE ctid IN (
		    SELECT ctid FROM sessions WHERE expira_em < $1 LIMIT $2
		  )`, time.Now(), limite,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
