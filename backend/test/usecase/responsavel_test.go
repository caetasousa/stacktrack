// Quem responde por cada card.
//
// O que estes testes mais exercitam é a regra que separa "atribuir" de
// "convidar": só dá para pendurar num card o nome de quem JÁ participa do
// quadro. Sem ela, a atribuição viraria um jeito de expor uma conta qualquer
// como responsável por um trabalho que ela não pode nem abrir.
package usecase_test

import (
	"context"
	"errors"
	"testing"

	dcard "stacktrack/internal/domain/card"
	"stacktrack/internal/domain/membro"
	ucboard "stacktrack/internal/usecase/board"
)

type atribuicao struct {
	*colaboracao
	responsavelUC *ucboard.ResponsavelUseCase
}

func novaAtribuicao(t *testing.T) *atribuicao {
	t.Helper()
	c := novaColaboracao(t)
	return &atribuicao{
		colaboracao:   c,
		responsavelUC: ucboard.NovoResponsavelUseCase(c.membros, c.colunas, c.cards, c.responsaveis),
	}
}

// cenarioComCard monta um quadro da ana com uma coluna e um card, e devolve o
// id do quadro e o do card.
func (a *atribuicao) cenarioComCard(t *testing.T, dono string) (boardID, cardID string) {
	t.Helper()
	boardID = a.criarQuadro(t, dono, "Estudos")
	colunaID := a.criarColuna(t, boardID, dono, "A fazer")
	return boardID, a.criarCard(t, colunaID, dono, "Migração")
}

func TestAtribuirMembroDoQuadro(t *testing.T) {
	a := novaAtribuicao(t)
	ana := a.conta(t, "Ana", "ana@exemplo.com")
	_, cardID := a.cenarioComCard(t, ana)

	if err := a.responsavelUC.Atribuir(context.Background(), cardID, ana, ana); err != nil {
		t.Fatalf("erro ao atribuir: %v", err)
	}

	lista, err := a.responsavelUC.Listar(context.Background(), cardID, ana)
	if err != nil {
		t.Fatalf("erro ao listar: %v", err)
	}
	if len(lista) != 1 || lista[0].UsuarioID != ana {
		t.Fatalf("responsáveis = %#v, esperado só a ana", lista)
	}
	if lista[0].Nome != "Ana" {
		t.Errorf("nome = %q, esperado Ana — o avatar precisa das iniciais", lista[0].Nome)
	}
}

// A regra que separa atribuir de convidar.
func TestNaoAtribuiQuemNaoParticipaDoQuadro(t *testing.T) {
	a := novaAtribuicao(t)
	ana := a.conta(t, "Ana", "ana@exemplo.com")
	estranho := a.conta(t, "Bruno", "bruno@exemplo.com")
	_, cardID := a.cenarioComCard(t, ana)

	err := a.responsavelUC.Atribuir(context.Background(), cardID, estranho, ana)
	if !errors.Is(err, membro.ErrNaoEMembro) {
		t.Fatalf("erro = %v, esperado ErrNaoEMembro", err)
	}

	lista, _ := a.responsavelUC.Listar(context.Background(), cardID, ana)
	if len(lista) != 0 {
		t.Errorf("atribuiu mesmo assim: %#v", lista)
	}
}

// Atribuir duas vezes leva ao mesmo estado — é o que a chave primária composta
// garante no banco, e o que a rota PUT promete a quem chama.
func TestAtribuirDuasVezesNaoDuplica(t *testing.T) {
	a := novaAtribuicao(t)
	ana := a.conta(t, "Ana", "ana@exemplo.com")
	_, cardID := a.cenarioComCard(t, ana)

	for i := 0; i < 2; i++ {
		if err := a.responsavelUC.Atribuir(context.Background(), cardID, ana, ana); err != nil {
			t.Fatalf("atribuição %d: %v", i+1, err)
		}
	}

	if lista, _ := a.responsavelUC.Listar(context.Background(), cardID, ana); len(lista) != 1 {
		t.Errorf("responsáveis = %d, esperado 1", len(lista))
	}
}

func TestLeitorNaoAtribui(t *testing.T) {
	a := novaAtribuicao(t)
	ana := a.conta(t, "Ana", "ana@exemplo.com")
	boardID, cardID := a.cenarioComCard(t, ana)

	leitor := a.conta(t, "Bruno", "bruno@exemplo.com")
	a.convidar(t, boardID, leitor, membro.PapelLeitor)

	err := a.responsavelUC.Atribuir(context.Background(), cardID, leitor, leitor)
	if !errors.Is(err, membro.ErrSemPermissao) {
		t.Errorf("erro = %v, esperado ErrSemPermissao", err)
	}
}

// Quem não participa do quadro recebe "card não encontrado", e não "sem
// permissão": confirmar que o card existe já seria informação.
func TestEstranhoNaoEnxergaOsResponsaveis(t *testing.T) {
	a := novaAtribuicao(t)
	ana := a.conta(t, "Ana", "ana@exemplo.com")
	_, cardID := a.cenarioComCard(t, ana)
	estranho := a.conta(t, "Bruno", "bruno@exemplo.com")

	_, err := a.responsavelUC.Listar(context.Background(), cardID, estranho)
	if !errors.Is(err, dcard.ErrNaoEncontrado) {
		t.Errorf("erro = %v, esperado ErrNaoEncontrado", err)
	}
}

