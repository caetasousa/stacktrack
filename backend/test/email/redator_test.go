// O texto do email de convite — o que a pessoa convidada lê.
//
// Função pura, cujo resultado o usuário vê: por isso o teste roda sem Docker.
// Como isto quebra:
//
//	R1 o título do quadro (escrito por quem convida) entra como HTML de verdade
//	   no corpo, e um quadro chamado "<a href=…>" vira link de phishing no email;
//	R2 o nome de quem convida carrega uma quebra de linha e injeta cabeçalho no
//	   assunto;
//	R3 o link do texto e o do HTML não são o mesmo, ou o email não diz o que
//	   fazer a quem ainda não tem conta;
//	R4 o assunto não diz quem chama nem para quê, e a pessoa ignora o email.
package email_test

import (
	"strings"
	"testing"
	"time"

	aemail "stacktrack/internal/adapter/email"
	"stacktrack/internal/domain/membro"
	ucboard "stacktrack/internal/usecase/board"
)

func dadosDeConvite() ucboard.DadosDoEmailDeConvite {
	return ucboard.DadosDoEmailDeConvite{
		Para:          "bia@exemplo.com",
		ConvidadoPor:  "Ana",
		ResponderPara: "ana@exemplo.com",
		TituloQuadro:  "Lançamento do app",
		Papel:         membro.PapelEditor,
		Link:          "https://stacktrack.d4ffy.com/convite/tokenDeTeste_123",
		ExpiraEm:      time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC),
	}
}

// R4 e R3
func TestOEmailDizQuemChamaParaOndeEComoEntrar(t *testing.T) {
	m, err := aemail.NovoRedator().Convite(dadosDeConvite())
	if err != nil {
		t.Fatalf("redigir: %v", err)
	}
	if err := m.Validar(); err != nil {
		t.Fatalf("mensagem inválida: %v", err)
	}
	if m.Para != "bia@exemplo.com" || m.ResponderPara != "ana@exemplo.com" {
		t.Errorf("para/responder = %q/%q", m.Para, m.ResponderPara)
	}
	if !strings.Contains(m.Assunto, "Ana") || !strings.Contains(m.Assunto, "Lançamento do app") {
		t.Errorf("assunto = %q, esperado quem chama e o quadro", m.Assunto)
	}

	link := dadosDeConvite().Link
	for nome, corpo := range map[string]string{"texto": m.Texto, "html": m.HTML} {
		if !strings.Contains(corpo, link) {
			t.Errorf("o corpo em %s não traz o link do convite", nome)
		}
		if !strings.Contains(corpo, "editor") {
			t.Errorf("o corpo em %s não diz com que papel a pessoa entra", nome)
		}
		if !strings.Contains(corpo, "criar uma conta") {
			t.Errorf("o corpo em %s não diz o que fazer sem conta", nome)
		}
		if !strings.Contains(corpo, "31/10/2026") {
			t.Errorf("o corpo em %s não diz até quando o convite vale", nome)
		}
	}
}

// R1
func TestTituloDoQuadroNaoViraHTMLNoEmail(t *testing.T) {
	d := dadosDeConvite()
	d.TituloQuadro = `<a href="https://golpe.exemplo">Clique aqui</a>`
	m, err := aemail.NovoRedator().Convite(d)
	if err != nil {
		t.Fatalf("redigir: %v", err)
	}
	if strings.Contains(m.HTML, `<a href="https://golpe.exemplo">`) {
		t.Fatal("o título do quadro entrou como HTML no corpo do email")
	}
	if !strings.Contains(m.HTML, "&lt;a href=") {
		t.Errorf("o título devia aparecer escapado no HTML:\n%s", m.HTML)
	}
}

// R2
func TestNomeComQuebraDeLinhaNaoInjetaCabecalho(t *testing.T) {
	d := dadosDeConvite()
	d.ConvidadoPor = "Ana\r\nBcc: todos@exemplo.com"
	m, err := aemail.NovoRedator().Convite(d)
	if err != nil {
		t.Fatalf("redigir: %v", err)
	}
	if strings.ContainsAny(m.Assunto, "\r\n") {
		t.Fatalf("o assunto carrega quebra de linha: %q", m.Assunto)
	}
	if err := m.Validar(); err != nil {
		t.Fatalf("a mensagem devia continuar válida, com o nome limpo: %v", err)
	}
}
