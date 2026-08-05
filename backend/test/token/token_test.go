package token_test

import (
	"testing"

	"kanbango/internal/pkg/token"
)

// Dois tokens iguais em 1000 gerações significaria gerador quebrado — e
// sessão de uma pessoa valendo para outra.
func TestGerarNaoRepeteTokens(t *testing.T) {
	vistos := make(map[string]bool, 1000)

	for i := 0; i < 1000; i++ {
		valor, err := token.Gerar()
		if err != nil {
			t.Fatalf("erro ao gerar token: %v", err)
		}
		if vistos[valor] {
			t.Fatalf("token repetido na geração %d: %q", i, valor)
		}
		vistos[valor] = true
	}
}

func TestHashEDeterministicoENaoDevolveOToken(t *testing.T) {
	valor, err := token.Gerar()
	if err != nil {
		t.Fatalf("erro ao gerar token: %v", err)
	}

	hash := token.Hash(valor)
	if hash != token.Hash(valor) {
		t.Error("o hash do mesmo token precisa ser sempre igual, senão a sessão não é encontrada")
	}
	if hash == valor {
		t.Error("o hash não pode ser o próprio token")
	}
	// 32 bytes em hexadecimal — o CHAR(64) da coluna token_hash.
	if len(hash) != 64 {
		t.Errorf("tamanho do hash = %d, esperado 64", len(hash))
	}
}

func TestHashesDeTokensDiferentesSaoDiferentes(t *testing.T) {
	if token.Hash("um") == token.Hash("outro") {
		t.Error("tokens diferentes não podem colidir no hash")
	}
}
