//go:build integracao

// Os papéis do banco: quem migra e quem serve.
//
// `deploy/postgres/papeis.sql` cria o papel da aplicação e lhe dá DML — e só
// DML. Este teste aplica o arquivo DE VERDADE, com o `psql` de dentro do
// container, e depois conecta COM AQUELE PAPEL para exercitar o que ele pode e
// o que não pode.
//
// Por que não basta ler o SQL: um GRANT esquecido não aparece em teste que só
// faz SELECT, e um REVOKE que não pegou não aparece em lugar nenhum até o dia
// em que uma falha de execução remota na API encontrar um `DROP TABLE` liberado.
// A asserção que importa aqui é a NEGATIVA.
package repository_test

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	donoDoBanco   = "teste" // o mesmo do postgres.WithUsername em TestMain
	bancoDeTeste  = "stacktrack_teste"
	papelDaApp    = "stacktrack_app_teste"
	senhaDaApp    = "senha-do-papel-de-teste"
	caminhoNoHost = "/tmp/papeis.sql"
)

// aplicarPapeis roda deploy/postgres/papeis.sql dentro do container, como o
// Ansible faz no servidor. Devolve a saída do psql.
func aplicarPapeis(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	origem, err := filepath.Abs(filepath.Join("..", "..", "..", "deploy", "postgres", "papeis.sql"))
	if err != nil {
		t.Fatalf("caminho do papeis.sql: %v", err)
	}
	if err := containerDoBanco.CopyFileToContainer(ctx, origem, caminhoNoHost, 0o644); err != nil {
		t.Fatalf("copiar papeis.sql para o container: %v", err)
	}

	codigo, saida, err := containerDoBanco.Exec(ctx, []string{
		"psql", "-v", "ON_ERROR_STOP=1",
		"-U", donoDoBanco, "-d", bancoDeTeste,
		"-v", "dono=" + donoDoBanco,
		"-v", "db=" + bancoDeTeste,
		"-v", "app_user=" + papelDaApp,
		"-v", "app_password=" + senhaDaApp,
		"-f", caminhoNoHost,
	})
	if err != nil {
		t.Fatalf("executar psql: %v", err)
	}
	texto := ""
	if saida != nil {
		bytes, _ := io.ReadAll(saida)
		texto = string(bytes)
	}
	if codigo != 0 {
		t.Fatalf("papeis.sql saiu %d:\n%s", codigo, texto)
	}
	return texto
}

// poolDaAplicacao conecta com o papel da APLICAÇÃO, não com o dono.
func poolDaAplicacao(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	// A DSN do container aponta para o dono; troca-se só a credencial, porque o
	// host e a porta mapeada são os mesmos.
	cfg, err := pgxpool.ParseConfig(urlDoBanco)
	if err != nil {
		t.Fatalf("parse da DSN: %v", err)
	}
	cfg.ConnConfig.User = papelDaApp
	cfg.ConnConfig.Password = senhaDaApp

	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pool da aplicação: %v", err)
	}
	if err := p.Ping(ctx); err != nil {
		p.Close()
		t.Fatalf("a aplicação não conseguiu conectar com o próprio papel: %v", err)
	}
	t.Cleanup(p.Close)
	return p
}

// O papel da aplicação faz tudo o que a aplicação faz — e nada além.
func TestPapelDaAplicacaoTemDMLENaoTemDDL(t *testing.T) {
	ctx := context.Background()
	aplicarPapeis(t)
	app := poolDaAplicacao(t)

	// --- o que ele PRECISA poder ---------------------------------------------

	id := uuid.NewString()
	if _, err := app.Exec(ctx,
		`INSERT INTO usuarios (id, nome, email, senha_hash, criado_em, atualizado_em)
		 VALUES ($1, $2, $3, $4, now(), now())`,
		id, "Papel", "papel-"+id+"@exemplo.com", "hash"); err != nil {
		t.Fatalf("INSERT pelo papel da aplicação: %v", err)
	}
	if _, err := app.Exec(ctx, `UPDATE usuarios SET nome = $2 WHERE id = $1`, id, "Papel II"); err != nil {
		t.Fatalf("UPDATE pelo papel da aplicação: %v", err)
	}
	var nome string
	if err := app.QueryRow(ctx, `SELECT nome FROM usuarios WHERE id = $1`, id).Scan(&nome); err != nil {
		t.Fatalf("SELECT pelo papel da aplicação: %v", err)
	}
	if nome != "Papel II" {
		t.Errorf("nome = %q, esperado %q", nome, "Papel II")
	}
	if _, err := app.Exec(ctx, `DELETE FROM usuarios WHERE id = $1`, id); err != nil {
		t.Fatalf("DELETE pelo papel da aplicação: %v", err)
	}

	// A sequência do BIGSERIAL é permissão separada da tabela, e é a que falta
	// primeiro quando alguém concede só o INSERT.
	var seq int64
	if err := app.QueryRow(ctx,
		`INSERT INTO board_events (board_id, tipo, payload, criado_em)
		 VALUES ($1, 'teste.papel', '{}'::jsonb, now()) RETURNING seq`,
		boardParaEvento(t)).Scan(&seq); err != nil {
		t.Fatalf("INSERT com sequência (BIGSERIAL) pelo papel da aplicação: %v", err)
	}

	// --- o que ele NÃO pode --------------------------------------------------

	proibidos := map[string]string{
		"CREATE TABLE":     `CREATE TABLE invasao (id int)`,
		"ALTER TABLE":      `ALTER TABLE cards ADD COLUMN invasao text`,
		"DROP TABLE":       `DROP TABLE cards`,
		"TRUNCATE":         `TRUNCATE cards`,
		"CREATE INDEX":     `CREATE INDEX invasao_idx ON cards (id)`,
		"CREATE EXTENSION": `CREATE EXTENSION IF NOT EXISTS pgcrypto`,
		"CREATE ROLE":      `CREATE ROLE invasor LOGIN`,
		"ALTER ROLE":       `ALTER ROLE ` + donoDoBanco + ` WITH PASSWORD 'trocada'`,
		"CREATE SCHEMA":    `CREATE SCHEMA invasao`,
	}
	for nome, sql := range proibidos {
		if _, err := app.Exec(ctx, sql); err == nil {
			t.Errorf("%s FUNCIONOU com o papel da aplicação — ele não devia ter DDL", nome)
		} else if !ehErroDePermissao(err) {
			// Um erro qualquer não serve: "tabela não existe" passaria por
			// recusa sem que privilégio nenhum tivesse sido negado.
			t.Errorf("%s falhou por outro motivo que não permissão: %v", nome, err)
		}
	}
}

