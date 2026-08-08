package repository

import (
	"context"
	"errors"

	"stacktrack/internal/domain/card"
	"stacktrack/internal/domain/cor"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CardPostgres persiste cards no PostgreSQL.
type CardPostgres struct {
	db consultante
}

// NovoCardPostgres cria o repositório de cards sobre o pool informado.
func NovoCardPostgres(pool *pgxpool.Pool) *CardPostgres {
	return &CardPostgres{db: pool}
}

// Salvar persiste um card novo.
func (r *CardPostgres) Salvar(ctx context.Context, c *card.Card) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO cards (id, coluna_id, titulo, descricao, cor, posicao, version, prazo, criado_em, atualizado_em)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		c.ID, c.ColunaID, c.Titulo, c.Descricao, vazioParaNulo(string(c.Cor)),
		c.Posicao, c.Version, c.Prazo, c.CriadoEm, c.AtualizadoEm,
	)
	return err
}

// Atualizar grava as alterações de um card existente, com BLOQUEIO OTIMISTA.
//
// O `AND version = $7 - 1` é o coração disto: o domínio já incrementou a versão
// em memória, então $7 é a nova e $7-1 é a que estava valendo quando o card foi
// lido. Se outra pessoa gravou nesse meio-tempo, a linha no banco já está numa
// versão diferente, a condição não casa e NENHUMA linha é afetada.
//
// Sem esta cláusula, duas pessoas editando o mesmo card ao mesmo tempo
// terminam com o texto de quem gravou por último, e o trabalho da outra some
// sem aviso — o "lost update" clássico. READ COMMITTED, que é o isolamento
// padrão do Postgres, não protege disso: os dois UPDATEs são válidos
// isoladamente.
//
// Zero linhas também acontece se o card foi APAGADO no meio do caminho. Os dois
// casos viram o mesmo erro de propósito: para quem escreveu, a diferença não
// muda o que fazer — recarregar e decidir de novo.
func (r *CardPostgres) Atualizar(ctx context.Context, c *card.Card) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE cards SET coluna_id = $2, titulo = $3, descricao = $4, cor = $5, posicao = $6,
		        version = $7, prazo = $8, atualizado_em = $9
		 WHERE id = $1 AND version = $7 - 1`,
		c.ID, c.ColunaID, c.Titulo, c.Descricao, vazioParaNulo(string(c.Cor)),
		c.Posicao, c.Version, c.Prazo, c.AtualizadoEm,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return card.ErrConflito
	}
	return nil
}

// BuscarPorID retorna (card, nil) quando encontra e (nil, nil) quando não existe.
func (r *CardPostgres) BuscarPorID(ctx context.Context, id string) (*card.Card, error) {
	var c card.Card
	var corLida *string
	err := r.db.QueryRow(ctx,
		`SELECT id, coluna_id, titulo, descricao, cor, posicao, version, prazo, criado_em, atualizado_em
		 FROM cards WHERE id = $1`, id,
	).Scan(&c.ID, &c.ColunaID, &c.Titulo, &c.Descricao, &corLida, &c.Posicao, &c.Version, &c.Prazo, &c.CriadoEm, &c.AtualizadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Cor = cor.Cor(valorOuVazio(corLida))
	return &c, nil
}

// ListarDoBoard devolve todos os cards do quadro em ordem de posição, numa
// consulta só — uma por coluna seria um N+1 que piora conforme o quadro cresce.
func (r *CardPostgres) ListarDoBoard(ctx context.Context, boardID string) ([]card.Card, error) {
	linhas, err := r.db.Query(ctx,
		`SELECT c.id, c.coluna_id, c.titulo, c.descricao, c.cor, c.posicao, c.version, c.prazo, c.criado_em, c.atualizado_em
		 FROM cards c
		 JOIN colunas col ON col.id = c.coluna_id
		 WHERE col.board_id = $1
		 ORDER BY c.posicao, c.id`, boardID,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	cards := make([]card.Card, 0)
	for linhas.Next() {
		var c card.Card
		var corLida *string
		if err := linhas.Scan(&c.ID, &c.ColunaID, &c.Titulo, &c.Descricao, &corLida, &c.Posicao, &c.Version, &c.Prazo, &c.CriadoEm, &c.AtualizadoEm); err != nil {
			return nil, err
		}
		c.Cor = cor.Cor(valorOuVazio(corLida))
		cards = append(cards, c)
	}
	return cards, linhas.Err()
}

// Apagar remove o card.
func (r *CardPostgres) Apagar(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM cards WHERE id = $1`, id)
	return err
}

// UltimaPosicao devolve a maior posição em uso na coluna, ou 0 quando ela está
// vazia.
func (r *CardPostgres) UltimaPosicao(ctx context.Context, colunaID string) (float64, error) {
	var ultima float64
	err := r.db.QueryRow(ctx,
		`SELECT COALESCE(MAX(posicao), 0) FROM cards WHERE coluna_id = $1`, colunaID,
	).Scan(&ultima)
	return ultima, err
}
