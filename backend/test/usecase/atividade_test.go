// O histórico de um card.
//
// É um read model sobre o log de eventos — não há tabela nova. O que estes
// testes trancam é a decisão que quase não foi tomada: o evento guarda os NOMES
// do que aconteceu, não só os ids. Sem isso o feed diria "moveu este card", que
// é a metade inútil da informação, e era exatamente a armadilha anotada no
// plano da fase 11.
package usecase_test

import (
	"context"
	"encoding/json"
	"testing"

	"stacktrack/internal/domain/evento"
	ucboard "stacktrack/internal/usecase/board"
)

type historico struct {
	*colaboracao
	atividadeUC *ucboard.AtividadeUseCase
}

func novoHistorico(t *testing.T) *historico {
	t.Helper()
	c := novaColaboracao(t)
	c.atividades.LigarUsuarios(c.usuarios)
	// O log entra como registro DOS usecases e como fonte do histórico: é a
	// mesma tabela, lida pelas duas pontas.
	for _, uc := range []interface {
		ComRegistro(ucboard.RegistroDeEventos)
	}{
		c.quadros, c.coluna, c.card,
	} {
		uc.ComRegistro(c.atividades)
	}
	return &historico{
		colaboracao: c,
		atividadeUC: ucboard.NovoAtividadeUseCase(c.membros, c.colunas, c.cards, c.atividades),
	}
}

// dadosDoCard remonta o payload gravado, que viaja como JSON.
func dadosDoCard(t *testing.T, a ucboard.Atividade) ucboard.DadosDoCard {
	t.Helper()
	bruto, err := json.Marshal(a.Dados)
	if err != nil {
		t.Fatalf("payload ilegível: %v", err)
	}
	var d ucboard.DadosDoCard
	if err := json.Unmarshal(bruto, &d); err != nil {
		t.Fatalf("payload não casa com DadosDoCard: %v", err)
	}
	return d
}

// A prova central da fase: o histórico sabe DE ONDE para ONDE.
func TestHistoricoDizDeQualColunaParaQual(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	boardID := h.criarQuadro(t, ana, "Estudos")
	aFazer := h.criarColuna(t, boardID, ana, "A fazer")
	pronto := h.criarColuna(t, boardID, ana, "Pronto")
	cardID := h.criarCard(t, aFazer, ana, "Migração")

	if _, err := h.card.Mover(context.Background(), cardID, ana, pronto, ucboard.Vizinhos{}); err != nil {
		t.Fatalf("mover: %v", err)
	}

	lista, err := h.atividadeUC.DoCard(context.Background(), cardID, ana)
	if err != nil {
		t.Fatalf("histórico: %v", err)
	}
	if len(lista) == 0 {
		t.Fatal("histórico vazio")
	}
	// Do mais recente para o mais antigo: o movimento é o primeiro.
	if lista[0].Tipo != evento.CardMovido {
		t.Fatalf("primeiro do histórico = %s, esperado card.movido", lista[0].Tipo)
	}

	d := dadosDoCard(t, lista[0])
	if d.DeColuna != "A fazer" {
		t.Errorf("deColuna = %q, esperado 'A fazer' — sem ela o feed não conta a história", d.DeColuna)
	}
	if d.Coluna != "Pronto" {
		t.Errorf("coluna = %q, esperado 'Pronto'", d.Coluna)
	}
	if d.Titulo != "Migração" {
		t.Errorf("titulo = %q, esperado Migração", d.Titulo)
	}
}

// Renomear guarda o título de antes: "renomeou de X para Y" precisa dos dois.
func TestHistoricoGuardaOTituloAnterior(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	boardID := h.criarQuadro(t, ana, "Estudos")
	colunaID := h.criarColuna(t, boardID, ana, "A fazer")
	cardID := h.criarCard(t, colunaID, ana, "Nome velho")

	if _, err := h.card.Editar(context.Background(), cardID, ana, "Nome novo", "", "", 0); err != nil {
		t.Fatalf("editar: %v", err)
	}

	lista, _ := h.atividadeUC.DoCard(context.Background(), cardID, ana)
	d := dadosDoCard(t, lista[0])
	if d.TituloAnterior != "Nome velho" {
		t.Errorf("tituloAnterior = %q, esperado 'Nome velho'", d.TituloAnterior)
	}
	if d.Titulo != "Nome novo" {
		t.Errorf("titulo = %q, esperado 'Nome novo'", d.Titulo)
	}
}

