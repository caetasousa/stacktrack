// A autorização é o assunto desta fase, e é o que estes testes mais exercitam:
// quem não participa do quadro não o enxerga, e quem participa só faz o que o
// papel permite.
package usecase_test

import (
	"context"
	"errors"
	"testing"

	dboard "stacktrack/internal/domain/board"
	dcard "stacktrack/internal/domain/card"
	dcoluna "stacktrack/internal/domain/coluna"
	"stacktrack/internal/domain/membro"
	ucboard "stacktrack/internal/usecase/board"
	"stacktrack/test/repository/memoria"
)

type quadro struct {
	boards       *memoria.Boards
	membros      *memoria.Membros
	colunas      *memoria.Colunas
	cards        *memoria.Cards
	etiquetas    *memoria.Etiquetas
	checklists   *memoria.Checklists
	anexos       *memoria.Anexos
	responsaveis *memoria.Responsaveis
	comentarios  *memoria.Comentarios
	atividades   *memoria.Atividades
	publicacoes  *memoria.Publicacoes
	armazem      *memoria.Armazem
	quadros      *ucboard.QuadroUseCase
	coluna       *ucboard.ColunaUseCase
	card         *ucboard.CardUseCase
	publicacao   *ucboard.PublicacaoUseCase
}

func novoQuadro() *quadro {
	membros := memoria.NovosMembros()
	cards := memoria.NovosCards()
	colunas := memoria.NovasColunas(cards)
	cards.LigarColunas(colunas)
	boards := memoria.NovosBoards(membros)
	etiquetas := memoria.NovasEtiquetas()
	checklists := memoria.NovasChecklists()
	anexos := memoria.NovosAnexos()
	responsaveis := memoria.NovosResponsaveis()
	comentarios := memoria.NovosComentarios()
	atividades := memoria.NovasAtividades()
	publicacoes := memoria.NovasPublicacoes()
	boards.LigarPublicacoes(publicacoes)
	// O armazém entra até nos testes que não mexem em anexo: apagar limpa o
	// volume, e sem ele o card apagado deixaria arquivo para trás.
	armazem := memoria.NovoArmazem()
	etiquetas.LigarQuadro(colunas, cards)
	checklists.LigarQuadro(colunas, cards)
	anexos.LigarQuadro(colunas, cards)
	responsaveis.LigarQuadro(colunas, cards)
	comentarios.LigarQuadro(colunas, cards)

	// A consulta do link público entra ligada, como em produção: sem isso todo
	// quadro se descreveria como privado nos testes, e a tela que avisa "este
	// quadro está público" nunca seria exercitada.
	quadros := ucboard.NovoQuadroUseCase(boards, membros, colunas, cards, etiquetas, checklists, anexos, responsaveis, comentarios, armazem)
	quadros.ComPublicacoes(publicacoes)
	// O selo de "quem moveu por último" lê o mesmo log. Ligá-lo aqui só resolve
	// a LEITURA: quem grava é o usecase de card, e ele só passa a registrar
	// eventos com ComRegistro — o que a fixture `historico` faz. Nos testes que
	// não ligam a escrita, o selo vem vazio, que é o correto: sem evento
	// gravado, não houve movimentação de que falar.
	quadros.ComAtividades(atividades)

	return &quadro{
		boards:       boards,
		membros:      membros,
		colunas:      colunas,
		cards:        cards,
		etiquetas:    etiquetas,
		checklists:   checklists,
		anexos:       anexos,
		responsaveis: responsaveis,
		comentarios:  comentarios,
		atividades:   atividades,
		publicacoes:  publicacoes,
		armazem:      armazem,
		quadros:      quadros,
		publicacao:   ucboard.NovoPublicacaoUseCase(publicacoes, membros, boards, colunas, cards, etiquetas, checklists),
		coluna:       ucboard.NovoColunaUseCase(membros, colunas, anexos, armazem),
		card:         ucboard.NovoCardUseCase(boards, membros, colunas, cards, etiquetas, checklists, anexos, responsaveis, comentarios, armazem),
	}
}

