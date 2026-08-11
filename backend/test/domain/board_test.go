package domain_test

import (
	"errors"
	"strings"
	"testing"

	"stacktrack/internal/domain/board"
	"stacktrack/internal/domain/card"
	"stacktrack/internal/domain/coluna"
	"stacktrack/internal/domain/membro"
	"stacktrack/internal/domain/ordem"
)

func TestNovoQuadroAparaOTitulo(t *testing.T) {
	b, err := board.Novo("id-1", "  Estudos  ")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if b.Titulo != "Estudos" {
		t.Errorf("título = %q, esperado sem espaços nas pontas", b.Titulo)
	}
}

func TestQuadroRecusaTituloInvalido(t *testing.T) {
	casos := map[string]struct {
		titulo   string
		esperado error
	}{
		"vazio":          {"", board.ErrTituloObrigatorio},
		"só espaços":     {"   ", board.ErrTituloObrigatorio},
		"longo demais":   {strings.Repeat("a", board.TamanhoMaximoTitulo+1), board.ErrTituloLongo},
		"no limite vale": {strings.Repeat("a", board.TamanhoMaximoTitulo), nil},
	}

	for nome, caso := range casos {
		t.Run(nome, func(t *testing.T) {
			_, err := board.Novo("id-1", caso.titulo)
			if !errors.Is(err, caso.esperado) {
				t.Errorf("erro = %v, esperado %v", err, caso.esperado)
			}
		})
	}
}

// O limite é contado em caracteres: com len(), 120 letras acentuadas contariam
// como 240 bytes e um título legítimo seria recusado.
func TestLimiteDeTituloContaCaracteresENaoBytes(t *testing.T) {
	if _, err := board.Novo("id-1", strings.Repeat("ã", board.TamanhoMaximoTitulo)); err != nil {
		t.Errorf("título de %d caracteres acentuados devia ser aceito: %v", board.TamanhoMaximoTitulo, err)
	}
}

func TestRenomearQuadroMantemOTituloAntigoQuandoONovoENaoValido(t *testing.T) {
	b, _ := board.Novo("id-1", "Estudos")

	if err := b.Renomear("  "); !errors.Is(err, board.ErrTituloObrigatorio) {
		t.Errorf("erro = %v, esperado ErrTituloObrigatorio", err)
	}
	if b.Titulo != "Estudos" {
		t.Errorf("título = %q, devia ter ficado como estava", b.Titulo)
	}
}

func TestPapeisEOQueCadaUmPode(t *testing.T) {
	casos := map[membro.Papel]struct{ ver, editar, administrar bool }{
		membro.PapelDono:   {true, true, true},
		membro.PapelEditor: {true, true, false},
		membro.PapelLeitor: {true, false, false},
	}

	for papel, esperado := range casos {
		t.Run(string(papel), func(t *testing.T) {
			m := membro.Membro{Papel: papel}
			if m.PodeVer() != esperado.ver {
				t.Errorf("PodeVer = %v, esperado %v", m.PodeVer(), esperado.ver)
			}
			if m.PodeEditar() != esperado.editar {
				t.Errorf("PodeEditar = %v, esperado %v", m.PodeEditar(), esperado.editar)
			}
			if m.PodeAdministrar() != esperado.administrar {
				t.Errorf("PodeAdministrar = %v, esperado %v", m.PodeAdministrar(), esperado.administrar)
			}
		})
	}
}

// Papel desconhecido — dado estragado no banco, ou papel novo que alguém
// esqueceu de tratar — não pode virar permissão por descuido.
func TestPapelDesconhecidoNaoPodeNada(t *testing.T) {
	m := membro.Membro{Papel: membro.Papel("administrador-supremo")}

	if m.PodeVer() || m.PodeEditar() || m.PodeAdministrar() {
		t.Error("papel desconhecido não pode receber permissão nenhuma")
	}
}

