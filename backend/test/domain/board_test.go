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
