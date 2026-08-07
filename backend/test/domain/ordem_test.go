package domain_test

import (
	"errors"
	"testing"

	"stacktrack/internal/domain/ordem"
)

func TestEntreDevolveOMeio(t *testing.T) {
	meio, err := ordem.Entre(2000, 3000)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if meio != 2500 {
		t.Errorf("meio = %v, esperado 2500", meio)
	}
}

// É a propriedade que faz a coisa toda funcionar: inserir entre dois itens
// escreve UMA linha, em vez de renumerar todos os seguintes.
func TestSempreCabeAlgoEntreDoisVizinhosComEspaco(t *testing.T) {
	anterior, proximo := 1024.0, 2048.0

	for i := 0; i < 20; i++ {
		meio, err := ordem.Entre(anterior, proximo)
		if err != nil {
			t.Fatalf("na inserção %d: %v", i, err)
		}
		if meio <= anterior || meio >= proximo {
			t.Fatalf("na inserção %d: %v não está entre %v e %v", i, meio, anterior, proximo)
		}
		// insere sempre no mesmo ponto, logo depois do anterior — o pior caso
		proximo = meio
	}
}

func TestPontasUsamOPasso(t *testing.T) {
	// lista vazia
	if p, err := ordem.Entre(0, 0); err != nil || p != ordem.Passo {
		t.Errorf("lista vazia: %v, %v", p, err)
	}
	// no fim
	if p, err := ordem.Entre(1024, 0); err != nil || p != 2048 {
		t.Errorf("no fim: %v, %v", p, err)
	}
	// no começo: divide por dois em vez de subtrair, para nunca ficar negativo
	if p, err := ordem.Entre(0, 1024); err != nil || p != 512 {
		t.Errorf("no começo: %v, %v", p, err)
	}
}

func TestNoComecoNuncaFicaNegativo(t *testing.T) {
	posicao := 1024.0
	for i := 0; i < 200; i++ {
		posicao = ordem.NoComeco(posicao)
		if posicao <= 0 {
			t.Fatalf("na inserção %d a posição virou %v", i, posicao)
		}
	}
}

// AQUI ESTÁ O LIMITE DO double precision, e é ele que a fase 9 conserta.
//
// A mantissa tem 53 bits: dividir sempre o mesmo intervalo pela metade esgota a
// precisão em algumas dezenas de inserções, e a média entre dois vizinhos passa
// a ser igual a um deles. O teste não existe para reprovar o código — existe
// para provar que o limite é real, medir onde ele está, e garantir que o
// domínio AVISA em vez de gravar em silêncio duas posições iguais.
func TestDividirOMesmoIntervaloAcabaEsgotandoAPrecisao(t *testing.T) {
	anterior, proximo := 1024.0, 2048.0
	insercoes := 0

	for {
		meio, err := ordem.Entre(anterior, proximo)
		if errors.Is(err, ordem.ErrSemEspaco) {
			break
		}
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		proximo = meio
		insercoes++
		if insercoes > 100 {
			t.Fatal("o esgotamento devia ter acontecido em algumas dezenas de inserções")
		}
	}

	if insercoes < 30 {
		t.Errorf("esgotou em %d inserções — cedo demais para um passo de %v", insercoes, ordem.Passo)
	}
	t.Logf("a precisão esgotou depois de %d inserções seguidas no mesmo ponto", insercoes)
}

func TestVizinhosInvertidosOuIguaisNaoTemEspaco(t *testing.T) {
	if _, err := ordem.Entre(3000, 2000); !errors.Is(err, ordem.ErrSemEspaco) {
		t.Errorf("invertidos: erro = %v", err)
	}
	if _, err := ordem.Entre(2000, 2000); !errors.Is(err, ordem.ErrSemEspaco) {
		t.Errorf("iguais: erro = %v", err)
	}
}
