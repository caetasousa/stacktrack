//go:build integracao

// A conversa contra um Postgres DE VERDADE.
//
// O que só o banco responde: se o JOIN traz o nome do autor, se o ORDER BY
// entrega a conversa na ordem certa, se `editado_em` volta NULL quando nunca
// houve edição — o fake copia a struct que recebeu e passaria por qualquer um
// desses erros sem reclamar — e se apagar o card leva a conversa junto.
package repository_test

import (
	"context"
	"testing"

	"stacktrack/internal/adapter/repository"
	dcomentario "stacktrack/internal/domain/comentario"

	"github.com/google/uuid"
)

func comentarioDeTeste(t *testing.T, cardID, autorID, texto string) *dcomentario.Comentario {
	t.Helper()
	c, err := dcomentario.Novo(uuid.NewString(), cardID, autorID, texto)
	if err != nil {
		t.Fatalf("montar comentário: %v", err)
	}
	if err := repository.NovoComentarioPostgres(pool).Salvar(context.Background(), c); err != nil {
		t.Fatalf("salvar comentário: %v", err)
	}
	return c
}

func TestComentarioVoltaComNomeDoAutor(t *testing.T) {
	ctx := context.Background()
	_, colunaID, dono := cenario(t)
	cardID := cardDeTeste(t, colunaID, "Migração")
	comentarioDeTeste(t, cardID, dono, "primeiro recado")

	lista, err := repository.NovoComentarioPostgres(pool).ListarDoCard(ctx, cardID)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(lista) != 1 {
		t.Fatalf("comentários = %d", len(lista))
	}
	if lista[0].AutorNome != "Ana" {
		t.Errorf("autor = %q, esperado Ana — o JOIN com usuarios não trouxe o nome", lista[0].AutorNome)
	}
	// Nunca editado tem de voltar NULL, e não a data de criação: é o que deixa a
	// tela dizer "editado" sem comparar timestamps que sempre diferem.
	if lista[0].Comentario.EditadoEm != nil {
		t.Errorf("editadoEm = %v, esperado nulo", lista[0].Comentario.EditadoEm)
	}
}

func TestConversaVoltaNaOrdemDeTempo(t *testing.T) {
	ctx := context.Background()
	_, colunaID, dono := cenario(t)
	cardID := cardDeTeste(t, colunaID, "Migração")

	for _, texto := range []string{"primeiro", "segundo", "terceiro"} {
		comentarioDeTeste(t, cardID, dono, texto)
	}

	lista, err := repository.NovoComentarioPostgres(pool).ListarDoCard(ctx, cardID)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	for i, esperado := range []string{"primeiro", "segundo", "terceiro"} {
		if lista[i].Comentario.Texto != esperado {
			t.Errorf("posição %d = %q, esperado %q", i, lista[i].Comentario.Texto, esperado)
		}
	}
}

func TestEdicaoGravaAMarcaDeEditado(t *testing.T) {
	ctx := context.Background()
	_, colunaID, dono := cenario(t)
	cardID := cardDeTeste(t, colunaID, "Migração")
	c := comentarioDeTeste(t, cardID, dono, "antes")

	if err := c.Editar("depois"); err != nil {
		t.Fatalf("editar: %v", err)
	}
	repo := repository.NovoComentarioPostgres(pool)
	if err := repo.Atualizar(ctx, c); err != nil {
		t.Fatalf("atualizar: %v", err)
	}

	lido, err := repo.BuscarPorID(ctx, c.ID)
	if err != nil || lido == nil {
		t.Fatalf("reler: %v", err)
	}
	if lido.Texto != "depois" {
		t.Errorf("texto = %q", lido.Texto)
	}
	if lido.EditadoEm == nil {
		t.Error("editadoEm voltou nulo depois de uma edição")
	}
}

// A conversa pertence ao card: apagar o card leva os comentários junto, pelo
// ON DELETE CASCADE. Sem isso sobrariam linhas apontando para card inexistente.
func TestApagarCardLevaAConversaJunto(t *testing.T) {
	ctx := context.Background()
	_, colunaID, dono := cenario(t)
	cardID := cardDeTeste(t, colunaID, "Migração")
	comentarioDeTeste(t, cardID, dono, "some comigo")

	if err := repository.NovoCardPostgres(pool).Apagar(ctx, cardID); err != nil {
		t.Fatalf("apagar card: %v", err)
	}

	var sobraram int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM comentarios WHERE card_id = $1`, cardID).Scan(&sobraram); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if sobraram != 0 {
		t.Errorf("sobraram %d comentários órfãos", sobraram)
	}
}

// A contagem por card atravessa card → coluna → quadro, e é a que alimenta o
// selo do card na tela do quadro.
func TestContagemDeComentariosPorCard(t *testing.T) {
	ctx := context.Background()
	_, colunaID, dono := cenario(t)
	comQuatro := cardDeTeste(t, colunaID, "Com conversa")
	semNenhum := cardDeTeste(t, colunaID, "Sem conversa")
	for i := 0; i < 4; i++ {
		comentarioDeTeste(t, comQuatro, dono, "mais um")
	}

	porCard, err := repository.NovoComentarioPostgres(pool).ContarPorCardDoBoard(ctx, boardDaColuna(t, colunaID))
	if err != nil {
		t.Fatalf("contar: %v", err)
	}
	if porCard[comQuatro] != 4 {
		t.Errorf("card com quatro comentários contou %d", porCard[comQuatro])
	}
	if _, tem := porCard[semNenhum]; tem {
		t.Errorf("card sem conversa apareceu no mapa")
	}
}
