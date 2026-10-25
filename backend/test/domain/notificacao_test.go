package domain_test

// As regras da notificação que o usuário enxerga: por quanto tempo ela fica
// guardada, quando um prazo avisa de novo e quais tipos mandam email.
//
// Como isto quebra:
//
//	T1 o dono escolhe uma retenção absurda (zero, negativa, dez anos) e o
//	   quadro aceita — com zero, a faxina apagaria o que acabou de ser lido;
//	T2 o quadro que nunca escolheu fica sem retenção nenhuma;
//	T3 o mesmo prazo avisa duas vezes, ou um prazo novo não avisa de novo;
//	T4 o convite manda um segundo email, além do email do próprio convite.

import (
	"errors"
	"testing"
	"time"

	"stacktrack/internal/domain/board"
	"stacktrack/internal/domain/notificacao"
)

func TestRetencaoFicaNaFaixaPermitida(t *testing.T) {
	for _, dias := range []int{-1, 0, notificacao.RetencaoMaximaDias + 1, 3650} {
		if err := notificacao.ValidarRetencao(dias); !errors.Is(err, notificacao.ErrRetencaoInvalida) {
			t.Errorf("T1: %d dias aceito (erro = %v)", dias, err)
		}
	}
	for _, dias := range []int{notificacao.RetencaoMinimaDias, 7, notificacao.RetencaoMaximaDias} {
		if err := notificacao.ValidarRetencao(dias); err != nil {
			t.Errorf("%d dias recusado: %v", dias, err)
		}
	}
}

func TestQuadroSemEscolhaUsaARetencaoPadrao(t *testing.T) {
	b, err := board.Novo("b-1", "Quadro")
	if err != nil {
		t.Fatal(err)
	}
	if got := b.RetencaoDeNotificacoes(); got != notificacao.RetencaoPadraoDias {
		t.Errorf("T2: retenção = %d, esperado o padrão %d", got, notificacao.RetencaoPadraoDias)
	}

	if err := b.DefinirRetencaoDeNotificacoes(7); err != nil {
		t.Fatalf("definir 7 dias: %v", err)
	}
	if err := b.DefinirRetencaoDeNotificacoes(0); !errors.Is(err, notificacao.ErrRetencaoInvalida) {
		t.Fatalf("T1: zero aceito (erro = %v)", err)
	}
	if got := b.RetencaoDeNotificacoes(); got != 7 {
		t.Errorf("a recusa mudou a retenção para %d", got)
	}
}

func TestOMesmoPrazoAvisaUmaVezEOPrazoNovoAvisaDeNovo(t *testing.T) {
	amanha := time.Date(2026, 10, 26, 15, 0, 0, 0, time.UTC)
	agora := amanha.Add(-20 * time.Hour)

	primeira := notificacao.NovaDePrazo("n1", "bruno", "b", "card", amanha, agora)
	segunda := notificacao.NovaDePrazo("n2", "bruno", "b", "card", amanha, agora.Add(time.Hour))
	if primeira.Chave != segunda.Chave {
		t.Errorf("T3: o mesmo prazo gerou chaves diferentes (%q, %q) — avisaria duas vezes", primeira.Chave, segunda.Chave)
	}
	adiado := notificacao.NovaDePrazo("n3", "bruno", "b", "card", amanha.Add(24*time.Hour), agora)
	if adiado.Chave == primeira.Chave {
		t.Error("T3: o prazo novo tem a chave do antigo — não avisaria de novo")
	}
	outraPessoa := notificacao.NovaDePrazo("n4", "carla", "b", "card", amanha, agora)
	if outraPessoa.Chave == primeira.Chave {
		t.Error("T3: duas pessoas do mesmo card dividem a chave — só uma seria avisada")
	}
}

func TestSoOConviteNaoMandaEmailProprio(t *testing.T) {
	if notificacao.TipoConvite.TemEmail() {
		t.Error("T4: o convite já tem o email dele; a notificação mandaria um segundo")
	}
	for _, tipo := range []notificacao.Tipo{notificacao.TipoAtribuicao, notificacao.TipoMencao, notificacao.TipoPrazo} {
		if !tipo.TemEmail() {
			t.Errorf("%s não manda email", tipo)
		}
	}
}