func TestDesatribuirTiraSoDoCard(t *testing.T) {
	a := novaAtribuicao(t)
	ana := a.conta(t, "Ana", "ana@exemplo.com")
	boardID, cardID := a.cenarioComCard(t, ana)

	if err := a.responsavelUC.Atribuir(context.Background(), cardID, ana, ana); err != nil {
		t.Fatalf("atribuir: %v", err)
	}
	if err := a.responsavelUC.Desatribuir(context.Background(), cardID, ana, ana); err != nil {
		t.Fatalf("desatribuir: %v", err)
	}

	if lista, _ := a.responsavelUC.Listar(context.Background(), cardID, ana); len(lista) != 0 {
		t.Errorf("continuou responsável: %#v", lista)
	}
	// E continua sendo membro do quadro: desatribuir não é remover.
	if vinculo, _ := a.membros.Buscar(context.Background(), boardID, ana); vinculo == nil {
		t.Error("desatribuir tirou a pessoa do quadro")
	}
}

// A decisão que o plano da fase 10 destacou: sair do quadro leva as atribuições
// junto. Mantê-las deixaria a lista de responsáveis mentindo — nomes de quem
// não tem mais acesso — e o filtro "meus cards" mostraria à pessoa removida
// cards que ela não consegue mais abrir.
func TestSairDoQuadroLevaAsAtribuicoesJunto(t *testing.T) {
	a := novaAtribuicao(t)
	ana := a.conta(t, "Ana", "ana@exemplo.com")
	boardID, cardID := a.cenarioComCard(t, ana)

	bruno := a.conta(t, "Bruno", "bruno@exemplo.com")
	a.convidar(t, boardID, bruno, membro.PapelEditor)

	if err := a.responsavelUC.Atribuir(context.Background(), cardID, bruno, ana); err != nil {
		t.Fatalf("atribuir: %v", err)
	}

	if err := a.membroUC.Remover(context.Background(), boardID, ana, bruno); err != nil {
		t.Fatalf("remover membro: %v", err)
	}

	lista, err := a.responsavelUC.Listar(context.Background(), cardID, ana)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(lista) != 0 {
		t.Errorf("o bruno saiu do quadro e continuou responsável: %#v", lista)
	}
}

// A atribuição é por quadro: sair de um não pode apagar a responsabilidade no
// outro.
func TestSairDeUmQuadroNaoMexeNoOutro(t *testing.T) {
	a := novaAtribuicao(t)
	ana := a.conta(t, "Ana", "ana@exemplo.com")
	bruno := a.conta(t, "Bruno", "bruno@exemplo.com")

	primeiro, cardDoPrimeiro := a.cenarioComCard(t, ana)
	segundo, cardDoSegundo := a.cenarioComCard(t, ana)
	a.convidar(t, primeiro, bruno, membro.PapelEditor)
	a.convidar(t, segundo, bruno, membro.PapelEditor)

	for _, cardID := range []string{cardDoPrimeiro, cardDoSegundo} {
		if err := a.responsavelUC.Atribuir(context.Background(), cardID, bruno, ana); err != nil {
			t.Fatalf("atribuir: %v", err)
		}
	}

	if err := a.membroUC.Remover(context.Background(), primeiro, ana, bruno); err != nil {
		t.Fatalf("remover: %v", err)
	}

	if lista, _ := a.responsavelUC.Listar(context.Background(), cardDoPrimeiro, ana); len(lista) != 0 {
		t.Errorf("o vínculo do quadro que ele deixou sobrou: %#v", lista)
	}
	if lista, _ := a.responsavelUC.Listar(context.Background(), cardDoSegundo, ana); len(lista) != 1 {
		t.Errorf("apagou a atribuição do OUTRO quadro: %#v", lista)
	}
}

// O quadro devolve os responsáveis junto com os cards, numa leitura só — é o
// que evita a tela abrir card por card para desenhar os avatares.
func TestQuadroTrazOsResponsaveisDeCadaCard(t *testing.T) {
	a := novaAtribuicao(t)
	ana := a.conta(t, "Ana", "ana@exemplo.com")
	boardID, cardID := a.cenarioComCard(t, ana)

	if err := a.responsavelUC.Atribuir(context.Background(), cardID, ana, ana); err != nil {
		t.Fatalf("atribuir: %v", err)
	}

	detalhado, err := a.quadros.Detalhar(context.Background(), boardID, ana)
	if err != nil {
		t.Fatalf("detalhar: %v", err)
	}

	cards := detalhado.Colunas[0].Cards
	if len(cards) != 1 {
		t.Fatalf("cards = %d", len(cards))
	}
	if len(cards[0].Responsaveis) != 1 || cards[0].Responsaveis[0].Nome != "Ana" {
		t.Errorf("responsáveis do card = %#v", cards[0].Responsaveis)
	}
}
