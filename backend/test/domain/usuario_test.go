package domain_test

import (
	"errors"
	"strings"
	"testing"

	"kanbango/internal/domain/usuario"
)

func TestNovoUsuarioNormalizaOEmail(t *testing.T) {
	u, err := usuario.Novo("id-1", "Ana", "  Ana@Exemplo.COM ", "hash")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if u.Email != "ana@exemplo.com" {
		t.Errorf("email = %q, esperado normalizado para minúsculas e sem espaços", u.Email)
	}
}

func TestNovoUsuarioAparaEspacosDoNome(t *testing.T) {
	u, err := usuario.Novo("id-1", "  Ana Souza  ", "ana@exemplo.com", "hash")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if u.Nome != "Ana Souza" {
		t.Errorf("nome = %q, esperado sem espaços nas pontas", u.Nome)
	}
}

func TestNovoUsuarioRecusaDadosInvalidos(t *testing.T) {
	casos := []struct {
		nome     string
		entrada  [3]string // nome, email, senhaHash
		esperado error
	}{
		{"nome vazio", [3]string{"", "ana@exemplo.com", "hash"}, usuario.ErrNomeObrigatorio},
		{"nome só com espaços", [3]string{"   ", "ana@exemplo.com", "hash"}, usuario.ErrNomeObrigatorio},
		{"email vazio", [3]string{"Ana", "", "hash"}, usuario.ErrEmailObrigatorio},
		{"email sem arroba", [3]string{"Ana", "ana.exemplo.com", "hash"}, usuario.ErrEmailInvalido},
		{"email sem domínio", [3]string{"Ana", "ana@", "hash"}, usuario.ErrEmailInvalido},
		{"email sem ponto no domínio", [3]string{"Ana", "ana@exemplo", "hash"}, usuario.ErrEmailInvalido},
		{"hash vazio", [3]string{"Ana", "ana@exemplo.com", ""}, usuario.ErrSenhaObrigatoria},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			_, err := usuario.Novo("id-1", caso.entrada[0], caso.entrada[1], caso.entrada[2])
			if !errors.Is(err, caso.esperado) {
				t.Errorf("erro = %v, esperado %v", err, caso.esperado)
			}
		})
	}
}

func TestValidarSenhaExigeTamanhoMinimo(t *testing.T) {
	casos := map[string]error{
		"":                      usuario.ErrSenhaObrigatoria,
		"1234567":               usuario.ErrSenhaCurta,
		"12345678":              nil,
		"uma senha longa e boa": nil,
	}

	for senha, esperado := range casos {
		if err := usuario.ValidarSenha(senha); !errors.Is(err, esperado) {
			t.Errorf("senha %q: erro = %v, esperado %v", senha, err, esperado)
		}
	}
}

// O mínimo é contado em CARACTERES, não em bytes: com len() uma senha de 4
// emojis (16 bytes) passaria, e uma de 8 letras acentuadas contaria 16.
func TestValidarSenhaContaCaracteresENaoBytes(t *testing.T) {
	if err := usuario.ValidarSenha("çãéíõu"); !errors.Is(err, usuario.ErrSenhaCurta) {
		t.Errorf("senha de 6 caracteres acentuados devia ser curta, erro = %v", err)
	}
}

func TestNormalizarEmail(t *testing.T) {
	if got := usuario.NormalizarEmail("  BOB@Exemplo.com\t"); got != "bob@exemplo.com" {
		t.Errorf("NormalizarEmail = %q", got)
	}
}

func TestDefinirSenhaRecusaHashVazio(t *testing.T) {
	u, err := usuario.Novo("id-1", "Ana", "ana@exemplo.com", "hash-antigo")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if err := u.DefinirSenha(""); !errors.Is(err, usuario.ErrSenhaObrigatoria) {
		t.Errorf("erro = %v, esperado ErrSenhaObrigatoria", err)
	}
	if u.SenhaHash != "hash-antigo" {
		t.Error("o hash não podia ter sido trocado por um vazio")
	}

	if err := u.DefinirSenha("hash-novo"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if u.SenhaHash != "hash-novo" {
		t.Error("o hash devia ter sido trocado")
	}
}

func TestMensagemDeSenhaCurtaCitaOMinimo(t *testing.T) {
	if !strings.Contains(usuario.ErrSenhaCurta.Error(), "8") {
		t.Error("a mensagem de senha curta precisa dizer qual é o mínimo")
	}
}
