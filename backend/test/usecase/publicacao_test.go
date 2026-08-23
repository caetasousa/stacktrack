// O link público é a única porta da aplicação que abre sem sessão, e estes
// testes cercam as duas perguntas que isso levanta: QUEM consegue abri-la, e o
// QUE sai por ela.
//
// A segunda é a que não se percebe sozinha. Um campo novo no quadro entra na
// projeção pública por descuido e ninguém repara — o teste passa, a tela
// funciona, e o nome de quem trabalha no card está na internet. Por isso os
// testes de vazamento olham o CONTEÚDO da resposta atrás de dado que não devia
// estar lá, em vez de conferirem campo a campo o que devia.
package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"stacktrack/internal/domain/evento"
	dmembro "stacktrack/internal/domain/membro"
	dpublicacao "stacktrack/internal/domain/publicacao"
	ucboard "stacktrack/internal/usecase/board"
)

// publicar liga o link e devolve o token, falhando o teste se não der.
func (q *quadro) publicar(t *testing.T, boardID, usuarioID string) string {
	t.Helper()
	p, err := q.publicacao.Publicar(context.Background(), boardID, usuarioID)
	if err != nil {
		t.Fatalf("erro ao publicar: %v", err)
	}
	return p.Token
}

// aberto é a colaboração com TUDO o que pendura nome de pessoa num card —
// responsável, comentário e, desde a auditoria, o selo de quem moveu por
// último. Existe para o teste de vazamento poder montar um quadro cheio de
// pessoas e então conferir que nenhuma delas sai pelo link público.
type aberto struct {
	*colaboracao
	responsavelUC *ucboard.ResponsavelUseCase
	comentarioUC  *ucboard.ComentarioUseCase
}

func novoAberto(t *testing.T) *aberto {
	t.Helper()
	c := novaColaboracao(t)
	c.comentarios.LigarUsuarios(c.usuarios)
	c.atividades.LigarUsuarios(c.usuarios)
	// O log precisa RECEBER as escritas, senão mover um card não grava evento
	// nenhum — e o teste de vazamento ficaria verde por não haver nome algum
	// para vazar, que é a pior forma de passar.
	c.card.ComRegistro(c.atividades)
	return &aberto{
		colaboracao:   c,
		responsavelUC: ucboard.NovoResponsavelUseCase(c.membros, c.colunas, c.cards, c.responsaveis),
		comentarioUC:  ucboard.NovoComentarioUseCase(c.membros, c.colunas, c.cards, c.comentarios),
	}
}

func TestPublicarSoODono(t *testing.T) {
	casos := []struct {
		papel    dmembro.Papel
		esperado error
	}{
		// Editor e leitor ENXERGAM o quadro, então a recusa não revela nada que
		// eles já não saibam: é ErrSemPermissao (403), não "não encontrado".
		{dmembro.PapelEditor, dmembro.ErrSemPermissao},
		{dmembro.PapelLeitor, dmembro.ErrSemPermissao},
	}

	for _, caso := range casos {
		t.Run(string(caso.papel), func(t *testing.T) {
			q := novoQuadro()
			boardID := q.criarQuadro(t, "ana", "Roadmap")
			q.convidar(t, boardID, "bob", caso.papel)

			_, err := q.publicacao.Publicar(context.Background(), boardID, "bob")
			if !errors.Is(err, caso.esperado) {
				t.Errorf("erro = %v, esperado %v", err, caso.esperado)
			}
			if p, _ := q.publicacoes.BuscarPorBoard(context.Background(), boardID); p != nil {
				t.Error("o quadro foi publicado por quem não é dono")
			}
		})
	}
}

// Quem não participa não recebe "sem permissão": recebe "não encontrado", como
// em toda a aplicação. Dizer "proibido" confirmaria que o quadro existe.
func TestPublicarQuadroAlheioRespondeNaoEncontrado(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Roadmap")

	_, err := q.publicacao.Publicar(context.Background(), boardID, "estranho")
	if err == nil || !strings.Contains(err.Error(), "não encontrado") {
		t.Errorf("erro = %v, esperado quadro não encontrado", err)
	}
}

// Publicar duas vezes tem de devolver o MESMO link. Um token novo a cada
// chamada invalidaria em silêncio o endereço que o dono já mandou para as
// pessoas — só abrir a tela de compartilhamento de novo quebraria o combinado.
func TestPublicarDuasVezesDevolveOMesmoLink(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Roadmap")

	primeiro := q.publicar(t, boardID, "ana")
	segundo := q.publicar(t, boardID, "ana")

	if primeiro != segundo {
		t.Errorf("o segundo publicar trocou o token: %q -> %q", primeiro, segundo)
	}
}

