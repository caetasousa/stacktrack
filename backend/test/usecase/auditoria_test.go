// A auditoria do quadro: quem mexeu na ordem das coisas.
//
// O problema que ela resolve não é técnico. Num quadro com muitos cards, alguém
// arrasta trinta deles e ninguém sabe quem foi — a informação SEMPRE esteve no
// log de eventos, mas só era alcançável abrindo card por card, que é o mesmo
// que não estar.
//
// Nada aqui inventa tabela: são duas leituras novas sobre `board_events`. O que
// estes testes trancam é que elas respondam a pergunta certa — a ÚLTIMA
// movimentação, e não uma qualquer — e que o recorte não deixe passar o que
// deveria filtrar.
package usecase_test

import (
	"context"
	"errors"
	"testing"

	dboard "stacktrack/internal/domain/board"
	"stacktrack/internal/domain/evento"
	dmembro "stacktrack/internal/domain/membro"
	ucboard "stacktrack/internal/usecase/board"
)

// moverPara arrasta o card para o fim da coluna informada, como quem.
func (h *historico) moverPara(t *testing.T, cardID, colunaID, quem string) {
	t.Helper()
	if _, err := h.card.Mover(context.Background(), cardID, quem, colunaID, ucboard.Vizinhos{}); err != nil {
		t.Fatalf("mover: %v", err)
	}
}

// selo devolve a última movimentação do card, lida do quadro detalhado — é
// exatamente o caminho que a tela percorre.
func (h *historico) selo(t *testing.T, boardID, cardID, quem string) *ucboard.Movimentacao {
	t.Helper()
	detalhado, err := h.quadros.Detalhar(context.Background(), boardID, quem)
	if err != nil {
		t.Fatalf("detalhar: %v", err)
	}
	for _, coluna := range detalhado.Colunas {
		for _, c := range coluna.Cards {
			if c.Card.ID == cardID {
				return c.UltimaMovimentacao
			}
		}
	}
	t.Fatalf("card %s não veio no quadro", cardID)
	return nil
}

// cenario monta um quadro com duas colunas e um card na primeira.
func (h *historico) cenario(t *testing.T, dono string) (boardID, aFazer, pronto, cardID string) {
	t.Helper()
	boardID = h.criarQuadro(t, dono, "Roadmap")
	aFazer = h.criarColuna(t, boardID, dono, "A fazer")
	pronto = h.criarColuna(t, boardID, dono, "Pronto")
	cardID = h.criarCard(t, aFazer, dono, "Revisar o contrato")
	return
}

// Card recém-criado não tem selo. Nil é a resposta certa: dizer "movido por
// quem criou" transformaria a auditoria em ficção — e é o tipo de mentira que
// ninguém confere, porque parece plausível.
func TestCardNuncaMovidoNaoTemSeloDeMovimentacao(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	boardID, _, _, cardID := h.cenario(t, ana)

	if m := h.selo(t, boardID, cardID, ana); m != nil {
		t.Errorf("card nunca movido veio com selo: %+v", m)
	}
}

func TestOSeloDizQuemMoveuEDeOndeParaOnde(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	bob := h.conta(t, "Roberto Silva", "roberto@exemplo.com")
	boardID, _, pronto, cardID := h.cenario(t, ana)
	h.convidar(t, boardID, bob, dmembro.PapelEditor)

	h.moverPara(t, cardID, pronto, bob)

	m := h.selo(t, boardID, cardID, ana)
	if m == nil {
		t.Fatal("o card movido devia ter selo")
	}
	if m.AutorID != bob || m.AutorNome != "Roberto Silva" {
		t.Errorf("autor = %q/%q, esperado o bob", m.AutorID, m.AutorNome)
	}
	if m.DeColuna != "A fazer" || m.ParaColuna != "Pronto" {
		t.Errorf("movimento = %q -> %q, esperado 'A fazer' -> 'Pronto'", m.DeColuna, m.ParaColuna)
	}
}

