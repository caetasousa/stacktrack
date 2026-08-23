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
	"strings"
	"testing"
	"time"

	"stacktrack/internal/adapter/repository"
	"stacktrack/internal/domain/card"
	"stacktrack/internal/domain/evento"
	"stacktrack/internal/domain/ordem"
	ucboard "stacktrack/internal/usecase/board"

	"github.com/google/uuid"
)

func novoQuadroUseCaseComTransacao() *ucboard.QuadroUseCase {
	uc := ucboard.NovoQuadroUseCase(
		repository.NovoBoardPostgres(pool),
		repository.NovoMembroPostgres(pool),
		repository.NovoColunaPostgres(pool),
		repository.NovoCardPostgres(pool),
		repository.NovoEtiquetaPostgres(pool),
		repository.NovoChecklistPostgres(pool),
		repository.NovoAnexoPostgres(pool),
		repository.NovoResponsavelPostgres(pool),
		repository.NovoComentarioPostgres(pool),
		nil,
	)
	uc.ComEscritaAtomica(novaUnidade(3 * time.Second))
	return uc
}

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

	unidade := novaUnidade(3 * time.Second)
	c.Mover(colunaID, "t")
	e := evento.Novo(evento.CardMovido, boardID, usuarioID, c)

	carimbado, err := unidade.Escrever(ctx, e, func(esc ucboard.Escrita) error {
		return esc.Cards.Atualizar(ctx, c)
	})
	if err != nil {
		t.Fatalf("escrever: %v", err)
	}
	// O evento volta CARIMBADO com os dois números. Devolver só o seq era o
	// defeito que deixava o evento entregue ao vivo sem revisão, com o log
	// perfeitamente correto — ver test/usecase.TestEventoPublicadoCarregaARevisao.
	if carimbado.Seq == 0 {
		t.Error("o banco não atribuiu seq ao evento")
	}
	if carimbado.Revisao == 0 {
		t.Error("a transação não devolveu a revisão: quem publica não teria como carimbar o evento ao vivo")
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

// Criar um quadro é a única mutação cujo lock ainda não existe quando a
// transação começa. Mesmo assim quadro, dono, revisão e evento precisam nascer
// juntos: a unidade relê a linha depois de a mudança inseri-la e então reserva a
// revisão inicial.
func TestCriarQuadroGravaDonoEEventoNoMesmoCommit(t *testing.T) {
	ctx := context.Background()
	usuarioID, _ := contaDeTeste(t, "Dona")

	b, err := novoQuadroUseCaseComTransacao().Criar(ctx, usuarioID, "Quadro atômico")
	if err != nil {
		t.Fatalf("criar quadro: %v", err)
	}

	var revisao int64
	if err := pool.QueryRow(ctx, `SELECT revisao FROM boards WHERE id = $1`, b.ID).Scan(&revisao); err != nil {
		t.Fatalf("ler revisão: %v", err)
	}
	if revisao != 1 {
		t.Errorf("revisão = %d, esperado 1", revisao)
	}

	var membros, eventos int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM board_membros WHERE board_id = $1 AND usuario_id = $2`,
		b.ID, usuarioID,
	).Scan(&membros); err != nil {
		t.Fatalf("contar dono: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM board_events WHERE board_id = $1 AND tipo = $2 AND revisao = 1`,
		b.ID, evento.QuadroCriado,
	).Scan(&eventos); err != nil {
		t.Fatalf("contar evento: %v", err)
	}
	if membros != 1 || eventos != 1 {
		t.Errorf("membros = %d, eventos = %d; esperado 1 e 1", membros, eventos)
	}
}