func TestPublicarERevogarIdempotentesNaoCriamEventoOuRevisaoFantasma(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Roadmap")
	atomica := &escritaAtomicaFalsa{repos: escritaDoQuadro(q)}
	espiao := &publicadorEspiao{}
	q.publicacao.ComEscritaAtomica(atomica)
	q.publicacao.ComPublicador(espiao)

	primeira, err := q.publicacao.Publicar(context.Background(), boardID, "ana")
	if err != nil {
		t.Fatalf("primeira publicação: %v", err)
	}
	segunda, err := q.publicacao.Publicar(context.Background(), boardID, "ana")
	if err != nil {
		t.Fatalf("publicação idempotente: %v", err)
	}
	if segunda.Token != primeira.Token {
		t.Fatal("a publicação idempotente trocou o token")
	}
	if len(atomica.registrados) != 1 || len(espiao.entregues) != 1 {
		t.Fatalf("publicar duas vezes gerou eventos persistidos=%d publicados=%d", len(atomica.registrados), len(espiao.entregues))
	}
	if atomica.registrados[0].Tipo != evento.QuadroPublicado || atomica.registrados[0].Dados != nil {
		t.Fatalf("evento de publicação = %#v; esperado tipo próprio e payload vazio", atomica.registrados[0])
	}
	if atomica.proximaRevisao != 1 {
		t.Fatalf("revisão depois da publicação idempotente = %d, esperado 1", atomica.proximaRevisao)
	}

	if err := q.publicacao.Revogar(context.Background(), boardID, "ana"); err != nil {
		t.Fatalf("primeira revogação: %v", err)
	}
	if err := q.publicacao.Revogar(context.Background(), boardID, "ana"); err != nil {
		t.Fatalf("revogação idempotente: %v", err)
	}
	if len(atomica.registrados) != 2 || len(espiao.entregues) != 2 {
		t.Fatalf("revogar duas vezes gerou eventos persistidos=%d publicados=%d", len(atomica.registrados), len(espiao.entregues))
	}
	ultimo := atomica.registrados[1]
	if ultimo.Tipo != evento.QuadroPublicacaoRevogada || ultimo.Dados != nil {
		t.Fatalf("evento de revogação = %#v; esperado tipo próprio e payload vazio", ultimo)
	}
	if atomica.proximaRevisao != 2 {
		t.Fatalf("revisão depois da revogação idempotente = %d, esperado 2", atomica.proximaRevisao)
	}
}

func TestTokensDeQuadrosDiferentesNaoSeRepetem(t *testing.T) {
	q := novoQuadro()
	um := q.publicar(t, q.criarQuadro(t, "ana", "Um"), "ana")
	outro := q.publicar(t, q.criarQuadro(t, "ana", "Outro"), "ana")

	if um == outro {
		t.Fatal("dois quadros receberam o mesmo token")
	}
	// 256 bits em base64url dão 43 caracteres. Um token curto seria um token
	// adivinhável, e é o tipo de regressão que só aparece se alguém olhar.
	if len(um) < 32 {
		t.Errorf("token com %d caracteres é curto demais para ser um segredo: %q", len(um), um)
	}
}

func TestRevogarDerrubaOLinkNaHora(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Roadmap")
	token := q.publicar(t, boardID, "ana")

	if _, err := q.publicacao.Ver(context.Background(), token); err != nil {
		t.Fatalf("o link devia funcionar antes de revogar: %v", err)
	}

	if err := q.publicacao.Revogar(context.Background(), boardID, "ana"); err != nil {
		t.Fatalf("erro ao revogar: %v", err)
	}

	if _, err := q.publicacao.Ver(context.Background(), token); !errors.Is(err, dpublicacao.ErrNaoEncontrada) {
		t.Errorf("erro = %v, esperado link inválido depois de revogar", err)
	}
}

// Revogar e publicar de novo NÃO pode ressuscitar o endereço antigo. É o que
// separa revogar de esconder: quem guardou a URL numa conversa não volta a
// entrar só porque o dono religou o compartilhamento.
func TestRepublicarNaoRessuscitaOLinkAntigo(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Roadmap")
	antigo := q.publicar(t, boardID, "ana")

	if err := q.publicacao.Revogar(context.Background(), boardID, "ana"); err != nil {
		t.Fatalf("erro ao revogar: %v", err)
	}
	novo := q.publicar(t, boardID, "ana")

	if novo == antigo {
		t.Fatal("republicar devolveu o token antigo — o link revogado voltou a valer")
	}
	if _, err := q.publicacao.Ver(context.Background(), antigo); !errors.Is(err, dpublicacao.ErrNaoEncontrada) {
		t.Errorf("o link antigo ainda abre o quadro (erro = %v)", err)
	}
}

