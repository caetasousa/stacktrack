// O backfill da fase 9.
//
// O que estes testes trancam é a única coisa que importa numa migração de
// dados: **a ordem que a pessoa via continua a mesma**. Um backfill que
// embaralhasse os cards seria pior que nenhum — o quadro abriria com tudo fora
// do lugar, e não haveria como saber qual era a ordem certa.
package usecase_test

import (
	"context"
	"sort"
	"testing"

	dcard "stacktrack/internal/domain/card"
	dcoluna "stacktrack/internal/domain/coluna"
	ucboard "stacktrack/internal/usecase/board"
)

// legado grava um card como as linhas de ANTES do expand: com posição e sem
// chave nenhuma.
func legado(t *testing.T, q *quadro, colunaID, titulo string, posicao float64) string {
	t.Helper()
	c, err := dcard.Novo("card-"+titulo, colunaID, titulo, "", "", posicao, "")
	if err != nil {
		t.Fatalf("montar card legado: %v", err)
	}
	c.Chave = "" // explícito: é o estado que o backfill vai encontrar
	if err := q.cards.Salvar(context.Background(), c); err != nil {
		t.Fatalf("salvar card legado: %v", err)
	}
	return c.ID
}

func colunaLegada(t *testing.T, q *quadro, boardID, titulo string, posicao float64) string {
	t.Helper()
	c, err := dcoluna.Nova("col-"+titulo, boardID, titulo, "", posicao, "")
	if err != nil {
		t.Fatalf("montar coluna legada: %v", err)
	}
	c.Chave = ""
	if err := q.colunas.Salvar(context.Background(), c); err != nil {
		t.Fatalf("salvar coluna legada: %v", err)
	}
	return c.ID
}

// A PROVA CENTRAL: a ordem por posição vira a mesma ordem por chave.
func TestOBackfillPreservaAOrdemQueAPessoaVia(t *testing.T) {
	q := novoQuadro()
	colunaID := colunaLegada(t, q, "b-1", "A fazer", 1024)

	// Gravados fora de ordem de propósito: quem manda é a posição, não a
	// ordem de inserção.
	legado(t, q, colunaID, "terceiro", 3072)
	legado(t, q, colunaID, "primeiro", 1024)
	legado(t, q, colunaID, "segundo", 2048)

	r, err := ucboard.NovoBackfillUseCase(q.colunas, q.cards).ExecutarTudo(context.Background(), 10)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if r.Cards != 3 || r.Colunas != 1 {
		t.Fatalf("preencheu %d cards e %d colunas, esperado 3 e 1", r.Cards, r.Colunas)
	}

	cards, err := q.cards.ListarDoBoard(context.Background(), "b-1")
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(cards) != 3 {
		t.Fatalf("cards = %d", len(cards))
	}

	// Ordenados pela CHAVE, têm de sair na mesma ordem que a posição ditava.
	porChave := append([]dcard.Card{}, cards...)
	sort.Slice(porChave, func(i, j int) bool { return porChave[i].Chave < porChave[j].Chave })

	esperado := []string{"primeiro", "segundo", "terceiro"}
	for i, titulo := range esperado {
		if porChave[i].Titulo != titulo {
			t.Errorf("posição %d = %q, esperado %q — o backfill embaralhou o quadro",
				i, porChave[i].Titulo, titulo)
		}
	}
	// Estritamente crescentes: chave repetida faria a ordem depender do
	// desempate por posicao, que o contract vai apagar.
	for i := 1; i < len(porChave); i++ {
		if porChave[i-1].Chave >= porChave[i].Chave {
			t.Fatalf("chaves repetidas ou fora de ordem: %q >= %q", porChave[i-1].Chave, porChave[i].Chave)
		}
	}
}

// Rodar duas vezes não pode estragar nada: o comando roda no start da
// aplicação, e todo deploy o executa de novo.
func TestOBackfillEIdempotente(t *testing.T) {
	q := novoQuadro()
	colunaID := colunaLegada(t, q, "b-1", "A fazer", 1024)
	legado(t, q, colunaID, "um", 1024)
	legado(t, q, colunaID, "dois", 2048)

	uc := ucboard.NovoBackfillUseCase(q.colunas, q.cards)
	if _, err := uc.ExecutarTudo(context.Background(), 10); err != nil {
		t.Fatalf("primeira passada: %v", err)
	}

	antes, _ := q.cards.ListarDoBoard(context.Background(), "b-1")
	chavesAntes := map[string]string{}
	for _, c := range antes {
		chavesAntes[c.ID] = c.Chave
	}

	r, err := uc.ExecutarTudo(context.Background(), 10)
	if err != nil {
		t.Fatalf("segunda passada: %v", err)
	}
	if !r.Completo() {
		t.Errorf("a segunda passada mexeu em %d cards e %d colunas — devia não achar nada", r.Cards, r.Colunas)
	}

	depois, _ := q.cards.ListarDoBoard(context.Background(), "b-1")
	for _, c := range depois {
		if c.Chave != chavesAntes[c.ID] {
			t.Errorf("a chave de %q mudou entre passadas: %q -> %q", c.Titulo, chavesAntes[c.ID], c.Chave)
		}
	}
}

// O backfill não é uma edição de ninguém: subir a version faria o bloqueio
// otimista recusar a próxima gravação legítima de quem estivesse com o card
// aberto no momento do deploy.
func TestOBackfillNaoSobeAVersaoDoCard(t *testing.T) {
	q := novoQuadro()
	colunaID := colunaLegada(t, q, "b-1", "A fazer", 1024)
	cardID := legado(t, q, colunaID, "um", 1024)

	antes, _ := q.cards.BuscarPorID(context.Background(), cardID)
	versaoAntes := antes.Version

	if _, err := ucboard.NovoBackfillUseCase(q.colunas, q.cards).ExecutarTudo(context.Background(), 10); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	depois, _ := q.cards.BuscarPorID(context.Background(), cardID)
	if depois.Version != versaoAntes {
		t.Errorf("version = %d, esperado %d — o backfill não é uma edição", depois.Version, versaoAntes)
	}
	if depois.Chave == "" {
		t.Error("a chave não foi preenchida")
	}
}