// convidar cria o vínculo direto no repositório — a fase 3 é que traz o convite
// de verdade; aqui ele só existe para exercitar os papéis.
func (q *quadro) convidar(t *testing.T, boardID, usuarioID string, papel membro.Papel) {
	t.Helper()
	m, err := membro.Novo(boardID, usuarioID, papel)
	if err != nil {
		t.Fatalf("vínculo inválido: %v", err)
	}
	if err := q.membros.Salvar(context.Background(), m); err != nil {
		t.Fatalf("erro ao salvar vínculo: %v", err)
	}
}

func (q *quadro) criarQuadro(t *testing.T, usuarioID, titulo string) string {
	t.Helper()
	b, err := q.quadros.Criar(context.Background(), usuarioID, titulo)
	if err != nil {
		t.Fatalf("erro ao criar quadro: %v", err)
	}
	return b.ID
}

func (q *quadro) criarColuna(t *testing.T, boardID, usuarioID, titulo string) string {
	t.Helper()
	c, err := q.coluna.Criar(context.Background(), boardID, usuarioID, titulo, "")
	if err != nil {
		t.Fatalf("erro ao criar coluna: %v", err)
	}
	return c.ID
}

func (q *quadro) criarCard(t *testing.T, colunaID, usuarioID, titulo string) string {
	t.Helper()
	c, err := q.card.Criar(context.Background(), colunaID, usuarioID, titulo, "", "")
	if err != nil {
		t.Fatalf("erro ao criar card: %v", err)
	}
	return c.ID
}

func TestCriarQuadroVinculaQuemCriouComoDono(t *testing.T) {
	q := novoQuadro()

	boardID := q.criarQuadro(t, "ana", "Estudos")

	vinculo, _ := q.membros.Buscar(context.Background(), boardID, "ana")
	if vinculo == nil {
		t.Fatal("quem criou o quadro devia ter virado membro")
	}
	if vinculo.Papel != membro.PapelDono {
		t.Errorf("papel = %q, esperado dono", vinculo.Papel)
	}
}

func TestListarSoTrazOsQuadrosDeQuemPede(t *testing.T) {
	q := novoQuadro()
	q.criarQuadro(t, "ana", "Da Ana")
	q.criarQuadro(t, "bob", "Do Bob")

	daAna, err := q.quadros.Listar(context.Background(), "ana")
	if err != nil {
		t.Fatalf("erro ao listar: %v", err)
	}

	if len(daAna) != 1 || daAna[0].Board.Titulo != "Da Ana" {
		t.Errorf("listagem da Ana = %+v, esperado só o quadro dela", daAna)
	}
}

func TestListarDeQuemNaoTemQuadroVemVazioENaoNulo(t *testing.T) {
	q := novoQuadro()

	lista, err := q.quadros.Listar(context.Background(), "ninguem")
	if err != nil {
		t.Fatalf("erro ao listar: %v", err)
	}
	if lista == nil {
		t.Error("a listagem vazia precisa ser slice vazia, não nil — o JSON viraria null")
	}
	if len(lista) != 0 {
		t.Errorf("listagem = %+v, esperado vazia", lista)
	}
}

// Quem não participa recebe "não encontrado", e NÃO "proibido": responder 403
// confirmaria que o quadro existe, e isso basta para varrer ids e mapear o que
// os outros têm.
func TestQuadroDeTerceiroRespondeNaoEncontradoENaoProibido(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Da Ana")

	_, err := q.quadros.Detalhar(context.Background(), boardID, "bob")

	if !errors.Is(err, dboard.ErrNaoEncontrado) {
		t.Errorf("erro = %v, esperado ErrNaoEncontrado", err)
	}
	if errors.Is(err, membro.ErrSemPermissao) {
		t.Error("responder 'sem permissão' revelaria que o quadro existe")
	}
}

