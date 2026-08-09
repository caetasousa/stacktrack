//go:build integracao

// O outbox transacional contra um Postgres DE VERDADE.
//
// Os testes em test/usecase provam o CONTRATO — que o usecase pede a escrita
// atômica e não publica quando ela falha. Aqui se prova a outra metade, a que
// só o banco pode responder: que a mudança e o evento realmente compartilham a
// transação, e que um erro no meio não deixa NENHUM dos dois para trás.
//
// É a diferença entre "o código chama a função certa" e "o Postgres desfaz as
// duas escritas juntas".
package repository_test

import (
	"context"
	"errors"
	"testing"

	"stacktrack/internal/adapter/repository"
	"stacktrack/internal/domain/card"
	"stacktrack/internal/domain/evento"
	"stacktrack/internal/domain/ordem"
	ucboard "stacktrack/internal/usecase/board"

	"github.com/google/uuid"
)

// contarEventos diz quantos eventos o quadro tem no log.
func contarEventos(t *testing.T, boardID string) int {
	t.Helper()
	var quantos int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM board_events WHERE board_id = $1`, boardID).Scan(&quantos); err != nil {
		t.Fatalf("contar eventos: %v", err)
	}
	return quantos
}

// O caminho feliz: o card muda e o evento aparece, com o seq atribuído pelo
// banco.
func TestUnidadeDeTrabalhoGravaCardEEventoJuntos(t *testing.T) {
	ctx := context.Background()
	boardID, colunaID, usuarioID := cenario(t)

	c, _ := card.Novo(uuid.NewString(), colunaID, "Migração", "", "", ordem.ChaveInicial)
	if err := repository.NovoCardPostgres(pool).Salvar(ctx, c); err != nil {
		t.Fatalf("salvar card: %v", err)
	}

	antes := contarEventos(t, boardID)

	unidade := repository.NovaUnidadeDeTrabalho(pool)
	c.Mover(colunaID, "t")
	e := evento.Novo(evento.CardMovido, boardID, usuarioID, c)

	seq, err := unidade.Escrever(ctx, e, func(esc ucboard.Escrita) error {
		return esc.Cards.Atualizar(ctx, c)
	})
	if err != nil {
		t.Fatalf("escrever: %v", err)
	}
	if seq == 0 {
		t.Error("o banco não atribuiu seq ao evento")
	}

	// O dado mudou...
	gravado, err := repository.NovoCardPostgres(pool).BuscarPorID(ctx, c.ID)
	if err != nil || gravado == nil {
		t.Fatalf("ler card: %v", err)
	}
	if gravado.Chave != "t" {
		t.Errorf("chave gravada = %q, esperado \"t\"", gravado.Chave)
	}
	// ...e o evento existe.
	if depois := contarEventos(t, boardID); depois != antes+1 {
		t.Errorf("eventos: antes %d, depois %d — esperado exatamente um a mais", antes, depois)
	}
}

// O teste que justifica a transação: se a mudança falha, o evento NÃO fica.
//
// Sem transação comum, o evento seria gravado assim mesmo — e o log do quadro
// passaria a afirmar que um card se moveu quando ele não se moveu. Quem
// reconectasse aplicaria uma mudança que nunca existiu no banco.
func TestFalhaNaMudancaNaoDeixaEventoOrfao(t *testing.T) {
	ctx := context.Background()
	boardID, colunaID, usuarioID := cenario(t)

	c, _ := card.Novo(uuid.NewString(), colunaID, "Migração", "", "", ordem.ChaveInicial)
	if err := repository.NovoCardPostgres(pool).Salvar(ctx, c); err != nil {
		t.Fatalf("salvar card: %v", err)
	}

	antes := contarEventos(t, boardID)

	unidade := repository.NovaUnidadeDeTrabalho(pool)
	e := evento.Novo(evento.CardMovido, boardID, usuarioID, c)
	quebrou := errors.New("a mudança falhou")

	_, err := unidade.Escrever(ctx, e, func(ucboard.Escrita) error {
		return quebrou
	})
	if !errors.Is(err, quebrou) {
		t.Fatalf("erro = %v, esperado o da mudança", err)
	}

	if depois := contarEventos(t, boardID); depois != antes {
		t.Errorf("o evento ficou órfão: antes %d, depois %d", antes, depois)
	}
}

// E o contrário: se a GRAVAÇÃO DO EVENTO falhar, a mudança do dado tem de
// voltar atrás junto.
//
// Um board_id inexistente viola a chave estrangeira de board_events, então o
// INSERT do evento falha depois de o UPDATE do card já ter acontecido dentro da
// transação. É exatamente a janela que o outbox fecha: sem o rollback, o card
// ficaria movido e ninguém jamais saberia.
func TestFalhaAoGravarEventoDesfazAMudanca(t *testing.T) {
	ctx := context.Background()
	_, colunaID, usuarioID := cenario(t)

	c, _ := card.Novo(uuid.NewString(), colunaID, "Migração", "", "", ordem.ChaveInicial)
	if err := repository.NovoCardPostgres(pool).Salvar(ctx, c); err != nil {
		t.Fatalf("salvar card: %v", err)
	}

	unidade := repository.NovaUnidadeDeTrabalho(pool)
	c.Mover(colunaID, "t")
	// Quadro que não existe: o INSERT em board_events vai bater na FK.
	e := evento.Novo(evento.CardMovido, uuid.NewString(), usuarioID, c)

	if _, err := unidade.Escrever(ctx, e, func(esc ucboard.Escrita) error {
		return esc.Cards.Atualizar(ctx, c)
	}); err == nil {
		t.Fatal("escrever devolveu sucesso com o evento violando a chave estrangeira")
	}

	// O card tem de ter voltado à chave original.
	gravado, err := repository.NovoCardPostgres(pool).BuscarPorID(ctx, c.ID)
	if err != nil || gravado == nil {
		t.Fatalf("ler card: %v", err)
	}
	if gravado.Chave != ordem.ChaveInicial {
		t.Errorf("chave = %q, esperado %q: o UPDATE não foi desfeito quando o evento falhou",
			gravado.Chave, ordem.ChaveInicial)
	}
}
