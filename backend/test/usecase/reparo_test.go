// O comando de manutenção que repara duplicidade de chave de ordenação.
//
// Ele é a pré-condição do contract `UNIQUE` que fecha A2: o `CREATE UNIQUE
// INDEX` recusa criar enquanto houver duplicidade herdada, e o CLAUDE.md proíbe
// resolver isso com `UPDATE` na migration — a decisão de com que chave as linhas
// antigas ficam é do domínio.
package usecase_test

import (
	"context"
	"errors"
	"testing"

	"stacktrack/internal/domain/evento"
	ucboard "stacktrack/internal/usecase/board"
)

// duplicidadesFalsas devolve o que foi programado, e muda de resposta depois do
// primeiro reparo — como o banco faria.
type duplicidadesFalsas struct {
	antes    []ucboard.Duplicidade
	depois   []ucboard.Duplicidade
	consulta int
	erro     error
}

func (d *duplicidadesFalsas) DuplicidadesDeChave(context.Context) ([]ucboard.Duplicidade, error) {
	d.consulta++
	if d.erro != nil {
		return nil, d.erro
	}
	if d.consulta == 1 {
		return d.antes, nil
	}
	return d.depois, nil
}

// Nada duplicado: o comando não abre transação nenhuma e diz que está limpo.
func TestReparoSemDuplicidadeNaoTocaNoBanco(t *testing.T) {
	q := novoQuadro()
	atomica, _ := comOutbox(q)
	leitor := &duplicidadesFalsas{}

	relatorio, err := ucboard.NovoReparoDeOrdenacao(leitor, atomica).
		Executar(context.Background(), "reparo-de-ordenacao")
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if !relatorio.Limpo() {
		t.Error("sem duplicidade, o relatório devia dizer que está limpo")
	}
	if atomica.mudancasAplicadas != 0 {
		t.Errorf("abriu %d transações sem ter o que reparar", atomica.mudancasAplicadas)
	}
	// E nem reconsulta: sem trabalho não há o que reconferir.
	if leitor.consulta != 1 {
		t.Errorf("consultas = %d, esperado 1", leitor.consulta)
	}
}

// O caminho central: cards duplicados numa coluna são redistribuídos, e o
// relatório mostra o antes e o depois.
func TestReparoRedistribuiOsCardsDaColunaAfetada(t *testing.T) {
	q := novoQuadro()
	atomica, espiao := comOutbox(q)

	boardID := q.criarQuadro(t, "ana", "Estudos")
	colunaID := q.criarColuna(t, boardID, "ana", "A fazer")
	for i := 0; i < 4; i++ {
		q.criarCard(t, colunaID, "ana", "card")
	}
	// A duplicidade herdada: dois cards com a MESMA chave, como a versão
	// anterior conseguia gravar numa rajada de inserções no mesmo ponto.
	cards, _ := q.cards.ListarDaColuna(context.Background(), colunaID)
	q.cards.ForcarChave(cards[0].ID, "m")
	q.cards.ForcarChave(cards[1].ID, "m")

	leitor := &duplicidadesFalsas{
		antes: []ucboard.Duplicidade{
			{BoardID: boardID, ColunaID: colunaID, Chave: "m", Ocorrencias: 2},
		},
	}

	reparo := ucboard.NovoReparoDeOrdenacao(leitor, atomica)
	reparo.ComPublicador(espiao)
	relatorio, err := reparo.Executar(context.Background(), "reparo-de-ordenacao")
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if !relatorio.Limpo() {
		t.Errorf("restaram %d duplicidades", len(relatorio.Restantes))
	}
	if len(relatorio.QuadrosReparados) != 1 || relatorio.QuadrosReparados[0] != boardID {
		t.Errorf("quadros reparados = %v, esperado [%s]", relatorio.QuadrosReparados, boardID)
	}

	// As chaves ficaram distintas — que é o que o índice vai exigir.
	depois, _ := q.cards.ListarDaColuna(context.Background(), colunaID)
	vistas := map[string]bool{}
	for _, c := range depois {
		if vistas[c.Chave] {
			t.Fatalf("a chave %q continua repetida depois do reparo", c.Chave)
		}
		vistas[c.Chave] = true
	}

	// E o reparo é uma MUTAÇÃO: sai evento, para quem está com o quadro aberto
	// reconciliar em vez de continuar vendo a ordem antiga.
	if _, achou := ultimoDoTipo(espiao.entregues, evento.OrdenacaoReparada); !achou {
		t.Error("o reparo não publicou evento: as abas abertas ficariam com a ordem antiga")
	}
}

