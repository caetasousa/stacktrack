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
	db consultante
}

// NovoAnexoPostgres cria o repositório de anexos sobre o pool informado.
func NovoAnexoPostgres(pool *pgxpool.Pool) *AnexoPostgres {
	return &AnexoPostgres{db: pool}
}

const camposAnexo = `id, card_id, tipo, nome, url, caminho, tamanho, mime, criado_por, criado_em`

// CaminhosDeArquivoDoCard, ...DaColuna e ...DoBoard devolvem os arquivos em
// disco que serão órfãos quando aquilo for apagado.
//
// Existem porque o `ON DELETE CASCADE` limpa a TABELA e não o volume: apagar um
// card levava as linhas de `anexos` junto e deixava os arquivos no disco para
// sempre, sem nenhuma linha que os referenciasse. Quem chama coleta os caminhos
// ANTES do DELETE — depois dele não há mais de onde tirá-los.
//
// Só TipoArquivo: link não ocupa disco.
func (r *AnexoPostgres) CaminhosDeArquivoDoCard(ctx context.Context, cardID string) ([]string, error) {
	return r.caminhos(ctx,
		`SELECT caminho FROM anexos WHERE card_id = $1 AND tipo = 'arquivo' AND caminho IS NOT NULL`, cardID)
}

func (r *AnexoPostgres) CaminhosDeArquivoDaColuna(ctx context.Context, colunaID string) ([]string, error) {
	return r.caminhos(ctx,
		`SELECT a.caminho FROM anexos a
		 JOIN cards c ON c.id = a.card_id
		 WHERE c.coluna_id = $1 AND a.tipo = 'arquivo' AND a.caminho IS NOT NULL`, colunaID)
}

func (r *AnexoPostgres) CaminhosDeArquivoDoBoard(ctx context.Context, boardID string) ([]string, error) {
	return r.caminhos(ctx,
		`SELECT a.caminho FROM anexos a
		 JOIN cards c   ON c.id = a.card_id
		 JOIN colunas l ON l.id = c.coluna_id
		 WHERE l.board_id = $1 AND a.tipo = 'arquivo' AND a.caminho IS NOT NULL`, boardID)
}

func (r *AnexoPostgres) caminhos(ctx context.Context, sql string, arg any) ([]string, error) {
	linhas, err := r.db.Query(ctx, sql, arg)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	lista := make([]string, 0)
	for linhas.Next() {
		var caminho string
		if err := linhas.Scan(&caminho); err != nil {
			return nil, err
		}
		lista = append(lista, caminho)
	}
	return lista, linhas.Err()
}

// Salvar persiste um anexo novo.
func (r *AnexoPostgres) Salvar(ctx context.Context, a *anexo.Anexo) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO anexos (`+camposAnexo+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		a.ID, a.CardID, string(a.Tipo), a.Nome,
		vazioParaNulo(a.URL), vazioParaNulo(a.Caminho),
		zeroParaNulo(a.Tamanho), vazioParaNulo(a.MIME),
		a.CriadoPor, a.CriadoEm,
	)
	return err
}

// BuscarPorID retorna (anexo, nil) quando encontra e (nil, nil) quando não existe.
func (r *AnexoPostgres) BuscarPorID(ctx context.Context, id string) (*anexo.Anexo, error) {
	linha := r.db.QueryRow(ctx,
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
func (r *AnexoPostgres) ListarDoCard(ctx context.Context, cardID string) ([]anexo.Anexo, error) {
	linhas, err := r.db.Query(ctx,
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
func (r *AnexoPostgres) Apagar(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM anexos WHERE id = $1`, id)
	return err
}

// ContarPorCardDoBoard devolve quantos anexos cada card do quadro tem — numa
// consulta só, pelo mesmo motivo do progresso de checklist.
func (r *AnexoPostgres) ContarPorCardDoBoard(ctx context.Context, boardID string) (map[string]int, error) {
	linhas, err := r.db.Query(ctx,
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
