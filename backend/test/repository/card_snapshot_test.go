//go:build integracao

// O modal do card precisa carregar o mesmo estado que a revisão devolvida
// descreve. Este teste força uma escrita a confirmar entre duas consultas do
// detalhe e prova, contra PostgreSQL real, que REPEATABLE READ impede a mistura.
package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"stacktrack/internal/adapter/repository"
	dcard "stacktrack/internal/domain/card"
	"stacktrack/internal/domain/ordem"
	ucboard "stacktrack/internal/usecase/board"

	"github.com/google/uuid"
)

type resultadoDoModal struct {
	detalhe *ucboard.CardDetalhado
	err     error
}

// esperarConsultaDeResponsaveis espera o detalhe chegar à primeira coleção
// pendurada no card. A tabela está bloqueada pelo teste, portanto nesse ponto o
// usecase já leu card, coluna, vínculo e revisão, mas ainda não leu comentários.
func esperarConsultaDeResponsaveis(t *testing.T) {
	t.Helper()
	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var bloqueada bool
		err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				  FROM pg_stat_activity
				 WHERE datname = current_database()
				   AND pid <> pg_backend_pid()
				   AND wait_event_type = 'Lock'
				   AND query LIKE '%card_responsaveis cr%'
			)`).Scan(&bloqueada)
		if err != nil {
			t.Fatalf("observar consulta bloqueada: %v", err)
		}
		if bloqueada {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("o detalhe não chegou à consulta bloqueada de responsáveis")
		case <-ticker.C:
		}
	}
}

func TestDetalharCardMantemEstadoERevisaoNoMesmoSnapshot(t *testing.T) {
	ctx := context.Background()
	boardID, colunaID, usuarioID := cenario(t)
	cardRepo := repository.NovoCardPostgres(pool)
	c, err := dcard.Novo(uuid.NewString(), colunaID, "Relatório", "", "", ordem.ChaveInicial)
	if err != nil {
		t.Fatalf("criar card: %v", err)
	}
	if err := cardRepo.Salvar(ctx, c); err != nil {
		t.Fatalf("salvar card: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE boards SET revisao = 7 WHERE id = $1`, boardID); err != nil {
		t.Fatalf("preparar revisão: %v", err)
	}

	uc := ucboard.NovoCardUseCase(
		repository.NovoBoardPostgres(pool),
		repository.NovoMembroPostgres(pool),
		repository.NovoColunaPostgres(pool),
		cardRepo,
		repository.NovoEtiquetaPostgres(pool),
		repository.NovoChecklistPostgres(pool),
		repository.NovoAnexoPostgres(pool),
		repository.NovoResponsavelPostgres(pool),
		repository.NovoComentarioPostgres(pool),
		nil,
	)
	uc.ComInstantaneo(repository.NovoInstantaneo(pool, 5*time.Second))

	// A consulta de responsáveis é o ponto de pausa determinístico entre a
	// leitura da revisão e a leitura dos comentários.
	bloqueador, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("abrir bloqueador: %v", err)
	}
	defer bloqueador.Rollback(ctx) //nolint:errcheck // segurança caso o teste falhe antes do Commit
	if _, err := bloqueador.Exec(ctx, `LOCK TABLE card_responsaveis IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatalf("bloquear responsáveis: %v", err)
	}

	resultado := make(chan resultadoDoModal, 1)
	go func() {
		detalhe, err := uc.Detalhar(context.Background(), c.ID, usuarioID)
		resultado <- resultadoDoModal{detalhe: detalhe, err: err}
	}()
	esperarConsultaDeResponsaveis(t)

	// Revisão 8 e comentário pertencem à mesma mutação confirmada. Um modal sob
	// READ COMMITTED misturaria a revisão 7, já lida, com este comentário novo.
	mudanca, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("abrir mutação concorrente: %v", err)
	}
	if _, err := mudanca.Exec(ctx, `UPDATE boards SET revisao = 8 WHERE id = $1`, boardID); err != nil {
		_ = mudanca.Rollback(ctx)
		t.Fatalf("avançar revisão: %v", err)
	}
	if _, err := mudanca.Exec(ctx, `
		INSERT INTO comentarios (id, card_id, autor_id, texto, criado_em)
		VALUES ($1, $2, $3, 'comentário da revisão 8', now())`, uuid.NewString(), c.ID, usuarioID); err != nil {
		_ = mudanca.Rollback(ctx)
		t.Fatalf("inserir comentário concorrente: %v", err)
	}
	if err := mudanca.Commit(ctx); err != nil {
		t.Fatalf("confirmar mutação concorrente: %v", err)
	}
	if err := bloqueador.Commit(ctx); err != nil {
		t.Fatalf("liberar detalhe: %v", err)
	}

	select {
	case obtido := <-resultado:
		if obtido.err != nil {
			t.Fatalf("detalhar: %v", obtido.err)
		}
		if obtido.detalhe.Revisao != 7 || len(obtido.detalhe.Comentarios) != 0 {
			t.Fatalf("snapshot misturado: revisão=%d comentários=%d; esperado estado antigo (7, 0)",
				obtido.detalhe.Revisao, len(obtido.detalhe.Comentarios))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("detalhe não terminou depois de liberar o bloqueio")
	}

	// Uma nova requisição abre outro snapshot e passa a ver o par novo inteiro.
	atual, err := uc.Detalhar(ctx, c.ID, usuarioID)
	if err != nil {
		t.Fatalf("detalhar estado novo: %v", err)
	}
	if atual.Revisao != 8 || len(atual.Comentarios) != 1 {
		t.Fatalf("estado novo = (%d, %d), esperado (8, 1): %s",
			atual.Revisao, len(atual.Comentarios), fmt.Sprint(atual.Comentarios))
	}
}
