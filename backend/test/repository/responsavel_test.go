//go:build integracao

// Os responsáveis contra um Postgres DE VERDADE.
//
// O que só o banco responde aqui: se o JOIN com `usuarios` traz o nome, se a
// chave primária composta realmente absorve a atribuição repetida, e se o
// DELETE por quadro alcança exatamente os cards daquele quadro — a consulta
// atravessa card → coluna → quadro, e um JOIN errado apagaria demais ou de
// menos sem nenhum teste em memória perceber.
package repository_test

import (
	"context"
	"testing"

	"stacktrack/internal/adapter/repository"
	"stacktrack/internal/domain/card"
	"stacktrack/internal/domain/ordem"

	"github.com/google/uuid"
)

// usuarioDeTeste cria uma conta real e devolve o id.
func usuarioDeTeste(t *testing.T, nome string) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO usuarios (id, nome, email, senha_hash, criado_em, atualizado_em)
		 VALUES ($1, $2, $3, 'hash', now(), now())`,
		id, nome, id+"@teste.dev"); err != nil {
		t.Fatalf("usuário: %v", err)
	}
	return id
}

func cardDeTeste(t *testing.T, colunaID, titulo string) string {
	t.Helper()
	c, _ := card.Novo(uuid.NewString(), colunaID, titulo, "", "", ordem.ChaveInicial)
	if err := repository.NovoCardPostgres(pool).Salvar(context.Background(), c); err != nil {
		t.Fatalf("salvar card: %v", err)
	}
	return c.ID
}

// O nome vem do JOIN. Se ele faltasse, o avatar do card ficaria sem iniciais —
// e nenhum fake pegaria isso, porque o fake devolve a struct que recebeu.
func TestResponsavelVoltaComNome(t *testing.T) {
	ctx := context.Background()
	_, colunaID, dono := cenario(t)
	cardID := cardDeTeste(t, colunaID, "Migração")

	repo := repository.NovoResponsavelPostgres(pool)
	if err := repo.Atribuir(ctx, cardID, dono); err != nil {
		t.Fatalf("atribuir: %v", err)
	}

	lista, err := repo.DoCard(ctx, cardID)
	if err != nil {
		t.Fatalf("ler: %v", err)
	}
	if len(lista) != 1 {
		t.Fatalf("responsáveis = %#v", lista)
	}
	if lista[0].UsuarioID != dono {
		t.Errorf("id = %q, esperado %q", lista[0].UsuarioID, dono)
	}
	if lista[0].Nome != "Ana" {
		t.Errorf("nome = %q, esperado Ana — o JOIN com usuarios não trouxe o nome", lista[0].Nome)
	}
}

// A chave primária composta é quem torna a operação idempotente. Sem o
// tratamento da violação, a segunda atribuição viraria erro 500.
func TestAtribuirDuasVezesNaoEstoura(t *testing.T) {
	ctx := context.Background()
	_, colunaID, dono := cenario(t)
	cardID := cardDeTeste(t, colunaID, "Migração")

	repo := repository.NovoResponsavelPostgres(pool)
	for i := 0; i < 2; i++ {
		if err := repo.Atribuir(ctx, cardID, dono); err != nil {
			t.Fatalf("atribuição %d: %v", i+1, err)
		}
	}

	if lista, _ := repo.DoCard(ctx, cardID); len(lista) != 1 {
		t.Errorf("responsáveis = %d, esperado 1", len(lista))
	}
}

// DoBoardPorCard atravessa card → coluna → quadro. É a consulta que a tela do
// quadro usa, e a que um JOIN errado quebraria em silêncio.
func TestResponsaveisDoBoardAgrupamPorCard(t *testing.T) {
	ctx := context.Background()
	_, colunaID, dono := cenario(t)
	primeiro := cardDeTeste(t, colunaID, "Primeiro")
	segundo := cardDeTeste(t, colunaID, "Segundo")
	outro := usuarioDeTeste(t, "Bruno")

	repo := repository.NovoResponsavelPostgres(pool)
	if err := repo.Atribuir(ctx, primeiro, dono); err != nil {
		t.Fatalf("atribuir: %v", err)
	}
	if err := repo.Atribuir(ctx, primeiro, outro); err != nil {
		t.Fatalf("atribuir: %v", err)
	}

	porCard, err := repo.DoBoardPorCard(ctx, boardDaColuna(t, colunaID))
	if err != nil {
		t.Fatalf("agrupar: %v", err)
	}
	if len(porCard[primeiro]) != 2 {
		t.Errorf("card com dois responsáveis veio com %d", len(porCard[primeiro]))
	}
	if _, tem := porCard[segundo]; tem {
		t.Errorf("card sem responsável apareceu no mapa")
	}
}

// Sair do quadro apaga as atribuições DAQUELE quadro, e só delas.
func TestRemoverDoBoardNaoAlcancaOutroQuadro(t *testing.T) {
	ctx := context.Background()
	_, colunaA, dono := cenario(t)
	_, colunaB, _ := cenario(t)
	cardA := cardDeTeste(t, colunaA, "No quadro A")
	cardB := cardDeTeste(t, colunaB, "No quadro B")

	repo := repository.NovoResponsavelPostgres(pool)
	for _, id := range []string{cardA, cardB} {
		if err := repo.Atribuir(ctx, id, dono); err != nil {
			t.Fatalf("atribuir: %v", err)
		}
	}

	if err := repo.RemoverDoBoard(ctx, boardDaColuna(t, colunaA), dono); err != nil {
		t.Fatalf("remover do quadro: %v", err)
	}

	if lista, _ := repo.DoCard(ctx, cardA); len(lista) != 0 {
		t.Errorf("a atribuição do quadro A sobrou: %#v", lista)
	}
	if lista, _ := repo.DoCard(ctx, cardB); len(lista) != 1 {
		t.Errorf("apagou a atribuição do quadro B: %#v", lista)
	}
}

// boardDaColuna descobre o quadro de uma coluna direto no banco.
func boardDaColuna(t *testing.T, colunaID string) string {
	t.Helper()
	var boardID string
	if err := pool.QueryRow(context.Background(),
		`SELECT board_id FROM colunas WHERE id = $1`, colunaID).Scan(&boardID); err != nil {
		t.Fatalf("quadro da coluna: %v", err)
	}
	return boardID
}
