package domain_test

import (
	"errors"
	"sort"
	"testing"

	"stacktrack/internal/domain/ordem"
)

// entre é o atalho que falha o teste em vez de devolver erro — a chave textual
// só erra por entrada inválida, e nenhum caso aqui informa entrada inválida.
func entre(t *testing.T, anterior, proximo string) string {
	t.Helper()
	k, err := ordem.ChaveEntre(anterior, proximo)
	if err != nil {
		t.Fatalf("ChaveEntre(%q, %q): %v", anterior, proximo, err)
	}
	return k
}

func TestChaveDaListaVaziaEOMeioDoAlfabeto(t *testing.T) {
	if k := entre(t, "", ""); k != ordem.ChaveInicial {
		t.Errorf("chave = %q, esperado %q", k, ordem.ChaveInicial)
	}
	// Começar no meio, e não em "a", é o que preserva margem para quem for
	// inserido ANTES do primeiro item.
	if ordem.ChaveInicial <= "b" {
		t.Errorf("a chave inicial %q deixa pouca margem à esquerda", ordem.ChaveInicial)
	}
}

// Entre duas letras vizinhas do alfabeto não cabe outra letra — cabe uma chave
// mais longa. O plano ilustrava com "entre a e b cabe an"; "a" não é chave
// válida aqui (termina no menor caractere), então o exemplo equivalente é
// entre "b" e "c".
func TestEntreDuasLetrasVizinhasCabeUmaChaveMaior(t *testing.T) {
	k := entre(t, "b", "c")

	if !(k > "b" && k < "c") {
		t.Fatalf("chave = %q, esperado entre 'b' e 'c'", k)
	}
	if k != "bn" {
		t.Errorf("chave = %q, esperado 'bn'", k)
	}
}

// A PROPRIEDADE QUE DEFINE A FASE 9, e o contraste direto com o float: inserir
// sempre no mesmo ponto nunca esgota. Com double precision isto falharia em
// algumas dezenas de tentativas (ver TestDividirOMesmoIntervaloAcabaEsgotando).
func TestInserirMilVezesNoMesmoPontoNuncaEsgota(t *testing.T) {
	anterior, proximo := "b", "c"

	for i := 0; i < 1000; i++ {
		k, err := ordem.ChaveEntre(anterior, proximo)
		if err != nil {
			t.Fatalf("na inserção %d: %v", i, err)
		}
		if !(k > anterior && k < proximo) {
			t.Fatalf("na inserção %d: %q não está entre %q e %q", i, k, anterior, proximo)
		}
		// Aperta sempre o mesmo intervalo — o pior caso possível.
		proximo = k
	}
}

// E o outro pior caso: sempre logo DEPOIS do mesmo item.
func TestInserirMilVezesLogoDepoisNuncaEsgota(t *testing.T) {
	anterior, proximo := "b", "c"

	for i := 0; i < 1000; i++ {
		k, err := ordem.ChaveEntre(anterior, proximo)
		if err != nil {
			t.Fatalf("na inserção %d: %v", i, err)
		}
		if !(k > anterior && k < proximo) {
			t.Fatalf("na inserção %d: %q não está entre %q e %q", i, k, anterior, proximo)
		}
		anterior = k
	}
}

// A invariante que sustenta tudo: nenhuma chave gerada termina no menor
// caractere. É ela que garante que sempre cabe alguém ANTES de qualquer item —
// sem ela, uma chave "a" seria um beco sem saída.
func TestNenhumaChaveGeradaTerminaNoMenorCaractere(t *testing.T) {
	chaves := []string{entre(t, "", "")}

	// Gera um monte de chaves por todos os caminhos: no topo, no fim e no meio.
	for i := 0; i < 200; i++ {
		primeira := chaves[0]
		ultima := chaves[len(chaves)-1]
		chaves = append([]string{entre(t, "", primeira)}, chaves...)
		chaves = append(chaves, entre(t, ultima, ""))
		if len(chaves) >= 2 {
			chaves = append(chaves, entre(t, chaves[0], chaves[1]))
		}
	}

	for _, k := range chaves {
		if k == "" {
			t.Fatal("chave vazia gerada")
		}
		if k[len(k)-1] == 'a' {
			t.Errorf("a chave %q termina no menor caractere — quebra a invariante", k)
		}
	}
}