// O selo é da ÚLTIMA movimentação, e essa é a única resposta útil: mostrar a
// primeira faria a auditoria apontar para quem organizou o quadro em vez de
// para quem o bagunçou depois.
func TestOSeloEODaUltimaMovimentacaoENaoDaPrimeira(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	bob := h.conta(t, "Bob", "bob@exemplo.com")
	boardID, aFazer, pronto, cardID := h.cenario(t, ana)
	h.convidar(t, boardID, bob, dmembro.PapelEditor)

	h.moverPara(t, cardID, pronto, ana)
	h.moverPara(t, cardID, aFazer, bob)

	m := h.selo(t, boardID, cardID, ana)
	if m == nil {
		t.Fatal("o card movido devia ter selo")
	}
	if m.AutorID != bob {
		t.Errorf("autor = %q, esperado o bob (quem moveu por último)", m.AutorID)
	}
	if m.DeColuna != "Pronto" || m.ParaColuna != "A fazer" {
		t.Errorf("movimento = %q -> %q, esperado o segundo", m.DeColuna, m.ParaColuna)
	}
}

// Reordenar dentro da própria coluna também é movimentação, e também tem dono:
// é justamente assim que uma coluna de prioridades é embaralhada. De e Para
// iguais é o que deixa a tela dizer "reordenou" em vez de inventar um trajeto.
func TestReordenarNaMesmaColunaContaComoMovimentacao(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	boardID, aFazer, _, cardID := h.cenario(t, ana)
	h.criarCard(t, aFazer, ana, "Outro card")

	h.moverPara(t, cardID, aFazer, ana)

	m := h.selo(t, boardID, cardID, ana)
	if m == nil {
		t.Fatal("reordenar devia contar como movimentação")
	}
	if m.DeColuna != m.ParaColuna {
		t.Errorf("de/para = %q/%q, esperados iguais numa reordenação", m.DeColuna, m.ParaColuna)
	}
}

// O selo de um card não pode vir do movimento de outro. É o defeito clássico de
// uma consulta em lote — agrupar errado e distribuir a mesma linha para todos.
func TestOSeloDeUmCardNaoVemDoMovimentoDeOutro(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	bob := h.conta(t, "Bob", "bob@exemplo.com")
	boardID, aFazer, pronto, primeiro := h.cenario(t, ana)
	h.convidar(t, boardID, bob, dmembro.PapelEditor)
	segundo := h.criarCard(t, aFazer, ana, "Outro card")

	h.moverPara(t, primeiro, pronto, bob)

	if m := h.selo(t, boardID, primeiro, ana); m == nil || m.AutorID != bob {
		t.Errorf("selo do card movido = %+v, esperado o bob", m)
	}
	if m := h.selo(t, boardID, segundo, ana); m != nil {
		t.Errorf("o card que ninguém moveu recebeu o selo do vizinho: %+v", m)
	}
}

// --- a auditoria do quadro -------------------------------------------------

func TestAuditoriaTrazSoAsMovimentacoesPorPadrao(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	boardID, _, pronto, cardID := h.cenario(t, ana)
	h.moverPara(t, cardID, pronto, ana)

	pagina, err := h.atividadeUC.DoBoard(context.Background(), boardID, ana, ucboard.FiltroDeAtividade{SoMovimentacoes: true})
	if err != nil {
		t.Fatalf("auditoria: %v", err)
	}
	if len(pagina.Linhas) == 0 {
		t.Fatal("a auditoria não trouxe a movimentação")
	}
	for _, a := range pagina.Linhas {
		if a.Tipo != evento.CardMovido {
			t.Errorf("o recorte de movimentações trouxe %q", a.Tipo)
		}
	}
}

// Sem o recorte, a criação da coluna e a do card aparecem — é o que serve para
// entender o quadro inteiro, e não só a ordem dele.
func TestAuditoriaSemRecorteTrazOResto(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	boardID, _, _, _ := h.cenario(t, ana)

	pagina, err := h.atividadeUC.DoBoard(context.Background(), boardID, ana, ucboard.FiltroDeAtividade{})
	if err != nil {
		t.Fatalf("auditoria: %v", err)
	}
	if len(pagina.Linhas) == 0 {
		t.Fatal("sem recorte, a auditoria devia trazer a criação de colunas e cards")
	}
}

// A pergunta central da auditoria: "o que ESTA pessoa fez".
func TestAuditoriaFiltraPorPessoa(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	bob := h.conta(t, "Bob", "bob@exemplo.com")
	boardID, aFazer, pronto, cardID := h.cenario(t, ana)
	h.convidar(t, boardID, bob, dmembro.PapelEditor)

	h.moverPara(t, cardID, pronto, ana)
	h.moverPara(t, cardID, aFazer, bob)

	pagina, err := h.atividadeUC.DoBoard(context.Background(), boardID, ana,
		ucboard.FiltroDeAtividade{SoMovimentacoes: true, AutorID: bob})
	if err != nil {
		t.Fatalf("auditoria: %v", err)
	}
	if len(pagina.Linhas) != 1 {
		t.Fatalf("movimentações do bob = %d, esperada 1", len(pagina.Linhas))
	}
	if pagina.Linhas[0].AutorID != bob || pagina.Linhas[0].AutorNome != "Bob" {
		t.Errorf("autor = %q/%q, esperado o bob", pagina.Linhas[0].AutorID, pagina.Linhas[0].AutorNome)
	}
}

