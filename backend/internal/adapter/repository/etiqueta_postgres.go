package repository

import (
	"context"
	"errors"
	"time"

	"stacktrack/internal/domain/etiqueta"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EtiquetaPostgres persiste etiquetas e a aplicação delas nos cards.
type EtiquetaPostgres struct {
	pool *pgxpool.Pool
}

// NovoEtiquetaPostgres cria o repositório de etiquetas sobre o pool informado.
func NovoEtiquetaPostgres(pool *pgxpool.Pool) *EtiquetaPostgres {
	return &EtiquetaPostgres{pool: pool}
}

// Salvar persiste uma etiqueta nova.
func (r *EtiquetaPostgres) Salvar(e *etiqueta.Etiqueta) error {
	_, err := r.pool.Exec(context.Background(),
		`INSERT INTO etiquetas (id, board_id, nome, cor, posicao, criado_em)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		e.ID, e.BoardID, e.Nome, string(e.Cor), e.Posicao, e.CriadoEm,
	)
	return err
}

// Atualizar grava nome e cor de uma etiqueta existente.
func (r *EtiquetaPostgres) Atualizar(e *etiqueta.Etiqueta) error {
	_, err := r.pool.Exec(context.Background(),
		`UPDATE etiquetas SET nome = $2, cor = $3, posicao = $4 WHERE id = $1`,
		e.ID, e.Nome, string(e.Cor), e.Posicao,
	)
	return err
}

// BuscarPorID retorna (etiqueta, nil) quando encontra e (nil, nil) quando não existe.
func (r *EtiquetaPostgres) BuscarPorID(id string) (*etiqueta.Etiqueta, error) {
	var e etiqueta.Etiqueta
	var cor string
	err := r.pool.QueryRow(context.Background(),
		`SELECT id, board_id, nome, cor, posicao, criado_em FROM etiquetas WHERE id = $1`, id,
	).Scan(&e.ID, &e.BoardID, &e.Nome, &cor, &e.Posicao, &e.CriadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.Cor = etiqueta.Cor(cor)
	return &e, nil
}

// ListarDoBoard devolve as etiquetas do quadro em ordem de posição.
func (r *EtiquetaPostgres) ListarDoBoard(boardID string) ([]etiqueta.Etiqueta, error) {
	linhas, err := r.pool.Query(context.Background(),
		`SELECT id, board_id, nome, cor, posicao, criado_em
		 FROM etiquetas WHERE board_id = $1 ORDER BY posicao, id`, boardID,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	return lerEtiquetas(linhas)
}

// EtiquetasDoCard devolve as etiquetas aplicadas a um card.
func (r *EtiquetaPostgres) EtiquetasDoCard(cardID string) ([]etiqueta.Etiqueta, error) {
	linhas, err := r.pool.Query(context.Background(),
		`SELECT e.id, e.board_id, e.nome, e.cor, e.posicao, e.criado_em
		 FROM card_etiquetas ce
		 JOIN etiquetas e ON e.id = ce.etiqueta_id
		 WHERE ce.card_id = $1
		 ORDER BY e.posicao, e.id`, cardID,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	return lerEtiquetas(linhas)
}

// EtiquetasDoBoardPorCard devolve, para cada card do quadro, os ids das
// etiquetas aplicadas — numa consulta só. Uma por card seria um N+1 que aparece
// justamente nos quadros grandes.
func (r *EtiquetaPostgres) EtiquetasDoBoardPorCard(boardID string) (map[string][]string, error) {
	linhas, err := r.pool.Query(context.Background(),
		`SELECT ce.card_id, ce.etiqueta_id
		 FROM card_etiquetas ce
		 JOIN cards c   ON c.id = ce.card_id
		 JOIN colunas l ON l.id = c.coluna_id
		 JOIN etiquetas e ON e.id = ce.etiqueta_id
		 WHERE l.board_id = $1
		 ORDER BY e.posicao, e.id`, boardID,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	porCard := make(map[string][]string)
	for linhas.Next() {
		var cardID, etiquetaID string
		if err := linhas.Scan(&cardID, &etiquetaID); err != nil {
			return nil, err
		}
		porCard[cardID] = append(porCard[cardID], etiquetaID)
	}
	return porCard, linhas.Err()
}

// Apagar remove a etiqueta do quadro; as aplicações nos cards vão junto pelo
// ON DELETE CASCADE.
func (r *EtiquetaPostgres) Apagar(id string) error {
	_, err := r.pool.Exec(context.Background(), `DELETE FROM etiquetas WHERE id = $1`, id)
	return err
}

// UltimaPosicao devolve a maior posição em uso no quadro, ou 0.
func (r *EtiquetaPostgres) UltimaPosicao(boardID string) (float64, error) {
	var ultima float64
	err := r.pool.QueryRow(context.Background(),
		`SELECT COALESCE(MAX(posicao), 0) FROM etiquetas WHERE board_id = $1`, boardID,
	).Scan(&ultima)
	return ultima, err
}

// Aplicar pendura a etiqueta no card. Aplicar de novo não é erro: a violação da
// chave primária composta é engolida porque o resultado pretendido — card com
// aquela etiqueta — já vale.
func (r *EtiquetaPostgres) Aplicar(cardID, etiquetaID string) error {
	_, err := r.pool.Exec(context.Background(),
		`INSERT INTO card_etiquetas (card_id, etiqueta_id, criado_em) VALUES ($1, $2, $3)`,
		cardID, etiquetaID, time.Now(),
	)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == codigoViolacaoUnique {
		return nil
	}
	return err
}

// Remover tira a etiqueta do card.
func (r *EtiquetaPostgres) Remover(cardID, etiquetaID string) error {
	_, err := r.pool.Exec(context.Background(),
		`DELETE FROM card_etiquetas WHERE card_id = $1 AND etiqueta_id = $2`, cardID, etiquetaID,
	)
	return err
}

func lerEtiquetas(linhas pgx.Rows) ([]etiqueta.Etiqueta, error) {
	etiquetas := make([]etiqueta.Etiqueta, 0)
	for linhas.Next() {
		var e etiqueta.Etiqueta
		var cor string
		if err := linhas.Scan(&e.ID, &e.BoardID, &e.Nome, &cor, &e.Posicao, &e.CriadoEm); err != nil {
			return nil, err
		}
		e.Cor = etiqueta.Cor(cor)
		etiquetas = append(etiquetas, e)
	}
	return etiquetas, linhas.Err()
}
