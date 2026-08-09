// Arrastar e soltar. O que estes testes cobrem é a promessa que faz a coisa
// funcionar: mover UM card escreve UMA posição, e a ordem que sai do
// repositório é a que a pessoa viu na tela.
package usecase_test

import (
	"context"
	"errors"
	"testing"

	dcoluna "stacktrack/internal/domain/coluna"
	"stacktrack/internal/domain/membro"
	ucboard "stacktrack/internal/usecase/board"
)

// ordemDaColuna devolve os títulos dos cards de uma coluna, na ordem em que o
// repositório os entrega — que é a ordem que a tela mostra.
func (q *quadro) ordemDaColuna(t *testing.T, boardID, colunaID, usuarioID string) []string {
	t.Helper()
	detalhado, err := q.quadros.Detalhar(context.Background(), boardID, usuarioID)
	if err != nil {
		t.Fatalf("erro ao detalhar: %v", err)
	}
	for _, cc := range detalhado.Colunas {
		if cc.Coluna.ID != colunaID {
			continue
		}
		titulos := make([]string, 0, len(cc.Cards))
		for _, c := range cc.Cards {
			titulos = append(titulos, c.Card.Titulo)
		}
		return titulos
	}
	t.Fatalf("coluna %s não encontrada", colunaID)
	return nil
}

func TestMoverCardParaOMeioDaMesmaColuna(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	col := q.criarColuna(t, boardID, "ana", "A fazer")
	a := q.criarCard(t, col, "ana", "A")
	b := q.criarCard(t, col, "ana", "B")
	c := q.criarCard(t, col, "ana", "C")

	// arrasta C para entre A e B
	if _, err := q.card.Mover(context.Background(), c, "ana", col, ucboard.Vizinhos{AnteriorID: a, ProximoID: b}); err != nil {
		t.Fatalf("erro ao mover: %v", err)
	}

	if got := q.ordemDaColuna(t, boardID, col, "ana"); got[0] != "A" || got[1] != "C" || got[2] != "B" {
		t.Errorf("ordem = %v, esperado [A C B]", got)
	}
}

// A propriedade central da ordenação fracionária: os vizinhos não são tocados.
// Com posição inteira sequencial, mover um item reescreveria todos abaixo dele.
func TestMoverEscreveApenasOCardMovido(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	col := q.criarColuna(t, boardID, "ana", "A fazer")
	a := q.criarCard(t, col, "ana", "A")
	b := q.criarCard(t, col, "ana", "B")
	c := q.criarCard(t, col, "ana", "C")

	antesA, _ := q.cards.BuscarPorID(context.Background(), a)
	antesB, _ := q.cards.BuscarPorID(context.Background(), b)

	q.card.Mover(context.Background(), c, "ana", col, ucboard.Vizinhos{AnteriorID: a, ProximoID: b})

	depoisA, _ := q.cards.BuscarPorID(context.Background(), a)
	depoisB, _ := q.cards.BuscarPorID(context.Background(), b)
	if depoisA.Posicao != antesA.Posicao || depoisB.Posicao != antesB.Posicao {
		t.Errorf("as posições dos vizinhos mudaram: A %v→%v, B %v→%v",
			antesA.Posicao, depoisA.Posicao, antesB.Posicao, depoisB.Posicao)
	}
	// e a versão dos vizinhos também não: eles não foram editados
	if depoisA.Version != antesA.Version {
		t.Errorf("a versão do vizinho subiu: %d → %d", antesA.Version, depoisA.Version)
	}
}

func TestMoverCardEntreColunas(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	origem := q.criarColuna(t, boardID, "ana", "A fazer")
	destino := q.criarColuna(t, boardID, "ana", "Fazendo")
	card := q.criarCard(t, origem, "ana", "Tarefa")

	movido, err := q.card.Mover(context.Background(), card, "ana", destino, ucboard.Vizinhos{})
	if err != nil {
		t.Fatalf("erro ao mover: %v", err)
	}

	if movido.ColunaID != destino {
		t.Errorf("coluna = %q, esperada a de destino", movido.ColunaID)
	}
	if len(q.ordemDaColuna(t, boardID, origem, "ana")) != 0 {
		t.Error("o card devia ter saído da coluna de origem")
	}
	if got := q.ordemDaColuna(t, boardID, destino, "ana"); len(got) != 1 || got[0] != "Tarefa" {
		t.Errorf("coluna de destino = %v", got)
	}
}

