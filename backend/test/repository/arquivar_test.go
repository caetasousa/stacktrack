//go:build integracao

// A armadilha da fase 13, contra um Postgres de verdade.
//
// Arquivar não muda uma regra: muda TODA leitura. Um `SELECT` esquecido faz o
// card arquivado reaparecer no quadro, e é o tipo de defeito que os fakes em
// memória não pegam — eles copiam a struct e não têm SQL onde esquecer o
// filtro. É aqui que os testes de repositório contra banco real se pagam.
package repository_test

import (
	"context"
	"testing"

	"stacktrack/internal/adapter/repository"
	"stacktrack/internal/domain/card"
	"stacktrack/internal/domain/coluna"
	"stacktrack/internal/domain/cor"

	"github.com/google/uuid"
)

// cardNoBanco cria e persiste um card na coluna informada.
func cardNoBanco(t *testing.T, colunaID, titulo, chave string) *card.Card {
	t.Helper()
	c, err := card.Novo(uuid.NewString(), colunaID, titulo, "", "", chave)
	if err != nil {
		t.Fatalf("montar %s: %v", titulo, err)
	}
	if err := repository.NovoCardPostgres(pool).Salvar(context.Background(), c); err != nil {
		t.Fatalf("salvar %s: %v", titulo, err)
	}
	return c
}

func titulos(cards []card.Card) []string {
	nomes := make([]string, 0, len(cards))
	for _, c := range cards {
		nomes = append(nomes, c.Titulo)
	}
	return nomes
}

func TestCardArquivadoSaiDoQuadroEEntraNoArquivo(t *testing.T) {
	ctx := context.Background()
	boardID, colunaID, _ := cenario(t)
	repo := repository.NovoCardPostgres(pool)

	fica := cardNoBanco(t, colunaID, "Fica no quadro", "b")
	sai := cardNoBanco(t, colunaID, "Vai para o arquivo", "n")

	if err := sai.Arquivar(); err != nil {
		t.Fatalf("arquivar: %v", err)
	}
	if err := repo.Atualizar(ctx, sai); err != nil {
		t.Fatalf("gravar o arquivamento: %v", err)
	}

	ativos, err := repo.ListarDoBoard(ctx, boardID)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(ativos) != 1 || ativos[0].ID != fica.ID {
		t.Errorf("o quadro devolveu %v — o card arquivado não saiu", titulos(ativos))
	}

	arquivados, err := repo.ListarArquivadosDoBoard(ctx, boardID)
	if err != nil {
		t.Fatalf("listar arquivados: %v", err)
	}
	if len(arquivados) != 1 || arquivados[0].ID != sai.ID {
		t.Fatalf("o arquivo devolveu %v", titulos(arquivados))
	}
	// O instante precisa sobreviver ao banco: é ele que ordena a tela de
	// arquivados, e um campo que volta nulo a deixaria em ordem arbitrária.
	if arquivados[0].ArquivadoEm == nil {
		t.Error("arquivado_em voltou nulo do banco")
	}
}

// Desarquivar devolve o card à MESMA coluna e à MESMA posição. É o que separa
// "desfazer" de "criar de novo".
func TestDesarquivarDevolveOCardAoLugarDeOndeSaiu(t *testing.T) {
	ctx := context.Background()
	boardID, colunaID, _ := cenario(t)
	repo := repository.NovoCardPostgres(pool)

	cardNoBanco(t, colunaID, "primeiro", "b")
	meio := cardNoBanco(t, colunaID, "meio", "n")
	cardNoBanco(t, colunaID, "ultimo", "t")

	_ = meio.Arquivar()
	if err := repo.Atualizar(ctx, meio); err != nil {
		t.Fatalf("arquivar: %v", err)
	}
	_ = meio.Desarquivar()
	if err := repo.Atualizar(ctx, meio); err != nil {
		t.Fatalf("desarquivar: %v", err)
	}

	ativos, err := repo.ListarDoBoard(ctx, boardID)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if nomes := titulos(ativos); len(nomes) != 3 || nomes[1] != "meio" {
		t.Errorf("ordem = %v, esperado o card de volta no meio", nomes)
	}
}

// Arquivar a COLUNA tira os cards dela do quadro sem arquivá-los.
//
// Sem o filtro pela coluna no SELECT de cards, eles ficariam órfãos na tela:
// visíveis, mas sem coluna a que pertencer.
func TestColunaArquivadaLevaOsCardsDelaParaForaDoQuadro(t *testing.T) {
	ctx := context.Background()
	boardID, colunaID, _ := cenario(t)
	cards := repository.NovoCardPostgres(pool)
	colunas := repository.NovoColunaPostgres(pool)

	cardNoBanco(t, colunaID, "vai junto", "n")

	col, err := colunas.BuscarPorID(ctx, colunaID)
	if err != nil || col == nil {
		t.Fatalf("buscar coluna: %v", err)
	}
	if err := col.Arquivar(); err != nil {
		t.Fatalf("arquivar coluna: %v", err)
	}
	if err := colunas.Atualizar(ctx, col); err != nil {
		t.Fatalf("gravar: %v", err)
	}

	ativas, err := colunas.ListarDoBoard(ctx, boardID)
	if err != nil {
		t.Fatalf("listar colunas: %v", err)
	}
	if len(ativas) != 0 {
		t.Errorf("o quadro ainda mostra %d coluna(s) arquivada(s)", len(ativas))
	}

	restantes, err := cards.ListarDoBoard(ctx, boardID)
	if err != nil {
		t.Fatalf("listar cards: %v", err)
	}
	if len(restantes) != 0 {
		t.Errorf("sobraram %d card(s) órfãos de uma coluna arquivada", len(restantes))
	}

	// E o card NÃO foi arquivado junto: ele volta com a coluna.
	arquivados, err := cards.ListarArquivadosDoBoard(ctx, boardID)
	if err != nil {
		t.Fatalf("listar arquivados: %v", err)
	}
	if len(arquivados) != 0 {
		t.Errorf("o card foi arquivado em cascata — desarquivar a coluna teria de adivinhar quais devolver")
	}
}

