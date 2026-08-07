package repository

import (
	"context"
	"errors"

	"stacktrack/internal/domain/anexo"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AnexoPostgres persiste os anexos dos cards. O conteúdo dos arquivos não fica
// aqui — só o registro; o binário vive no armazém.
type AnexoPostgres struct {
	pool *pgxpool.Pool
}

// NovoAnexoPostgres cria o repositório de anexos sobre o pool informado.
func NovoAnexoPostgres(pool *pgxpool.Pool) *AnexoPostgres {
	return &AnexoPostgres{pool: pool}
}

const camposAnexo = `id, card_id, tipo, nome, url, caminho, tamanho, mime, criado_por, criado_em`

// Salvar persiste um anexo novo.
func (r *AnexoPostgres) Salvar(a *anexo.Anexo) error {
	_, err := r.pool.Exec(context.Background(),
		`INSERT INTO anexos (`+camposAnexo+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		a.ID, a.CardID, string(a.Tipo), a.Nome,
		vazioParaNulo(a.URL), vazioParaNulo(a.Caminho),
		zeroParaNulo(a.Tamanho), vazioParaNulo(a.MIME),
		a.CriadoPor, a.CriadoEm,
	)
	return err
}

// BuscarPorID retorna (anexo, nil) quando encontra e (nil, nil) quando não existe.
func (r *AnexoPostgres) BuscarPorID(id string) (*anexo.Anexo, error) {
	linha := r.pool.QueryRow(context.Background(),
		`SELECT `+camposAnexo+` FROM anexos WHERE id = $1`, id)

	a, err := lerAnexo(linha)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

// ListarDoCard devolve os anexos do card, do mais recente para o mais antigo.
func (r *AnexoPostgres) ListarDoCard(cardID string) ([]anexo.Anexo, error) {
	linhas, err := r.pool.Query(context.Background(),
		`SELECT `+camposAnexo+` FROM anexos WHERE card_id = $1 ORDER BY criado_em DESC`, cardID)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	anexos := make([]anexo.Anexo, 0)
	for linhas.Next() {
		a, err := lerAnexo(linhas)
		if err != nil {
			return nil, err
		}
		anexos = append(anexos, *a)
	}
	return anexos, linhas.Err()
}

// Apagar remove o registro do anexo. O arquivo no armazém é apagado pelo
// usecase, que é quem sabe se havia arquivo.
func (r *AnexoPostgres) Apagar(id string) error {
	_, err := r.pool.Exec(context.Background(), `DELETE FROM anexos WHERE id = $1`, id)
	return err
}

// ContarPorCardDoBoard devolve quantos anexos cada card do quadro tem — numa
// consulta só, pelo mesmo motivo do progresso de checklist.
func (r *AnexoPostgres) ContarPorCardDoBoard(boardID string) (map[string]int, error) {
	linhas, err := r.pool.Query(context.Background(),
		`SELECT a.card_id, COUNT(*)
		 FROM anexos a
		 JOIN cards c   ON c.id = a.card_id
		 JOIN colunas l ON l.id = c.coluna_id
		 WHERE l.board_id = $1
		 GROUP BY a.card_id`, boardID,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	porCard := make(map[string]int)
	for linhas.Next() {
		var cardID string
		var total int
		if err := linhas.Scan(&cardID, &total); err != nil {
			return nil, err
		}
		porCard[cardID] = total
	}
	return porCard, linhas.Err()
}

// linhaLegivel é o que QueryRow e Rows têm em comum para o Scan.
type linhaLegivel interface {
	Scan(dest ...any) error
}

func lerAnexo(linha linhaLegivel) (*anexo.Anexo, error) {
	var a anexo.Anexo
	var tipo string
	var url, caminho, mime *string
	var tamanho *int64

	if err := linha.Scan(&a.ID, &a.CardID, &tipo, &a.Nome, &url, &caminho, &tamanho, &mime,
		&a.CriadoPor, &a.CriadoEm); err != nil {
		return nil, err
	}

	a.Tipo = anexo.Tipo(tipo)
	a.URL = valorOuVazio(url)
	a.Caminho = valorOuVazio(caminho)
	a.MIME = valorOuVazio(mime)
	if tamanho != nil {
		a.Tamanho = *tamanho
	}
	return &a, nil
}

// vazioParaNulo grava NULL em vez de string vazia: a coluna só se aplica a um
// dos dois tipos de anexo, e ” fingiria que ela foi preenchida.
func vazioParaNulo(valor string) *string {
	if valor == "" {
		return nil
	}
	return &valor
}

func zeroParaNulo(valor int64) *int64 {
	if valor == 0 {
		return nil
	}
	return &valor
}

func valorOuVazio(valor *string) string {
	if valor == nil {
		return ""
	}
	return *valor
}
