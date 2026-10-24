// O texto do email de confirmação de conta.
//
// Função pura, cujo resultado o usuário vê: por isso o teste roda sem Docker.
// Como isto quebra:
//
//	K1 o link do texto e o do HTML não são o mesmo, ou o email não diz para que
//	   serve confirmar — e a pessoa ignora um email que parece burocracia;
//	K2 o email não diz o que fazer a quem não criou a conta.
package email_test

import (
	"strings"
	"testing"
	"time"

	aemail "stacktrack/internal/adapter/email"
	ucauth "stacktrack/internal/usecase/auth"
)

func dadosDeConfirmacao() ucauth.DadosDoEmailDeConfirmacao {
	return ucauth.DadosDoEmailDeConfirmacao{
		Para:     "helena@exemplo.com",
		Nome:     "Helena",
		Link:     "https://stacktrack.d4ffy.com/confirmar-email/tokenDeTeste_123",
		ExpiraEm: time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC),
	}
}

func TestOEmailDeConfirmacaoDizParaQueServeEOQueFazerSeNaoFoiVoce(t *testing.T) {
	m, err := aemail.NovoRedator().Confirmacao(dadosDeConfirmacao())
	if err != nil {
		t.Fatalf("redigir: %v", err)
	}
	if err := m.Validar(); err != nil {
		t.Fatalf("mensagem inválida: %v", err)
	}
	if m.ResponderPara != "" {
		t.Errorf("o email de confirmação leva Reply-To %q", m.ResponderPara)
	}
	if !strings.Contains(m.Assunto, "Confirme seu email") {
		t.Errorf("assunto = %q", m.Assunto)
	}
	for nome, corpo := range map[string]string{"texto": m.Texto, "html": m.HTML} {
		if !strings.Contains(corpo, dadosDeConfirmacao().Link) {
			t.Errorf("o corpo em %s não traz o link", nome)
		}
		if !strings.Contains(corpo, "convites") {
			t.Errorf("o corpo em %s não diz para que serve confirmar", nome)
		}
		if !strings.Contains(corpo, "não criou") {
			t.Errorf("o corpo em %s não diz o que fazer a quem não criou a conta", nome)
		}
	}
}