// Do mais recente para o mais antigo: é a ordem em que se audita, e é o que faz
// o cursor da página seguinte ser o menor seq recebido.
func TestAuditoriaVemDoMaisRecenteParaOMaisAntigo(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	boardID, aFazer, pronto, cardID := h.cenario(t, ana)
	h.moverPara(t, cardID, pronto, ana)
	h.moverPara(t, cardID, aFazer, ana)

	pagina, err := h.atividadeUC.DoBoard(context.Background(), boardID, ana, ucboard.FiltroDeAtividade{SoMovimentacoes: true})
	if err != nil {
		t.Fatalf("auditoria: %v", err)
	}
	if len(pagina.Linhas) != 2 {
		t.Fatalf("movimentações = %d, esperadas 2", len(pagina.Linhas))
	}
	if pagina.Linhas[0].Seq <= pagina.Linhas[1].Seq {
		t.Errorf("ordem = %d, %d — esperado o mais recente primeiro", pagina.Linhas[0].Seq, pagina.Linhas[1].Seq)
	}
}

// A paginação é por cursor, e não por deslocamento: o log recebe escrita o tempo
// todo, e um OFFSET faria a segunda página pular linhas que entraram entre um
// pedido e o outro. Numa auditoria, pular em silêncio é o pior defeito possível.
func TestAuditoriaPaginaPorCursorSemPularLinha(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	boardID, aFazer, pronto, cardID := h.cenario(t, ana)
	for i := 0; i < 5; i++ {
		destino := pronto
		if i%2 == 1 {
			destino = aFazer
		}
		h.moverPara(t, cardID, destino, ana)
	}

	paginaUm, err := h.atividadeUC.DoBoard(context.Background(), boardID, ana,
		ucboard.FiltroDeAtividade{SoMovimentacoes: true, Limite: 2})
	if err != nil {
		t.Fatalf("primeira página: %v", err)
	}
	if len(paginaUm.Linhas) != 2 {
		t.Fatalf("primeira página = %d, esperadas 2", len(paginaUm.Linhas))
	}

	// Entre as duas páginas, alguém move de novo. O cursor tem de segurar a
	// janela: a linha nova é MAIS recente, então não pode entrar na página 2 nem
	// empurrar nada para fora dela.
	h.moverPara(t, cardID, pronto, ana)

	paginaDois, err := h.atividadeUC.DoBoard(context.Background(), boardID, ana,
		ucboard.FiltroDeAtividade{SoMovimentacoes: true, Limite: 2, AntesDe: paginaUm.Linhas[1].Seq})
	if err != nil {
		t.Fatalf("segunda página: %v", err)
	}
	if len(paginaDois.Linhas) != 2 {
		t.Fatalf("segunda página = %d, esperadas 2", len(paginaDois.Linhas))
	}
	for _, a := range paginaDois.Linhas {
		if a.Seq >= paginaUm.Linhas[1].Seq {
			t.Errorf("a segunda página repetiu o seq %d, que já estava na primeira", a.Seq)
		}
	}
	// Sem buraco: os seq são consecutivos na ordem em que foram gravados.
	if paginaDois.Linhas[0].Seq != paginaUm.Linhas[1].Seq-1 {
		t.Errorf("houve um salto entre as páginas: %d depois de %d", paginaDois.Linhas[0].Seq, paginaUm.Linhas[1].Seq)
	}
}

// Quem não participa do quadro não audita nada — e recebe "não encontrado", não
// "sem permissão", como no resto da aplicação.
func TestAuditoriaDeQuadroAlheioRespondeNaoEncontrado(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	boardID, _, _, _ := h.cenario(t, ana)

	_, err := h.atividadeUC.DoBoard(context.Background(), boardID, "estranho", ucboard.FiltroDeAtividade{})

	if !errors.Is(err, dboard.ErrNaoEncontrado) {
		t.Errorf("erro = %v, esperado ErrNaoEncontrado", err)
	}
}

