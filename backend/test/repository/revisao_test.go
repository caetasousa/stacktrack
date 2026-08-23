//go:build integracao

// A revisão por quadro: contígua, na ordem de commit, e por quadro.
//
// É o que `seq` não consegue ser. Como BIGSERIAL, ele registra a ordem de
// ALOCAÇÃO do número, não a de COMMIT — duas transações concorrentes pegam 42 e
// 43 nessa ordem e podem comitar na inversa, e um cliente que use o seq como
// cursor pula para sempre o que comitou tarde.
package repository_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"stacktrack/internal/adapter/repository"
	"stacktrack/internal/domain/evento"
	ucboard "stacktrack/internal/usecase/board"
)

// revisaoDoQuadro lê a revisão atual direto da linha do quadro.
func revisaoDoQuadro(t *testing.T, boardID string) int64 {
	t.Helper()
	var revisao *int64
	if err := pool.QueryRow(context.Background(),
		`SELECT revisao FROM boards WHERE id = $1`, boardID).Scan(&revisao); err != nil {
		t.Fatalf("ler revisão: %v", err)
	}
	if revisao == nil {
		return 0
	}
	return *revisao
}

// Cada mutação incrementa a revisão exatamente uma vez, e o evento sai
// carimbado com a revisão em que foi confirmado.
func TestCadaMutacaoIncrementaARevisaoUmaVez(t *testing.T) {
	ctx := context.Background()
	boardID, _, _ := cenario(t)

	// O quadro nasce com revisão NULL (nenhuma mutação numerada).
	if r := revisaoDoQuadro(t, boardID); r != 0 {
		t.Fatalf("revisão inicial = %d, esperado 0", r)
	}

	u := novaUnidade(3 * time.Second)
	for esperada := int64(1); esperada <= 5; esperada++ {
		if _, err := u.Escrever(ctx, eventoDe(boardID), func(ucboard.Escrita) error {
			return nil
		}); err != nil {
			t.Fatalf("escrever: %v", err)
		}
		if r := revisaoDoQuadro(t, boardID); r != esperada {
			t.Fatalf("revisão = %d, esperado %d", r, esperada)
		}
	}

	// E o log carrega as revisões 1..5, sem buraco e sem repetição.
	linhas, err := pool.Query(ctx,
		`SELECT revisao, indice, quantidade FROM board_events
		  WHERE board_id = $1 ORDER BY revisao`, boardID)
	if err != nil {
		t.Fatalf("consultar log: %v", err)
	}
	defer linhas.Close()

	var revisoes []int64
	for linhas.Next() {
		var revisao int64
		var indice, quantidade int
		if err := linhas.Scan(&revisao, &indice, &quantidade); err != nil {
			t.Fatalf("scan: %v", err)
		}
		// Uma mutação, um evento: o grupo nasce completo com um item só.
		if indice != 0 || quantidade != 1 {
			t.Errorf("grupo da revisão %d = (%d, %d), esperado (0, 1)", revisao, indice, quantidade)
		}
		revisoes = append(revisoes, revisao)
	}
	for i, r := range revisoes {
		if r != int64(i+1) {
			t.Fatalf("revisões = %v, esperado 1..5 sem buraco", revisoes)
		}
	}
}

