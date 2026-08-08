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
	db consultante
}

// NovoChecklistPostgres cria o repositório de checklists sobre o pool informado.
func NovoChecklistPostgres(pool *pgxpool.Pool) *ChecklistPostgres {
	return &ChecklistPostgres{db: pool}
}

// Salvar persiste uma checklist nova.
func (r *ChecklistPostgres) Salvar(ctx context.Context, c *checklist.Checklist) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO checklists (id, card_id, titulo, posicao, criado_em, atualizado_em)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		c.ID, c.CardID, c.Titulo, c.Posicao, c.CriadoEm, c.AtualizadoEm,
	)
	return err
}

// Atualizar grava as alterações de uma checklist existente.
func (r *ChecklistPostgres) Atualizar(ctx context.Context, c *checklist.Checklist) error {
	_, err := r.db.Exec(ctx,
		`UPDATE checklists SET titulo = $2, posicao = $3, atualizado_em = $4 WHERE id = $1`,
		c.ID, c.Titulo, c.Posicao, c.AtualizadoEm,
	)
	return err
}

// BuscarPorID retorna (checklist, nil) quando encontra e (nil, nil) quando não existe.
func (r *ChecklistPostgres) BuscarPorID(ctx context.Context, id string) (*checklist.Checklist, error) {
	var c checklist.Checklist
	err := r.db.QueryRow(ctx,
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
func (r *ChecklistPostgres) ListarDoCard(ctx context.Context, cardID string) ([]checklist.Checklist, error) {
	linhas, err := r.db.Query(ctx,
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
func (r *ChecklistPostgres) Apagar(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM checklists WHERE id = $1`, id)
	return err
}

// UltimaPosicao devolve a maior posição em uso no card, ou 0.
func (r *ChecklistPostgres) UltimaPosicao(ctx context.Context, cardID string) (float64, error) {
	var ultima float64
	err := r.db.QueryRow(ctx,
		`SELECT COALESCE(MAX(posicao), 0) FROM checklists WHERE card_id = $1`, cardID,
	).Scan(&ultima)
	return ultima, err
}

// SalvarItem persiste uma linha nova.
func (r *ChecklistPostgres) SalvarItem(ctx context.Context, i *checklist.Item) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO checklist_itens (id, checklist_id, texto, concluido, posicao, criado_em, atualizado_em)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		i.ID, i.ChecklistID, i.Texto, i.Concluido, i.Posicao, i.CriadoEm, i.AtualizadoEm,
	)
	return err
}

// AtualizarItem grava texto e marcação de uma linha existente.
func (r *ChecklistPostgres) AtualizarItem(ctx context.Context, i *checklist.Item) error {
	_, err := r.db.Exec(ctx,
		`UPDATE checklist_itens SET texto = $2, concluido = $3, posicao = $4, atualizado_em = $5
		 WHERE id = $1`,
		i.ID, i.Texto, i.Concluido, i.Posicao, i.AtualizadoEm,
	)
	return err
}

// BuscarItem retorna (item, nil) quando encontra e (nil, nil) quando não existe.
func (r *ChecklistPostgres) BuscarItem(ctx context.Context, id string) (*checklist.Item, error) {
	var i checklist.Item
	err := r.db.QueryRow(ctx,
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
func (r *ChecklistPostgres) ListarItens(ctx context.Context, checklistID string) ([]checklist.Item, error) {
	linhas, err := r.db.Query(ctx,
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
func (r *ChecklistPostgres) ApagarItem(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM checklist_itens WHERE id = $1`, id)
	return err
}

// UltimaPosicaoItem devolve a maior posição em uso na checklist, ou 0.
func (r *ChecklistPostgres) UltimaPosicaoItem(ctx context.Context, checklistID string) (float64, error) {
	var ultima float64
	err := r.db.QueryRow(ctx,
		`SELECT COALESCE(MAX(posicao), 0) FROM checklist_itens WHERE checklist_id = $1`, checklistID,
	).Scan(&ultima)
	return ultima, err
}

// ProgressoDoBoard devolve, por card do quadro, quantos itens estão concluídos
// e quantos existem — numa consulta só, agregada no banco. Trazer todos os
// itens para contar no Go seria carregar o quadro inteiro só para desenhar
// "2/5".
func (r *ChecklistPostgres) ProgressoDoBoard(ctx context.Context, boardID string) (map[string]ucboard.Progresso, error) {
	linhas, err := r.db.Query(ctx,
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
