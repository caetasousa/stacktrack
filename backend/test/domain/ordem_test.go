// A posição fracionária, que sobrou para etiqueta e checklist.
//
// Cards e colunas migraram para a chave textual na fase 9 — ver chave_test.go.
// O que ficou aqui só ACRESCENTA no fim, e é essa restrição que faz o limite do
// double precision não existir na prática: quem nunca insere entre dois
// vizinhos nunca divide o mesmo intervalo.
//
// O teste que MEDIA o esgotamento (52 inserções sucessivas no mesmo ponto)
// saiu junto com `ordem.Entre`, que era o que ele exercitava. O registro do
// problema e da saída está no PLANO.md, fase 9.
package domain_test

import (
	"testing"

	"stacktrack/internal/domain/ordem"
)

// O passo largo é o que dá espaço para inserir no meio sem renumerar ninguém.
func TestNoFimDeixaEspacoParaInserirNoMeio(t *testing.T) {
	primeira := ordem.NoFim(0)
	segunda := ordem.NoFim(primeira)

	if primeira >= segunda {
		t.Fatalf("as posições precisam crescer: %v depois %v", primeira, segunda)
	}
	meio := (primeira + segunda) / 2
	if meio <= primeira || meio >= segunda {
		t.Errorf("não coube nada entre %v e %v — o passo é estreito demais", primeira, segunda)
	}
}

// A lista vazia começa no passo, e não em zero: assim sobra espaço à esquerda
// para quem for inserido antes do primeiro item.
func TestListaVaziaComecaNoPasso(t *testing.T) {
	if p := ordem.NoFim(0); p != ordem.Passo {
		t.Errorf("posição inicial = %v, esperado %v", p, ordem.Passo)
	}
}