func TestRevogarSoODono(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Roadmap")
	token := q.publicar(t, boardID, "ana")
	q.convidar(t, boardID, "bob", dmembro.PapelEditor)

	if err := q.publicacao.Revogar(context.Background(), boardID, "bob"); !errors.Is(err, dmembro.ErrSemPermissao) {
		t.Errorf("erro = %v, esperado sem permissão", err)
	}
	if _, err := q.publicacao.Ver(context.Background(), token); err != nil {
		t.Errorf("o editor conseguiu derrubar o link do dono: %v", err)
	}
}

// O token é o segredo do link. Entregá-lo a um editor ou a um leitor seria
// deixá-los publicar o quadro por conta própria, repassando o que receberam.
func TestOTokenSoSaiParaODono(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Roadmap")
	q.publicar(t, boardID, "ana")
	q.convidar(t, boardID, "bob", dmembro.PapelEditor)

	if _, err := q.publicacao.Atual(context.Background(), boardID, "bob"); !errors.Is(err, dmembro.ErrSemPermissao) {
		t.Errorf("erro = %v, esperado sem permissão", err)
	}
}

func TestQuadroNaoPublicadoNaoAbrePorTokenNenhum(t *testing.T) {
	q := novoQuadro()
	q.criarQuadro(t, "ana", "Roadmap")

	for _, token := range []string{"", "inventado", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"} {
		if _, err := q.publicacao.Ver(context.Background(), token); !errors.Is(err, dpublicacao.ErrNaoEncontrada) {
			t.Errorf("token %q: erro = %v, esperado link inválido", token, err)
		}
	}
}

func TestVerPublicoValidaOTokenDentroDoInstantaneo(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Roadmap")
	token := q.publicar(t, boardID, "ana")
	usouInstantaneo := false
	q.publicacao.ComInstantaneo(&instantaneoInterceptado{
		leitura: escritaDoQuadro(q),
		antes: func() {
			usouInstantaneo = true
			_ = q.publicacoes.Remover(context.Background(), boardID)
		},
	})

	_, err := q.publicacao.Ver(context.Background(), token)
	if !errors.Is(err, dpublicacao.ErrNaoEncontrada) {
		t.Fatalf("erro = %v, esperado link revogado no início do snapshot", err)
	}
	if !usouInstantaneo {
		t.Fatal("a projeção pública ignorou o InstantaneoConsistente")
	}
}

// Apagar o quadro derruba o link junto, pelo CASCADE do schema. Sem isto, um
// quadro apagado continuaria sendo servido pela rota pública.
func TestApagarOQuadroDerrubaOLink(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Roadmap")
	token := q.publicar(t, boardID, "ana")

	if err := q.quadros.Apagar(context.Background(), boardID, "ana"); err != nil {
		t.Fatalf("erro ao apagar o quadro: %v", err)
	}

	if _, err := q.publicacao.Ver(context.Background(), token); !errors.Is(err, dpublicacao.ErrNaoEncontrada) {
		t.Errorf("erro = %v, esperado link inválido depois de apagar o quadro", err)
	}
}

func TestVerPeloLinkTrazOConteudoDoQuadro(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Roadmap")
	colunaID := q.criarColuna(t, boardID, "ana", "A fazer")
	q.criarCard(t, colunaID, "ana", "Revisar o contrato")
	token := q.publicar(t, boardID, "ana")

	publico, err := q.publicacao.Ver(context.Background(), token)
	if err != nil {
		t.Fatalf("erro ao ver o quadro público: %v", err)
	}

	if publico.Titulo != "Roadmap" {
		t.Errorf("titulo = %q, esperado Roadmap", publico.Titulo)
	}
	if len(publico.Colunas) != 1 || publico.Colunas[0].Titulo != "A fazer" {
		t.Fatalf("colunas = %+v, esperada só 'A fazer'", publico.Colunas)
	}
	if len(publico.Colunas[0].Cards) != 1 || publico.Colunas[0].Cards[0].Titulo != "Revisar o contrato" {
		t.Errorf("cards = %+v, esperado o card criado", publico.Colunas[0].Cards)
	}
}

