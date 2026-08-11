// Arquivar, do ponto de vista da AUTORIZAÇÃO e da orquestração.
//
// O que os fakes provam bem: quem pode arquivar, quem não pode, e que a tela de
// arquivados devolve o que devolve. O que eles NÃO provam é o filtro no SQL —
// isso mora em test/repository/arquivar_test.go, contra Postgres de verdade.
package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	dboard "stacktrack/internal/domain/board"
	dcard "stacktrack/internal/domain/card"
	dcoluna "stacktrack/internal/domain/coluna"
	"stacktrack/internal/domain/evento"
	"stacktrack/internal/domain/membro"
	ucboard "stacktrack/internal/usecase/board"
)

func TestArquivarTiraOCardDoQuadroEDesarquivarODevolve(t *testing.T) {
	ctx := context.Background()
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	colunaID := q.criarColuna(t, boardID, "ana", "A fazer")
	cardID := q.criarCard(t, colunaID, "ana", "Migração")

	if _, err := q.card.ArquivarCard(ctx, cardID, "ana"); err != nil {
		t.Fatalf("arquivar: %v", err)
	}

	detalhado, err := q.quadros.Detalhar(ctx, boardID, "ana")
	if err != nil {
		t.Fatalf("detalhar: %v", err)
	}
	if total := len(detalhado.Colunas[0].Cards); total != 0 {
		t.Errorf("o quadro ainda mostra %d card(s) arquivado(s)", total)
	}

	arquivados, err := q.quadros.ListarArquivados(ctx, boardID, "ana")
	if err != nil {
		t.Fatalf("listar arquivados: %v", err)
	}
	if len(arquivados.Cards) != 1 || arquivados.Cards[0].ID != cardID {
		t.Fatalf("o arquivo devolveu %d card(s)", len(arquivados.Cards))
	}
	// A origem vem resolvida: a tela precisa do NOME para dizer onde o card
	// cai ao voltar.
	if arquivados.ColunaDe[cardID] != "A fazer" {
		t.Errorf("coluna de origem = %q, esperado \"A fazer\"", arquivados.ColunaDe[cardID])
	}

	if _, err := q.card.DesarquivarCard(ctx, cardID, "ana"); err != nil {
		t.Fatalf("desarquivar: %v", err)
	}
	detalhado, _ = q.quadros.Detalhar(ctx, boardID, "ana")
	if total := len(detalhado.Colunas[0].Cards); total != 1 {
		t.Errorf("o card não voltou ao quadro: %d card(s)", total)
	}
}

// Arquivar a coluna tira os cards dela da tela SEM arquivá-los: eles não
// aparecem no arquivo de cards, e voltam quando a coluna volta.
func TestArquivarColunaEscondeOsCardsSemArquivaLos(t *testing.T) {
	ctx := context.Background()
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	colunaID := q.criarColuna(t, boardID, "ana", "A fazer")
	q.criarCard(t, colunaID, "ana", "Migração")

	if _, err := q.coluna.ArquivarColuna(ctx, colunaID, "ana"); err != nil {
		t.Fatalf("arquivar coluna: %v", err)
	}

	detalhado, _ := q.quadros.Detalhar(ctx, boardID, "ana")
	if len(detalhado.Colunas) != 0 {
		t.Errorf("o quadro ainda mostra %d coluna(s) arquivada(s)", len(detalhado.Colunas))
	}

	arquivados, err := q.quadros.ListarArquivados(ctx, boardID, "ana")
	if err != nil {
		t.Fatalf("listar arquivados: %v", err)
	}
	if len(arquivados.Colunas) != 1 {
		t.Errorf("o arquivo devolveu %d coluna(s)", len(arquivados.Colunas))
	}
	if len(arquivados.Cards) != 0 {
		t.Error("o card foi arquivado em cascata — desarquivar teria de adivinhar quais devolver")
	}

	if _, err := q.coluna.DesarquivarColuna(ctx, colunaID, "ana"); err != nil {
		t.Fatalf("desarquivar coluna: %v", err)
	}
	detalhado, _ = q.quadros.Detalhar(ctx, boardID, "ana")
	if len(detalhado.Colunas) != 1 || len(detalhado.Colunas[0].Cards) != 1 {
		t.Error("a coluna não voltou com o card dela")
	}
}

// Arquivar é ESCRITA: o leitor não pode, mesmo participando do quadro.
func TestLeitorNaoArquiva(t *testing.T) {
	ctx := context.Background()
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	colunaID := q.criarColuna(t, boardID, "ana", "A fazer")
	cardID := q.criarCard(t, colunaID, "ana", "Migração")
	q.convidar(t, boardID, "bob", membro.PapelLeitor)

	if _, err := q.card.ArquivarCard(ctx, cardID, "bob"); !errors.Is(err, membro.ErrSemPermissao) {
		t.Errorf("erro = %v, esperado falta de permissão", err)
	}
	if _, err := q.coluna.ArquivarColuna(ctx, colunaID, "bob"); !errors.Is(err, membro.ErrSemPermissao) {
		t.Errorf("erro ao arquivar coluna = %v, esperado falta de permissão", err)
	}

	// Mas LER o arquivo ele pode: ver o que saiu do quadro é leitura.
	if _, err := q.quadros.ListarArquivados(ctx, boardID, "bob"); err != nil {
		t.Errorf("o leitor devia poder ver o arquivo: %v", err)
	}
}