// Inserir no topo mil vezes também nunca esgota — é o caso que a invariante
// existe para proteger.
func TestInserirMilVezesNoTopoNuncaEsgota(t *testing.T) {
	primeira := ordem.ChaveInicial

	for i := 0; i < 1000; i++ {
		k, err := ordem.ChaveEntre("", primeira)
		if err != nil {
			t.Fatalf("na inserção %d: %v", i, err)
		}
		if k >= primeira {
			t.Fatalf("na inserção %d: %q não vem antes de %q", i, k, primeira)
		}
		primeira = k
	}
}

func TestInserirMilVezesNoFimNuncaEsgota(t *testing.T) {
	ultima := ordem.ChaveInicial

	for i := 0; i < 1000; i++ {
		k, err := ordem.ChaveEntre(ultima, "")
		if err != nil {
			t.Fatalf("na inserção %d: %v", i, err)
		}
		if k <= ultima {
			t.Fatalf("na inserção %d: %q não vem depois de %q", i, k, ultima)
		}
		ultima = k
	}
}

// A prova de que o esquema serve para ORDENAR: uma sequência de inserções em
// posições arbitrárias, lida de volta em ordem alfabética, devolve exatamente a
// ordem em que os itens foram colocados.
func TestAOrdemAlfabeticaReproduzAOrdemDasInsercoes(t *testing.T) {
	// Começa com três itens.
	lista := []string{entre(t, "", "")}
	lista = append(lista, entre(t, lista[0], ""))
	lista = append([]string{entre(t, "", lista[0])}, lista...)

	// Insere no meio, repetidamente, em pontos variados.
	for i := 0; i < 100; i++ {
		pos := (i * 7) % (len(lista) - 1)
		nova := entre(t, lista[pos], lista[pos+1])

		resto := append([]string{}, lista[pos+1:]...)
		lista = append(append(lista[:pos+1], nova), resto...)
	}

	ordenada := append([]string{}, lista...)
	sort.Strings(ordenada)

	for i := range lista {
		if lista[i] != ordenada[i] {
			t.Fatalf("na posição %d a ordem alfabética discorda da ordem de inserção: %q != %q",
				i, ordenada[i], lista[i])
		}
	}
}

func TestVizinhosForaDeOrdemSaoRecusados(t *testing.T) {
	if _, err := ordem.ChaveEntre("n", "b"); !errors.Is(err, ordem.ErrForaDeOrdem) {
		t.Errorf("invertidos: erro = %v", err)
	}
	if _, err := ordem.ChaveEntre("n", "n"); !errors.Is(err, ordem.ErrForaDeOrdem) {
		t.Errorf("iguais: erro = %v", err)
	}
}

func TestChaveInvalidaERecusada(t *testing.T) {
	invalidas := []string{"A", "n1", "n ", "ná", "na"}
	for _, k := range invalidas {
		if _, err := ordem.ChaveEntre(k, ""); !errors.Is(err, ordem.ErrChaveInvalida) {
			t.Errorf("ChaveEntre(%q, \"\") aceitou: %v", k, err)
		}
	}
}

func TestNormalizarChave(t *testing.T) {
	if k, err := ordem.NormalizarChave("  ng  "); err != nil || k != "ng" {
		t.Errorf("normalizar = %q, %v", k, err)
	}
	if _, err := ordem.NormalizarChave("na"); !errors.Is(err, ordem.ErrChaveInvalida) {
		t.Errorf("chave terminada no menor caractere foi aceita: %v", err)
	}
}

// A chave cresce quando se insere sempre no mesmo ponto — é o preço do esquema,
// e vale medir para saber que ele é modesto.
func TestOCrescimentoDaChaveEModesto(t *testing.T) {
	anterior, proximo := "b", "c"
	for i := 0; i < 100; i++ {
		proximo = entre(t, anterior, proximo)
	}

	if len(proximo) > 110 {
		t.Errorf("depois de 100 inserções no mesmo ponto a chave tem %d caracteres", len(proximo))
	}
	t.Logf("100 inserções no mesmo ponto: chave de %d caracteres (%q)", len(proximo), proximo)
}