// O teste que mais importa desta rota: o que sai por ela não pode ter PESSOA
// nenhuma dentro.
//
// Ele monta um quadro com tudo o que carrega nome — responsável, autor de
// comentário, membro — e depois procura esses nomes no JSON inteiro da
// resposta. É de propósito que a busca seja no texto serializado e não campo a
// campo: um campo NOVO que alguém pendure em QuadroPublico amanhã cai nesta
// rede sem ninguém precisar lembrar de atualizar o teste.
func TestOQuadroPublicoNaoVazaPessoas(t *testing.T) {
	a := novoAberto(t)
	ana := a.conta(t, "Ana", "ana@exemplo.com")
	bob := a.conta(t, "Roberto Silva", "roberto@exemplo.com")
	boardID := a.criarQuadro(t, ana, "Roadmap")
	a.convidar(t, boardID, bob, dmembro.PapelEditor)
	colunaID := a.criarColuna(t, boardID, ana, "A fazer")
	pronto := a.criarColuna(t, boardID, ana, "Pronto")
	cardID := a.criarCard(t, colunaID, ana, "Revisar o contrato")

	if err := a.responsavelUC.Atribuir(context.Background(), cardID, bob, ana); err != nil {
		t.Fatalf("erro ao atribuir responsável: %v", err)
	}
	if _, err := a.comentarioUC.Criar(context.Background(), cardID, bob, "isto aqui é confidencial"); err != nil {
		t.Fatalf("erro ao comentar: %v", err)
	}
	// O bob também MOVE o card: assim o nome dele passa a existir no selo de
	// última movimentação, e a busca abaixo cobre também esse caminho — que é o
	// mais recente, e portanto o menos vigiado.
	if _, err := a.card.Mover(context.Background(), cardID, bob, pronto, ucboard.Vizinhos{}); err != nil {
		t.Fatalf("erro ao mover o card: %v", err)
	}
	if m, _ := a.atividades.UltimaMovimentacaoPorCard(context.Background(), boardID); m[cardID].AutorNome != "Roberto Silva" {
		t.Fatalf("o cenário não gravou a movimentação do bob (%+v) — o teste de vazamento não provaria nada", m[cardID])
	}

	publico, err := a.publicacao.Ver(context.Background(), a.publicar(t, boardID, ana))
	if err != nil {
		t.Fatalf("erro ao ver o quadro público: %v", err)
	}

	corpo := serializar(t, publico)
	for _, proibido := range []string{
		"Roberto Silva",            // nome de quem responde pelo card
		"roberto@exemplo.com",      // email de membro
		bob,                        // id de usuário
		"isto aqui é confidencial", // texto de comentário
		cardID,                     // ids endereçáveis em outras rotas
		colunaID,
		boardID,
	} {
		if strings.Contains(corpo, proibido) {
			t.Errorf("o quadro público vazou %q.\nResposta: %s", proibido, corpo)
		}
	}
}

// Descrição e prazo SAEM de propósito: são o conteúdo da tarefa, e publicar o
// quadro é publicá-los. O teste existe para que tirá-los seja uma decisão, e
// não um efeito colateral de mexer na projeção.
func TestOQuadroPublicoTrazOConteudoDaTarefa(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Roadmap")
	colunaID := q.criarColuna(t, boardID, "ana", "A fazer")
	cardID := q.criarCard(t, colunaID, "ana", "Revisar o contrato")
	if _, err := q.card.Editar(context.Background(), cardID, "ana", "Revisar o contrato", "com o jurídico", "", 0); err != nil {
		t.Fatalf("erro ao editar o card: %v", err)
	}

	publico, err := q.publicacao.Ver(context.Background(), q.publicar(t, boardID, "ana"))
	if err != nil {
		t.Fatalf("erro ao ver o quadro público: %v", err)
	}

	if publico.Colunas[0].Cards[0].Descricao != "com o jurídico" {
		t.Errorf("descricao = %q, esperada a descrição do card", publico.Colunas[0].Cards[0].Descricao)
	}
}

// Quem edita precisa saber que o quadro está à vista de fora — e precisa saber
// ANTES de escrever, não depois. É por isso que o aviso vai para todo membro,
// e não só para o dono.
func TestOQuadroDizAQuemEditaQueEstaPublico(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Roadmap")
	q.convidar(t, boardID, "bob", dmembro.PapelEditor)

	antes, err := q.quadros.Detalhar(context.Background(), boardID, "bob")
	if err != nil {
		t.Fatalf("erro ao detalhar: %v", err)
	}
	if antes.Publico {
		t.Error("quadro nunca publicado se descreveu como público")
	}

	q.publicar(t, boardID, "ana")

	depois, err := q.quadros.Detalhar(context.Background(), boardID, "bob")
	if err != nil {
		t.Fatalf("erro ao detalhar: %v", err)
	}
	if !depois.Publico {
		t.Error("o editor não é avisado de que o quadro está público")
	}
}

func serializar(t *testing.T, v *ucboard.QuadroPublico) string {
	t.Helper()
	bytes, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("erro ao serializar o quadro público: %v", err)
	}
	return string(bytes)
}
