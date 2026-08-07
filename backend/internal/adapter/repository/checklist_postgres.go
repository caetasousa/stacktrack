package repository

import (
	"context"
	"errors"

	"stacktrack/internal/domain/checklist"
	ucboard "stacktrack/internal/usecase/board"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ChecklistPostgres persiste checklists e os itens delas.
type ChecklistPostgres struct {
	pool *pgxpool.Pool
}

// NovoChecklistPostgres cria o repositório de checklists sobre o pool informado.
func NovoChecklistPostgres(pool *pgxpool.Pool) *ChecklistPostgres {
	return &ChecklistPostgres{pool: pool}
}

// Salvar persiste uma checklist nova.
func (r *ChecklistPostgres) Salvar(c *checklist.Checklist) error {
	_, err := r.pool.Exec(context.Background(),
		`INSERT INTO checklists (id, card_id, titulo, posicao, criado_em, atualizado_em)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		c.ID, c.CardID, c.Titulo, c.Posicao, c.CriadoEm, c.AtualizadoEm,
	)
	return err
}

// Atualizar grava as alterações de uma checklist existente.
func (r *ChecklistPostgres) Atualizar(c *checklist.Checklist) error {
	_, err := r.pool.Exec(context.Background(),
		`UPDATE checklists SET titulo = $2, posicao = $3, atualizado_em = $4 WHERE id = $1`,
		c.ID, c.Titulo, c.Posicao, c.AtualizadoEm,
	)
	return err
}

// BuscarPorID retorna (checklist, nil) quando encontra e (nil, nil) quando não existe.
func (r *ChecklistPostgres) BuscarPorID(id string) (*checklist.Checklist, error) {
	var c checklist.Checklist
	err := r.pool.QueryRow(context.Background(),
		`SELECT id, card_id, titulo, posicao, criado_em, atualizado_em FROM checklists WHERE id = $1`, id,
	).Scan(&c.ID, &c.CardID, &c.Titulo, &c.Posicao, &c.CriadoEm, &c.AtualizadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListarDoCard devolve as checklists do card em ordem de posição.
func (r *ChecklistPostgres) ListarDoCard(cardID string) ([]checklist.Checklist, error) {
	linhas, err := r.pool.Query(context.Background(),
		`SELECT id, card_id, titulo, posicao, criado_em, atualizado_em
		 FROM checklists WHERE card_id = $1 ORDER BY posicao, id`, cardID,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	listas := make([]checklist.Checklist, 0)
	for linhas.Next() {
		var c checklist.Checklist
		if err := linhas.Scan(&c.ID, &c.CardID, &c.Titulo, &c.Posicao, &c.CriadoEm, &c.AtualizadoEm); err != nil {
			return nil, err
		}
		listas = append(listas, c)
	}
	return listas, linhas.Err()
}

// Apagar remove a checklist; os itens vão junto pelo ON DELETE CASCADE.
func (r *ChecklistPostgres) Apagar(id string) error {
	_, err := r.pool.Exec(context.Background(), `DELETE FROM checklists WHERE id = $1`, id)
	return err
}

// UltimaPosicao devolve a maior posição em uso no card, ou 0.
func (r *ChecklistPostgres) UltimaPosicao(cardID string) (float64, error) {
	var ultima float64
	err := r.pool.QueryRow(context.Background(),
		`SELECT COALESCE(MAX(posicao), 0) FROM checklists WHERE card_id = $1`, cardID,
	).Scan(&ultima)
	return ultima, err
}

// SalvarItem persiste uma linha nova.
func (r *ChecklistPostgres) SalvarItem(i *checklist.Item) error {
	_, err := r.pool.Exec(context.Background(),
		`INSERT INTO checklist_itens (id, checklist_id, texto, concluido, posicao, criado_em, atualizado_em)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		i.ID, i.ChecklistID, i.Texto, i.Concluido, i.Posicao, i.CriadoEm, i.AtualizadoEm,
	)
	return err
}

// AtualizarItem grava texto e marcação de uma linha existente.
func (r *ChecklistPostgres) AtualizarItem(i *checklist.Item) error {
	_, err := r.pool.Exec(context.Background(),
		`UPDATE checklist_itens SET texto = $2, concluido = $3, posicao = $4, atualizado_em = $5
		 WHERE id = $1`,
		i.ID, i.Texto, i.Concluido, i.Posicao, i.AtualizadoEm,
	)
	return err
}

// BuscarItem retorna (item, nil) quando encontra e (nil, nil) quando não existe.
func (r *ChecklistPostgres) BuscarItem(id string) (*checklist.Item, error) {
	var i checklist.Item
	err := r.pool.QueryRow(context.Background(),
		`SELECT id, checklist_id, texto, concluido, posicao, criado_em, atualizado_em
		 FROM checklist_itens WHERE id = $1`, id,
	).Scan(&i.ID, &i.ChecklistID, &i.Texto, &i.Concluido, &i.Posicao, &i.CriadoEm, &i.AtualizadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &i, nil
}

// ListarItens devolve as linhas da checklist em ordem de posição.
func (r *ChecklistPostgres) ListarItens(checklistID string) ([]checklist.Item, error) {
	linhas, err := r.pool.Query(context.Background(),
		`SELECT id, checklist_id, texto, concluido, posicao, criado_em, atualizado_em
		 FROM checklist_itens WHERE checklist_id = $1 ORDER BY posicao, id`, checklistID,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	itens := make([]checklist.Item, 0)
	for linhas.Next() {
		var i checklist.Item
		if err := linhas.Scan(&i.ID, &i.ChecklistID, &i.Texto, &i.Concluido, &i.Posicao, &i.CriadoEm, &i.AtualizadoEm); err != nil {
			return nil, err
		}
		itens = append(itens, i)
	}
	return itens, linhas.Err()
}

// ApagarItem remove a linha.
func (r *ChecklistPostgres) ApagarItem(id string) error {
	_, err := r.pool.Exec(context.Background(), `DELETE FROM checklist_itens WHERE id = $1`, id)
	return err
}

// UltimaPosicaoItem devolve a maior posição em uso na checklist, ou 0.
func (r *ChecklistPostgres) UltimaPosicaoItem(checklistID string) (float64, error) {
	var ultima float64
	err := r.pool.QueryRow(context.Background(),
		`SELECT COALESCE(MAX(posicao), 0) FROM checklist_itens WHERE checklist_id = $1`, checklistID,
	).Scan(&ultima)
	return ultima, err
}

// ProgressoDoBoard devolve, por card do quadro, quantos itens estão concluídos
// e quantos existem — numa consulta só, agregada no banco. Trazer todos os
// itens para contar no Go seria carregar o quadro inteiro só para desenhar
// "2/5".
func (r *ChecklistPostgres) ProgressoDoBoard(boardID string) (map[string]ucboard.Progresso, error) {
	linhas, err := r.pool.Query(context.Background(),
		`SELECT ch.card_id,
		        COUNT(*) FILTER (WHERE i.concluido) AS concluidos,
		        COUNT(*)                            AS total
		 FROM checklist_itens i
		 JOIN checklists ch ON ch.id = i.checklist_id
		 JOIN cards c       ON c.id = ch.card_id
		 JOIN colunas l     ON l.id = c.coluna_id
		 WHERE l.board_id = $1
		 GROUP BY ch.card_id`, boardID,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	porCard := make(map[string]ucboard.Progresso)
	for linhas.Next() {
		var cardID string
		var p ucboard.Progresso
		if err := linhas.Scan(&cardID, &p.Concluidos, &p.Total); err != nil {
			return nil, err
		}
		porCard[cardID] = p
	}
	return porCard, linhas.Err()
}