// Leitor audita. Acompanhar o que aconteceu é ver, não mexer — é a mesma regra
// do histórico de um card, e mudar de critério aqui deixaria a mesma informação
// com duas permissões diferentes conforme a rota.
func TestLeitorPodeAuditar(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	bob := h.conta(t, "Bob", "bob@exemplo.com")
	boardID, _, _, _ := h.cenario(t, ana)
	h.convidar(t, boardID, bob, dmembro.PapelLeitor)

	if _, err := h.atividadeUC.DoBoard(context.Background(), boardID, bob, ucboard.FiltroDeAtividade{}); err != nil {
		t.Errorf("o leitor devia poder auditar: %v", err)
	}
}

// O teto é do servidor, e não de quem chama: sem ele, um `limite` vindo da URL
// montaria a história inteira do quadro em memória.
func TestAuditoriaAplicaOTetoMesmoComLimiteAbsurdo(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	boardID, aFazer, pronto, cardID := h.cenario(t, ana)
	for i := 0; i < 3; i++ {
		h.moverPara(t, cardID, pronto, ana)
		h.moverPara(t, cardID, aFazer, ana)
	}

	pagina, err := h.atividadeUC.DoBoard(context.Background(), boardID, ana,
		ucboard.FiltroDeAtividade{Limite: 1_000_000})
	if err != nil {
		t.Fatalf("auditoria: %v", err)
	}
	if len(pagina.Linhas) > ucboard.LimiteDaAuditoria {
		t.Errorf("linhas = %d, o teto de %d não foi aplicado", len(pagina.Linhas), ucboard.LimiteDaAuditoria)
	}
}

// TemMais é o que impede a tela de oferecer um "carregar mais" que devolve
// lista vazia — um botão mentindo. O servidor pede uma linha a mais do que
// devolve, e é assim que ele sabe responder sem uma segunda consulta.
func TestAPaginaDizSeExisteAProxima(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	boardID, aFazer, pronto, cardID := h.cenario(t, ana)
	h.moverPara(t, cardID, pronto, ana)
	h.moverPara(t, cardID, aFazer, ana)
	h.moverPara(t, cardID, pronto, ana)

	cheia, err := h.atividadeUC.DoBoard(context.Background(), boardID, ana,
		ucboard.FiltroDeAtividade{SoMovimentacoes: true, Limite: 2})
	if err != nil {
		t.Fatalf("auditoria: %v", err)
	}
	if len(cheia.Linhas) != 2 {
		t.Fatalf("linhas = %d, esperadas 2", len(cheia.Linhas))
	}
	if !cheia.TemMais {
		t.Error("faltou dizer que existe a próxima página — a tela esconderia o botão cedo demais")
	}

	// A última página não pode dizer que há mais: o botão apareceria e o clique
	// devolveria nada.
	ultima, err := h.atividadeUC.DoBoard(context.Background(), boardID, ana,
		ucboard.FiltroDeAtividade{SoMovimentacoes: true, Limite: 2, AntesDe: cheia.Linhas[1].Seq})
	if err != nil {
		t.Fatalf("última página: %v", err)
	}
	if ultima.TemMais {
		t.Errorf("a última página (%d linhas) disse que ainda há mais", len(ultima.Linhas))
	}
}

// A linha extra pedida ao repositório não pode VAZAR para quem chama: pedir 2 e
// receber 3 faria a tela mostrar uma linha a mais por página, e o cursor pularia
// justamente essa na página seguinte.
func TestALinhaExtraDoTemMaisNaoVazaParaAResposta(t *testing.T) {
	h := novoHistorico(t)
	ana := h.conta(t, "Ana", "ana@exemplo.com")
	boardID, aFazer, pronto, cardID := h.cenario(t, ana)
	for i := 0; i < 6; i++ {
		destino := pronto
		if i%2 == 1 {
			destino = aFazer
		}
		h.moverPara(t, cardID, destino, ana)
	}

	pagina, err := h.atividadeUC.DoBoard(context.Background(), boardID, ana,
		ucboard.FiltroDeAtividade{SoMovimentacoes: true, Limite: 3})
	if err != nil {
		t.Fatalf("auditoria: %v", err)
	}
	if len(pagina.Linhas) != 3 {
		t.Errorf("linhas = %d, esperadas exatamente 3 — a linha de sondagem vazou", len(pagina.Linhas))
	}
}
