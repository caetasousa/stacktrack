package domain_test

import (
	"sort"
	"testing"

	"stacktrack/internal/domain/ordem"
)

// O contrato de Redistribuir: chaves válidas, crescentes, distintas e com
// espaço sobrando entre elas.
func TestRedistribuirProduzChavesValidasECrescentes(t *testing.T) {
	for _, quantidade := range []int{1, 2, 3, 25, 26, 27, 100, 1000} {
		chaves, err := ordem.Redistribuir(quantidade)
		if err != nil {
			t.Fatalf("quantidade %d: %v", quantidade, err)
		}
		if len(chaves) != quantidade {
			t.Fatalf("quantidade %d: vieram %d chaves", quantidade, len(chaves))
		}

		vistas := map[string]bool{}
		for i, k := range chaves {
			// Válida pela mesma régua da fronteira: se NormalizarChave recusa,
			// o banco e o próprio domínio recusariam depois.
			if _, err := ordem.NormalizarChave(k); err != nil {
				t.Errorf("quantidade %d: chave %q inválida: %v", quantidade, k, err)
			}
			if vistas[k] {
				t.Errorf("quantidade %d: chave repetida %q", quantidade, k)
			}
			vistas[k] = true
			if i > 0 && chaves[i-1] >= k {
				t.Errorf("quantidade %d: %q não vem depois de %q", quantidade, k, chaves[i-1])
			}
		}

		if !sort.SliceIsSorted(chaves, func(i, j int) bool { return chaves[i] < chaves[j] }) {
			t.Errorf("quantidade %d: a saída não está ordenada", quantidade)
		}
	}
}

// O ponto de redistribuir é deixar espaço: se as chaves saíssem coladas, o
// primeiro movimento depois do rebalanceamento pediria outro.
func TestRedistribuirDeixaEspacoEntreAsChaves(t *testing.T) {
	chaves, err := ordem.Redistribuir(50)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	for i := 1; i < len(chaves); i++ {
		nova, err := ordem.ChaveEntre(chaves[i-1], chaves[i])
		if err != nil {
			t.Fatalf("não coube nada entre %q e %q: %v", chaves[i-1], chaves[i], err)
		}
		if nova <= chaves[i-1] || nova >= chaves[i] {
			t.Errorf("chave %q não ficou entre %q e %q", nova, chaves[i-1], chaves[i])
		}
	}
}

// E cabe alguém ANTES da primeira e DEPOIS da última — as duas pontas.
func TestRedistribuirPreservaAsPontas(t *testing.T) {
	chaves, err := ordem.Redistribuir(10)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	antes, err := ordem.ChaveEntre("", chaves[0])
	if err != nil || antes >= chaves[0] {
		t.Errorf("não coube nada antes de %q: %q, %v", chaves[0], antes, err)
	}
	ultima := chaves[len(chaves)-1]
	depois, err := ordem.ChaveEntre(ultima, "")
	if err != nil || depois <= ultima {
		t.Errorf("não coube nada depois de %q: %q, %v", ultima, depois, err)
	}
}

func TestRedistribuirDeListaVaziaNaoProduzChave(t *testing.T) {
	chaves, err := ordem.Redistribuir(0)
	if err != nil || len(chaves) != 0 {
		t.Errorf("chaves = %v, erro = %v, esperado vazio", chaves, err)
	}
}