// Um card criado DEPOIS do expand já nasce com chave, e o backfill não deve
// tocá-lo — nem reordená-lo.
func TestOBackfillNaoTocaEmQuemJaTemChave(t *testing.T) {
	q := novoQuadro()
	ana := "u-ana"
	boardID := q.criarQuadro(t, ana, "Estudos")
	colunaID := q.criarColuna(t, boardID, ana, "A fazer")
	novoID := q.criarCard(t, colunaID, ana, "nasceu com chave")

	antes, _ := q.cards.BuscarPorID(context.Background(), novoID)
	if antes.Chave == "" {
		t.Fatal("card criado depois do expand nasceu sem chave")
	}
	chaveOriginal := antes.Chave

	// E um legado no meio, para o backfill ter o que fazer.
	legado(t, q, colunaID, "legado", 4096)

	if _, err := ucboard.NovoBackfillUseCase(q.colunas, q.cards).ExecutarTudo(context.Background(), 10); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	depois, _ := q.cards.BuscarPorID(context.Background(), novoID)
	if depois.Chave != chaveOriginal {
		t.Errorf("a chave de quem já tinha mudou: %q -> %q", chaveOriginal, depois.Chave)
	}
}

// O quadro vizinho não se mistura: cada quadro e cada coluna têm a própria
// sequência de chaves.
func TestOBackfillNaoMisturaQuadros(t *testing.T) {
	q := novoQuadro()
	primeira := colunaLegada(t, q, "b-1", "Primeira", 1024)
	segunda := colunaLegada(t, q, "b-2", "Segunda", 1024)
	legado(t, q, primeira, "do primeiro", 1024)
	legado(t, q, segunda, "do segundo", 1024)

	if _, err := ucboard.NovoBackfillUseCase(q.colunas, q.cards).ExecutarTudo(context.Background(), 10); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	for _, boardID := range []string{"b-1", "b-2"} {
		cards, _ := q.cards.ListarDoBoard(context.Background(), boardID)
		if len(cards) != 1 {
			t.Errorf("quadro %s = %d cards, esperado 1", boardID, len(cards))
		}
		if len(cards) > 0 && cards[0].Chave == "" {
			t.Errorf("quadro %s: card sem chave depois do backfill", boardID)
		}
	}
}

// titulos extrai os títulos na ordem em que a leitura devolveu.
func titulos(cards []dcard.Card) []string {
	fora := make([]string, 0, len(cards))
	for _, c := range cards {
		fora = append(fora, c.Titulo)
	}
	return fora
}

// COLUNA MISTA: linha antiga (sem chave) convivendo com card novo (com chave).
//
// É o estado que sobra quando um backfill anterior falhou no meio de um lote e
// a aplicação seguiu servindo — o erro é registrado e não impede o start.
//
// Este teste tranca um defeito real: o backfill encadeava a partir da MAIOR
// chave existente, o que mandava a linha antiga para o FIM da coluna. Só que a
// leitura ordena por `chave NULLS FIRST`, então ela aparecia ANTES na tela — e
// o comando que existe para preservar a ordem era justamente quem a
// embaralhava.
func TestOBackfillNaoReordenaColunaMista(t *testing.T) {
	q := novoQuadro()
	ana := "u-ana"
	boardID := q.criarQuadro(t, ana, "Estudos")
	colunaID := q.criarColuna(t, boardID, ana, "A fazer")

	// Legado com posição BAIXA: na tela ele vem primeiro.
	legado(t, q, colunaID, "antigo", 10)
	// E um card criado depois do expand, que já nasce com chave.
	q.criarCard(t, colunaID, ana, "novo")

	antes, _ := q.cards.ListarDoBoard(context.Background(), boardID)
	ordemAntes := titulos(antes)

	if _, err := ucboard.NovoBackfillUseCase(q.colunas, q.cards).ExecutarTudo(context.Background(), 10); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	depois, _ := q.cards.ListarDoBoard(context.Background(), boardID)
	ordemDepois := titulos(depois)

	for i := range ordemAntes {
		if ordemAntes[i] != ordemDepois[i] {
			t.Fatalf("o backfill reordenou o quadro: %v -> %v", ordemAntes, ordemDepois)
		}
	}

	// ⚠️ As chaves precisam ficar ESTRITAMENTE crescentes, e não só a ordem
	// final estar certa.
	//
	// A diferença não é acadêmica: com o defeito, os dois cards recebiam a MESMA
	// chave ("n"), e a ordem só continuava certa porque `posicao` desempatava.
	// O contract vai apagar `posicao` — e aí a ordem entre chaves iguais passa a
	// depender do id, ou seja, vira aleatória. Conferir só a ordem deixava o
	// defeito passar, e foi exatamente o que aconteceu.
	for i := 1; i < len(depois); i++ {
		if depois[i-1].Chave >= depois[i].Chave {
			t.Fatalf("chaves não são estritamente crescentes: %q (%s) >= %q (%s)",
				depois[i-1].Chave, depois[i-1].Titulo, depois[i].Chave, depois[i].Titulo)
		}
	}
	for _, c := range depois {
		if c.Chave == "" {
			t.Errorf("%q ficou sem chave", c.Titulo)
		}
	}
}