func TestNovoMembroRecusaPapelInvalido(t *testing.T) {
	if _, err := membro.Novo("b-1", "u-1", membro.Papel("chefe")); !errors.Is(err, membro.ErrPapelInvalido) {
		t.Errorf("erro = %v, esperado ErrPapelInvalido", err)
	}
}

func TestColunaUsaAMesmaReguaDeTituloDoQuadro(t *testing.T) {
	if _, err := coluna.Nova("id-1", "b-1", "  ", "", ordem.ChaveInicial); !errors.Is(err, board.ErrTituloObrigatorio) {
		t.Errorf("erro = %v, esperado ErrTituloObrigatorio", err)
	}

	c, err := coluna.Nova("id-1", "b-1", "  A fazer ", "", ordem.ChaveInicial)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if c.Titulo != "A fazer" {
		t.Errorf("título = %q", c.Titulo)
	}
}

func TestNovoCardNasceNaVersaoUm(t *testing.T) {
	c, err := card.Novo("id-1", "col-1", "Migração", "", "", ordem.ChaveInicial)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if c.Version != 1 {
		t.Errorf("Version = %d, esperado 1", c.Version)
	}
}

// A versão é o contador do bloqueio otimista da fase 6: se ela não subir a cada
// edição, a checagem que virá depois não terá o que comparar.
func TestEditarCardIncrementaAVersao(t *testing.T) {
	c, _ := card.Novo("id-1", "col-1", "Migração", "", "", ordem.ChaveInicial)

	if err := c.Editar("Migração V2", "com rollback", ""); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if c.Version != 2 {
		t.Errorf("Version = %d, esperado 2", c.Version)
	}
	if c.Titulo != "Migração V2" || c.Descricao != "com rollback" {
		t.Errorf("card = %+v", c)
	}
}

func TestCardExigeTituloMasNaoDescricao(t *testing.T) {
	if _, err := card.Novo("id-1", "col-1", "  ", "descrição", "", ordem.ChaveInicial); !errors.Is(err, card.ErrTituloObrigatorio) {
		t.Errorf("erro = %v, esperado ErrTituloObrigatorio", err)
	}
	if _, err := card.Novo("id-1", "col-1", "Tarefa", "", "", ordem.ChaveInicial); err != nil {
		t.Errorf("descrição vazia devia ser aceita: %v", err)
	}
}

// Título é rótulo curto e ganha trim; descrição é texto livre, onde quebra de
// linha e indentação podem ser intencionais.
func TestDescricaoDoCardNaoEAparada(t *testing.T) {
	c, err := card.Novo("id-1", "col-1", "Tarefa", "\n  passo 1\n  passo 2\n", "", ordem.ChaveInicial)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if c.Descricao != "\n  passo 1\n  passo 2\n" {
		t.Errorf("descrição = %q, devia ter sido preservada", c.Descricao)
	}
}

func TestCardRecusaTextoLongoDemais(t *testing.T) {
	if _, err := card.Novo("id-1", "col-1", strings.Repeat("a", card.TamanhoMaximoTitulo+1), "", "", ordem.ChaveInicial); !errors.Is(err, card.ErrTituloLongo) {
		t.Error("título longo demais devia ser recusado")
	}
	if _, err := card.Novo("id-1", "col-1", "Tarefa", strings.Repeat("a", card.TamanhoMaximoDescricao+1), "", ordem.ChaveInicial); !errors.Is(err, card.ErrDescricaoLonga) {
		t.Error("descrição longa demais devia ser recusada")
	}
}

// --- arquivar (fase 13) -----------------------------------------------------

// cardValido e colunaValida montam o sujeito dos testes de arquivamento, cujo
// assunto não é a validação de título.
func cardValido(t *testing.T) *card.Card {
	t.Helper()
	c, err := card.Novo("id-1", "col-1", "Migração", "", "", ordem.ChaveInicial)
	if err != nil {
		t.Fatalf("montar card: %v", err)
	}
	return c
}