// Quem não participa não enxerga nem o arquivo — 404, e não 403, que
// confirmaria que o quadro existe.
func TestEstranhoNaoVeOArquivo(t *testing.T) {
	ctx := context.Background()
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	colunaID := q.criarColuna(t, boardID, "ana", "A fazer")
	cardID := q.criarCard(t, colunaID, "ana", "Migração")

	if _, err := q.quadros.ListarArquivados(ctx, boardID, "estranho"); !errors.Is(err, dboard.ErrNaoEncontrado) {
		t.Errorf("erro = %v, esperado ErrNaoEncontrado", err)
	}
	if _, err := q.card.ArquivarCard(ctx, cardID, "estranho"); !errors.Is(err, dcard.ErrNaoEncontrado) {
		t.Errorf("erro = %v, esperado ErrNaoEncontrado", err)
	}
	if _, err := q.coluna.ArquivarColuna(ctx, colunaID, "estranho"); !errors.Is(err, dcoluna.ErrNaoEncontrada) {
		t.Errorf("erro = %v, esperado ErrNaoEncontrada", err)
	}
}

// Duas pessoas arquivando o mesmo card: a segunda merece saber que não foi ela.
func TestArquivarDuasVezesRecusaEmVezDeFingir(t *testing.T) {
	ctx := context.Background()
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	colunaID := q.criarColuna(t, boardID, "ana", "A fazer")
	cardID := q.criarCard(t, colunaID, "ana", "Migração")

	if _, err := q.card.ArquivarCard(ctx, cardID, "ana"); err != nil {
		t.Fatalf("arquivar: %v", err)
	}
	if _, err := q.card.ArquivarCard(ctx, cardID, "ana"); !errors.Is(err, dcard.ErrJaArquivado) {
		t.Errorf("erro = %v, esperado ErrJaArquivado", err)
	}
	if _, err := q.card.DesarquivarCard(ctx, cardID, "ana"); err != nil {
		t.Fatalf("desarquivar: %v", err)
	}
	if _, err := q.card.DesarquivarCard(ctx, cardID, "ana"); !errors.Is(err, dcard.ErrNaoArquivado) {
		t.Errorf("erro = %v, esperado ErrNaoArquivado", err)
	}
}

// O evento é PRÓPRIO, e não card.alterado: o histórico precisa poder dizer
// "tirou do quadro", e quem está com o quadro aberto vê o card sumir.
func TestArquivarPublicaEventoProprioComOTitulo(t *testing.T) {
	ctx := context.Background()
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	colunaID := q.criarColuna(t, boardID, "ana", "A fazer")
	cardID := q.criarCard(t, colunaID, "ana", "Migração")

	_, espiao := comOutbox(q)

	if _, err := q.card.ArquivarCard(ctx, cardID, "ana"); err != nil {
		t.Fatalf("arquivar: %v", err)
	}
	if _, err := q.card.DesarquivarCard(ctx, cardID, "ana"); err != nil {
		t.Fatalf("desarquivar: %v", err)
	}

	if len(espiao.entregues) != 2 {
		t.Fatalf("eventos entregues = %d, esperado 2", len(espiao.entregues))
	}
	if espiao.entregues[0].Tipo != evento.CardArquivado {
		t.Errorf("tipo = %q, esperado %q", espiao.entregues[0].Tipo, evento.CardArquivado)
	}
	if espiao.entregues[1].Tipo != evento.CardDesarquivado {
		t.Errorf("tipo = %q, esperado %q", espiao.entregues[1].Tipo, evento.CardDesarquivado)
	}

	// O título viaja no payload: quem lê o log meses depois não tem o card na
	// tela para consultar.
	dados, ok := espiao.entregues[0].Dados.(ucboard.DadosDoCard)
	if !ok {
		t.Fatalf("payload = %#v, esperado DadosDoCard", espiao.entregues[0].Dados)
	}
	if dados.Titulo != "Migração" || dados.CardID != cardID {
		t.Errorf("payload = %+v — sem título ou sem id", dados)
	}
}

// Arquivar NÃO apaga arquivo nenhum: o card volta inteiro, com os anexos.
func TestArquivarNaoMexeNoVolume(t *testing.T) {
	ctx := context.Background()
	e := novoExtras()
	_, cardID := e.montar(t, "ana")

	if _, err := e.anexoUC.AnexarArquivo(ctx, cardID, "ana", "a.txt", "text/plain",
		5, strings.NewReader("teste")); err != nil {
		t.Fatalf("anexar: %v", err)
	}

	if _, err := e.card.ArquivarCard(ctx, cardID, "ana"); err != nil {
		t.Fatalf("arquivar: %v", err)
	}
	if e.armazem.Quantidade() != 1 {
		t.Error("arquivar apagou o anexo — o card não voltaria inteiro")
	}
}
