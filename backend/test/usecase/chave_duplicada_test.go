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

// A CONCORRÊNCIA DE VERDADE: duas pessoas soltam no mesmo ponto ao mesmo tempo,
// as duas enxergando os mesmos vizinhos.
//
// Aqui a repetição É possível — é uma propriedade conhecida da indexação
// fracionária, e o sorteio dentro da folga só reduz a chance, não a elimina. O
// que este teste tranca é a CONSEQUÊNCIA: mesmo empatadas, as duas gravações
// dão certo, e o quadro continua legível.
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