// Soltar num espaço vazio abaixo dos cards significa "no fim", e não "no
// começo" — sem vizinho de nenhum lado, a posição vai depois do último.
func TestSemVizinhosOCardVaiParaOFim(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	origem := q.criarColuna(t, boardID, "ana", "A fazer")
	destino := q.criarColuna(t, boardID, "ana", "Fazendo")
	q.criarCard(t, destino, "ana", "Já estava")
	card := q.criarCard(t, origem, "ana", "Chegando")

	q.card.Mover(context.Background(), card, "ana", destino, ucboard.Vizinhos{})

	if got := q.ordemDaColuna(t, boardID, destino, "ana"); got[1] != "Chegando" {
		t.Errorf("ordem = %v, esperado o card novo no fim", got)
	}
}

// Sem essa checagem, quem participa de dois quadros arrastaria um card de um
// para o outro informando o id da coluna de destino — e o card sumiria da vista
// de quem participa só do quadro de origem.
func TestNaoDaParaMoverCardParaColunaDeOutroQuadro(t *testing.T) {
	q := novoQuadro()
	boardA := q.criarQuadro(t, "ana", "Quadro A")
	colA := q.criarColuna(t, boardA, "ana", "A fazer")
	card := q.criarCard(t, colA, "ana", "Tarefa")

	boardB := q.criarQuadro(t, "ana", "Quadro B")
	colB := q.criarColuna(t, boardB, "ana", "Outra")

	_, err := q.card.Mover(context.Background(), card, "ana", colB, ucboard.Vizinhos{})

	if !errors.Is(err, dcoluna.ErrNaoEncontrada) {
		t.Errorf("erro = %v, esperado ErrNaoEncontrada", err)
	}
	if got := q.ordemDaColuna(t, boardA, colA, "ana"); len(got) != 1 {
		t.Error("o card devia ter ficado onde estava")
	}
}

// Vizinho de outra coluna produziria uma posição que não faz sentido no
// destino, e o card pousaria em lugar nenhum.
func TestVizinhoDeOutraColunaERecusado(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	col1 := q.criarColuna(t, boardID, "ana", "A fazer")
	col2 := q.criarColuna(t, boardID, "ana", "Fazendo")
	cardDeOutra := q.criarCard(t, col2, "ana", "De outra coluna")
	card := q.criarCard(t, col1, "ana", "Tarefa")

	_, err := q.card.Mover(context.Background(), card, "ana", col1, ucboard.Vizinhos{AnteriorID: cardDeOutra})

	if err == nil {
		t.Error("vizinho de outra coluna devia ser recusado")
	}
}

func TestLeitorNaoMoveNada(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	col := q.criarColuna(t, boardID, "ana", "A fazer")
	card := q.criarCard(t, col, "ana", "Tarefa")
	q.convidar(t, boardID, "bob", membro.PapelLeitor)

	if _, err := q.card.Mover(context.Background(), card, "bob", col, ucboard.Vizinhos{}); !errors.Is(err, membro.ErrSemPermissao) {
		t.Errorf("card: erro = %v", err)
	}
	if _, err := q.coluna.Mover(context.Background(), col, "bob", ucboard.Vizinhos{}); !errors.Is(err, membro.ErrSemPermissao) {
		t.Errorf("coluna: erro = %v", err)
	}
}

func TestQuemNaoParticipaNaoMove(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	col := q.criarColuna(t, boardID, "ana", "A fazer")
	card := q.criarCard(t, col, "ana", "Tarefa")

	_, err := q.card.Mover(context.Background(), card, "bob", col, ucboard.Vizinhos{})

	if errors.Is(err, membro.ErrSemPermissao) {
		t.Error("'sem permissão' confirmaria que o card existe")
	}
	if err == nil {
		t.Fatal("quem não participa não pode mover")
	}
}

func TestMoverColunaReordenaOQuadro(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	a := q.criarColuna(t, boardID, "ana", "A")
	b := q.criarColuna(t, boardID, "ana", "B")
	c := q.criarColuna(t, boardID, "ana", "C")

	// arrasta C para o começo
	if _, err := q.coluna.Mover(context.Background(), c, "ana", ucboard.Vizinhos{ProximoID: a}); err != nil {
		t.Fatalf("erro ao mover coluna: %v", err)
	}

	detalhado, _ := q.quadros.Detalhar(context.Background(), boardID, "ana")
	titulos := []string{
		detalhado.Colunas[0].Coluna.Titulo,
		detalhado.Colunas[1].Coluna.Titulo,
		detalhado.Colunas[2].Coluna.Titulo,
	}
	if titulos[0] != "C" || titulos[1] != "A" || titulos[2] != "B" {
		t.Errorf("ordem = %v, esperado [C A B]", titulos)
	}
	_ = b
}

