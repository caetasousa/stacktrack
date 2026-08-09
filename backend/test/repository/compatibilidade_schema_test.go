//go:build integracao

// O guard de expand/contract, que transforma a regra do CLAUDE.md em falha de
// build.
//
// A regra: SET NOT NULL, DROP COLUMN, RENAME e UNIQUE novo quebram a versão
// ANTERIOR da aplicação — que continua no ar durante o deploy e é para onde um
// rollback volta. Como o Flyway é forward-only, o banco não volta junto com a
// imagem: o rollback sobe sem erro e quebra no primeiro INSERT.
//
// Este teste aplica as migrations até a penúltima, fotografa o schema, aplica a
// última e compara. Coluna nova obrigatória, coluna removida e coluna apertada
// param o build com o nome do que quebrou.
//
// O CONTRACT DELIBERADO é declarado no próprio arquivo, numa linha:
//
//	-- CONTRACT: cards.posicao, colunas.posicao
//
// Sem essa saída o guard tornaria o ciclo expand/contract IMPOSSÍVEL de fechar —
// ele reprovaria justamente o segundo passo, que é o objetivo do primeiro. Mas
// a declaração não desliga a checagem: ela lista o que pode quebrar, e tudo o
// que estiver fora da lista continua reprovando. É a diferença entre autorizar
// uma mudança e abrir uma exceção geral.
package repository_test

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// colunaDoSchema é o que interessa comparar entre dois momentos do banco.
type colunaDoSchema struct {
	Tabela   string
	Nome     string
	Tipo     string
	Anulavel bool
}

func (c colunaDoSchema) String() string {
	return fmt.Sprintf("%s.%s (%s)", c.Tabela, c.Nome, c.Tipo)
}

func TestUltimaMigrationNaoQuebraAVersaoAnterior(t *testing.T) {
	caminhos, err := filepath.Glob(filepath.Join("..", "..", "migrations", "V*.sql"))
	if err != nil {
		t.Fatalf("listar migrations: %v", err)
	}
	if len(caminhos) < 2 {
		t.Skip("são precisas ao menos duas migrations para comparar")
	}
	ordenarPorVersao(caminhos)
	anteriores, ultima := caminhos[:len(caminhos)-1], caminhos[len(caminhos)-1]

	ctx := context.Background()
	poolIsolado, encerrar := bancoDescartavel(t, ctx)
	defer encerrar()

	if err := aplicarNoPool(ctx, poolIsolado, anteriores); err != nil {
		t.Fatalf("aplicar migrations anteriores: %v", err)
	}
	antes := fotografarSchema(t, ctx, poolIsolado)
	tabelasAntigas := tabelasDe(antes)

	if err := aplicarNoPool(ctx, poolIsolado, []string{ultima}); err != nil {
		t.Fatalf("aplicar %s: %v", filepath.Base(ultima), err)
	}
	depois := fotografarSchema(t, ctx, poolIsolado)

	nome := filepath.Base(ultima)
	autorizadas, err := contractDeclarado(ultima)
	if err != nil {
		t.Fatalf("ler %s: %v", nome, err)
	}
	if len(autorizadas) > 0 {
		t.Logf("%s declara contract sobre: %v", nome, chavesOrdenadas(autorizadas))
	}
	usadas := make(map[string]bool, len(autorizadas))

	for chave, coluna := range antes {
		nova, continua := depois[chave]
		if !continua {
			if autorizadas[chave] {
				usadas[chave] = true
				continue
			}
			t.Errorf(
				"%s REMOVE a coluna %s.\n"+
					"A versão anterior da aplicação ainda a escreve, e é para onde um rollback volta.\n"+
					"Faça em dois deploys: primeiro o código para de usá-la, depois ela sai.",
				nome, coluna)
			continue
		}
		if coluna.Anulavel && !nova.Anulavel {
			if autorizadas[chave] {
				usadas[chave] = true
				continue
			}
			t.Errorf(
				"%s APERTA a coluna %s para NOT NULL.\n"+
					"A versão anterior insere linhas sem ela e passaria a falhar no primeiro INSERT.\n"+
					"Faça em dois deploys: primeiro o código passa a preenchê-la sempre, depois o SET NOT NULL.",
				nome, coluna)
		}
	}

	for chave, coluna := range depois {
		if _, existia := antes[chave]; existia {
			continue
		}
		// Tabela NOVA pode nascer com colunas obrigatórias à vontade: a versão
		// anterior da aplicação não a conhece e nunca insere nela. O problema é
		// coluna obrigatória numa tabela que JÁ existia — aí o INSERT antigo,
		// que não a preenche, passa a falhar.
		if _, jaExistia := tabelasAntigas[coluna.Tabela]; !jaExistia {
			continue
		}
		if !coluna.Anulavel {
			t.Errorf(
				"%s cria a coluna %s já OBRIGATÓRIA.\n"+
					"A versão anterior não a preenche, e todo INSERT dela falharia durante o deploy.\n"+
					"Faça em dois deploys: primeiro anulável, depois o SET NOT NULL.",
				nome, coluna)
		}
	}

	// Declarar o que não aconteceu é sinal de que a migration mudou e a
	// declaração ficou para trás — e uma lista desatualizada autoriza em
	// silêncio a próxima quebra que cair no mesmo nome.
	for chave := range autorizadas {
		if !usadas[chave] {
			t.Errorf(
				"%s declara contract sobre %s, mas nada mudou nessa coluna.\n"+
					"Uma declaração que sobra vira autorização esquecida: remova-a.",
				nome, chave)
		}
	}
}

