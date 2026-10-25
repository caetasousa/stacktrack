// O FORMATO dos emails do sistema — convite, recuperação de senha,
// confirmação e notificações.
//
// Função pura, cujo resultado o usuário vê: por isso o teste roda sem Docker.
// Como isto quebra:
//
//	F1 o HTML tem cara de email de marketing — tabela de layout, fundo
//	   colorido, botão estilizado, cabeçalho com a marca, imagem — e o Gmail o
//	   classifica em Promoções, onde a pessoa não procura um convite que está
//	   esperando;
//	F2 o link só existe dentro de um botão: quem lê o email em texto, ou num
//	   leitor que não mostra o botão, não tem o endereço para copiar.
package email_test

import (
	"strings"
	"testing"

	aemail "stacktrack/internal/adapter/email"
	"stacktrack/internal/domain/notificacao"
)

func TestOsEmailsTemFormatoDeMensagemPessoalENaoDePromocao(t *testing.T) {
	redator := aemail.NovoRedator()
	convite, err := redator.Convite(dadosDeConvite())
	if err != nil {
		t.Fatalf("redigir convite: %v", err)
	}
	recuperacao, err := redator.Recuperacao(dadosDeRecuperacao())
	if err != nil {
		t.Fatalf("redigir recuperação: %v", err)
	}

	confirmacao, err := redator.Confirmacao(dadosDeConfirmacao())
	if err != nil {
		t.Fatalf("redigir confirmação: %v", err)
	}

	casos := map[string]struct{ html, link string }{
		"convite":     {convite.HTML, dadosDeConvite().Link},
		"recuperação": {recuperacao.HTML, dadosDeRecuperacao().Link},
		"confirmação": {confirmacao.HTML, dadosDeConfirmacao().Link},
	}
	for _, tipo := range []notificacao.Tipo{notificacao.TipoAtribuicao, notificacao.TipoMencao, notificacao.TipoPrazo} {
		m, err := redator.Notificacao(dadosDeNotificacao(tipo))
		if err != nil {
			t.Fatalf("redigir notificação de %s: %v", tipo, err)
		}
		casos["notificação de "+string(tipo)] = struct{ html, link string }{m.HTML, dadosDeNotificacao(tipo).Link}
	}
	// O que transforma uma mensagem em "newsletter" aos olhos do classificador.
	marcasDePromocao := []string{"<table", "<img", "<h1", "background", "border-radius", "text-transform", "letter-spacing"}

	for nome, c := range casos {
		html := strings.ToLower(c.html)
		for _, marca := range marcasDePromocao {
			if strings.Contains(html, marca) {
				t.Errorf("F1: o HTML do %s tem %q", nome, marca)
			}
		}
		// F2: o endereço aparece como texto do próprio link.
		if !strings.Contains(c.html, `>`+c.link+`</a>`) {
			t.Errorf("F2: no HTML do %s o link não aparece por extenso", nome)
		}
	}
}