func TestDetalharTrazColunasECardsEmOrdem(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	primeira := q.criarColuna(t, boardID, "ana", "A fazer")
	q.criarColuna(t, boardID, "ana", "Fazendo")
	q.criarCard(t, primeira, "ana", "Card 1")
	q.criarCard(t, primeira, "ana", "Card 2")

	detalhado, err := q.quadros.Detalhar(context.Background(), boardID, "ana")
	if err != nil {
		t.Fatalf("erro ao detalhar: %v", err)
	}

	if len(detalhado.Colunas) != 2 {
		t.Fatalf("colunas = %d, esperado 2", len(detalhado.Colunas))
	}
	if detalhado.Colunas[0].Coluna.Titulo != "A fazer" || detalhado.Colunas[1].Coluna.Titulo != "Fazendo" {
		t.Errorf("ordem das colunas = %q, %q", detalhado.Colunas[0].Coluna.Titulo, detalhado.Colunas[1].Coluna.Titulo)
	}
	if len(detalhado.Colunas[0].Cards) != 2 {
		t.Fatalf("cards da primeira coluna = %d, esperado 2", len(detalhado.Colunas[0].Cards))
	}
	if detalhado.Colunas[0].Cards[0].Card.Titulo != "Card 1" {
		t.Errorf("o card criado primeiro devia vir primeiro, veio %q", detalhado.Colunas[0].Cards[0].Card.Titulo)
	}
	if detalhado.Papel != membro.PapelDono {
		t.Errorf("papel = %q, esperado dono", detalhado.Papel)
	}
}

// Coluna sem card precisa sair como slice vazia: em JSON, nil vira null e o
// frontend teria de tratar os dois casos em toda coluna.
func TestColunaSemCardsVemComoSliceVazia(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	q.criarColuna(t, boardID, "ana", "A fazer")

	detalhado, _ := q.quadros.Detalhar(context.Background(), boardID, "ana")

	if detalhado.Colunas[0].Cards == nil {
		t.Error("os cards de uma coluna vazia precisam ser slice vazia, não nil")
	}
}

func TestLeitorEnxergaMasNaoEdita(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	q.convidar(t, boardID, "bob", membro.PapelLeitor)

	if _, err := q.quadros.Detalhar(context.Background(), boardID, "bob"); err != nil {
		t.Errorf("o leitor devia enxergar o quadro: %v", err)
	}

	// Quem já enxerga o quadro recebe 'sem permissão' — a recusa não revela
	// nada que ele ainda não saiba.
	if _, err := q.coluna.Criar(context.Background(), boardID, "bob", "Nova", ""); !errors.Is(err, membro.ErrSemPermissao) {
		t.Errorf("erro = %v, esperado ErrSemPermissao", err)
	}
}

func TestEditorMexeNoConteudoMasNaoNoQuadro(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	q.convidar(t, boardID, "bob", membro.PapelEditor)

	if _, err := q.coluna.Criar(context.Background(), boardID, "bob", "Nova", ""); err != nil {
		t.Errorf("o editor devia poder criar coluna: %v", err)
	}
	if _, err := q.quadros.Renomear(context.Background(), boardID, "bob", "Outro nome"); !errors.Is(err, membro.ErrSemPermissao) {
		t.Errorf("erro = %v, esperado ErrSemPermissao ao renomear o quadro", err)
	}
	if err := q.quadros.Apagar(context.Background(), boardID, "bob"); !errors.Is(err, membro.ErrSemPermissao) {
		t.Errorf("erro = %v, esperado ErrSemPermissao ao apagar o quadro", err)
	}
}