func colunaValida(t *testing.T) *coluna.Coluna {
	t.Helper()
	c, err := coluna.Nova("col-1", "b-1", "A fazer", "", ordem.ChaveInicial)
	if err != nil {
		t.Fatalf("montar coluna: %v", err)
	}
	return c
}

//
// Arquivar existe porque apagar é definitivo: o DELETE de um card leva por
// cascata comentários, checklists, anexos, responsáveis e etiquetas aplicadas,
// e não há de onde trazer nada de volta.

func TestArquivarTiraOCardDoQuadroSemPerderOndeEleEstava(t *testing.T) {
	c := cardValido(t)
	colunaAntes, chaveAntes := c.ColunaID, c.Chave

	if err := c.Arquivar(); err != nil {
		t.Fatalf("arquivar: %v", err)
	}

	if !c.Arquivado() {
		t.Error("o card devia estar arquivado")
	}
	// O que faz desarquivar devolver ao MESMO lugar, e não ao fim da coluna.
	if c.ColunaID != colunaAntes || c.Chave != chaveAntes {
		t.Errorf("arquivar mexeu na posição: coluna %q→%q, chave %q→%q",
			colunaAntes, c.ColunaID, chaveAntes, c.Chave)
	}
}

func TestDesarquivarDevolveOCardAoQuadro(t *testing.T) {
	c := cardValido(t)
	_ = c.Arquivar()

	if err := c.Desarquivar(); err != nil {
		t.Fatalf("desarquivar: %v", err)
	}
	if c.Arquivado() {
		t.Error("o card devia ter voltado ao quadro")
	}
}

// Duas pessoas arquivando o mesmo card: a segunda merece saber que não foi ela.
func TestArquivarDuasVezesEhRecusado(t *testing.T) {
	c := cardValido(t)
	_ = c.Arquivar()

	if err := c.Arquivar(); err != card.ErrJaArquivado {
		t.Errorf("erro = %v, esperado ErrJaArquivado", err)
	}
}

func TestDesarquivarOQueNaoEstaArquivadoEhRecusado(t *testing.T) {
	c := cardValido(t)

	if err := c.Desarquivar(); err != card.ErrNaoArquivado {
		t.Errorf("erro = %v, esperado ErrNaoArquivado", err)
	}
}

// A version sobe: arquivar é uma escrita no card como qualquer outra, e o
// bloqueio otimista precisa enxergá-la. Sem isto, quem tinha o card aberto
// gravaria por cima de um card que já saiu do quadro.
func TestArquivarESubirAVersao(t *testing.T) {
	c := cardValido(t)
	antes := c.Version

	_ = c.Arquivar()
	if c.Version != antes+1 {
		t.Errorf("version = %d, esperado %d", c.Version, antes+1)
	}

	_ = c.Desarquivar()
	if c.Version != antes+2 {
		t.Errorf("version = %d, esperado %d", c.Version, antes+2)
	}
}

// A coluna arquivada NÃO arquiva os cards dela. Ver o comentário do campo em
// domain/coluna: o contrário obrigaria o desarquivamento a adivinhar quais
// cards já estavam fora do quadro antes.
func TestArquivarColunaEDesarquivarDeVolta(t *testing.T) {
	col := colunaValida(t)

	if err := col.Arquivar(); err != nil {
		t.Fatalf("arquivar: %v", err)
	}
	if !col.Arquivada() {
		t.Fatal("a coluna devia estar arquivada")
	}
	if err := col.Arquivar(); err != coluna.ErrJaArquivada {
		t.Errorf("erro = %v, esperado ErrJaArquivada", err)
	}

	if err := col.Desarquivar(); err != nil {
		t.Fatalf("desarquivar: %v", err)
	}
	if err := col.Desarquivar(); err != coluna.ErrNaoArquivada {
		t.Errorf("erro = %v, esperado ErrNaoArquivada", err)
	}
}
