// Chave repetida: quando acontece, e o que o sistema faz.
//
// Chave duplicada não corrompe nada por si só — a leitura desempata, e a ordem
// entre dois cards soltos no mesmo ponto é arbitrária de qualquer forma, porque
// ninguém pediu uma. O que importa é que ela não deixe o quadro num estado que
// a pessoa não consiga desfazer.
package usecase_test

import (
	"context"
	"testing"

	"stacktrack/internal/domain/ordem"
	ucboard "stacktrack/internal/usecase/board"
)

func afirmarChavesUnicas(t *testing.T, chaves []string) {
	t.Helper()
	vistas := make(map[string]struct{}, len(chaves))
	for _, chave := range chaves {
		if _, existe := vistas[chave]; existe {
			t.Fatalf("chave %q ficou duplicada: %v", chave, chaves)
		}
		vistas[chave] = struct{}{}
	}
}

// chavesDaColuna devolve as chaves na ordem em que a leitura entrega.
func chavesDaColuna(t *testing.T, q *quadro, boardID, colunaID string) []string {
	t.Helper()
	cards, err := q.cards.ListarDoBoard(context.Background(), boardID)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	fora := make([]string, 0, len(cards))
	for _, c := range cards {
		if c.ColunaID == colunaID {
			fora = append(fora, c.Chave)
		}
	}
	return fora
}

// O USO NORMAL não produz repetição.
//
// A tela recalcula os vizinhos a cada arraste — quem solta no topo pela segunda
// vez tem como `próximo` o card que acabou de subir, e não o antigo primeiro. É
// esse recálculo que mantém as chaves distintas.
func TestArrastarSeguidamenteNaoRepeteChave(t *testing.T) {
	q := novoQuadro()
	ana := "u-ana"
	boardID := q.criarQuadro(t, ana, "Estudos")
	colunaID := q.criarColuna(t, boardID, ana, "A fazer")

	ids := make([]string, 0, 20)
	for i := 0; i < 20; i++ {
		ids = append(ids, q.criarCard(t, colunaID, ana, "c"))
	}

	// Solta no topo, repetidamente — como a tela faria.
	primeiro := ids[0]
	for _, id := range ids[10:] {
		if _, err := q.card.Mover(context.Background(), id, ana, colunaID,
			ucboard.Vizinhos{ProximoID: primeiro}); err != nil {
			t.Fatalf("mover para o topo: %v", err)
		}
		primeiro = id // é o que a tela passaria na próxima soltura
	}

	vistas := map[string]bool{}
	for _, k := range chavesDaColuna(t, q, boardID, colunaID) {
		if vistas[k] {
			t.Fatalf("chave %q repetida no uso normal", k)
		}
		vistas[k] = true
	}
}

// Duas pessoas podem soltar no mesmo ponto com a mesma visão antiga dos
// vizinhos. O lock serializa os cálculos e a candidata é conferida contra as
// chaves em uso, então ambas as gravações passam sem deixar empate.
func TestDuasSolturasSimultaneasNoMesmoPontoNaoQuebram(t *testing.T) {
	q := novoQuadro()
	ana := "u-ana"
	boardID := q.criarQuadro(t, ana, "Estudos")
	colunaID := q.criarColuna(t, boardID, ana, "A fazer")

	alvo := q.criarCard(t, colunaID, ana, "alvo")
	primeiro := q.criarCard(t, colunaID, ana, "primeiro")
	segundo := q.criarCard(t, colunaID, ana, "segundo")

	// As duas pessoas informam o MESMO próximo: nenhuma viu o movimento da
	// outra.
	for _, id := range []string{primeiro, segundo} {
		if _, err := q.card.Mover(context.Background(), id, ana, colunaID,
			ucboard.Vizinhos{ProximoID: alvo}); err != nil {
			t.Fatalf("as duas gravações precisam dar certo: %v", err)
		}
	}

	// O quadro continua com os três cards, e todos com chave.
	chaves := chavesDaColuna(t, q, boardID, colunaID)
	if len(chaves) != 3 {
		t.Fatalf("cards = %d, esperado 3", len(chaves))
	}
	for _, k := range chaves {
		if k == "" {
			t.Error("card ficou sem chave")
		}
	}
	afirmarChavesUnicas(t, chaves)
}