// Nem o id da coluna vindo da URL pode ser aceito sem checar de quem ela é: o
// usecase resolve coluna → quadro antes de autorizar.
func TestNaoDaParaMexerEmColunaDeQuadroAlheio(t *testing.T) {
	q := novoQuadro()
	daAna := q.criarQuadro(t, "ana", "Da Ana")
	colunaDaAna := q.criarColuna(t, daAna, "ana", "A fazer")
	q.criarQuadro(t, "bob", "Do Bob")

	casos := map[string]error{
		"renomear": func() error {
			_, err := q.coluna.Renomear(context.Background(), colunaDaAna, "bob", "Invadida", "")
			return err
		}(),
		"apagar": q.coluna.Apagar(context.Background(), colunaDaAna, "bob"),
		"criar card": func() error {
			_, err := q.card.Criar(context.Background(), colunaDaAna, "bob", "Invasor", "", "")
			return err
		}(),
	}

	for nome, err := range casos {
		t.Run(nome, func(t *testing.T) {
			if !errors.Is(err, dcoluna.ErrNaoEncontrada) {
				t.Errorf("erro = %v, esperado ErrNaoEncontrada", err)
			}
		})
	}
	if q.colunas.Quantidade() != 1 {
		t.Error("a coluna da Ana não podia ter sido apagada")
	}
}

// O caminho card → coluna → quadro é o que faz a autorização valer para o card,
// que não guarda o quadro a que pertence.
func TestNaoDaParaMexerEmCardDeQuadroAlheio(t *testing.T) {
	q := novoQuadro()
	daAna := q.criarQuadro(t, "ana", "Da Ana")
	colunaDaAna := q.criarColuna(t, daAna, "ana", "A fazer")
	cardDaAna := q.criarCard(t, colunaDaAna, "ana", "Segredo")

	if _, err := q.card.Editar(context.Background(), cardDaAna, "bob", "Invadido", "", "", 0); !errors.Is(err, dcard.ErrNaoEncontrado) {
		t.Errorf("erro = %v, esperado ErrNaoEncontrado", err)
	}
	if err := q.card.Apagar(context.Background(), cardDaAna, "bob"); !errors.Is(err, dcard.ErrNaoEncontrado) {
		t.Errorf("erro = %v, esperado ErrNaoEncontrado", err)
	}
	if q.cards.Quantidade() != 1 {
		t.Error("o card da Ana não podia ter sido apagado")
	}
}

func TestApagarQuadroLevaColunasECardsJunto(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	colunaID := q.criarColuna(t, boardID, "ana", "A fazer")
	q.criarCard(t, colunaID, "ana", "Tarefa")

	if err := q.quadros.Apagar(context.Background(), boardID, "ana"); err != nil {
		t.Fatalf("erro ao apagar: %v", err)
	}

	lista, _ := q.quadros.Listar(context.Background(), "ana")
	if len(lista) != 0 {
		t.Errorf("o quadro devia ter sumido da listagem: %+v", lista)
	}
	if _, err := q.quadros.Detalhar(context.Background(), boardID, "ana"); !errors.Is(err, dboard.ErrNaoEncontrado) {
		t.Errorf("erro = %v, esperado ErrNaoEncontrado", err)
	}
}

func TestApagarColunaLevaOsCardsDela(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	colunaID := q.criarColuna(t, boardID, "ana", "A fazer")
	q.criarCard(t, colunaID, "ana", "Tarefa")

	if err := q.coluna.Apagar(context.Background(), colunaID, "ana"); err != nil {
		t.Fatalf("erro ao apagar coluna: %v", err)
	}

	if q.cards.Quantidade() != 0 {
		t.Errorf("cards restantes = %d, esperado 0", q.cards.Quantidade())
	}
}

func TestCadaColunaNovaVaiParaOFim(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")

	primeira, _ := q.coluna.Criar(context.Background(), boardID, "ana", "A fazer", "")
	segunda, _ := q.coluna.Criar(context.Background(), boardID, "ana", "Fazendo", "")
	terceira, _ := q.coluna.Criar(context.Background(), boardID, "ana", "Pronto", "")

	if !(primeira.Chave < segunda.Chave && segunda.Chave < terceira.Chave) {
		t.Errorf("chaves fora de ordem: %q, %q, %q", primeira.Chave, segunda.Chave, terceira.Chave)
	}
}

