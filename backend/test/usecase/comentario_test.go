// A conversa de um card.
//
// O que estes testes mais exercitam é a assimetria entre editar e apagar:
// qualquer participante comenta, só o autor edita, e quem administra o quadro
// apaga o de qualquer um. São três regras diferentes, e confundi-las deixaria
// alguém reescrevendo a fala de outra pessoa.
package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	dcomentario "stacktrack/internal/domain/comentario"
	"stacktrack/internal/domain/membro"
	ucboard "stacktrack/internal/usecase/board"
)

type conversa struct {
	*colaboracao
	comentarioUC *ucboard.ComentarioUseCase
}

func novaConversa(t *testing.T) *conversa {
	t.Helper()
	c := novaColaboracao(t)
	c.comentarios.LigarUsuarios(c.usuarios)
	return &conversa{
		colaboracao:  c,
		comentarioUC: ucboard.NovoComentarioUseCase(c.membros, c.colunas, c.cards, c.comentarios),
	}
}

func (c *conversa) cardDe(t *testing.T, dono string) (boardID, cardID string) {
	t.Helper()
	boardID = c.criarQuadro(t, dono, "Estudos")
	colunaID := c.criarColuna(t, boardID, dono, "A fazer")
	return boardID, c.criarCard(t, colunaID, dono, "Migração")
}

