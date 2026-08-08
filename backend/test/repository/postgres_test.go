//go:build integracao

// Testes de repositório contra um PostgreSQL DE VERDADE, subido pelo
// Testcontainers e derrubado no fim.
//
// Existem por uma razão específica e documentada: os fakes em memória copiam a
// struct inteira, então um campo que o SQL não grava passa por eles sem
// reclamar. Aconteceu duas vezes — cards.prazo e boards.fundo existiam em todas
// as camadas, a API respondia 200, e o dado sumia. Só uma escrita seguida de
// leitura no banco real pega isso.
//
//	make -C backend test-integracao
//
// Ficam atrás de build tag porque exigem Docker: sem a tag, o `go test ./...`
// de quem não o tem passaria a falhar.
package repository_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// pool é compartilhado por todos os testes do pacote: subir um Postgres por
// caso custaria dezenas de segundos e não provaria nada a mais. O isolamento
// entre casos vem dos ids, que são únicos.
var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("stacktrack_teste"),
		postgres.WithUsername("teste"),
		postgres.WithPassword("teste"),
		testcontainers.WithWaitStrategy(
			// Duas ocorrências: o entrypoint do Postgres sobe o servidor uma
			// primeira vez para rodar os scripts de inicialização e o derruba.
			// Esperar a primeira daria um banco que fecha na cara do teste.
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		panic("não foi possível subir o Postgres de teste: " + err.Error())
	}

	url, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(err)
	}
	pool, err = pgxpool.New(ctx, url)
	if err != nil {
		panic(err)
	}

	if err := aplicarMigrations(ctx); err != nil {
		panic("migrations: " + err.Error())
	}

	codigo := m.Run()

	pool.Close()
	// testcontainers.CleanupContainer também roda por Ryuk se o processo
	// morrer no meio; este Terminate é o caminho limpo.
	_ = container.Terminate(ctx)
	os.Exit(codigo)
}

// aplicarMigrations roda os .sql na ordem, direto — sem Flyway.
//
// O Flyway é a ferramenta de produção; aqui ele acrescentaria um container e
// uma tabela de controle sem cobrir nada que interesse ao teste. O que importa
// é o SCHEMA ser o de verdade, e ele é: são os mesmos arquivos.
func aplicarMigrations(ctx context.Context) error {
	caminhos, err := filepath.Glob(filepath.Join("..", "..", "migrations", "V*.sql"))
	if err != nil {
		return err
	}
	ordenarPorVersao(caminhos)

	for _, caminho := range caminhos {
		sql, err := os.ReadFile(caminho)
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return err
		}
	}
	return nil
}