// BuscarPorID NÃO filtra: é por ele que passam o desarquivamento e toda
// verificação de acesso. Filtrar ali faria desarquivar responder 404.
func TestCardArquivadoContinuaAcessivelPorID(t *testing.T) {
	ctx := context.Background()
	_, colunaID, _ := cenario(t)
	repo := repository.NovoCardPostgres(pool)

	c := cardNoBanco(t, colunaID, "arquivado", "n")
	_ = c.Arquivar()
	if err := repo.Atualizar(ctx, c); err != nil {
		t.Fatalf("arquivar: %v", err)
	}

	lido, err := repo.BuscarPorID(ctx, c.ID)
	if err != nil || lido == nil {
		t.Fatalf("o card arquivado sumiu do BuscarPorID: %v", err)
	}
	if !lido.Arquivado() {
		t.Error("o card voltou do banco como se estivesse ativo")
	}
}

// A chave do card arquivado continua ocupada.
//
// Se UltimaChave ignorasse os arquivados, um card novo nasceria com a mesma
// chave — e ao desarquivar os dois disputariam a posição, desempatados pelo id,
// que não significa nada para quem olha.
func TestChaveDeCardArquivadoContinuaReservada(t *testing.T) {
	ctx := context.Background()
	_, colunaID, _ := cenario(t)
	repo := repository.NovoCardPostgres(pool)

	ultimo := cardNoBanco(t, colunaID, "último", "t")
	_ = ultimo.Arquivar()
	if err := repo.Atualizar(ctx, ultimo); err != nil {
		t.Fatalf("arquivar: %v", err)
	}

	chave, err := repo.UltimaChave(ctx, colunaID)
	if err != nil {
		t.Fatalf("ultima chave: %v", err)
	}
	if chave != "t" {
		t.Errorf("ultimaChave = %q, esperado \"t\" — a chave do arquivado foi liberada", chave)
	}
}

// O bloqueio otimista vale para arquivar como para qualquer escrita: quem
// arquiva com uma versão velha é recusado, e não sobrescreve a edição alheia.
func TestArquivarComVersaoDefasadaEhRecusado(t *testing.T) {
	ctx := context.Background()
	_, colunaID, _ := cenario(t)
	repo := repository.NovoCardPostgres(pool)

	c := cardNoBanco(t, colunaID, "disputado", "n")
	copiaAna, _ := repo.BuscarPorID(ctx, c.ID)
	copiaBob, _ := repo.BuscarPorID(ctx, c.ID)

	_ = copiaAna.Editar("editado pela ana", "", "")
	if err := repo.Atualizar(ctx, copiaAna); err != nil {
		t.Fatalf("a edição devia passar: %v", err)
	}

	_ = copiaBob.Arquivar()
	if err := repo.Atualizar(ctx, copiaBob); err != card.ErrConflito {
		t.Errorf("erro = %v, esperado ErrConflito", err)
	}

	final, _ := repo.BuscarPorID(ctx, c.ID)
	if final.Arquivado() {
		t.Error("o arquivamento defasado passou por cima da edição")
	}
}

// A coluna arquivada também continua acessível por id, e a chave dela segue
// reservada — mesmo motivo do card.
func TestColunaArquivadaContinuaAcessivelEComAChaveReservada(t *testing.T) {
	ctx := context.Background()
	boardID, _, _ := cenario(t)
	repo := repository.NovoColunaPostgres(pool)

	nova, err := coluna.Nova(uuid.NewString(), boardID, "Fazendo", cor.Azul, "t")
	if err != nil {
		t.Fatalf("montar coluna: %v", err)
	}
	if err := repo.Salvar(ctx, nova); err != nil {
		t.Fatalf("salvar: %v", err)
	}
	_ = nova.Arquivar()
	if err := repo.Atualizar(ctx, nova); err != nil {
		t.Fatalf("arquivar: %v", err)
	}

	lida, err := repo.BuscarPorID(ctx, nova.ID)
	if err != nil || lida == nil {
		t.Fatalf("a coluna arquivada sumiu do BuscarPorID: %v", err)
	}
	if !lida.Arquivada() {
		t.Error("a coluna voltou do banco como se estivesse ativa")
	}

	chave, err := repo.UltimaChave(ctx, boardID)
	if err != nil {
		t.Fatalf("ultima chave: %v", err)
	}
	if chave != "t" {
		t.Errorf("ultimaChave = %q — a chave da coluna arquivada foi liberada", chave)
	}

	arquivadas, err := repo.ListarArquivadasDoBoard(ctx, boardID)
	if err != nil {
		t.Fatalf("listar arquivadas: %v", err)
	}
	if len(arquivadas) != 1 || arquivadas[0].ID != nova.ID {
		t.Errorf("o arquivo de colunas devolveu %d item(ns)", len(arquivadas))
	}
}