func TestComentarioApareceNaConversa(t *testing.T) {
	c := novaConversa(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	_, cardID := c.cardDe(t, ana)

	if _, err := c.comentarioUC.Criar(context.Background(), cardID, ana, "  Começando por aqui  "); err != nil {
		t.Fatalf("comentar: %v", err)
	}

	lista, err := c.comentarioUC.Listar(context.Background(), cardID, ana)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(lista) != 1 {
		t.Fatalf("comentários = %d, esperado 1", len(lista))
	}
	// O texto é aparado nas pontas: sobra em branco vem de copiar e colar.
	if lista[0].Comentario.Texto != "Começando por aqui" {
		t.Errorf("texto = %q", lista[0].Comentario.Texto)
	}
	if lista[0].AutorNome != "Ana" {
		t.Errorf("autor = %q, esperado Ana — a conversa precisa do nome de quem falou", lista[0].AutorNome)
	}
	if lista[0].Comentario.EditadoEm != nil {
		t.Error("comentário novo nasceu marcado como editado")
	}
}

func TestComentarioVazioERecusado(t *testing.T) {
	c := novaConversa(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	_, cardID := c.cardDe(t, ana)

	_, err := c.comentarioUC.Criar(context.Background(), cardID, ana, "   \n  ")
	if !errors.Is(err, dcomentario.ErrTextoObrigatorio) {
		t.Errorf("erro = %v, esperado ErrTextoObrigatorio", err)
	}
}

func TestComentarioLongoDemaisERecusado(t *testing.T) {
	c := novaConversa(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	_, cardID := c.cardDe(t, ana)

	_, err := c.comentarioUC.Criar(context.Background(), cardID, ana,
		strings.Repeat("a", dcomentario.TamanhoMaximoTexto+1))
	if !errors.Is(err, dcomentario.ErrTextoLongo) {
		t.Errorf("erro = %v, esperado ErrTextoLongo", err)
	}
}

// Comentar exige PARTICIPAÇÃO, não papel de edição: acompanhar e responder é
// ver, não mexer. Um leitor que não pudesse comentar não teria como revisar.
func TestLeitorComenta(t *testing.T) {
	c := novaConversa(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	boardID, cardID := c.cardDe(t, ana)

	leitor := c.conta(t, "Bruno", "bruno@exemplo.com")
	c.convidar(t, boardID, leitor, membro.PapelLeitor)

	if _, err := c.comentarioUC.Criar(context.Background(), cardID, leitor, "Revisei, está certo"); err != nil {
		t.Fatalf("o leitor não conseguiu comentar: %v", err)
	}
}

func TestEstranhoNaoLeNemComenta(t *testing.T) {
	c := novaConversa(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	_, cardID := c.cardDe(t, ana)
	estranho := c.conta(t, "Bruno", "bruno@exemplo.com")

	if _, err := c.comentarioUC.Listar(context.Background(), cardID, estranho); err == nil {
		t.Error("quem não participa leu a conversa")
	}
	if _, err := c.comentarioUC.Criar(context.Background(), cardID, estranho, "oi"); err == nil {
		t.Error("quem não participa comentou")
	}
}

func TestAutorEditaOProprioComentario(t *testing.T) {
	c := novaConversa(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	_, cardID := c.cardDe(t, ana)

	criado, err := c.comentarioUC.Criar(context.Background(), cardID, ana, "primeira versão")
	if err != nil {
		t.Fatalf("comentar: %v", err)
	}

	editado, err := c.comentarioUC.Editar(context.Background(), criado.ID, ana, "segunda versão")
	if err != nil {
		t.Fatalf("editar: %v", err)
	}
	if editado.Texto != "segunda versão" {
		t.Errorf("texto = %q", editado.Texto)
	}
	// A marca de edição é o que deixa a tela dizer "editado" sem comparar datas.
	if editado.EditadoEm == nil {
		t.Error("editar não marcou editadoEm")
	}
}

// A regra que separa editar de apagar: nem o dono do quadro reescreve a fala de
// outra pessoa.
func TestNemODonoEditaComentarioAlheio(t *testing.T) {
	c := novaConversa(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	boardID, cardID := c.cardDe(t, ana)

	bruno := c.conta(t, "Bruno", "bruno@exemplo.com")
	c.convidar(t, boardID, bruno, membro.PapelEditor)

	doBruno, err := c.comentarioUC.Criar(context.Background(), cardID, bruno, "o que o bruno disse")
	if err != nil {
		t.Fatalf("comentar: %v", err)
	}

	_, err = c.comentarioUC.Editar(context.Background(), doBruno.ID, ana, "o que a ana queria que ele dissesse")
	if !errors.Is(err, dcomentario.ErrNaoEhAutor) {
		t.Fatalf("erro = %v, esperado ErrNaoEhAutor", err)
	}

	lista, _ := c.comentarioUC.Listar(context.Background(), cardID, ana)
	if lista[0].Comentario.Texto != "o que o bruno disse" {
		t.Errorf("o texto foi alterado assim mesmo: %q", lista[0].Comentario.Texto)
	}
}

// Mas apagar o dono pode: é dele a responsabilidade pelo que fica no quadro.
func TestDonoApagaComentarioAlheio(t *testing.T) {
	c := novaConversa(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	boardID, cardID := c.cardDe(t, ana)

	bruno := c.conta(t, "Bruno", "bruno@exemplo.com")
	c.convidar(t, boardID, bruno, membro.PapelEditor)

	doBruno, err := c.comentarioUC.Criar(context.Background(), cardID, bruno, "texto a remover")
	if err != nil {
		t.Fatalf("comentar: %v", err)
	}

	if err := c.comentarioUC.Apagar(context.Background(), doBruno.ID, ana); err != nil {
		t.Fatalf("o dono não conseguiu apagar: %v", err)
	}
	if lista, _ := c.comentarioUC.Listar(context.Background(), cardID, ana); len(lista) != 0 {
		t.Errorf("o comentário sobrou: %#v", lista)
	}
}

// E um editor qualquer não apaga o dos outros: editar o quadro não é
// administrar a conversa.
func TestEditorNaoApagaComentarioAlheio(t *testing.T) {
	c := novaConversa(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	boardID, cardID := c.cardDe(t, ana)

	bruno := c.conta(t, "Bruno", "bruno@exemplo.com")
	c.convidar(t, boardID, bruno, membro.PapelEditor)

	daAna, err := c.comentarioUC.Criar(context.Background(), cardID, ana, "texto da ana")
	if err != nil {
		t.Fatalf("comentar: %v", err)
	}

	err = c.comentarioUC.Apagar(context.Background(), daAna.ID, bruno)
	if !errors.Is(err, membro.ErrSemPermissao) {
		t.Errorf("erro = %v, esperado ErrSemPermissao", err)
	}
}

func TestAutorApagaOProprioComentario(t *testing.T) {
	c := novaConversa(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	boardID, cardID := c.cardDe(t, ana)

	bruno := c.conta(t, "Bruno", "bruno@exemplo.com")
	c.convidar(t, boardID, bruno, membro.PapelEditor)

	doBruno, err := c.comentarioUC.Criar(context.Background(), cardID, bruno, "me arrependi")
	if err != nil {
		t.Fatalf("comentar: %v", err)
	}

	if err := c.comentarioUC.Apagar(context.Background(), doBruno.ID, bruno); err != nil {
		t.Fatalf("o autor não conseguiu apagar o próprio: %v", err)
	}
}

// A conversa se lê do mais antigo para o mais novo — é como se lê conversa.
func TestConversaSaiEmOrdemDeTempo(t *testing.T) {
	c := novaConversa(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	_, cardID := c.cardDe(t, ana)

	for _, texto := range []string{"primeiro", "segundo", "terceiro"} {
		if _, err := c.comentarioUC.Criar(context.Background(), cardID, ana, texto); err != nil {
			t.Fatalf("comentar %q: %v", texto, err)
		}
	}

	lista, _ := c.comentarioUC.Listar(context.Background(), cardID, ana)
	if len(lista) != 3 {
		t.Fatalf("comentários = %d", len(lista))
	}
	for i, esperado := range []string{"primeiro", "segundo", "terceiro"} {
		if lista[i].Comentario.Texto != esperado {
			t.Errorf("posição %d = %q, esperado %q", i, lista[i].Comentario.Texto, esperado)
		}
	}
}

// O card mostra quantos comentários tem, e a contagem vem junto com o quadro —
// sem uma consulta por card.
func TestQuadroContaOsComentariosDeCadaCard(t *testing.T) {
	c := novaConversa(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	boardID, cardID := c.cardDe(t, ana)

	for i := 0; i < 2; i++ {
		if _, err := c.comentarioUC.Criar(context.Background(), cardID, ana, "mais um"); err != nil {
			t.Fatalf("comentar: %v", err)
		}
	}

	detalhado, err := c.quadros.Detalhar(context.Background(), boardID, ana)
	if err != nil {
		t.Fatalf("detalhar: %v", err)
	}
	if got := detalhado.Colunas[0].Cards[0].QtdComentarios; got != 2 {
		t.Errorf("qtdComentarios = %d, esperado 2", got)
	}
}