// Cada coluna tem a sua sequência de posições: os cards da segunda coluna não
// continuam de onde os da primeira pararam.
func TestPosicaoDosCardsEPorColuna(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	colunaA := q.criarColuna(t, boardID, "ana", "A fazer")
	colunaB := q.criarColuna(t, boardID, "ana", "Fazendo")

	primeiroDeA, _ := q.card.Criar(context.Background(), colunaA, "ana", "A1", "", "")
	q.criarCard(t, colunaA, "ana", "A2")
	primeiroDeB, _ := q.card.Criar(context.Background(), colunaB, "ana", "B1", "", "")

	// Cada coluna tem a PRÓPRIA sequência: a chave do primeiro card de B é
	// calculada sem olhar para A.
	//
	// A afirmação não é de igualdade — as chaves iniciais são sorteadas, e
	// exigir que coincidam travaria o teste no sorteio em vez da regra. O que
	// importa é que a chave exista e que a coluna A tenha ordenado a sua.
	if primeiroDeA.Chave == "" || primeiroDeB.Chave == "" {
		t.Fatalf("chaves iniciais = %q e %q", primeiroDeA.Chave, primeiroDeB.Chave)
	}
	cardsDeA, _ := q.cards.ListarDoBoard(context.Background(), boardID)
	var deA []string
	for _, c := range cardsDeA {
		if c.ColunaID == colunaA {
			deA = append(deA, c.Chave)
		}
	}
	if len(deA) != 2 || deA[0] >= deA[1] {
		t.Errorf("a coluna A não ficou ordenada: %v", deA)
	}
}

func TestEditarCardTrocaOTextoESobeAVersao(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	colunaID := q.criarColuna(t, boardID, "ana", "A fazer")
	cardID := q.criarCard(t, colunaID, "ana", "Rascunho")

	editado, err := q.card.Editar(context.Background(), cardID, "ana", "Definitivo", "com descrição", "", 0)
	if err != nil {
		t.Fatalf("erro ao editar: %v", err)
	}

	if editado.Titulo != "Definitivo" || editado.Descricao != "com descrição" {
		t.Errorf("card = %+v", editado)
	}
	if editado.Version != 2 {
		t.Errorf("Version = %d, esperado 2", editado.Version)
	}
}

func TestOperacoesEmRecursoInexistenteRespondemNaoEncontrado(t *testing.T) {
	q := novoQuadro()

	if _, err := q.quadros.Detalhar(context.Background(), "quadro-que-nao-existe", "ana"); !errors.Is(err, dboard.ErrNaoEncontrado) {
		t.Errorf("quadro: erro = %v", err)
	}
	if _, err := q.coluna.Renomear(context.Background(), "coluna-que-nao-existe", "ana", "x", ""); !errors.Is(err, dcoluna.ErrNaoEncontrada) {
		t.Errorf("coluna: erro = %v", err)
	}
	if _, err := q.card.Editar(context.Background(), "card-que-nao-existe", "ana", "x", "", "", 0); !errors.Is(err, dcard.ErrNaoEncontrado) {
		t.Errorf("card: erro = %v", err)
	}
}

// Falha de infraestrutura não pode ser confundida com "não encontrado": um
// banco fora do ar responderia 404 e o defeito passaria por dado ausente.
func TestFalhaDeInfraestruturaNaoViraNaoEncontrado(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	falha := errors.New("conexão recusada")
	q.membros.ErroForcado = falha

	_, err := q.quadros.Detalhar(context.Background(), boardID, "ana")

	if !errors.Is(err, falha) {
		t.Errorf("erro = %v, esperado a falha de infraestrutura", err)
	}
	if errors.Is(err, dboard.ErrNaoEncontrado) {
		t.Error("falha de infraestrutura não pode ser reportada como 'não encontrado'")
	}
}
