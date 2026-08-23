package repository

import (
	"context"
	"errors"
	"time"

	"stacktrack/internal/domain/convite"
	"stacktrack/internal/domain/membro"
	"stacktrack/internal/domain/usuario"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ConvitePostgres persiste convites de quadro no PostgreSQL.
type ConvitePostgres struct {
	db consultante
}

// NovoConvitePostgres cria o repositório de convites sobre o pool informado.
func NovoConvitePostgres(pool Fonte) *ConvitePostgres {
	return &ConvitePostgres{db: pool}
}

const camposConvite = `id, board_id, email, papel, token_hash, criado_por, criado_em, expira_em, aceito_em, revogado_em`

// Salvar persiste um convite novo. Traduz a violação do índice único parcial em
// convite.ErrJaConvidado: entre a checagem do usecase e o INSERT cabe outro
// convite para o mesmo email, e quem descobre a colisão é o banco.
func (r *ConvitePostgres) Salvar(ctx context.Context, c *convite.Convite) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO convites_board (`+camposConvite+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		c.ID, c.BoardID, c.Email, string(c.Papel), c.TokenHash, c.CriadoPor,
		c.CriadoEm, c.ExpiraEm, c.AceitoEm, c.RevogadoEm,
	)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == codigoViolacaoUnique {
		return convite.ErrJaConvidado
	}
	return err
}

// Aceitar marca o convite como aceito, e SÓ se ele ainda estiver pendente
// naquele instante.
//
// A condição está no WHERE, e não numa leitura anterior seguida de escrita.
// Entre um SELECT e um UPDATE cabe outra transação inteira: duas abas clicando
// no link ao mesmo tempo liam "pendente" as duas, gravavam as duas, e o convite
// era consumido duas vezes — com dois eventos "entrou no quadro" para a mesma
// pessoa. Aqui, quem perde a corrida vê RowsAffected = 0.
//
// `expira_em >= $2` usa o MESMO instante que o domínio usou para decidir, e não
// now(): o relógio do banco e o do processo não são o mesmo, e deixar a
// fronteira do vencimento depender de qual dos dois foi consultado tornaria o
// resultado irreprodutível justamente no caso de borda.
//
// Devolve convite.ErrJaResolvido quando nenhuma linha muda — o convite foi
// aceito, revogado ou venceu entre a leitura e esta escrita.
func (r *ConvitePostgres) Aceitar(ctx context.Context, id string, em time.Time) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE convites_board SET aceito_em = $2
		  WHERE id = $1
		    AND aceito_em IS NULL
		    AND revogado_em IS NULL
		    AND expira_em >= $2`,
		id, em,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return convite.ErrJaResolvido
	}
	return nil
}

// Revogar marca o convite como revogado, e SÓ se ele ainda não for terminal.
//
// Ao contrário de Aceitar, o VENCIDO é revogável: é assim que a vaga do índice
// de pendência é liberada para um convite novo ao mesmo email. O que não se
// revoga é o que já foi aceito ou já foi revogado.
//
// Antes isto era um DELETE. Apagar a linha levava junto a resposta para "quem
// convidou quem, e quando" e, no caminho concorrente, tornava indistinguível
// "revoguei agora" de "nunca existiu".
//
// Devolve convite.ErrJaResolvido quando nenhuma linha muda.
func (r *ConvitePostgres) Revogar(ctx context.Context, id string, em time.Time) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE convites_board SET revogado_em = $2
		  WHERE id = $1
		    AND aceito_em IS NULL
		    AND revogado_em IS NULL`,
		id, em,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return convite.ErrJaResolvido
	}
	return nil
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
		 WHERE board_id = $1 AND email = $2 AND aceito_em IS NULL AND revogado_em IS NULL`,
		boardID, usuario.NormalizarEmail(email),
	).Scan(&c.ID, &c.BoardID, &c.Email, &papel, &c.TokenHash, &c.CriadoPor,
		&c.CriadoEm, &c.ExpiraEm, &c.AceitoEm, &c.RevogadoEm)
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
		 WHERE board_id = $1 AND aceito_em IS NULL AND revogado_em IS NULL
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
			&c.CriadoEm, &c.ExpiraEm, &c.AceitoEm, &c.RevogadoEm); err != nil {
			return nil, err
		}
		c.Papel = membro.Papel(papel)
		convites = append(convites, c)
	}
	return convites, linhas.Err()
}

// Nome identifica esta limpeza no log e na métrica.
func (r *ConvitePostgres) Nome() string { return "convites_terminais" }

// LimparLote apaga convites TERMINAIS e antigos: aceitos ou revogados há mais
// de RetencaoDeConviteTerminal.
//
// Terminal, e não "vencido": um convite vencido ainda ocupa a vaga do índice de
// pendência e é o domínio que o revoga ao convidar de novo — apagá-lo aqui
// faria a limpeza tomar uma decisão de negócio que não é dela.
//
// A retenção existe porque a linha ainda responde "quem convidou quem, e
// quando" logo depois do fato. Passados alguns meses, essa resposta já está no
// log de eventos (convite.criado, convite.revogado, membro.entrou), que é
// append-only e é a fonte da auditoria.
func (r *ConvitePostgres) LimparLote(ctx context.Context, limite int) (int64, error) {
	corte := time.Now().Add(-RetencaoDeConviteTerminal)
	tag, err := r.db.Exec(ctx,
		`DELETE FROM convites_board
		  WHERE ctid IN (
		    SELECT ctid FROM convites_board
		     WHERE (aceito_em IS NOT NULL AND aceito_em < $1)
		        OR (revogado_em IS NOT NULL AND revogado_em < $1)
		     LIMIT $2
		  )`, corte, limite,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// RetencaoDeConviteTerminal é por quanto tempo um convite já resolvido continua
// no banco depois de aceito ou revogado.
const RetencaoDeConviteTerminal = 90 * 24 * time.Hour

func (r *ConvitePostgres) buscar(ctx context.Context, filtro string, arg any) (*convite.Convite, error) {
	var c convite.Convite
	var papel string
	err := r.db.QueryRow(ctx,
		`SELECT `+camposConvite+` FROM convites_board `+filtro, arg,
	).Scan(&c.ID, &c.BoardID, &c.Email, &papel, &c.TokenHash, &c.CriadoPor,
		&c.CriadoEm, &c.ExpiraEm, &c.AceitoEm, &c.RevogadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Papel = membro.Papel(papel)
	return &c, nil
}
