package repository

import (
	"context"
	"errors"
	"time"

	"stacktrack/internal/domain/membro"
	ucboard "stacktrack/internal/usecase/board"

	"github.com/jackc/pgx/v5"
)

// DominioPostgres persiste a abertura dos quadros ao domínio do email e os
// bloqueios de quem o dono tirou do quadro.
type DominioPostgres struct {
	db consultante
}

// NovoDominioPostgres cria o repositório sobre o pool informado.
func NovoDominioPostgres(pool Fonte) *DominioPostgres {
	return &DominioPostgres{db: pool}
}

// Definir grava SÓ as duas colunas do domínio — ver BoardPostgres.Renomear
// para o porquê de ser estreito. Domínio vazio grava os dois nulos.
func (r *DominioPostgres) Definir(ctx context.Context, boardID, dominio string, papel membro.Papel, em time.Time) error {
	if dominio == "" {
		papel = ""
	}
	_, err := r.db.Exec(ctx,
		`UPDATE boards SET dominio_acesso = $2, dominio_papel = $3, atualizado_em = $4 WHERE id = $1`,
		boardID, vazioParaNulo(dominio), vazioParaNulo(string(papel)), em,
	)
	return err
}

// DoQuadro devolve o domínio e o papel; vazios quando o quadro está fechado ou
// não existe.
func (r *DominioPostgres) DoQuadro(ctx context.Context, boardID string) (string, membro.Papel, error) {
	var dominio, papel *string
	err := r.db.QueryRow(ctx,
		`SELECT dominio_acesso, dominio_papel FROM boards WHERE id = $1`, boardID,
	).Scan(&dominio, &papel)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	return valorOuVazio(dominio), membro.Papel(valorOuVazio(papel)), nil
}

// AbertosPara parte do índice parcial do domínio, e exclui na própria consulta
// quem já participa e quem foi tirado: filtrar depois, no Go, faria a lista
// depender de alguém lembrar das duas exclusões.
func (r *DominioPostgres) AbertosPara(ctx context.Context, dominio, usuarioID string) ([]ucboard.QuadroDoDominio, error) {
	linhas, err := r.db.Query(ctx,
		`SELECT b.id, b.titulo, b.dominio_papel,
		        COALESCE((SELECT u.nome
		                    FROM board_membros d
		                    JOIN usuarios u ON u.id = d.usuario_id
		                   WHERE d.board_id = b.id AND d.papel = $3
		                   ORDER BY d.criado_em, d.usuario_id
		                   LIMIT 1), ''),
		        (SELECT count(*) FROM board_membros c WHERE c.board_id = b.id)
		   FROM boards b
		  WHERE b.dominio_acesso = $1
		    AND NOT EXISTS (SELECT 1 FROM board_membros m WHERE m.board_id = b.id AND m.usuario_id = $2)
		    AND NOT EXISTS (SELECT 1 FROM board_bloqueios x WHERE x.board_id = b.id AND x.usuario_id = $2)
		  ORDER BY b.titulo, b.id`, dominio, usuarioID, string(membro.PapelDono),
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	quadros := make([]ucboard.QuadroDoDominio, 0)
	for linhas.Next() {
		var q ucboard.QuadroDoDominio
		var papel string
		if err := linhas.Scan(&q.ID, &q.Titulo, &papel, &q.Dono, &q.Participantes); err != nil {
			return nil, err
		}
		q.Papel = membro.Papel(papel)
		quadros = append(quadros, q)
	}
	return quadros, linhas.Err()
}

// Bloquear é idempotente: tirar de novo quem já foi tirado mantém a data da
// primeira vez.
func (r *DominioPostgres) Bloquear(ctx context.Context, boardID, usuarioID string, em time.Time) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO board_bloqueios (board_id, usuario_id, criado_em) VALUES ($1, $2, $3)
		 ON CONFLICT (board_id, usuario_id) DO NOTHING`,
		boardID, usuarioID, em,
	)
	return err
}

// Desbloquear apaga o bloqueio, se houver.
func (r *DominioPostgres) Desbloquear(ctx context.Context, boardID, usuarioID string) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM board_bloqueios WHERE board_id = $1 AND usuario_id = $2`, boardID, usuarioID,
	)
	return err
}

// Bloqueado informa se a pessoa foi tirada do quadro.
func (r *DominioPostgres) Bloqueado(ctx context.Context, boardID, usuarioID string) (bool, error) {
	var existe bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM board_bloqueios WHERE board_id = $1 AND usuario_id = $2)`,
		boardID, usuarioID,
	).Scan(&existe)
	return existe, err
}