// Editar sem mexer no título não pode alegar renomeação.
func TestEditarSemTrocarOTituloNaoAlegaRenomeacao(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	boardID := h.criarQuadro(t, ana, "Estudos")
	colunaID := h.criarColuna(t, boardID, ana, "A fazer")
	cardID := h.criarCard(t, colunaID, ana, "Mesmo nome")

	if _, err := h.card.Editar(context.Background(), cardID, ana, "Mesmo nome", "só a descrição mudou", "", 0); err != nil {
		t.Fatalf("editar: %v", err)
	}

	lista, _ := h.atividadeUC.DoCard(context.Background(), cardID, ana)
	if d := dadosDoCard(t, lista[0]); d.TituloAnterior != "" {
		t.Errorf("tituloAnterior = %q, esperado vazio — o título não mudou", d.TituloAnterior)
	}
}

// O card apagado deixa o nome no histórico: depois do DELETE não há de onde
// tirá-lo, e "apagou um card" é bem menos útil que "apagou Migração".
func TestHistoricoGuardaONomeDoCardApagado(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	boardID := h.criarQuadro(t, ana, "Estudos")
	colunaID := h.criarColuna(t, boardID, ana, "A fazer")
	cardID := h.criarCard(t, colunaID, ana, "Some daqui")

	if err := h.card.Apagar(context.Background(), cardID, ana); err != nil {
		t.Fatalf("apagar: %v", err)
	}

	// O card sumiu, então o histórico é lido pelo log direto.
	lista, err := h.atividades.DoCard(context.Background(), cardID, ucboard.LimiteDaAtividade)
	if err != nil {
		t.Fatalf("histórico: %v", err)
	}
	if lista[0].Tipo != evento.CardApagado {
		t.Fatalf("último evento = %s, esperado card.apagado", lista[0].Tipo)
	}
	if d := dadosDoCard(t, lista[0]); d.Titulo != "Some daqui" {
		t.Errorf("titulo = %q, esperado 'Some daqui'", d.Titulo)
	}
}

// O histórico é do CARD: o que aconteceu com o vizinho não entra.
func TestHistoricoNaoMisturaCards(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	boardID := h.criarQuadro(t, ana, "Estudos")
	colunaID := h.criarColuna(t, boardID, ana, "A fazer")
	primeiro := h.criarCard(t, colunaID, ana, "Primeiro")
	segundo := h.criarCard(t, colunaID, ana, "Segundo")

	if _, err := h.card.Editar(context.Background(), segundo, ana, "Segundo editado", "", "", 0); err != nil {
		t.Fatalf("editar: %v", err)
	}

	doPrimeiro, _ := h.atividadeUC.DoCard(context.Background(), primeiro, ana)
	for _, a := range doPrimeiro {
		if d := dadosDoCard(t, a); d.CardID != primeiro {
			t.Errorf("evento de outro card no histórico: %+v", d)
		}
	}
	doSegundo, _ := h.atividadeUC.DoCard(context.Background(), segundo, ana)
	if len(doSegundo) != 2 {
		t.Errorf("o segundo card devia ter criação e edição, tem %d", len(doSegundo))
	}
}

// Quem não participa do quadro não lê o histórico — é dado do quadro como
// qualquer outro.
func TestEstranhoNaoLeOHistorico(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	boardID := h.criarQuadro(t, ana, "Estudos")
	colunaID := h.criarColuna(t, boardID, ana, "A fazer")
	cardID := h.criarCard(t, colunaID, ana, "Migração")
	estranho := h.conta(t, "Bruno", "bruno@exemplo.com")

	if _, err := h.atividadeUC.DoCard(context.Background(), cardID, estranho); err == nil {
		t.Error("quem não participa leu o histórico")
	}
}

// O nome de quem agiu vem junto: um histórico com id cru não diz nada.
func TestHistoricoDizQuemFez(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	boardID := h.criarQuadro(t, ana, "Estudos")
	colunaID := h.criarColuna(t, boardID, ana, "A fazer")
	cardID := h.criarCard(t, colunaID, ana, "Migração")

	lista, _ := h.atividadeUC.DoCard(context.Background(), cardID, ana)
	if len(lista) == 0 {
		t.Fatal("histórico vazio")
	}
	if lista[0].AutorNome != "Ana" {
		t.Errorf("autorNome = %q, esperado Ana", lista[0].AutorNome)
	}
}
