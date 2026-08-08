package usecase_test

// Bloqueio otimista: duas pessoas editando o mesmo card não podem terminar com
// o trabalho de uma apagado em silêncio.

import (
	"context"
	"errors"
	"testing"

	dcard "stacktrack/internal/domain/card"
	"stacktrack/internal/domain/membro"
)

// O caso que a fase 6 existe para resolver.
//
// Ana e Bob abrem o mesmo card — os dois enxergam a versão 1. Ana salva
// primeiro e o card vai para a 2. Quando Bob salva, ele ainda acha que está na
// 1: a escrita dele é recusada, em vez de apagar o texto da Ana.
func TestSegundaEdicaoComVersaoVelhaDa409(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	q.convidar(t, boardID, "bob", membro.PapelEditor)
	col := q.criarColuna(t, boardID, "ana", "A fazer")
	cardID := q.criarCard(t, col, "ana", "Original")

	// Os dois carregaram o card e viram a mesma versão.
	antes, _ := q.cards.BuscarPorID(context.Background(), cardID)
	versaoQueOsDoisViram := antes.Version

	if _, err := q.card.Editar(context.Background(), cardID, "ana", "Texto da ana", "", "", versaoQueOsDoisViram); err != nil {
		t.Fatalf("a primeira edição devia passar: %v", err)
	}

	_, err := q.card.Editar(context.Background(), cardID, "bob", "Texto do bob", "", "", versaoQueOsDoisViram)
	if !errors.Is(err, dcard.ErrConflito) {
		t.Errorf("erro = %v, esperado ErrConflito", err)
	}

	// E o texto da ana continua lá: recusar é o ponto todo.
	depois, _ := q.cards.BuscarPorID(context.Background(), cardID)
	if depois.Titulo != "Texto da ana" {
		t.Errorf("título = %q, o do bob sobrescreveu o da ana", depois.Titulo)
	}
}

// Sem versão informada, a escrita passa. É o que o arraste manda: mover um card
// é posicional, e a última pessoa a soltar decide — não há texto de ninguém
// para perder.
func TestSemVersaoInformadaNaoHaConferencia(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	col := q.criarColuna(t, boardID, "ana", "A fazer")
	cardID := q.criarCard(t, col, "ana", "Original")

	if _, err := q.card.Editar(context.Background(), cardID, "ana", "Primeira", "", "", 0); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if _, err := q.card.Editar(context.Background(), cardID, "ana", "Segunda", "", "", 0); err != nil {
		t.Errorf("sem versão informada não devia haver conflito: %v", err)
	}
}

// A versão sobe a cada escrita: é o contador em que o bloqueio se apoia. Se ela
// parar de subir, o WHERE do SQL passa a casar sempre e a proteção some sem
// nenhum teste ficar vermelho — por isso este caso existe separado.
func TestCadaEdicaoIncrementaAVersao(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	col := q.criarColuna(t, boardID, "ana", "A fazer")
	cardID := q.criarCard(t, col, "ana", "Original")

	inicial, _ := q.cards.BuscarPorID(context.Background(), cardID)
	for i := 0; i < 3; i++ {
		atual, _ := q.cards.BuscarPorID(context.Background(), cardID)
		if _, err := q.card.Editar(context.Background(), cardID, "ana", "Texto", "", "", atual.Version); err != nil {
			t.Fatalf("edição %d: %v", i, err)
		}
	}

	depois, _ := q.cards.BuscarPorID(context.Background(), cardID)
	if depois.Version != inicial.Version+3 {
		t.Errorf("version = %d, esperado %d", depois.Version, inicial.Version+3)
	}
}

// O repositório também recusa sozinho, sem depender de o usecase conferir.
//
// São duas redes com alcances diferentes: a conferência do usecase pega o
// conflito LENTO (abriu o card e salvou minutos depois), e o WHERE do SQL pega
// o SIMULTÂNEO — duas requisições que leram a mesma linha e escrevem no mesmo
// instante, quando nenhuma das duas versões informadas está errada.
func TestORepositorioRecusaEscritaComVersaoDefasada(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	col := q.criarColuna(t, boardID, "ana", "A fazer")
	cardID := q.criarCard(t, col, "ana", "Original")

	// Duas cópias em memória da mesma linha, como duas requisições que leram
	// juntas teriam.
	copiaAna, _ := q.cards.BuscarPorID(context.Background(), cardID)
	copiaBob, _ := q.cards.BuscarPorID(context.Background(), cardID)

	if err := copiaAna.Editar("Da ana", "", ""); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if err := q.cards.Atualizar(context.Background(), copiaAna); err != nil {
		t.Fatalf("a primeira escrita devia passar: %v", err)
	}

	if err := copiaBob.Editar("Do bob", "", ""); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if err := q.cards.Atualizar(context.Background(), copiaBob); !errors.Is(err, dcard.ErrConflito) {
		t.Errorf("erro = %v, esperado ErrConflito do repositório", err)
	}
}