// E se as duas EMPATAREM, arrastar entre elas não pode virar "erro interno".
//
// Não existe chave entre duas iguais, então o domínio recusa — e é o certo. O
// que não pode é essa recusa chegar como 500: ela é um estado previsto, e a
// tela resolve recarregando. O handler traduz ErrForaDeOrdem em 409.
func TestArrastarEntreDuasChavesIguaisRecusaComErroDeDominio(t *testing.T) {
	_, err := ordem.ChaveEntre("m", "m")

	if err == nil {
		t.Fatal("chave entre duas iguais devia ser recusada")
	}
	if err != ordem.ErrForaDeOrdem {
		t.Errorf("erro = %v, esperado ErrForaDeOrdem — é ele que o handler traduz em 409", err)
	}
}

func TestMoverNaoAceitaCandidataJaEmUso(t *testing.T) {
	q := novoQuadro()
	ana := "u-ana"
	boardID := q.criarQuadro(t, ana, "Estudos")
	colunaID := q.criarColuna(t, boardID, ana, "A fazer")

	anterior := q.criarCard(t, colunaID, ana, "anterior")
	ocupante := q.criarCard(t, colunaID, ana, "ocupante")
	proximo := q.criarCard(t, colunaID, ana, "próximo")
	movel := q.criarCard(t, colunaID, ana, "móvel")
	// Entre b e d existe uma única chave de um caractere: c, já ocupada. O
	// cálculo antigo aceitava a candidata sem conferir e criava duplicidade.
	for id, chave := range map[string]string{anterior: "b", ocupante: "c", proximo: "d", movel: "f"} {
		card, _ := q.cards.BuscarPorID(context.Background(), id)
		card.Chave = chave
		if err := q.cards.Salvar(context.Background(), card); err != nil {
			t.Fatalf("preparar chave: %v", err)
		}
	}

	if _, err := q.card.Mover(context.Background(), movel, ana, colunaID,
		ucboard.Vizinhos{AnteriorID: anterior, ProximoID: proximo}); err != nil {
		t.Fatalf("mover: %v", err)
	}
	afirmarChavesUnicas(t, chavesDaColuna(t, q, boardID, colunaID))
}

func TestMoverRecarregaVersaoDepoisDeRebalancear(t *testing.T) {
	q := novoQuadro()
	ana := "u-ana"
	boardID := q.criarQuadro(t, ana, "Estudos")
	colunaID := q.criarColuna(t, boardID, ana, "A fazer")

	primeiro := q.criarCard(t, colunaID, ana, "primeiro")
	segundo := q.criarCard(t, colunaID, ana, "segundo")
	movel := q.criarCard(t, colunaID, ana, "móvel")
	// A duplicidade força o rebalanceamento, que incrementa também a versão do
	// card móvel. Usar o agregado anterior à transação conflitaria consigo
	// mesmo no UPDATE otimista e desfaria o reparo.
	for _, id := range []string{primeiro, segundo} {
		card, _ := q.cards.BuscarPorID(context.Background(), id)
		card.Chave = "m"
		if err := q.cards.Salvar(context.Background(), card); err != nil {
			t.Fatalf("preparar duplicidade: %v", err)
		}
	}

	if _, err := q.card.Mover(context.Background(), movel, ana, colunaID, ucboard.Vizinhos{}); err != nil {
		t.Fatalf("o reparo não devia conflitar com a própria versão: %v", err)
	}
	afirmarChavesUnicas(t, chavesDaColuna(t, q, boardID, colunaID))
}
