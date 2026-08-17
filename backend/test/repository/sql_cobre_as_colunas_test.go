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
	orfas := orfasDeclaradas(t)
	usadas := map[string]bool{}

	for tabela, colunas := range tabelas {
		for _, coluna := range colunas {
			chave := tabela + "." + coluna
			if mencionada(sql, coluna) {
				if orfas[chave] {
					t.Errorf("%s está declarada como ÓRFÃ, mas aparece no SQL do repositório.\n"+
						"Ou o código voltou a usá-la — e a declaração deve sair —, ou a\n"+
						"declaração está no arquivo errado.", chave)
				}
				continue
			}
			if orfas[chave] {
				usadas[chave] = true
				continue
			}
			t.Errorf("a coluna %s existe no banco mas não aparece em nenhum SQL de "+
				"internal/adapter/repository: o dado nunca é gravado nem lido", chave)
		}
	}

	// Declaração que sobra é autorização esquecida — a mesma regra do guard de
	// expand/contract. Se a coluna não existe mais, a linha `-- ORFA:` some com
	// a necessidade dela.
	for chave := range orfas {
		if !usadas[chave] {
			t.Errorf("%s está declarada como ÓRFÃ e não existe mais nas migrations: remova a declaração", chave)
		}
	}
}

// declaracaoDeOrfas é o arquivo que lista as colunas deliberadamente sem SQL.
//
// NÃO é uma migration, e o motivo é caro: a primeira tentativa pôs a declaração
// como comentário DENTRO da migration que criara as colunas, e o Flyway recusou
// subir. Ele guarda o checksum de cada migration aplicada e valida na partida —
// editar o arquivo, ainda que só para acrescentar um comentário, muda o checksum
// e derruba o start. Migration aplicada é imutável inclusive nos comentários.
//
// (Aquela migration não existe mais com aquele número: o conjunto foi
// consolidado quando o banco de produção foi zerado. A lição vale igual, e volta
// a valer no minuto em que houver dado real no banco.)
//
// `.md` é ignorado pelo Flyway, que só recolhe o que casa com `V*.sql`.
const declaracaoDeOrfas = "COLUNAS-ORFAS.md"

// orfasDeclaradas lê as colunas declaradas como deliberadamente sem SQL, nas
// linhas `- ORFA: tabela.coluna, ...` de migrations/COLUNAS-ORFAS.md.
//
// Existe para o intervalo entre DESISTIR de uma funcionalidade e APAGAR a
// coluna dela, que são obrigatoriamente dois deploys: enquanto a versão
// anterior ainda estiver no ar — e é para onde um rollback volta —, a coluna
// precisa continuar existindo no banco. Nesse meio-tempo ela é exatamente o que
// este guard caça: coluna que ninguém lê nem escreve.
//
// A diferença entre uma órfã declarada e o defeito que o guard existe para
// pegar é uma frase escrita por alguém. Sem a declaração as duas são idênticas
// — e foi assim que `cards.prazo` e `boards.fundo` passaram despercebidas.
func orfasDeclaradas(t *testing.T) map[string]bool {
	t.Helper()

	caminho := filepath.Join("..", "..", "migrations", declaracaoDeOrfas)
	conteudo, err := os.ReadFile(caminho)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}
		}
		t.Fatalf("erro ao ler %s: %v", caminho, err)
	}

	orfas := map[string]bool{}
	for _, linha := range strings.Split(string(conteudo), "\n") {
		linha = strings.TrimSpace(linha)
		// O traço da lista markdown é opcional na leitura, para a declaração
		// poder ser lida como texto e como dado sem duas grafias.
		linha = strings.TrimPrefix(linha, "- ")
		if !strings.HasPrefix(linha, "ORFA:") {
			continue
		}
		for _, item := range strings.Split(strings.TrimPrefix(linha, "ORFA:"), ",") {
			if item = strings.TrimSpace(item); item != "" {
				orfas[item] = true
			}
		}
	}
	return orfas
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
