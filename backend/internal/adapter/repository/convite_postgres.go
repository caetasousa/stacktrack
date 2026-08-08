package repository

import (
	"context"
	"errors"

	"stacktrack/internal/domain/convite"
	"stacktrack/internal/domain/membro"
	"stacktrack/internal/domain/usuario"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ConvitePostgres persiste convites de quadro no PostgreSQL.
type ConvitePostgres struct {
	db consultante
}

// NovoConvitePostgres cria o repositório de convites sobre o pool informado.
func NovoConvitePostgres(pool *pgxpool.Pool) *ConvitePostgres {
	return &ConvitePostgres{db: pool}
}

const camposConvite = `id, board_id, email, papel, token_hash, criado_por, criado_em, expira_em, aceito_em`

// Salvar persiste um convite novo. Traduz a violação do índice único parcial em
// convite.ErrJaConvidado: entre a checagem do usecase e o INSERT cabe outro
// convite para o mesmo email, e quem descobre a colisão é o banco.
func (r *ConvitePostgres) Salvar(ctx context.Context, c *convite.Convite) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO convites_board (`+camposConvite+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		c.ID, c.BoardID, c.Email, string(c.Papel), c.TokenHash, c.CriadoPor,
		c.CriadoEm, c.ExpiraEm, c.AceitoEm,
	)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == codigoViolacaoUnique {
		return convite.ErrJaConvidado
	}
	return err
}

// Atualizar grava a aceitação do convite.
func (r *ConvitePostgres) Atualizar(ctx context.Context, c *convite.Convite) error {
	_, err := r.db.Exec(ctx,
		`UPDATE convites_board SET papel = $2, aceito_em = $3 WHERE id = $1`,
		c.ID, string(c.Papel), c.AceitoEm,
	)
	return err
}

// BuscarPorID retorna (convite, nil) quando encontra e (nil, nil) quando não existe.
func (r *ConvitePostgres) BuscarPorID(ctx context.Context, id string) (*convite.Convite, error) {
	return r.buscar(ctx, `WHERE id = $1`, id)
}

// BuscarPorTokenHash retorna (convite, nil) quando encontra e (nil, nil) quando
// nenhum convite corresponde ao hash.
func (r *ConvitePostgres) BuscarPorTokenHash(ctx context.Context, hash string) (*convite.Convite, error) {
	return r.buscar(ctx, `WHERE token_hash = $1`, hash)
}

// BuscarPendentePorEmail retorna o convite ainda não aceito daquele email no
// quadro, ou (nil, nil) se não houver.
func (r *ConvitePostgres) BuscarPendentePorEmail(ctx context.Context, boardID, email string) (*convite.Convite, error) {
	var c convite.Convite
	var papel string
	err := r.db.QueryRow(ctx,
		`SELECT `+camposConvite+` FROM convites_board
		 WHERE board_id = $1 AND email = $2 AND aceito_em IS NULL`,
		boardID, usuario.NormalizarEmail(email),
	).Scan(&c.ID, &c.BoardID, &c.Email, &papel, &c.TokenHash, &c.CriadoPor,
		&c.CriadoEm, &c.ExpiraEm, &c.AceitoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Papel = membro.Papel(papel)
	return &c, nil
}

// ListarPendentes devolve os convites do quadro que ainda não foram aceitos,
// inclusive os vencidos — a tela mostra o vencimento e deixa o dono decidir
// entre revogar e convidar de novo.
func (r *ConvitePostgres) ListarPendentes(ctx context.Context, boardID string) ([]convite.Convite, error) {
	linhas, err := r.db.Query(ctx,
		`SELECT `+camposConvite+` FROM convites_board
		 WHERE board_id = $1 AND aceito_em IS NULL
		 ORDER BY criado_em DESC`, boardID,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	convites := make([]convite.Convite, 0)
	for linhas.Next() {
		var c convite.Convite
		var papel string
		if err := linhas.Scan(&c.ID, &c.BoardID, &c.Email, &papel, &c.TokenHash, &c.CriadoPor,
			&c.CriadoEm, &c.ExpiraEm, &c.AceitoEm); err != nil {
			return nil, err
		}
		c.Papel = membro.Papel(papel)
		convites = append(convites, c)
	}
	return convites, linhas.Err()
}

// Remover apaga o convite, invalidando o link já entregue.
func (r *ConvitePostgres) Remover(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM convites_board WHERE id = $1`, id)
	return err
}

func (r *ConvitePostgres) buscar(ctx context.Context, filtro string, arg any) (*convite.Convite, error) {
	var c convite.Convite
	var papel string
	err := r.db.QueryRow(ctx,
		`SELECT `+camposConvite+` FROM convites_board `+filtro, arg,
	).Scan(&c.ID, &c.BoardID, &c.Email, &papel, &c.TokenHash, &c.CriadoPor,
		&c.CriadoEm, &c.ExpiraEm, &c.AceitoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Papel = membro.Papel(papel)
	return &c, nil
}