// A tabela que a PRÓXIMA migration criar já nasce visível para a aplicação.
//
// É o furo que o ALTER DEFAULT PRIVILEGES fecha, e o único cuja ausência não
// aparece no dia em que se aplica: aparece no primeiro INSERT depois do deploy
// seguinte, que é quando ninguém está olhando para permissão de banco.
func TestTabelaCriadaDepoisJaNasceAcessivelParaAAplicacao(t *testing.T) {
	ctx := context.Background()
	aplicarPapeis(t)

	// Criada pelo DONO, que é quem o Flyway usa.
	if _, err := pool.Exec(ctx, `CREATE TABLE migration_futura (id BIGSERIAL PRIMARY KEY, texto TEXT NOT NULL)`); err != nil {
		t.Fatalf("criar a tabela como dono: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TABLE IF EXISTS migration_futura`)
	})

	app := poolDaAplicacao(t)
	var id int64
	if err := app.QueryRow(ctx,
		`INSERT INTO migration_futura (texto) VALUES ('depois') RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("a aplicação não enxergou a tabela nova: %v\n"+
			"É o ALTER DEFAULT PRIVILEGES de deploy/postgres/papeis.sql que garante isto.", err)
	}
}

// Rodar duas vezes é seguro — e precisa ser, porque roda a cada
// provisionamento. A segunda passada não pode falhar nem mudar o resultado.
func TestPapeisSaoIdempotentes(t *testing.T) {
	primeira := aplicarPapeis(t)
	segunda := aplicarPapeis(t)

	if !strings.Contains(segunda, papelDaApp) {
		t.Fatalf("a conferência do papeis.sql não saiu na segunda passada:\n%s", segunda)
	}
	// A conferência final imprime o estado; ele tem de ser o mesmo das duas
	// vezes, senão a segunda execução mudou alguma coisa.
	if conferencia(primeira) != conferencia(segunda) {
		t.Errorf("o estado mudou entre as duas execuções:\n--- 1 ---\n%s\n--- 2 ---\n%s",
			conferencia(primeira), conferencia(segunda))
	}
	// E o que o SQL afirma tem de bater com a intenção: sem CREATE no schema.
	if !strings.Contains(conferencia(primeira), "| f") {
		t.Errorf("a conferência não mostra nenhum privilégio negado — cria_tabela devia ser f:\n%s",
			conferencia(primeira))
	}
}

// conferencia isola o SELECT final do papeis.sql, que é a única parte da saída
// que descreve estado — o resto são ecos de comando.
func conferencia(saida string) string {
	linhas := strings.Split(saida, "\n")
	for i, linha := range linhas {
		if strings.Contains(linha, "papel") && strings.Contains(linha, "conecta") {
			fim := i + 3
			if fim > len(linhas) {
				fim = len(linhas)
			}
			return strings.Join(linhas[i:fim], "\n")
		}
	}
	return saida
}

// ehErroDePermissao distingue "o banco recusou por privilégio" de "o comando
// nem chegou lá".
func ehErroDePermissao(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// 42501 insufficient_privilege · 42P01 quando o objeto está fora de
		// alcance do papel.
		return pgErr.Code == "42501"
	}
	return strings.Contains(strings.ToLower(err.Error()), "permission denied")
}

// boardParaEvento cria um quadro mínimo pelo DONO, para o INSERT em
// board_events ter a que se referir.
func boardParaEvento(t *testing.T) string {
	t.Helper()
	_, colunaID, _ := cenario(t)
	var boardID string
	if err := pool.QueryRow(context.Background(),
		`SELECT board_id FROM colunas WHERE id = $1`, colunaID).Scan(&boardID); err != nil {
		t.Fatalf("board do cenário: %v", err)
	}
	return boardID
}
