//go:build integracao

// O histórico contra um Postgres DE VERDADE.
//
// O que só o banco responde: se `card_id` chega gravado na coluna nova, se o
// LEFT JOIN devolve o autor, se a ordem é a do `seq DESC` — e, o caso que mais
// importa, se o histórico de um card APAGADO continua de pé. Não há chave
// estrangeira para `cards` justamente por isso, e um fake nunca provaria essa
// ausência.
package repository_test

import (
	"context"
	"testing"

	"stacktrack/internal/adapter/repository"
	"stacktrack/internal/domain/evento"
	ucboard "stacktrack/internal/usecase/board"
)

func registrar(t *testing.T, boardID, cardID, autorID string, tipo evento.Tipo, dados any) int64 {
	t.Helper()
	e := evento.Novo(tipo, boardID, autorID, dados).NoCard(cardID)
	seq, err := repository.NovoEventoPostgres(pool).Registrar(context.Background(), e)
	if err != nil {
		t.Fatalf("registrar evento: %v", err)
	}
	return seq
}

func TestHistoricoDoCardVoltaComAutorEOrdem(t *testing.T) {
	ctx := context.Background()
	boardID, colunaID, dono := cenario(t)
	cardID := cardDeTeste(t, colunaID, "Migração")

	registrar(t, boardID, cardID, dono, evento.CardCriado,
		ucboard.DadosDoCard{CardID: cardID, Titulo: "Migração", Coluna: "A fazer"})
	registrar(t, boardID, cardID, dono, evento.CardMovido,
		ucboard.DadosDoCard{CardID: cardID, Titulo: "Migração", DeColuna: "A fazer", Coluna: "Pronto"})

	lista, err := repository.NovoEventoPostgres(pool).DoCard(ctx, cardID, ucboard.LimiteDaAtividade)
	if err != nil {
		t.Fatalf("histórico: %v", err)
	}
	if len(lista) != 2 {
		t.Fatalf("entradas = %d, esperado 2", len(lista))
	}
	// Do mais recente para o mais antigo.
	if lista[0].Tipo != evento.CardMovido {
		t.Errorf("primeiro = %s, esperado card.movido", lista[0].Tipo)
	}
	if lista[0].AutorNome != "Ana" {
		t.Errorf("autorNome = %q, esperado Ana — o LEFT JOIN não trouxe o nome", lista[0].AutorNome)
	}
	if lista[0].OcorridoEm == "" {
		t.Error("ocorridoEm veio vazio")
	}
}

// O histórico é do card: evento de outro card não entra, e evento sem card
// nenhum (coluna criada, membros alterados) também não.
func TestHistoricoNaoTrazEventoDeOutroCardNemDoQuadro(t *testing.T) {
	ctx := context.Background()
	boardID, colunaID, dono := cenario(t)
	meu := cardDeTeste(t, colunaID, "Meu")
	outro := cardDeTeste(t, colunaID, "Outro")

	registrar(t, boardID, meu, dono, evento.CardCriado, ucboard.DadosDoCard{CardID: meu})
	registrar(t, boardID, outro, dono, evento.CardCriado, ucboard.DadosDoCard{CardID: outro})
	// Evento do quadro, sem card: card_id fica NULL.
	registrar(t, boardID, "", dono, evento.ColunaCriada, ucboard.DadosDaColuna{Titulo: "Nova"})

	lista, err := repository.NovoEventoPostgres(pool).DoCard(ctx, meu, ucboard.LimiteDaAtividade)
	if err != nil {
		t.Fatalf("histórico: %v", err)
	}
	if len(lista) != 1 {
		t.Fatalf("entradas = %d, esperado 1", len(lista))
	}
}

// O caso que justifica não haver chave estrangeira para `cards`: é justamente o
// evento "card apagado" que precisaria sobreviver ao CASCADE.
func TestHistoricoSobreviveAoCardApagado(t *testing.T) {
	ctx := context.Background()
	boardID, colunaID, dono := cenario(t)
	cardID := cardDeTeste(t, colunaID, "Some daqui")

	registrar(t, boardID, cardID, dono, evento.CardApagado,
		ucboard.DadosDoCard{CardID: cardID, Titulo: "Some daqui"})

	if err := repository.NovoCardPostgres(pool).Apagar(ctx, cardID); err != nil {
		t.Fatalf("apagar card: %v", err)
	}

	lista, err := repository.NovoEventoPostgres(pool).DoCard(ctx, cardID, ucboard.LimiteDaAtividade)
	if err != nil {
		t.Fatalf("histórico: %v", err)
	}
	if len(lista) != 1 {
		t.Fatalf("o histórico do card apagado sumiu: %d entradas", len(lista))
	}
}

// O teto é do repositório, não da tela: um card muito antigo não pode devolver
// uma resposta gigante.
func TestHistoricoRespeitaOTeto(t *testing.T) {
	ctx := context.Background()
	boardID, colunaID, dono := cenario(t)
	cardID := cardDeTeste(t, colunaID, "Movimentado")

	for i := 0; i < 8; i++ {
		registrar(t, boardID, cardID, dono, evento.CardAlterado, ucboard.DadosDoCard{CardID: cardID})
	}

	lista, err := repository.NovoEventoPostgres(pool).DoCard(ctx, cardID, 5)
	if err != nil {
		t.Fatalf("histórico: %v", err)
	}
	if len(lista) != 5 {
		t.Errorf("entradas = %d, esperado o teto de 5", len(lista))
	}
}

// O autor some, o histórico fica. `autor_id` não tem chave estrangeira, e o
// JOIN é LEFT — a linha continua, só sem nome.
func TestHistoricoSobreviveAoAutorRemovido(t *testing.T) {
	ctx := context.Background()
	boardID, colunaID, _ := cenario(t)
	cardID := cardDeTeste(t, colunaID, "Migração")
	efemero := usuarioDeTeste(t, "Efêmero")

	registrar(t, boardID, cardID, efemero, evento.CardCriado, ucboard.DadosDoCard{CardID: cardID})

	if _, err := pool.Exec(ctx, `DELETE FROM usuarios WHERE id = $1`, efemero); err != nil {
		t.Fatalf("apagar usuário: %v", err)
	}

	lista, err := repository.NovoEventoPostgres(pool).DoCard(ctx, cardID, ucboard.LimiteDaAtividade)
	if err != nil {
		t.Fatalf("histórico: %v", err)
	}
	if len(lista) != 1 {
		t.Fatalf("a entrada sumiu com a conta: %d", len(lista))
	}
	if lista[0].AutorNome != "" {
		t.Errorf("autorNome = %q, esperado vazio", lista[0].AutorNome)
	}
}
