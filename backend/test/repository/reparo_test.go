//go:build integracao

// O comando de reparo contra um PostgreSQL de verdade.
//
// O teste em test/usecase prova a REGRA com fakes; aqui se prova a outra metade,
// a que só o banco responde: que a consulta de duplicidade encontra o que o
// `CREATE UNIQUE INDEX` encontraria, e que depois do reparo o índice do contract
// pode de fato ser criado.
//
// Essa última asserção é o ponto do arquivo. Um relatório dizendo "limpo" não
// vale nada se o índice ainda falhar na janela de manutenção — então o teste
// tenta criar o índice de verdade.
package repository_test

import (
	"context"
	"testing"
	"time"

	"stacktrack/internal/adapter/repository"
	"stacktrack/internal/domain/card"
	"stacktrack/internal/domain/coluna"
	"stacktrack/internal/domain/cor"
	"stacktrack/internal/domain/ordem"
	ucboard "stacktrack/internal/usecase/board"

	"github.com/google/uuid"
)

// forcarChaveDoCard grava a chave direto no banco, sem passar pelo domínio.
//
// É como a versão ANTERIOR da aplicação conseguia deixar o dado: duas inserções
// simultâneas no mesmo ponto calculavam a mesma chave. Pelo caminho de hoje isso
// não acontece mais — e é por não acontecer mais que o teste precisa forçá-lo
// para ter o que reparar.
func forcarChaveDoCard(t *testing.T, cardID, chave string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE cards SET chave = $2 WHERE id = $1`, cardID, chave); err != nil {
		t.Fatalf("forçar chave do card: %v", err)
	}
}

func forcarChaveDaColuna(t *testing.T, colunaID, chave string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE colunas SET chave = $2 WHERE id = $1`, colunaID, chave); err != nil {
		t.Fatalf("forçar chave da coluna: %v", err)
	}
}

// cardNaColuna cria um card real na coluna informada.
func cardNaColuna(t *testing.T, colunaID, titulo, chave string) string {
	t.Helper()
	c, err := card.Novo(uuid.NewString(), colunaID, titulo, "", "", chave)
	if err != nil {
		t.Fatalf("card: %v", err)
	}
	if err := repository.NovoCardPostgres(pool).Salvar(context.Background(), c); err != nil {
		t.Fatalf("salvar card: %v", err)
	}
	return c.ID
}

// indiceDoContractCria tenta criar os índices que o contract vai criar, e os
// desfaz em seguida. Devolve o erro do banco.
//
// É a asserção que importa: o relatório do comando só vale se o índice de
// verdade puder ser construído depois dele.
func indiceDoContractCria(t *testing.T) error {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`CREATE UNIQUE INDEX teste_cards_chave ON cards (coluna_id, chave COLLATE "C")`); err != nil {
		return err
	}
	defer pool.Exec(ctx, `DROP INDEX IF EXISTS teste_cards_chave`) //nolint:errcheck

	if _, err := pool.Exec(ctx,
		`CREATE UNIQUE INDEX teste_colunas_chave ON colunas (board_id, chave COLLATE "C")`); err != nil {
		return err
	}
	_, _ = pool.Exec(ctx, `DROP INDEX IF EXISTS teste_colunas_chave`)
	return nil
}

func TestReparoDeixaOBancoProntoParaOContract(t *testing.T) {
	ctx := context.Background()
	boardID, colunaID, _ := cenario(t)

	// Quatro cards, dois deles com a MESMA chave.
	primeiro := cardNaColuna(t, colunaID, "um", "b")
	segundo := cardNaColuna(t, colunaID, "dois", "c")
	cardNaColuna(t, colunaID, "três", "d")
	cardNaColuna(t, colunaID, "quatro", "e")
	forcarChaveDoCard(t, primeiro, "m")
	forcarChaveDoCard(t, segundo, "m")

	// E duas colunas do quadro com a mesma chave.
	segundaColuna, err := coluna.Nova(uuid.NewString(), boardID, "Fazendo", cor.Azul, "t")
	if err != nil {
		t.Fatalf("coluna: %v", err)
	}
	if err := repository.NovoColunaPostgres(pool).Salvar(ctx, segundaColuna); err != nil {
		t.Fatalf("salvar coluna: %v", err)
	}
	forcarChaveDaColuna(t, colunaID, "p")
	forcarChaveDaColuna(t, segundaColuna.ID, "p")

	leitor := repository.NovoDuplicidadePostgres(pool)

	// A consulta enxerga as duas duplicidades deste quadro.
	achadas, err := leitor.DuplicidadesDeChave(ctx)
	if err != nil {
		t.Fatalf("consultar: %v", err)
	}
	deCards, deColunas := 0, 0
	for _, d := range achadas {
		if d.BoardID != boardID {
			continue // outros testes do pacote também deixam quadros no banco
		}
		if d.EhDeColunas() {
			deColunas++
		} else {
			deCards++
		}
	}
	if deCards != 1 || deColunas != 1 {
		t.Fatalf("duplicidades do quadro = %d de cards e %d de colunas, esperado 1 e 1", deCards, deColunas)
	}

	reparo := ucboard.NovoReparoDeOrdenacao(leitor, novaUnidade(5*time.Second))
	relatorio, err := reparo.Executar(ctx, "reparo-de-ordenacao")
	if err != nil {
		t.Fatalf("reparo: %v", err)
	}
	if !relatorio.Limpo() {
		t.Fatalf("restaram duplicidades: %+v", relatorio.Restantes)
	}

	// A asserção que fecha o assunto: o índice do contract agora CRIA.
	if err := indiceDoContractCria(t); err != nil {
		t.Fatalf("o contract ainda não pode ser aplicado depois do reparo: %v", err)
	}
}

