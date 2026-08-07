// Guarda contra a falha que já aconteceu duas vezes: a coluna entra na
// migration, no domínio, no DTO e no handler — e ninguém a escreve no SQL. A
// API responde 200, o dado não persiste, e nenhum teste reclama, porque os
// fakes em memória copiam a struct inteira e nunca passam pelo SQL de verdade.
//
// Foi assim com `cards.prazo` e com `boards.fundo`. O teste é grosseiro de
// propósito: só exige que cada coluna criada nas migrations apareça em algum
// lugar do SQL do repositório. Não sabe dizer se ela está no INSERT mas falta
// no UPDATE — para isso são precisos os testes contra banco de verdade, que
// chegam na fase 8 com Testcontainers.
package repository_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	criaTabela  = regexp.MustCompile(`(?is)CREATE TABLE(?: IF NOT EXISTS)?\s+(\w+)\s*\((.*?)\n\)`)
	adicionaCol = regexp.MustCompile(`(?i)ALTER TABLE\s+(\w+)\s+ADD COLUMN\s+(?:IF NOT EXISTS\s+)?(\w+)`)
	comentario  = regexp.MustCompile(`--[^\n]*`)
)

// naoSaoColunas são as cláusulas que aparecem no corpo do CREATE TABLE sem
// nomear uma coluna.
var naoSaoColunas = []string{"PRIMARY KEY", "FOREIGN KEY", "UNIQUE", "CONSTRAINT", "CHECK"}

func TestTodaColunaDasMigrationsApareceNoSQLDoRepositorio(t *testing.T) {
	tabelas := colunasDasMigrations(t)
	if len(tabelas) == 0 {
		t.Fatal("nenhuma tabela encontrada nas migrations — o caminho deve estar errado")
	}
	sql := fonteDosRepositorios(t)

	for tabela, colunas := range tabelas {
		for _, coluna := range colunas {
			if !mencionada(sql, coluna) {
				t.Errorf("a coluna %s.%s existe no banco mas não aparece em nenhum SQL de "+
					"internal/adapter/repository: o dado nunca é gravado nem lido", tabela, coluna)
			}
		}
	}
}

// colunasDasMigrations devolve, por tabela, as colunas que as migrations criam.
func colunasDasMigrations(t *testing.T) map[string][]string {
	t.Helper()

	arquivos, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.sql"))
	if err != nil {
		t.Fatalf("erro ao listar as migrations: %v", err)
	}

	tabelas := make(map[string][]string)
	for _, arquivo := range arquivos {
		conteudo, err := os.ReadFile(arquivo)
		if err != nil {
			t.Fatalf("erro ao ler %s: %v", arquivo, err)
		}
		// Os comentários saem antes: "-- cria a tabela" viraria uma coluna "--".
		sql := comentario.ReplaceAllString(string(conteudo), "")

		for _, achado := range criaTabela.FindAllStringSubmatch(sql, -1) {
			tabela := achado[1]
			for _, linha := range strings.Split(achado[2], "\n") {
				if coluna, ok := nomeDaColuna(linha); ok {
					tabelas[tabela] = append(tabelas[tabela], coluna)
				}
			}
		}
		for _, achado := range adicionaCol.FindAllStringSubmatch(sql, -1) {
			tabelas[achado[1]] = append(tabelas[achado[1]], achado[2])
		}
	}
	return tabelas
}

// nomeDaColuna extrai o nome da coluna de uma linha do corpo de um CREATE
// TABLE, ou informa que aquela linha não declara coluna nenhuma.
func nomeDaColuna(linha string) (string, bool) {
	l := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(linha), ","))
	if l == "" {
		return "", false
	}
	for _, clausula := range naoSaoColunas {
		if strings.HasPrefix(strings.ToUpper(l), clausula) {
			return "", false
		}
	}
	return strings.Fields(l)[0], true
}

// fonteDosRepositorios concatena o código do pacote de repositórios — é onde
// todo o SQL de escrita e leitura mora.
func fonteDosRepositorios(t *testing.T) string {
	t.Helper()

	arquivos, err := filepath.Glob(filepath.Join("..", "..", "internal", "adapter", "repository", "*.go"))
	if err != nil {
		t.Fatalf("erro ao listar os repositórios: %v", err)
	}

	var tudo strings.Builder
	for _, arquivo := range arquivos {
		conteudo, err := os.ReadFile(arquivo)
		if err != nil {
			t.Fatalf("erro ao ler %s: %v", arquivo, err)
		}
		tudo.Write(conteudo)
	}
	return tudo.String()
}

// mencionada procura a coluna como palavra inteira: sem a fronteira, "cor"
// casaria com "criado_por" e o teste nunca acusaria nada.
func mencionada(fonte, coluna string) bool {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(coluna) + `\b`).MatchString(fonte)
}
