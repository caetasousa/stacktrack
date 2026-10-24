// O texto do email de recuperação de senha — o que a pessoa lê quando esqueceu
// a senha.
//
// Função pura, cujo resultado o usuário vê: por isso o teste roda sem Docker.
// Como isto quebra:
//
//	P1 o link do texto e o do HTML não são o mesmo, ou o email não diz até
//	   quando ele vale;
//	P2 o email não diz o que fazer a quem NÃO pediu — e a pessoa acha que a
//	   conta foi invadida, ou clica achando que precisa;
//	P3 o nome da conta carrega HTML ou quebra de linha e vira marcação no corpo
//	   ou cabeçalho no assunto;
//	P4 o email leva uma resposta para alguém (Reply-To): na recuperação não há
//	   pessoa do outro lado, e a resposta iria para quem não devia.
package email_test

import (
	"strings"
	"testing"
	"time"

	aemail "stacktrack/internal/adapter/email"
	ucauth "stacktrack/internal/usecase/auth"
)

func dadosDeRecuperacao() ucauth.DadosDoEmailDeRecuperacao {
	return ucauth.DadosDoEmailDeRecuperacao{
		Para:     "carla@exemplo.com",
		Nome:     "Carla",
		Link:     "https://stacktrack.d4ffy.com/redefinir-senha/tokenDeTeste_123",
		ExpiraEm: time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC),
	}
}

// P1, P2 e P4
func TestOEmailDeRecuperacaoTrazOLinkAValidadeEOQueFazerSeNaoPediu(t *testing.T) {
	m, err := aemail.NovoRedator().Recuperacao(dadosDeRecuperacao())
	if err != nil {
		t.Fatalf("redigir: %v", err)
	}
	if err := m.Validar(); err != nil {
		t.Fatalf("mensagem inválida: %v", err)
	}
	if m.Para != "carla@exemplo.com" {
		t.Errorf("para = %q", m.Para)
	}
	if m.ResponderPara != "" {
		t.Errorf("o email de recuperação leva Reply-To %q", m.ResponderPara)
	}
	if !strings.Contains(strings.ToLower(m.Assunto), "senha") {
		t.Errorf("assunto = %q, esperado que fale de senha", m.Assunto)
	}

	link := dadosDeRecuperacao().Link
	for nome, corpo := range map[string]string{"texto": m.Texto, "html": m.HTML} {
		if !strings.Contains(corpo, link) {
			t.Errorf("o corpo em %s não traz o link", nome)
		}
		// 15h UTC é meio-dia em Brasília.
		if !strings.Contains(corpo, "31/10/2026 às 12h00") {
			t.Errorf("o corpo em %s não diz até quando o link vale, no fuso de Brasília", nome)
		}
		if !strings.Contains(corpo, "não pediu") {
			t.Errorf("o corpo em %s não diz o que fazer a quem não pediu", nome)
		}
	}
}

// P3
func TestONomeDaContaNaoViraMarcacaoNoEmailDeRecuperacao(t *testing.T) {
	d := dadosDeRecuperacao()
	d.Nome = "<b>Carla</b>\r\nBcc: alguem@fora.com"
	m, err := aemail.NovoRedator().Recuperacao(d)
	if err != nil {
		t.Fatalf("redigir: %v", err)
	}
	if strings.Contains(m.HTML, "<b>Carla</b>") {
		t.Error("o nome entrou como HTML no corpo")
	}
	if strings.ContainsAny(m.Assunto, "\r\n") {
		t.Errorf("o assunto carrega quebra de linha: %q", m.Assunto)
	}
}
