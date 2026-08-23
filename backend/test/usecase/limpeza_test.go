// A exclusão RECUPERÁVEL de arquivos.
//
// O `ON DELETE CASCADE` limpa a TABELA de anexos e não toca no disco. Por muito
// tempo isso foi corrigido apagando o arquivo logo depois do commit — o que
// resolvia o vazamento e criava um problema pior: a remoção é IRREVERSÍVEL, e
// se o backup mais recente for anterior à exclusão, a restauração traz de volta
// uma linha cujo arquivo já não existe em lugar nenhum. Anexo que aparece na
// tela e não abre, para sempre.
//
// Agora a exclusão é REGISTRADA no outbox, na mesma transação do CASCADE, e os
// bytes só saem quando um backup externo comprovar que ela está coberta. Estes
// testes provam as duas metades: que o registro acontece, e que o arquivo NÃO
// some antes da prova.
package usecase_test

import (
	"context"
	"strings"
	"testing"
)

// Apagar um card REGISTRA a exclusão e NÃO apaga o arquivo.
//
// O teste antigo afirmava o contrário — "sobraram arquivos órfãos no volume"
// era a falha. A inversão é deliberada: o arquivo no disco deixou de ser lixo e
// passou a ser a cópia que a restauração vai precisar.
func TestApagarCardRegistraAExclusaoESemApagarOArquivo(t *testing.T) {
	ctx := context.Background()
	e := novoExtras()
	_, cardID := e.montar(t, "ana")

	if _, err := e.anexoUC.AnexarArquivo(ctx, cardID, "ana", "relatorio.csv", strings.NewReader("a,b\n1")); err != nil {
		t.Fatalf("anexar: %v", err)
	}
	if e.armazem.Quantidade() != 1 {
		t.Fatalf("o arquivo não foi guardado: %d", e.armazem.Quantidade())
	}

	if err := e.card.Apagar(ctx, cardID, "ana"); err != nil {
		t.Fatalf("apagar card: %v", err)
	}

	if e.exclusoes.Registradas() != 1 {
		t.Errorf("exclusões registradas = %d, esperado 1", e.exclusoes.Registradas())
	}
	// O arquivo CONTINUA no disco: sem prova de backup, nada sai.
	if e.armazem.Quantidade() != 1 {
		t.Errorf("o arquivo foi apagado sem cobertura de backup")
	}
}

func TestApagarColunaRegistraAsExclusoesDosCardsDela(t *testing.T) {
	ctx := context.Background()
	e := novoExtras()
	boardID := e.criarQuadro(t, "ana", "Estudos")
	colunaID := e.criarColuna(t, boardID, "ana", "A fazer")
	cardID := e.criarCard(t, colunaID, "ana", "Tarefa")

	if _, err := e.anexoUC.AnexarArquivo(ctx, cardID, "ana", "a.txt", strings.NewReader("teste")); err != nil {
		t.Fatalf("anexar: %v", err)
	}
	if err := e.coluna.Apagar(ctx, colunaID, "ana"); err != nil {
		t.Fatalf("apagar coluna: %v", err)
	}

	if e.exclusoes.Registradas() != 1 {
		t.Errorf("exclusões registradas = %d, esperado 1", e.exclusoes.Registradas())
	}
	if e.armazem.Quantidade() != 1 {
		t.Error("o arquivo foi apagado sem cobertura de backup")
	}
}

func TestApagarQuadroRegistraAsExclusoesDeleTodo(t *testing.T) {
	ctx := context.Background()
	e := novoExtras()
	boardID := e.criarQuadro(t, "ana", "Estudos")
	colunaID := e.criarColuna(t, boardID, "ana", "A fazer")
	cardID := e.criarCard(t, colunaID, "ana", "Tarefa")

	if _, err := e.anexoUC.AnexarArquivo(ctx, cardID, "ana", "a.txt", strings.NewReader("teste")); err != nil {
		t.Fatalf("anexar: %v", err)
	}
	if err := e.quadros.Apagar(ctx, boardID, "ana"); err != nil {
		t.Fatalf("apagar quadro: %v", err)
	}

	if e.exclusoes.Registradas() != 1 {
		t.Errorf("exclusões registradas = %d, esperado 1", e.exclusoes.Registradas())
	}
	if e.armazem.Quantidade() != 1 {
		t.Error("o arquivo foi apagado sem cobertura de backup")
	}
}

// A CHAVE FÍSICA registrada é a do arquivo, e não o nome que quem enviou
// escolheu. Registrar o nome original faria o coletor procurar um arquivo que
// nunca existiu com esse nome no volume.
func TestOOutboxRegistraAChaveFisicaENaoONomeOriginal(t *testing.T) {
	ctx := context.Background()
	e := novoExtras()
	_, cardID := e.montar(t, "ana")

	anexo, err := e.anexoUC.AnexarArquivo(ctx, cardID, "ana", "relatorio confidencial.csv", strings.NewReader("a,b"))
	if err != nil {
		t.Fatalf("anexar: %v", err)
	}
	if err := e.card.Apagar(ctx, cardID, "ana"); err != nil {
		t.Fatalf("apagar: %v", err)
	}

	caminhos := e.exclusoes.Caminhos()
	if len(caminhos) != 1 {
		t.Fatalf("caminhos = %v, esperado um", caminhos)
	}
	if caminhos[0] != anexo.Caminho {
		t.Errorf("registrado %q, esperado a chave física %q", caminhos[0], anexo.Caminho)
	}
	if strings.Contains(caminhos[0], "confidencial") {
		t.Error("o nome original de quem enviou foi para o outbox como caminho")
	}
}

// Anexo do tipo LINK não entra no outbox: não há byte nenhum a remover.
func TestLinkNaoEntraNoOutboxDeExclusao(t *testing.T) {
	ctx := context.Background()
	e := novoExtras()
	_, cardID := e.montar(t, "ana")

	if _, err := e.anexoUC.AnexarLink(ctx, cardID, "ana", "PR 12", "https://exemplo.com"); err != nil {
		t.Fatalf("anexar link: %v", err)
	}
	if err := e.card.Apagar(ctx, cardID, "ana"); err != nil {
		t.Fatalf("apagar: %v", err)
	}
	if e.exclusoes.Registradas() != 0 {
		t.Errorf("um link entrou no outbox de exclusão de arquivo")
	}
}