// O reparo é uma MUTAÇÃO: avança a revisão do quadro e deixa evento no log.
//
// Sem isso, a ordem do quadro mudaria por baixo de quem está com ele aberto sem
// nada que explicasse depois por que ela mudou.
func TestReparoAvancaARevisaoEDeixaEvento(t *testing.T) {
	ctx := context.Background()
	boardID, colunaID, _ := cenario(t)

	primeiro := cardNaColuna(t, colunaID, "um", "b")
	segundo := cardNaColuna(t, colunaID, "dois", "c")
	forcarChaveDoCard(t, primeiro, "m")
	forcarChaveDoCard(t, segundo, "m")

	antes := revisaoDoQuadro(t, boardID)

	reparo := ucboard.NovoReparoDeOrdenacao(
		repository.NovoDuplicidadePostgres(pool), novaUnidade(5*time.Second))
	if _, err := reparo.Executar(ctx, "reparo-de-ordenacao"); err != nil {
		t.Fatalf("reparo: %v", err)
	}

	if depois := revisaoDoQuadro(t, boardID); depois != antes+1 {
		t.Errorf("revisão: antes %d, depois %d — o reparo devia avançar exatamente uma", antes, depois)
	}

	var eventos int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM board_events WHERE board_id = $1 AND tipo = 'ordenacao.reparada'`,
		boardID).Scan(&eventos); err != nil {
		t.Fatalf("contar eventos: %v", err)
	}
	if eventos != 1 {
		t.Errorf("eventos de reparo = %d, esperado 1", eventos)
	}
}

// Rodar duas vezes é seguro: a segunda passada não encontra o que reparar e não
// abre transação nenhuma. É o que permite pôr o comando num pipeline.
func TestReparoEhIdempotente(t *testing.T) {
	ctx := context.Background()
	boardID, colunaID, _ := cenario(t)

	primeiro := cardNaColuna(t, colunaID, "um", "b")
	segundo := cardNaColuna(t, colunaID, "dois", "c")
	forcarChaveDoCard(t, primeiro, "m")
	forcarChaveDoCard(t, segundo, "m")

	reparo := ucboard.NovoReparoDeOrdenacao(
		repository.NovoDuplicidadePostgres(pool), novaUnidade(5*time.Second))
	if _, err := reparo.Executar(ctx, "reparo-de-ordenacao"); err != nil {
		t.Fatalf("primeiro reparo: %v", err)
	}

	revisao := revisaoDoQuadro(t, boardID)
	relatorio, err := reparo.Executar(ctx, "reparo-de-ordenacao")
	if err != nil {
		t.Fatalf("segundo reparo: %v", err)
	}
	for _, d := range relatorio.Encontradas {
		if d.BoardID == boardID {
			t.Errorf("a segunda passada ainda encontrou duplicidade neste quadro: %+v", d)
		}
	}
	if depois := revisaoDoQuadro(t, boardID); depois != revisao {
		t.Errorf("a segunda passada mexeu no quadro: revisão %d -> %d", revisao, depois)
	}
}

// A ordem RELATIVA dos cards é preservada: redistribuir não pode embaralhar o
// quadro de ninguém.
//
// O empate entre as duas chaves iguais é desempatado pelo id, que é a mesma
// regra que a leitura já usava — então nem esse par muda de lugar de forma
// arbitrária.
func TestReparoPreservaAOrdemRelativa(t *testing.T) {
	ctx := context.Background()
	_, colunaID, _ := cenario(t)

	// Chaves distintas e crescentes, exceto o par duplicado no meio.
	cardNaColuna(t, colunaID, "primeiro", "b")
	duplicadoUm := cardNaColuna(t, colunaID, "meio-a", "x")
	duplicadoDois := cardNaColuna(t, colunaID, "meio-b", "y")
	cardNaColuna(t, colunaID, "ultimo", "z")
	forcarChaveDoCard(t, duplicadoUm, "m")
	forcarChaveDoCard(t, duplicadoDois, "m")

	cards := repository.NovoCardPostgres(pool)
	antes, err := cards.ListarDaColuna(ctx, colunaID)
	if err != nil {
		t.Fatalf("listar antes: %v", err)
	}
	titulosAntes := make([]string, 0, len(antes))
	for _, c := range antes {
		titulosAntes = append(titulosAntes, c.Titulo)
	}

	reparo := ucboard.NovoReparoDeOrdenacao(
		repository.NovoDuplicidadePostgres(pool), novaUnidade(5*time.Second))
	if _, err := reparo.Executar(ctx, "reparo-de-ordenacao"); err != nil {
		t.Fatalf("reparo: %v", err)
	}

	depois, err := cards.ListarDaColuna(ctx, colunaID)
	if err != nil {
		t.Fatalf("listar depois: %v", err)
	}
	for i, c := range depois {
		if c.Titulo != titulosAntes[i] {
			t.Fatalf("a ordem mudou: antes %v, depois %v", titulosAntes, tituloDe(depois))
		}
	}

	// E ainda cabe alguém entre dois vizinhos — o espaçamento é o motivo de
	// redistribuir em vez de só desempatar.
	for i := 1; i < len(depois); i++ {
		if _, err := ordem.ChaveEntre(depois[i-1].Chave, depois[i].Chave); err != nil {
			t.Errorf("não coube nada entre %q e %q: %v", depois[i-1].Chave, depois[i].Chave, err)
		}
	}
}

func tituloDe(cards []card.Card) []string {
	titulos := make([]string, 0, len(cards))
	for _, c := range cards {
		titulos = append(titulos, c.Titulo)
	}
	return titulos
}