// O que ANTES falhava com 409 agora passa.
//
// Este teste afirmava o contrário até a fase 9: com a posição em float, dois
// vizinhos colados não comportavam mais ninguém, e o movimento era recusado com
// ErrSemEspaco — o erro que a pessoa não tinha como resolver pela interface.
//
// Agora quem ordena é a chave textual, e entre duas chaves sempre cabe outra. A
// posição continua sendo gravada enquanto o expand não termina, mas o
// esgotamento dela deixou de barrar o movimento: se barrasse, a falha estaria
// de pé apesar da fase inteira.
func TestMovimentoEntreVizinhosColadosPassaPelaChave(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	col := q.criarColuna(t, boardID, "ana", "A fazer")
	primeiro := q.criarCard(t, col, "ana", "Primeiro")
	segundo := q.criarCard(t, col, "ana", "Segundo")
	movel := q.criarCard(t, col, "ana", "Móvel")

	// Empurra o "Segundo" para a MESMA posição do "Primeiro": no float, não
	// sobra intervalo nenhum entre os dois.
	//
	// Salvar, e não Atualizar: esta é preparação de cenário, gravando um estado
	// que o domínio não produziria. Atualizar exige a versão anterior — é o
	// bloqueio otimista —, e passar por ele aqui faria a preparação falhar em
	// silêncio, deixando o teste verde por não ter montado o caso que queria.
	posInicial, _ := q.cards.BuscarPorID(context.Background(), primeiro)
	alvo, _ := q.cards.BuscarPorID(context.Background(), segundo)
	alvo.Posicao = posInicial.Posicao
	if err := q.cards.Salvar(context.Background(), alvo); err != nil {
		t.Fatalf("preparação do cenário: %v", err)
	}

	depois, err := q.card.Mover(context.Background(), movel, "ana", col,
		ucboard.Vizinhos{AnteriorID: primeiro, ProximoID: segundo})
	if err != nil {
		t.Fatalf("o movimento devia passar pela chave: %v", err)
	}

	// E a chave ficou de fato entre as dos vizinhos.
	if depois.Chave <= posInicial.Chave || depois.Chave >= alvo.Chave {
		t.Errorf("chave = %q, esperado entre %q e %q", depois.Chave, posInicial.Chave, alvo.Chave)
	}
}

// A PROVA DA FASE 9, e o contraste direto com o teste do float acima.
//
// ⚠️ Este teste precisa APERTAR o intervalo — cada movimento entra entre o
// primeiro e o card movido ANTES. Uma versão anterior dele movia sempre entre
// os MESMOS dois vizinhos, o que não aperta nada: o intervalo ficava igual, o
// float nunca esgotava, e o teste passava sem provar coisa alguma. Ele deixou
// passar um defeito real, em que a posição legada ainda abortava o movimento na
// 53ª vez mesmo com a chave tendo espaço de sobra.
//
// Setenta movimentos é o número certo: o float esgota em 52, então qualquer
// coisa acima disso exercita o caminho que só a chave textual sustenta.
func TestApertarOMesmoIntervaloAlemDoLimiteDoFloat(t *testing.T) {
	q := novoQuadro()
	ana := "u-ana"
	boardID := q.criarQuadro(t, ana, "Estudos")
	colunaID := q.criarColuna(t, boardID, ana, "A fazer")

	primeiro := q.criarCard(t, colunaID, ana, "primeiro")
	proximo := q.criarCard(t, colunaID, ana, "ancora")

	moveis := make([]string, 0, 70)
	for i := 0; i < 70; i++ {
		moveis = append(moveis, q.criarCard(t, colunaID, ana, "c"))
	}

	for i, cid := range moveis {
		_, err := q.card.Mover(context.Background(), cid, ana, colunaID,
			ucboard.Vizinhos{AnteriorID: primeiro, ProximoID: proximo})
		if err != nil {
			t.Fatalf("no movimento %d: %v — a chave textual não devia esgotar", i+1, err)
		}
		// O próximo aperto é entre o primeiro e ESTE: é isso que encolhe o
		// intervalo a cada volta.
		proximo = cid
	}

	// E a ordem continua correta depois de tudo isso.
	cards, err := q.cards.ListarDoBoard(context.Background(), boardID)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	chaves := make([]string, 0, len(cards))
	for _, c := range cards {
		chaves = append(chaves, c.Chave)
	}
	for i := 1; i < len(chaves); i++ {
		if chaves[i-1] >= chaves[i] {
			t.Fatalf("a ordem se perdeu entre as posições %d e %d: %q >= %q",
				i-1, i, chaves[i-1], chaves[i])
		}
	}
}