// Mutações CONCORRENTES no mesmo quadro produzem revisões distintas e
// contíguas. É o que o lock compra: a numeração acontece serializada, então a
// ordem dos números é a ordem dos commits.
func TestMutacoesConcorrentesProduzemRevisoesContiguas(t *testing.T) {
	ctx := context.Background()
	boardID, _, _ := cenario(t)

	const quantas = 12
	u := novaUnidade(5 * time.Second)

	var largada, fim sync.WaitGroup
	largada.Add(1)
	fim.Add(quantas)
	erros := make([]error, quantas)
	for i := 0; i < quantas; i++ {
		go func(i int) {
			defer fim.Done()
			largada.Wait()
			_, erros[i] = u.Escrever(ctx, eventoDe(boardID), func(ucboard.Escrita) error {
				return nil
			})
		}(i)
	}
	largada.Done()
	fim.Wait()

	for i, err := range erros {
		if err != nil {
			t.Fatalf("escrita %d falhou: %v", i, err)
		}
	}

	if r := revisaoDoQuadro(t, boardID); r != quantas {
		t.Errorf("revisão final = %d, esperado %d", r, quantas)
	}

	// Nenhuma revisão repetida no log: duas mutações NÃO podem compartilhar
	// número, senão o cliente confirmaria uma e daria a outra por aplicada.
	var repetidas int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM (
		   SELECT revisao FROM board_events WHERE board_id = $1
		   GROUP BY revisao HAVING count(*) > 1
		 ) AS r`, boardID).Scan(&repetidas); err != nil {
		t.Fatalf("contar repetidas: %v", err)
	}
	if repetidas != 0 {
		t.Errorf("%d revisões repetidas no log do quadro", repetidas)
	}
}

// A revisão é POR QUADRO: escrever num quadro não avança o outro. É o que torna
// o cursor de um cliente independente do movimento dos outros quadros — com um
// contador global, cada escrita alheia forçaria um replay vazio.
func TestARevisaoEhPorQuadro(t *testing.T) {
	ctx := context.Background()
	boardA, _, _ := cenario(t)
	boardB, _, _ := cenario(t)

	u := novaUnidade(3 * time.Second)
	for i := 0; i < 3; i++ {
		if _, err := u.Escrever(ctx, eventoDe(boardA), func(ucboard.Escrita) error { return nil }); err != nil {
			t.Fatalf("escrever em A: %v", err)
		}
	}
	if _, err := u.Escrever(ctx, eventoDe(boardB), func(ucboard.Escrita) error { return nil }); err != nil {
		t.Fatalf("escrever em B: %v", err)
	}

	if r := revisaoDoQuadro(t, boardA); r != 3 {
		t.Errorf("revisão de A = %d, esperado 3", r)
	}
	if r := revisaoDoQuadro(t, boardB); r != 1 {
		t.Errorf("revisão de B = %d, esperado 1 — a escrita em A vazou", r)
	}
}

// O replay por revisão devolve o intervalo pedido, em ordem, e não devolve o
// que o cliente já confirmou.
func TestReplayPorRevisaoDevolveOIntervaloCerto(t *testing.T) {
	ctx := context.Background()
	boardID, _, _ := cenario(t)

	u := novaUnidade(3 * time.Second)
	for i := 0; i < 4; i++ {
		if _, err := u.Escrever(ctx, eventoDe(boardID), func(ucboard.Escrita) error { return nil }); err != nil {
			t.Fatalf("escrever: %v", err)
		}
	}

	log := repository.NovoEventoPostgres(pool)
	perdidos, err := log.DesdeRevisao(ctx, boardID, 2, 100)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(perdidos) != 2 {
		t.Fatalf("eventos = %d, esperado 2 (revisões 3 e 4)", len(perdidos))
	}
	if perdidos[0].Revisao != 3 || perdidos[1].Revisao != 4 {
		t.Errorf("revisões = (%d, %d), esperado (3, 4)", perdidos[0].Revisao, perdidos[1].Revisao)
	}
	if perdidos[0].Quantidade != 1 {
		t.Errorf("quantidade = %d, esperado 1", perdidos[0].Quantidade)
	}

	atual, err := log.RevisaoAtual(ctx, boardID)
	if err != nil || atual != 4 {
		t.Errorf("revisão atual = %d (%v), esperado 4", atual, err)
	}
}

func TestReplayPorRevisaoPreservaCardID(t *testing.T) {
	ctx := context.Background()
	boardID, colunaID, usuarioID := cenario(t)
	cardID := cardDeTeste(t, colunaID, "Com identidade")

	e := evento.Novo(evento.CardMovido, boardID, usuarioID, nil).NoCard(cardID)
	if _, err := novaUnidade(3*time.Second).Escrever(ctx, e, func(ucboard.Escrita) error { return nil }); err != nil {
		t.Fatalf("registrar evento: %v", err)
	}

	replay, err := repository.NovoEventoPostgres(pool).DesdeRevisao(ctx, boardID, 0, 100)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(replay) != 1 {
		t.Fatalf("eventos = %d, esperado 1", len(replay))
	}
	if replay[0].CardID != cardID {
		t.Fatalf("cardId relido = %q, esperado %q", replay[0].CardID, cardID)
	}
}

// A revisão atual vem da LINHA DO QUADRO, e não do máximo do log. A diferença
// aparece com eventos legados (sem revisão): o máximo do log os ignoraria e
// poderia devolver um número atrasado.
func TestRevisaoAtualIgnoraEventoLegadoSemRevisao(t *testing.T) {
	ctx := context.Background()
	boardID, _, _ := cenario(t)

	// Um evento gravado como a versão anterior gravava: sem revisão.
	if _, err := pool.Exec(ctx,
		`INSERT INTO board_events (board_id, tipo, payload, criado_em)
		 VALUES ($1, $2, NULL, now())`, boardID, string(evento.CardMovido)); err != nil {
		t.Fatalf("evento legado: %v", err)
	}

	log := repository.NovoEventoPostgres(pool)
	atual, err := log.RevisaoAtual(ctx, boardID)
	if err != nil {
		t.Fatalf("revisão atual: %v", err)
	}
	if atual != 0 {
		t.Errorf("revisão atual = %d, esperado 0: o evento legado não numera o quadro", atual)
	}

	// E o replay novo NÃO devolve o legado: ele não tem posição na sequência, e
	// entregá-lo faria o cliente confirmar uma revisão que não existe.
	perdidos, err := log.DesdeRevisao(ctx, boardID, 0, 100)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(perdidos) != 0 {
		t.Errorf("replay devolveu %d eventos legados", len(perdidos))
	}
}
