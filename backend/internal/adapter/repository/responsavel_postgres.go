package repository

import (
	"context"
	"errors"
	"time"

	ucboard "stacktrack/internal/usecase/board"

	"github.com/jackc/pgx/v5/pgconn"
)

// ResponsavelPostgres persiste quem é responsável por cada card.
type ResponsavelPostgres struct {
	db consultante
}

// NovoResponsavelPostgres cria o repositório de responsáveis sobre o pool informado.
func NovoResponsavelPostgres(pool Fonte) *ResponsavelPostgres {
	return &ResponsavelPostgres{db: pool}
}

// Atribuir marca a pessoa como responsável pelo card.
//
// Atribuir quem já é responsável não é erro: o resultado pretendido já vale, e
// a chave primária composta é quem garante isso. Tratar a violação aqui evita
// que a tela precise saber o estado antes de agir.
func (r *ResponsavelPostgres) Atribuir(ctx context.Context, cardID, usuarioID string) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO card_responsaveis (card_id, usuario_id, criado_em) VALUES ($1, $2, $3)`,
		cardID, usuarioID, time.Now(),
	)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == codigoViolacaoUnique {
		return nil
	}
	return err
}

// Remover tira a pessoa da responsabilidade do card. Remover quem não é
// responsável não é erro, pelo mesmo motivo.
func (r *ResponsavelPostgres) Remover(ctx context.Context, cardID, usuarioID string) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM card_responsaveis WHERE card_id = $1 AND usuario_id = $2`,
		cardID, usuarioID,
	)
	return err
}

// RemoverDoBoard apaga todas as atribuições da pessoa naquele quadro.
//
// É o que roda quando alguém sai do quadro. Sem isto, a lista de responsáveis
// passaria a exibir nomes de quem não tem mais acesso — e o filtro "meus cards"
// mostraria cards que a pessoa não consegue mais abrir.
func (r *ResponsavelPostgres) RemoverDoBoard(ctx context.Context, boardID, usuarioID string) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM card_responsaveis cr
		  USING cards c, colunas l
		  WHERE cr.card_id = c.id
		    AND c.coluna_id = l.id
		    AND l.board_id = $1
		    AND cr.usuario_id = $2`,
		boardID, usuarioID,
	)
	return err
}

// DoCard devolve os responsáveis de um card, já com o nome — é o que o modal
// mostra.
func (r *ResponsavelPostgres) DoCard(ctx context.Context, cardID string) ([]ucboard.Responsavel, error) {
	linhas, err := r.db.Query(ctx,
		`SELECT u.id, u.nome
		   FROM card_responsaveis cr
		   JOIN usuarios u ON u.id = cr.usuario_id
		  WHERE cr.card_id = $1
		  ORDER BY u.nome, u.id`, cardID,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	return coletarResponsaveis(linhas)
}

// DoBoardPorCard devolve, para cada card do quadro, quem responde por ele —
// numa consulta só. Uma por card seria um N+1 que piora conforme o quadro
// cresce, do mesmo jeito que nas etiquetas.
func (r *ResponsavelPostgres) DoBoardPorCard(ctx context.Context, boardID string) (map[string][]ucboard.Responsavel, error) {
	linhas, err := r.db.Query(ctx,
		`SELECT cr.card_id, u.id, u.nome
		   FROM card_responsaveis cr
		   JOIN cards c    ON c.id = cr.card_id
		   JOIN colunas l  ON l.id = c.coluna_id
		   JOIN usuarios u ON u.id = cr.usuario_id
		  WHERE l.board_id = $1
		  ORDER BY u.nome, u.id`, boardID,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	porCard := make(map[string][]ucboard.Responsavel)
	for linhas.Next() {
		var cardID string
		var p ucboard.Responsavel
		if err := linhas.Scan(&cardID, &p.UsuarioID, &p.Nome); err != nil {
			return nil, err
		}
		porCard[cardID] = append(porCard[cardID], p)
	}
	return porCard, linhas.Err()
}

func coletarResponsaveis(linhas interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]ucboard.Responsavel, error) {
	fora := make([]ucboard.Responsavel, 0)
	for linhas.Next() {
		var p ucboard.Responsavel
		if err := linhas.Scan(&p.UsuarioID, &p.Nome); err != nil {
			return nil, err
		}
		fora = append(fora, p)
	}
	return fora, linhas.Err()
}
