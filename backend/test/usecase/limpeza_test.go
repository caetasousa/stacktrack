// A limpeza do VOLUME quando algo é apagado.
//
// O `ON DELETE CASCADE` limpa a TABELA de anexos e não toca no disco. Sem estes
// testes, apagar deixava os arquivos no volume para sempre, sem nenhuma linha
// que os referenciasse — invisível para quem usa, e crescendo.
package usecase_test

import (
	"context"
	"strings"
	"testing"
)

func TestApagarCardLimpaOsArquivosDele(t *testing.T) {
	ctx := context.Background()
	e := novoExtras()
	_, cardID := e.montar(t, "ana")

	if _, err := e.anexoUC.AnexarArquivo(ctx, cardID, "ana", "relatorio.csv", "text/csv",
		5, strings.NewReader("a,b\n1")); err != nil {
		t.Fatalf("anexar: %v", err)
	}
	if e.armazem.Quantidade() != 1 {
		t.Fatalf("o arquivo não foi guardado: %d", e.armazem.Quantidade())
	}

	if err := e.card.Apagar(ctx, cardID, "ana"); err != nil {
		t.Fatalf("apagar card: %v", err)
	}
	if sobraram := e.armazem.Quantidade(); sobraram != 0 {
		t.Errorf("sobraram %d arquivo(s) órfão(s) no volume", sobraram)
	}
}

func TestApagarColunaLimpaOsArquivosDosCardsDela(t *testing.T) {
	ctx := context.Background()
	e := novoExtras()
	boardID := e.criarQuadro(t, "ana", "Estudos")
	colunaID := e.criarColuna(t, boardID, "ana", "A fazer")
	cardID := e.criarCard(t, colunaID, "ana", "Tarefa")

	if _, err := e.anexoUC.AnexarArquivo(ctx, cardID, "ana", "a.txt", "text/plain",
		5, strings.NewReader("teste")); err != nil {
		t.Fatalf("anexar: %v", err)
	}

	if err := e.coluna.Apagar(ctx, colunaID, "ana"); err != nil {
		t.Fatalf("apagar coluna: %v", err)
	}
	if sobraram := e.armazem.Quantidade(); sobraram != 0 {
		t.Errorf("sobraram %d arquivo(s) órfão(s) no volume", sobraram)
	}
}

func TestApagarQuadroLimpaOsArquivosDeleTodo(t *testing.T) {
	ctx := context.Background()
	e := novoExtras()
	boardID, cardID := e.montar(t, "ana")

	if _, err := e.anexoUC.AnexarArquivo(ctx, cardID, "ana", "a.txt", "text/plain",
		5, strings.NewReader("teste")); err != nil {
		t.Fatalf("anexar: %v", err)
	}

	if err := e.quadros.Apagar(ctx, boardID, "ana"); err != nil {
		t.Fatalf("apagar quadro: %v", err)
	}
	if sobraram := e.armazem.Quantidade(); sobraram != 0 {
		t.Errorf("sobraram %d arquivo(s) órfão(s) no volume", sobraram)
	}
}
