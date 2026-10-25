// O texto dos emails de notificação — atribuição, menção e prazo.
//
// Função pura, cujo resultado o usuário vê: por isso o teste roda sem Docker.
// Como isto quebra:
//
//	N1 o assunto não diz quem fez nem sobre qual card, e a pessoa ignora o
//	   email entre tantos outros;
//	N2 o título do card ou do quadro (escritos por usuários) entra como HTML
//	   de verdade no corpo, e um card chamado "<a href=…>" vira link de phishing;
//	N3 o nome de quem atribuiu, ou o título, carrega quebra de linha e injeta
//	   cabeçalho no assunto;
//	N4 o link do texto e o do HTML não são o mesmo, ou não aparece por extenso;
//	N5 o email da menção leva o texto do comentário — que sai da proteção do
//	   sistema e fica numa caixa de entrada que ninguém controla;
//	N6 o email do prazo não diz a data, ou a diz no fuso errado (o prazo é
//	   gravado ao meio-dia de Brasília, e em UTC ele "viraria" de dia).
package email_test

import (
	"strings"
	"testing"
	"time"

	aemail "stacktrack/internal/adapter/email"
	"stacktrack/internal/domain/notificacao"
	ucboard "stacktrack/internal/usecase/board"
)

func dadosDeNotificacao(tipo notificacao.Tipo) ucboard.DadosDoEmailDeNotificacao {
	return ucboard.DadosDoEmailDeNotificacao{
		Tipo:         tipo,
		Para:         "bruno@exemplo.com",
		Nome:         "Bruno",
		Autor:        "Ana",
		TituloQuadro: "Lançamento do app",
		TituloCard:   "Migrar o banco",
		// Meio-dia em Brasília: 15h em UTC.
		Prazo: time.Date(2026, 10, 26, 15, 0, 0, 0, time.UTC),
		Link:  "https://stacktrack.d4ffy.com/notificacoes/11111111-1111-4111-8111-111111111111",
	}
}

func redigirNotificacao(t *testing.T, d ucboard.DadosDoEmailDeNotificacao) (assunto, texto, html string) {
	t.Helper()
	m, err := aemail.NovoRedator().Notificacao(d)
	if err != nil {
		t.Fatalf("redigir %s: %v", d.Tipo, err)
	}
	if err := m.Validar(); err != nil {
		t.Fatalf("mensagem de %s inválida: %v", d.Tipo, err)
	}
	if m.Para != d.Para {
		t.Errorf("para = %q", m.Para)
	}
	return m.Assunto, m.Texto, m.HTML
}

func TestN1N4OEmailDizQuemFezSobreOQueEOndeClicar(t *testing.T) {
	casos := map[notificacao.Tipo]string{
		notificacao.TipoAtribuicao: "responsável",
		notificacao.TipoMencao:     "mencionou",
		notificacao.TipoPrazo:      "prazo",
	}
	for tipo, verbo := range casos {
		d := dadosDeNotificacao(tipo)
		assunto, texto, html := redigirNotificacao(t, d)
		if !strings.Contains(assunto, "Migrar o banco") || !strings.Contains(assunto, verbo) {
			t.Errorf("N1 %s: assunto = %q", tipo, assunto)
		}
		if tipo != notificacao.TipoPrazo && !strings.Contains(assunto, "Ana") {
			t.Errorf("N1 %s: o assunto não diz quem fez: %q", tipo, assunto)
		}
		for nome, corpo := range map[string]string{"texto": texto, "html": html} {
			if !strings.Contains(corpo, d.Link) {
				t.Errorf("N4 %s: o corpo em %s não traz o link", tipo, nome)
			}
			if !strings.Contains(corpo, "Lançamento do app") {
				t.Errorf("%s: o corpo em %s não diz de que quadro é", tipo, nome)
			}
		}
		if !strings.Contains(html, `>`+d.Link+`</a>`) {
			t.Errorf("N4 %s: no HTML o link não aparece por extenso", tipo)
		}
	}
}

func TestN2OTituloDoCardEntraComoTextoNoHTML(t *testing.T) {
	d := dadosDeNotificacao(notificacao.TipoAtribuicao)
	d.TituloCard = `<a href="https://golpe.exemplo">Clique</a>`
	_, _, html := redigirNotificacao(t, d)
	if strings.Contains(html, `<a href="https://golpe.exemplo">`) {
		t.Error("N2: o título do card virou link no HTML")
	}
}

func TestN3QuebraDeLinhaNaoChegaAoAssunto(t *testing.T) {
	d := dadosDeNotificacao(notificacao.TipoMencao)
	d.Autor = "Ana\r\nBcc: todo-mundo@exemplo.com"
	d.TituloCard = "Card\nX-Injetado: sim"
	assunto, _, _ := redigirNotificacao(t, d)
	if strings.ContainsAny(assunto, "\r\n") {
		t.Errorf("N3: assunto com quebra de linha: %q", assunto)
	}
}

func TestN5AMencaoNaoLevaOTextoDoComentario(t *testing.T) {
	// O redator nem recebe o texto — DadosDoEmailDeNotificacao não tem o
	// campo. Este teste guarda a frase: o email diz ONDE, e não O QUE.
	_, texto, _ := redigirNotificacao(t, dadosDeNotificacao(notificacao.TipoMencao))
	if !strings.Contains(texto, "comentário") {
		t.Errorf("N5: o email da menção não diz que é num comentário:\n%s", texto)
	}
}

func TestN6OPrazoSaiComADataDeBrasilia(t *testing.T) {
	d := dadosDeNotificacao(notificacao.TipoPrazo)
	// 01h de 27/10 em UTC ainda é 26/10 em Brasília.
	d.Prazo = time.Date(2026, 10, 27, 1, 0, 0, 0, time.UTC)
	assunto, texto, _ := redigirNotificacao(t, d)
	if !strings.Contains(assunto, "26/10") || !strings.Contains(texto, "26/10/2026") {
		t.Errorf("N6: data no fuso errado — assunto %q, texto:\n%s", assunto, texto)
	}
}

func TestOConviteNaoTemEmailDeNotificacao(t *testing.T) {
	if _, err := aemail.NovoRedator().Notificacao(dadosDeNotificacao(notificacao.TipoConvite)); err == nil {
		t.Error("o redator escreveu um email de notificação para o convite, que já tem o email dele")
	}
}