// Duplicidade entre COLUNAS do quadro usa o outro ramo — o contêiner ali é o
// próprio quadro, e o comando estreito (DefinirChave) é que reescreve.
func TestReparoRedistribuiAsColunasDoQuadro(t *testing.T) {
	q := novoQuadro()
	atomica, _ := comOutbox(q)

	boardID := q.criarQuadro(t, "ana", "Estudos")
	primeira := q.criarColuna(t, boardID, "ana", "A fazer")
	segunda := q.criarColuna(t, boardID, "ana", "Fazendo")
	q.colunas.ForcarChave(primeira, "m")
	q.colunas.ForcarChave(segunda, "m")

	leitor := &duplicidadesFalsas{
		antes: []ucboard.Duplicidade{{BoardID: boardID, Chave: "m", Ocorrencias: 2}},
	}

	if _, err := ucboard.NovoReparoDeOrdenacao(leitor, atomica).
		Executar(context.Background(), "reparo-de-ordenacao"); err != nil {
		t.Fatalf("erro: %v", err)
	}

	colunas, _ := q.colunas.ListarDoBoard(context.Background(), boardID)
	if len(colunas) != 2 {
		t.Fatalf("colunas = %d, esperado 2", len(colunas))
	}
	if colunas[0].Chave == colunas[1].Chave {
		t.Errorf("as duas colunas continuam com a chave %q", colunas[0].Chave)
	}
	// O reparo de colunas NÃO pode ter mexido no título: o comando é estreito
	// justamente para não desfazer uma renomeação simultânea.
	nomes := map[string]bool{colunas[0].Titulo: true, colunas[1].Titulo: true}
	if !nomes["A fazer"] || !nomes["Fazendo"] {
		t.Errorf("os títulos mudaram no reparo: %v", nomes)
	}
}

// Um quadro por transação, e não tudo numa só: o lock é da linha do quadro, e
// uma transação única pararia a escrita em todos os quadros afetados ao mesmo
// tempo.
func TestReparoAbreUmaTransacaoPorQuadro(t *testing.T) {
	q := novoQuadro()
	atomica, _ := comOutbox(q)

	primeiro := q.criarQuadro(t, "ana", "Um")
	colunaUm := q.criarColuna(t, primeiro, "ana", "A fazer")
	segundo := q.criarQuadro(t, "ana", "Dois")
	colunaDois := q.criarColuna(t, segundo, "ana", "A fazer")

	leitor := &duplicidadesFalsas{
		antes: []ucboard.Duplicidade{
			{BoardID: primeiro, ColunaID: colunaUm, Chave: "m", Ocorrencias: 2},
			{BoardID: segundo, ColunaID: colunaDois, Chave: "m", Ocorrencias: 2},
		},
	}

	antes := atomica.mudancasAplicadas
	relatorio, err := ucboard.NovoReparoDeOrdenacao(leitor, atomica).
		Executar(context.Background(), "reparo-de-ordenacao")
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(relatorio.QuadrosReparados) != 2 {
		t.Fatalf("quadros reparados = %v, esperado 2", relatorio.QuadrosReparados)
	}
	if atomica.mudancasAplicadas-antes != 2 {
		t.Errorf("transações = %d, esperado uma por quadro", atomica.mudancasAplicadas-antes)
	}
}

// Duplicidade que SOBRA depois do reparo faz o comando dizer que não está
// limpo. É o que impede o contract de ser aplicado numa janela de manutenção
// achando que a pré-condição foi satisfeita.
func TestReparoQueNaoLimpaTudoRelataAsRestantes(t *testing.T) {
	q := novoQuadro()
	atomica, _ := comOutbox(q)

	boardID := q.criarQuadro(t, "ana", "Estudos")
	colunaID := q.criarColuna(t, boardID, "ana", "A fazer")

	restante := ucboard.Duplicidade{BoardID: boardID, ColunaID: colunaID, Chave: "m", Ocorrencias: 2}
	leitor := &duplicidadesFalsas{
		antes:  []ucboard.Duplicidade{restante},
		depois: []ucboard.Duplicidade{restante},
	}

	relatorio, err := ucboard.NovoReparoDeOrdenacao(leitor, atomica).
		Executar(context.Background(), "reparo-de-ordenacao")
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if relatorio.Limpo() {
		t.Error("o relatório diz que está limpo com duplicidade restante")
	}
	if len(relatorio.Restantes) != 1 {
		t.Errorf("restantes = %d, esperado 1", len(relatorio.Restantes))
	}
}

// Falha ao consultar não vira "nada a fazer": um erro de leitura que passasse
// por limpo autorizaria o contract sobre um banco que ninguém conferiu.
func TestReparoPropagaFalhaDaConsulta(t *testing.T) {
	q := novoQuadro()
	atomica, _ := comOutbox(q)
	quebrou := errors.New("banco fora do ar")

	_, err := ucboard.NovoReparoDeOrdenacao(&duplicidadesFalsas{erro: quebrou}, atomica).
		Executar(context.Background(), "reparo-de-ordenacao")
	if !errors.Is(err, quebrou) {
		t.Errorf("erro = %v, esperado o da consulta", err)
	}
}