func TestFalhaAoCriarDonoDesfazOQuadro(t *testing.T) {
	ctx := context.Background()
	titulo := "órfão-" + uuid.NewString()

	if _, err := novoQuadroUseCaseComTransacao().Criar(ctx, uuid.NewString(), titulo); err == nil {
		t.Fatal("criação com usuário inexistente deveria falhar pela chave estrangeira")
	}

	var quadros int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM boards WHERE titulo = $1`, titulo).Scan(&quadros); err != nil {
		t.Fatalf("contar quadros: %v", err)
	}
	if quadros != 0 {
		t.Errorf("sobrou %d quadro sem dono depois do rollback", quadros)
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

	unidade := novaUnidade(3 * time.Second)
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

	unidade := novaUnidade(3 * time.Second)
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

func TestPublicacaoIdempotenteNaoAvancaRevisaoNemDuplicaEvento(t *testing.T) {
	ctx := context.Background()
	boardID, _, usuarioID := cenario(t)
	publicacoes := repository.NovoPublicacaoPostgres(pool)
	uc := ucboard.NovoPublicacaoUseCase(
		publicacoes,
		repository.NovoMembroPostgres(pool),
		repository.NovoBoardPostgres(pool),
		repository.NovoColunaPostgres(pool),
		repository.NovoCardPostgres(pool),
		repository.NovoEtiquetaPostgres(pool),
		repository.NovoChecklistPostgres(pool),
	)
	uc.ComEscritaAtomica(novaUnidade(3 * time.Second))

	primeira, err := uc.Publicar(ctx, boardID, usuarioID)
	if err != nil {
		t.Fatalf("publicar: %v", err)
	}
	var revisaoPublicada int64
	if err := pool.QueryRow(ctx, `SELECT revisao FROM boards WHERE id = $1`, boardID).Scan(&revisaoPublicada); err != nil {
		t.Fatalf("ler revisão publicada: %v", err)
	}
	if _, err := uc.Publicar(ctx, boardID, usuarioID); err != nil {
		t.Fatalf("repetir publicação: %v", err)
	}
	var revisaoDepoisDaRepeticao int64
	if err := pool.QueryRow(ctx, `SELECT revisao FROM boards WHERE id = $1`, boardID).Scan(&revisaoDepoisDaRepeticao); err != nil {
		t.Fatalf("reler revisão: %v", err)
	}
	if revisaoDepoisDaRepeticao != revisaoPublicada {
		t.Fatalf("publicação idempotente avançou revisão: %d -> %d", revisaoPublicada, revisaoDepoisDaRepeticao)
	}

	var eventosPublicacao int
	var payload string
	if err := pool.QueryRow(ctx,
		`SELECT count(*), COALESCE(max(payload::text), '') FROM board_events WHERE board_id = $1 AND tipo = $2`,
		boardID, evento.QuadroPublicado,
	).Scan(&eventosPublicacao, &payload); err != nil {
		t.Fatalf("ler evento de publicação: %v", err)
	}
	if eventosPublicacao != 1 {
		t.Fatalf("eventos de publicação = %d, esperado 1", eventosPublicacao)
	}
	if strings.Contains(payload, primeira.Token) {
		t.Fatal("o token secreto vazou no payload do evento")
	}

	if err := uc.Revogar(ctx, boardID, usuarioID); err != nil {
		t.Fatalf("revogar: %v", err)
	}
	var revisaoRevogada int64
	if err := pool.QueryRow(ctx, `SELECT revisao FROM boards WHERE id = $1`, boardID).Scan(&revisaoRevogada); err != nil {
		t.Fatalf("ler revisão revogada: %v", err)
	}
	if err := uc.Revogar(ctx, boardID, usuarioID); err != nil {
		t.Fatalf("repetir revogação: %v", err)
	}
	var revisaoFinal int64
	if err := pool.QueryRow(ctx, `SELECT revisao FROM boards WHERE id = $1`, boardID).Scan(&revisaoFinal); err != nil {
		t.Fatalf("ler revisão final: %v", err)
	}
	if revisaoFinal != revisaoRevogada {
		t.Fatalf("revogação idempotente avançou revisão: %d -> %d", revisaoRevogada, revisaoFinal)
	}
	if revisaoRevogada != revisaoPublicada+1 {
		t.Fatalf("revogação real avançou de %d para %d, esperado +1", revisaoPublicada, revisaoRevogada)
	}
	var eventosRevogacao int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM board_events WHERE board_id = $1 AND tipo = $2`,
		boardID, evento.QuadroPublicacaoRevogada,
	).Scan(&eventosRevogacao); err != nil {
		t.Fatalf("contar revogações: %v", err)
	}
	if eventosRevogacao != 1 {
		t.Fatalf("eventos de revogação = %d, esperado 1", eventosRevogacao)
	}
}