// contractDeclarado lê as colunas que a migration declara que vai quebrar.
//
// O formato é uma linha de comentário SQL:
//
//	-- CONTRACT: tabela.coluna, outra.coluna
func contractDeclarado(caminho string) (map[string]bool, error) {
	conteudo, err := os.ReadFile(caminho)
	if err != nil {
		return nil, err
	}
	autorizadas := map[string]bool{}
	for _, linha := range strings.Split(string(conteudo), "\n") {
		linha = strings.TrimSpace(linha)
		if !strings.HasPrefix(linha, "-- CONTRACT:") {
			continue
		}
		for _, item := range strings.Split(strings.TrimPrefix(linha, "-- CONTRACT:"), ",") {
			if item = strings.TrimSpace(item); item != "" {
				autorizadas[item] = true
			}
		}
	}
	return autorizadas, nil
}

func chavesOrdenadas(m map[string]bool) []string {
	fora := make([]string, 0, len(m))
	for k := range m {
		fora = append(fora, k)
	}
	sort.Strings(fora)
	return fora
}

// tabelasDe extrai o conjunto de tabelas de uma fotografia do schema.
func tabelasDe(schema map[string]colunaDoSchema) map[string]struct{} {
	tabelas := make(map[string]struct{})
	for _, c := range schema {
		tabelas[c.Tabela] = struct{}{}
	}
	return tabelas
}

// fotografarSchema lê as colunas das tabelas da aplicação.
//
// A tabela de controle do Flyway fica de fora: ela é da ferramenta, muda
// sozinha e não faz parte do contrato entre as versões da aplicação.
func fotografarSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) map[string]colunaDoSchema {
	t.Helper()
	linhas, err := pool.Query(ctx,
		`SELECT table_name, column_name, data_type, is_nullable
		   FROM information_schema.columns
		  WHERE table_schema = 'public' AND table_name <> 'flyway_schema_history'`)
	if err != nil {
		t.Fatalf("ler schema: %v", err)
	}
	defer linhas.Close()

	schema := make(map[string]colunaDoSchema)
	for linhas.Next() {
		var c colunaDoSchema
		var anulavel string
		if err := linhas.Scan(&c.Tabela, &c.Nome, &c.Tipo, &anulavel); err != nil {
			t.Fatalf("ler coluna: %v", err)
		}
		c.Anulavel = strings.EqualFold(anulavel, "YES")
		schema[c.Tabela+"."+c.Nome] = c
	}
	if err := linhas.Err(); err != nil {
		t.Fatalf("ler schema: %v", err)
	}
	return schema
}

// bancoDescartavel sobe um Postgres só para esta comparação.
//
// Não dá para reusar o pool do TestMain: ele já tem TODAS as migrations
// aplicadas, e este teste precisa parar na penúltima para ver o antes.
func bancoDescartavel(t *testing.T, ctx context.Context) (*pgxpool.Pool, func()) {
	t.Helper()
	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("compat_"+uuid.NewString()[:8]),
		postgres.WithUsername("teste"),
		postgres.WithPassword("teste"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("subir Postgres: %v", err)
	}
	url, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("string de conexão: %v", err)
	}
	p, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	return p, func() {
		p.Close()
		_ = container.Terminate(ctx)
	}
}

func aplicarNoPool(ctx context.Context, pool *pgxpool.Pool, caminhos []string) error {
	for _, caminho := range caminhos {
		sql, err := os.ReadFile(caminho)
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(caminho), err)
		}
	}
	return nil
}
