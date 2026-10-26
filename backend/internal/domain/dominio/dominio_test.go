package dominio

// Como a regra do domínio pode quebrar, antes do código:
//
//   - o domínio sai com maiúscula, espaço ou o "@" junto, e "Acme.com.br" vira
//     um domínio diferente de "acme.com.br" — a colega não encontra o quadro;
//   - um subdomínio passa pelo pai ("vendas.acme.com.br" abre "acme.com.br",
//     ou o contrário) — gente de fora da área, ou da empresa, entra;
//   - um email pessoal (gmail, outlook, o hotmail.com.br) é aceito como
//     domínio de empresa — o quadro fica aberto a qualquer pessoa com conta lá;
//   - o dono escolhe o papel "dono" para quem entra pelo domínio — qualquer
//     colega passa a poder apagar o quadro;
//   - conta sem email confirmado abre o próprio domínio — basta cadastrar o
//     email de outra pessoa para abrir quadros "da empresa" dela.

import (
	"errors"
	"testing"

	"stacktrack/internal/domain/membro"
)

func TestDoEmailNormalizaODominio(t *testing.T) {
	casos := map[string]string{
		"ana@acme.com.br":       "acme.com.br",
		"  Ana@ACME.com.br  ":   "acme.com.br",
		"bruno@vendas.acme.com": "vendas.acme.com",
		"sem-arroba":            "",
		"ana@":                  "",
		"":                      "",
	}
	for email, esperado := range casos {
		if obtido := DoEmail(email); obtido != esperado {
			t.Errorf("DoEmail(%q) = %q; esperado %q", email, obtido, esperado)
		}
	}
}

func TestSubdominioNaoEOMesmoDominio(t *testing.T) {
	if Pertence("ana@vendas.acme.com.br", "acme.com.br") {
		t.Error("o subdomínio passou pelo domínio pai")
	}
	if Pertence("ana@acme.com.br", "vendas.acme.com.br") {
		t.Error("o domínio pai passou pelo subdomínio")
	}
	if !Pertence("Ana@Acme.com.br", "acme.com.br") {
		t.Error("o mesmo domínio, em outra caixa, não passou")
	}
}

func TestEmailPessoalNaoAbreQuadro(t *testing.T) {
	for _, email := range []string{"ana@gmail.com", "ana@outlook.com", "ana@hotmail.com.br", "ana@yahoo.com.br", "ana@icloud.com"} {
		if _, err := ParaAbrir(email, true, membro.PapelEditor); !errors.Is(err, ErrDominioPessoal) {
			t.Errorf("%s: esperado ErrDominioPessoal, veio %v", email, err)
		}
	}
}

func TestQuemEntraPeloDominioNuncaEDono(t *testing.T) {
	if _, err := ParaAbrir("ana@acme.com.br", true, membro.PapelDono); !errors.Is(err, ErrPapelDoDominio) {
		t.Errorf("esperado ErrPapelDoDominio, veio %v", err)
	}
	for _, papel := range []membro.Papel{membro.PapelEditor, membro.PapelLeitor} {
		d, err := ParaAbrir("ana@acme.com.br", true, papel)
		if err != nil || d != "acme.com.br" {
			t.Errorf("%s: esperado acme.com.br, veio %q, %v", papel, d, err)
		}
	}
}

func TestEmailNaoConfirmadoNaoAbreODominio(t *testing.T) {
	if _, err := ParaAbrir("ana@acme.com.br", false, membro.PapelEditor); !errors.Is(err, ErrEmailNaoConfirmado) {
		t.Errorf("esperado ErrEmailNaoConfirmado, veio %v", err)
	}
}
