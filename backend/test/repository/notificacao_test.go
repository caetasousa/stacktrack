//go:build integracao

// As notificações com os repositórios, a UnidadeDeTrabalho e o outbox de email
// de verdade. Os únicos dublês são os da fronteira de fora: o servidor SMTP de
// mentira e os relógios do worker e do vigia, que o teste adianta em vez de
// esperar.
//
// Como isto quebra — cada teste abaixo cobre um item:
//
//	S1  a atribuição comita e o aviso não; ou a transação cai e sobra um aviso
//	    (ou um email) de uma atribuição que não aconteceu;
//	S2  o email sai na hora, antes de quem está com o sistema aberto ter tido a
//	    chance de ler; ou ler no sistema não cancela o email que ainda não saiu;
//	S3  desfazer a atribuição deixa o aviso contando no sino e o email saindo;
//	    ou reatribuir por engano, antes de a pessoa ler, gera um segundo aviso;
//	S4  quem atribui a si mesmo, ou atribui quem já respondia pelo card, gera
//	    aviso;
//	S5  a menção não chega a quem foi chamado, chega a quem não participa ou ao
//	    próprio autor; editar tirando a menção deixa o aviso (e o email); editar
//	    acrescentando não avisa; apagar o comentário deixa o aviso para trás;
//	S6  o convite aparece no sino de conta NÃO confirmada, não aparece na
//	    confirmada, continua contando depois de recusado ou revogado, ou o
//	    reenvio duplica o aviso;
//	S7  quem sai do quadro continua vendo os avisos dele, ou recebe depois o
//	    email de algo de um quadro de que já não participa;
//	S8  o vigia avisa o mesmo prazo duas vezes, não avisa o prazo novo, avisa
//	    quem não responde pelo card ou avisa prazo que já passou;
//	S9  uma pessoa recebe emails de notificação sem teto;
//	S10 alguém marca como lido o aviso de outra pessoa, ou "marcar todas" deixa
//	    um email na fila;
//	S11 a faxina ignora a retenção escolhida pelo dono, apaga aviso não lido que
//	    ainda vale, ou deixa avisos para sempre;
//	S12 qualquer membro troca a retenção, um valor absurdo é aceito, ou a troca
//	    não fica no histórico;
//	S13 a página do link do email mostra o aviso de outra pessoa;
//	S14 o "carregar mais" do sino repete ou pula avisos;
//	S15 os emails de notificação gastam a cota de convites de quem atribui.
package repository_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	aemail "stacktrack/internal/adapter/email"
	"stacktrack/internal/adapter/repository"
	"stacktrack/internal/adapter/security"
	"stacktrack/internal/domain/membro"
	"stacktrack/internal/domain/notificacao"
	ucboard "stacktrack/internal/usecase/board"
	"stacktrack/internal/usecase/correio"
	ucnotificacao "stacktrack/internal/usecase/notificacao"
	"stacktrack/test/smtpfalso"

	"github.com/google/uuid"
)

// ambienteDeNotificacao é um quadro da Ana com o Bruno (editor) e a Carla
// (leitora), um card, os casos de uso que avisam, o sino, o vigia de prazos, o
// worker de email e o SMTP de mentira.
type ambienteDeNotificacao struct {
	boardID, colunaID, cardID string
	ana, bruno, carla         string

	responsaveis *ucboard.ResponsavelUseCase
	comentarios  *ucboard.ComentarioUseCase
	membros      *ucboard.MembroUseCase
	cards        *ucboard.CardUseCase
	quadros      *ucboard.QuadroUseCase
	sino         *ucnotificacao.UseCase
	vigia        *ucboard.VigiaDePrazos
	repo         *repository.NotificacaoPostgres
	carteiro     *correio.Carteiro
	smtp         *smtpfalso.Servidor
	// desvio é quanto os relógios do worker e do vigia estão à frente do real.
	desvio time.Duration
}

func (a *ambienteDeNotificacao) agora() time.Time        { return time.Now().Add(a.desvio) }
func (a *ambienteDeNotificacao) avancar(d time.Duration) { a.desvio += d }

func novoAmbienteDeNotificacao(t *testing.T) *ambienteDeNotificacao {
	t.Helper()
	limparFilaDeEmail(t)
	ctx := context.Background()
	a := &ambienteDeNotificacao{}
	a.boardID, a.colunaID, a.ana = cenario(t)
	a.bruno = usuarioDeTeste(t, "Bruno Costa")
	a.carla = usuarioDeTeste(t, "Carla Dias")
	for id, papel := range map[string]membro.Papel{a.bruno: membro.PapelEditor, a.carla: membro.PapelLeitor} {
		m, _ := membro.Novo(a.boardID, id, papel)
		if err := repository.NovoMembroPostgres(pool).Salvar(ctx, m); err != nil {
			t.Fatalf("membro: %v", err)
		}
	}
	a.cardID = cardDeTeste(t, a.colunaID, "Migrar o banco")

	cifra, err := security.NovaCifra(chaveAleatoria(t))
	if err != nil {
		t.Fatalf("cifra: %v", err)
	}
	emails := repository.NovoEmailPostgres(pool, cifra)
	unidade := novaUnidade(3 * time.Second).ComCifraDeEmail(cifra)
	registro := repository.NovoEventoPostgres(pool)
	a.repo = repository.NovoNotificacaoPostgres(pool)
	redator := aemail.NovoRedator()
	mudo := slog.New(slog.NewTextHandler(io.Discard, nil))

	notificador := ucboard.NovoNotificador(nil).
		ComEmail(redator, origemDeTeste, notificacao.EsperaDoEmail, nil)

	usuarios := repository.NovoUsuarioPostgres(pool)
	boards := repository.NovoBoardPostgres(pool)
	membros := repository.NovoMembroPostgres(pool)
	colunas := repository.NovoColunaPostgres(pool)
	cards := repository.NovoCardPostgres(pool)
	responsaveis := repository.NovoResponsavelPostgres(pool)
	comentarios := repository.NovoComentarioPostgres(pool)

	a.responsaveis = ucboard.NovoResponsavelUseCase(membros, colunas, cards, responsaveis)
	a.comentarios = ucboard.NovoComentarioUseCase(membros, colunas, cards, comentarios)
	a.membros = ucboard.NovoMembroUseCase(membros, repository.NovoConvitePostgres(pool), usuarios, boards, responsaveis)
	a.membros.ComCorreio(emails, redator, origemDeTeste, nil)
	a.membros.ComIntervaloDeReenvio(0)
	anexos := repository.NovoAnexoPostgres(pool)
	etiquetas := repository.NovoEtiquetaPostgres(pool)
	checklists := repository.NovoChecklistPostgres(pool)
	a.cards = ucboard.NovoCardUseCase(boards, membros, colunas, cards, etiquetas, checklists, anexos, responsaveis, comentarios, nil)
	a.quadros = ucboard.NovoQuadroUseCase(boards, membros, colunas, cards, etiquetas, checklists, anexos, responsaveis, comentarios, nil)
	for _, uc := range []interface {
		ComEscritaAtomica(ucboard.EscritaAtomica)
		ComRegistro(ucboard.RegistroDeEventos)
		ComNotificador(*ucboard.Notificador)
	}{a.responsaveis, a.comentarios, a.membros, a.cards, a.quadros} {
		uc.ComEscritaAtomica(unidade)
		uc.ComRegistro(registro)
		uc.ComNotificador(notificador)
	}

	a.sino = ucnotificacao.Novo(a.repo, nil).ComRelogio(a.agora)
	a.vigia = ucboard.NovoVigiaDePrazos(a.repo, unidade, notificador, mudo).ComRelogio(a.agora)

	a.smtp = smtpfalso.Iniciar(t)
	host, porta := a.smtp.Endereco()
	enviador := aemail.NovoEnviadorSMTP(aemail.ConfigSMTP{
		Host: host, Porta: porta, Remetente: "avisos@stacktrack.teste", NomeDoRemetente: "stacktrack",
		Prazo: 5 * time.Second,
	})
	a.carteiro = correio.NovoCarteiro(emails, enviador, mudo).ComRelogio(a.agora).ComConferencia(a.repo)
	return a
}

// passada roda UMA volta do worker de email.
func (a *ambienteDeNotificacao) passada(t *testing.T) {
	t.Helper()
	if _, err := a.carteiro.Passada(context.Background()); err != nil {
		t.Fatalf("passada do worker: %v", err)
	}
}

// emailsPara conta os emails que o SMTP de mentira recebeu para a pessoa.
func (a *ambienteDeNotificacao) emailsPara(t *testing.T, usuarioID string) int {
	t.Helper()
	endereco := usuarioID + "@teste.dev"
	n := 0
	for _, r := range a.smtp.Recebidas() {
		if len(r.Para) > 0 && r.Para[0] == endereco {
			n++
		}
	}
	return n
}

func (a *ambienteDeNotificacao) pagina(t *testing.T, usuarioID string) *ucnotificacao.Pagina {
	t.Helper()
	p, err := a.sino.Listar(context.Background(), usuarioID, "")
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	return p
}

func (a *ambienteDeNotificacao) atribuir(t *testing.T, cardID, alvo, quem string) {
	t.Helper()
	if err := a.responsaveis.Atribuir(context.Background(), cardID, alvo, quem); err != nil {
		t.Fatalf("atribuir: %v", err)
	}
}

// estadosDosEmails devolve o estado de cada email de notificação da pessoa.
func estadosDosEmails(t *testing.T, usuarioID string) []string {
	t.Helper()
	linhas, err := pool.Query(context.Background(),
		`SELECT e.estado FROM email_outbox e JOIN notificacoes n ON n.id = e.notificacao_id
		  WHERE n.usuario_id = $1 ORDER BY e.criado_em`, usuarioID)
	if err != nil {
		t.Fatalf("ler outbox: %v", err)
	}
	defer linhas.Close()
	var estados []string
	for linhas.Next() {
		var e string
		_ = linhas.Scan(&e)
		estados = append(estados, e)
	}
	return estados
}

func contarNotificacoes(t *testing.T, filtro string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM notificacoes WHERE `+filtro, args...).Scan(&n); err != nil {
		t.Fatalf("contar notificações: %v", err)
	}
	return n
}

func TestS1AtribuicaoAvisoEEmailNascemECaemJuntos(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeNotificacao(t)

	a.atribuir(t, a.cardID, a.bruno, a.ana)
	p := a.pagina(t, a.bruno)
	if p.NaoLidas != 1 || len(p.Itens) != 1 {
		t.Fatalf("sino do Bruno: %d não lidas, %d itens — esperado 1 e 1", p.NaoLidas, len(p.Itens))
	}
	item := p.Itens[0]
	if item.Tipo != notificacao.TipoAtribuicao || item.AutorNome != "Ana" || item.TituloCard != "Migrar o banco" ||
		item.Situacao != notificacao.SituacaoVigente {
		t.Errorf("aviso = %+v", item)
	}
	if e := estadosDosEmails(t, a.bruno); len(e) != 1 || e[0] != "pendente" {
		t.Errorf("emails do Bruno = %v, esperado um pendente", e)
	}

	// A falha é provocada no banco, DEPOIS de a atribuição, o aviso e o email
	// terem sido gravados: o evento é a última escrita da transação.
	outro := cardDeTeste(t, a.colunaID, "Card que vai falhar")
	if _, err := pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION falha_provocada_no_evento() RETURNS trigger AS $$
		BEGIN
			IF NEW.board_id = '`+a.boardID+`'::uuid THEN
				RAISE EXCEPTION 'falha provocada pelo teste';
			END IF;
			RETURN NEW;
		END $$ LANGUAGE plpgsql;
		CREATE TRIGGER falha_provocada_no_evento BEFORE INSERT ON board_events
			FOR EACH ROW EXECUTE FUNCTION falha_provocada_no_evento();`); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `
			DROP TRIGGER IF EXISTS falha_provocada_no_evento ON board_events;
			DROP FUNCTION IF EXISTS falha_provocada_no_evento();`)
	})
	if err := a.responsaveis.Atribuir(ctx, outro, a.bruno, a.ana); err == nil ||
		!strings.Contains(err.Error(), "falha provocada pelo teste") {
		t.Fatalf("a atribuição devia ter falhado pelo evento recusado, veio: %v", err)
	}
	if n := contarNotificacoes(t, `card_id = $1`, outro); n != 0 {
		t.Errorf("S1: %d aviso(s) de uma atribuição desfeita pelo rollback", n)
	}
	if e := estadosDosEmails(t, a.bruno); len(e) != 1 {
		t.Errorf("S1: emails do Bruno depois do rollback = %v, esperado só o primeiro", e)
	}
}

func TestS2OEmailEsperaELerNoSistemaOCancela(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeNotificacao(t)

	a.atribuir(t, a.cardID, a.bruno, a.ana)
	a.passada(t)
	if n := a.emailsPara(t, a.bruno); n != 0 {
		t.Fatalf("S2: o email saiu na hora (%d), sem a espera", n)
	}
	a.avancar(notificacao.EsperaDoEmail + time.Second)
	a.passada(t)
	if n := a.emailsPara(t, a.bruno); n != 1 {
		t.Fatalf("depois da espera, %d email(s) para o Bruno — esperado 1", n)
	}

	// A Carla lê no sistema antes de a espera acabar: o email não sai.
	outro := cardDeTeste(t, a.colunaID, "Revisar o contrato")
	a.atribuir(t, outro, a.carla, a.ana)
	p := a.pagina(t, a.carla)
	if err := a.sino.MarcarLida(ctx, p.Itens[0].ID, a.carla); err != nil {
		t.Fatalf("marcar lida: %v", err)
	}
	a.avancar(notificacao.EsperaDoEmail + time.Second)
	a.passada(t)
	if n := a.emailsPara(t, a.carla); n != 0 {
		t.Errorf("S2: a Carla leu no sistema e recebeu %d email(s)", n)
	}
	if e := estadosDosEmails(t, a.carla); len(e) != 1 || e[0] != "cancelado" {
		t.Errorf("S2: email da Carla = %v, esperado cancelado", e)
	}
}

func TestS3DesfazerAAtribuicaoTiraDoSinoECancelaOEmail(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeNotificacao(t)

	a.atribuir(t, a.cardID, a.bruno, a.ana)
	if err := a.responsaveis.Desatribuir(ctx, a.cardID, a.bruno, a.ana); err != nil {
		t.Fatalf("desatribuir: %v", err)
	}
	p := a.pagina(t, a.bruno)
	if p.NaoLidas != 0 {
		t.Errorf("S3: atribuição desfeita contando no sino (%d)", p.NaoLidas)
	}
	if len(p.Itens) != 1 || p.Itens[0].Situacao != notificacao.SituacaoEncerrada {
		t.Errorf("S3: o aviso devia continuar na lista como encerrado: %+v", p.Itens)
	}
	a.avancar(notificacao.EsperaDoEmail + time.Second)
	a.passada(t)
	if n := a.emailsPara(t, a.bruno); n != 0 {
		t.Errorf("S3: %d email(s) de uma atribuição desfeita", n)
	}
	if e := estadosDosEmails(t, a.bruno); len(e) != 1 || e[0] != "cancelado" {
		t.Errorf("S3: email = %v, esperado cancelado pela conferência", e)
	}

	// Reatribuir antes de ler: o aviso que já existe volta a valer, sem outro.
	a.atribuir(t, a.cardID, a.bruno, a.ana)
	p = a.pagina(t, a.bruno)
	if p.NaoLidas != 1 || len(p.Itens) != 1 {
		t.Errorf("S3: reatribuir antes de ler deu %d não lidas em %d avisos, esperado 1 em 1", p.NaoLidas, len(p.Itens))
	}
}

func TestS4NaoAvisaQuemSeAtribuiNemQuemJaRespondia(t *testing.T) {
	a := novoAmbienteDeNotificacao(t)

	a.atribuir(t, a.cardID, a.bruno, a.bruno)
	if n := contarNotificacoes(t, `usuario_id = $1`, a.bruno); n != 0 {
		t.Errorf("S4: quem se atribuiu recebeu %d aviso(s)", n)
	}

	a.atribuir(t, a.cardID, a.carla, a.ana)
	p := a.pagina(t, a.carla)
	if err := a.sino.MarcarLida(context.Background(), p.Itens[0].ID, a.carla); err != nil {
		t.Fatal(err)
	}
	// Atribuir de novo quem já responde (a rota é idempotente) não é fato novo.
	a.atribuir(t, a.cardID, a.carla, a.ana)
	if n := contarNotificacoes(t, `usuario_id = $1`, a.carla); n != 1 {
		t.Errorf("S4: atribuir quem já respondia gerou outro aviso (%d no total)", n)
	}
}

func TestS5MencaoAcompanhaOTextoDoComentario(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeNotificacao(t)
	estranho := usuarioDeTeste(t, "Zé Ninguém")

	c, err := a.comentarios.Criar(ctx, a.cardID, a.ana, "@Bruno Costa, olha isto. @Zé Ninguém também, e @Ana")
	if err != nil {
		t.Fatalf("comentar: %v", err)
	}
	p := a.pagina(t, a.bruno)
	if p.NaoLidas != 1 || p.Itens[0].Tipo != notificacao.TipoMencao || !strings.Contains(p.Itens[0].Trecho, "olha isto") {
		t.Fatalf("S5: sino do Bruno = %+v", p.Itens)
	}
	if n := contarNotificacoes(t, `usuario_id = ANY($1::uuid[])`, []string{estranho, a.ana, a.carla}); n != 0 {
		t.Errorf("S5: %d aviso(s) para quem não participa, para a autora ou para quem não foi chamada", n)
	}

	// Editar: sai o Bruno, entra a Carla pelo primeiro nome.
	if _, err := a.comentarios.Editar(ctx, c.ID, a.ana, "Pensando melhor, @Carla decide."); err != nil {
		t.Fatalf("editar: %v", err)
	}
	if n := contarNotificacoes(t, `usuario_id = $1 AND comentario_id = $2`, a.bruno, c.ID); n != 0 {
		t.Errorf("S5: o Bruno deixou de ser chamado e o aviso ficou")
	}
	if e := estadosDosEmails(t, a.bruno); len(e) != 0 {
		t.Errorf("S5: o email da menção retirada continuou na fila: %v", e)
	}
	if n := contarNotificacoes(t, `usuario_id = $1 AND comentario_id = $2`, a.carla, c.ID); n != 1 {
		t.Errorf("S5: a Carla passou a ser chamada e não foi avisada")
	}

	// Editar sem mexer na menção não avisa de novo.
	if _, err := a.comentarios.Editar(ctx, c.ID, a.ana, "Pensando melhor, @Carla decide. (corrigido)"); err != nil {
		t.Fatal(err)
	}
	if n := contarNotificacoes(t, `usuario_id = $1`, a.carla); n != 1 {
		t.Errorf("S5: editar sem mexer na menção avisou de novo (%d)", n)
	}

	if err := a.comentarios.Apagar(ctx, c.ID, a.ana); err != nil {
		t.Fatalf("apagar: %v", err)
	}
	if n := contarNotificacoes(t, `comentario_id = $1`, c.ID); n != 0 {
		t.Errorf("S5: o comentário apagado deixou %d aviso(s)", n)
	}
}

func TestS6OConviteNoSinoSegueARegraDoPainel(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeNotificacao(t)
	julia, emailDaJulia := contaDeTeste(t, "Julia")
	kleber, emailDoKleber := contaDeTeste(t, "Kleber")
	if _, err := pool.Exec(ctx, `UPDATE usuarios SET email_confirmado_em = now() WHERE id = $1`, julia); err != nil {
		t.Fatal(err)
	}

	paraJulia, err := a.membros.Convidar(ctx, a.boardID, a.ana, emailDaJulia, membro.PapelEditor)
	if err != nil {
		t.Fatalf("convidar a Julia: %v", err)
	}
	if _, err := a.membros.Convidar(ctx, a.boardID, a.ana, emailDoKleber, membro.PapelEditor); err != nil {
		t.Fatalf("convidar o Kleber: %v", err)
	}
	if p := a.pagina(t, julia); p.NaoLidas != 1 || p.Itens[0].Tipo != notificacao.TipoConvite {
		t.Errorf("S6: a conta confirmada não vê o convite no sino: %+v", p)
	}
	if p := a.pagina(t, kleber); p.NaoLidas != 0 || len(p.Itens) != 0 {
		t.Errorf("S6: a conta NÃO confirmada vê o convite no sino: %+v", p.Itens)
	}
	// Confirmado depois, o aviso aparece — ele já existia, esperando a prova.
	if _, err := pool.Exec(ctx, `UPDATE usuarios SET email_confirmado_em = now() WHERE id = $1`, kleber); err != nil {
		t.Fatal(err)
	}
	if p := a.pagina(t, kleber); p.NaoLidas != 1 {
		t.Errorf("S6: o Kleber confirmou e não vê o convite (%d)", p.NaoLidas)
	}

	// Reenviar não duplica.
	if _, err := a.membros.ReenviarConvite(ctx, a.boardID, paraJulia.Convite.ID, a.ana); err != nil {
		t.Fatalf("reenviar: %v", err)
	}
	if n := contarNotificacoes(t, `usuario_id = $1`, julia); n != 1 {
		t.Errorf("S6: o reenvio duplicou o aviso (%d)", n)
	}

	if err := a.membros.RecusarRecebido(ctx, paraJulia.Convite.ID, julia); err != nil {
		t.Fatalf("recusar: %v", err)
	}
	p := a.pagina(t, julia)
	if p.NaoLidas != 0 || p.Itens[0].Situacao != notificacao.SituacaoConviteRecusado {
		t.Errorf("S6: convite recusado = %d não lidas, situação %q", p.NaoLidas, p.Itens[0].Situacao)
	}

	lista, _ := a.membros.ListarConvites(ctx, a.boardID, a.ana)
	if err := a.membros.RevogarConvite(ctx, lista[0].Convite.ID, a.ana); err != nil {
		t.Fatalf("revogar: %v", err)
	}
	if p := a.pagina(t, kleber); p.NaoLidas != 0 || p.Itens[0].Situacao != notificacao.SituacaoConviteRevogado {
		t.Errorf("S6: convite revogado = %d não lidas, situação %q", p.NaoLidas, p.Itens[0].Situacao)
	}
	// O convite não manda um segundo email: só o do próprio convite.
	if e := estadosDosEmails(t, julia); len(e) != 0 {
		t.Errorf("S6: a notificação do convite mandou email próprio: %v", e)
	}
}

func TestS7QuemSaiDoQuadroDeixaDeVerOsAvisosDele(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeNotificacao(t)

	a.atribuir(t, a.cardID, a.bruno, a.ana)
	if err := a.membros.Remover(ctx, a.boardID, a.ana, a.bruno); err != nil {
		t.Fatalf("remover: %v", err)
	}
	if p := a.pagina(t, a.bruno); p.NaoLidas != 0 || len(p.Itens) != 0 {
		t.Errorf("S7: quem saiu ainda vê %d aviso(s), %d não lido(s)", len(p.Itens), p.NaoLidas)
	}
	if n := contarNotificacoes(t, `usuario_id = $1 AND board_id = $2`, a.bruno, a.boardID); n != 0 {
		t.Errorf("S7: %d aviso(s) do quadro ficaram para quem saiu", n)
	}
	a.avancar(notificacao.EsperaDoEmail + time.Second)
	a.passada(t)
	if n := a.emailsPara(t, a.bruno); n != 0 {
		t.Errorf("S7: quem saiu recebeu %d email(s) do quadro", n)
	}
}

func TestS8OVigiaAvisaCadaPrazoUmaVez(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeNotificacao(t)
	a.atribuir(t, a.cardID, a.bruno, a.ana)
	semResponsavel := cardDeTeste(t, a.colunaID, "Card sem ninguém")
	vencido := cardDeTeste(t, a.colunaID, "Card já vencido")
	a.atribuir(t, vencido, a.bruno, a.ana)

	amanha := time.Now().Add(20 * time.Hour)
	ontem := time.Now().Add(-20 * time.Hour)
	for card, prazo := range map[string]time.Time{a.cardID: amanha, semResponsavel: amanha, vencido: ontem} {
		p := prazo
		if _, err := a.cards.DefinirPrazo(ctx, card, a.ana, &p); err != nil {
			t.Fatalf("prazo: %v", err)
		}
	}

	if n, err := a.vigia.Passada(ctx); err != nil || n != 1 {
		t.Fatalf("S8: primeira passada avisou %d (erro %v), esperado só o card de amanhã", n, err)
	}
	if n, _ := a.vigia.Passada(ctx); n != 0 {
		t.Errorf("S8: a segunda passada avisou o mesmo prazo de novo (%d)", n)
	}
	porTipo := func() (prazos, vigentes int) {
		for _, item := range a.pagina(t, a.bruno).Itens {
			if item.Tipo == notificacao.TipoPrazo {
				prazos++
				if item.Situacao == notificacao.SituacaoVigente {
					vigentes++
				}
			}
		}
		return
	}
	if p, v := porTipo(); p != 1 || v != 1 {
		t.Fatalf("S8: avisos de prazo do Bruno = %d (%d valendo), esperado 1", p, v)
	}
	// Adiar o prazo: o aviso antigo deixa de valer, e o novo avisa quando chegar.
	depois := time.Now().Add(40 * time.Hour)
	if _, err := a.cards.DefinirPrazo(ctx, a.cardID, a.ana, &depois); err != nil {
		t.Fatal(err)
	}
	if p, v := porTipo(); p != 1 || v != 0 {
		t.Errorf("S8: depois de adiar, %d aviso(s) de prazo e %d valendo — esperado o antigo encerrado", p, v)
	}
	if n, _ := a.vigia.Passada(ctx); n != 0 {
		t.Errorf("S8: o prazo novo, ainda longe, já avisou (%d)", n)
	}
	a.avancar(21 * time.Hour)
	if n, _ := a.vigia.Passada(ctx); n != 1 {
		t.Errorf("S8: o prazo novo chegou à véspera e avisou %d vez(es), esperado 1", n)
	}
	if n := contarNotificacoes(t, `card_id = ANY($1::uuid[]) AND tipo = 'prazo'`, []string{semResponsavel, vencido}); n != 0 {
		t.Errorf("S8: %d aviso(s) de prazo de card sem responsável ou já vencido", n)
	}
}

func TestS9OEmailDeNotificacaoTemTetoDiarioPorPessoa(t *testing.T) {
	a := novoAmbienteDeNotificacao(t)
	total := notificacao.MaximoDeEmailsPorDia + 3
	for i := 0; i < total; i++ {
		a.atribuir(t, cardDeTeste(t, a.colunaID, "Card "+uuid.NewString()[:8]), a.bruno, a.ana)
	}
	if n := contarNotificacoes(t, `usuario_id = $1`, a.bruno); n != total {
		t.Errorf("os avisos no sino não têm teto: esperado %d, veio %d", total, n)
	}
	if e := estadosDosEmails(t, a.bruno); len(e) != notificacao.MaximoDeEmailsPorDia {
		t.Errorf("S9: %d email(s) de notificação, esperado o teto de %d", len(e), notificacao.MaximoDeEmailsPorDia)
	}
}

func TestS10LerEhSoDeQuemRecebeuEMarcarTodasCancelaOsEmails(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeNotificacao(t)
	a.atribuir(t, a.cardID, a.bruno, a.ana)
	a.atribuir(t, cardDeTeste(t, a.colunaID, "Outro card"), a.bruno, a.ana)
	p := a.pagina(t, a.bruno)

	if err := a.sino.MarcarLida(ctx, p.Itens[0].ID, a.carla); !errors.Is(err, notificacao.ErrNaoEncontrada) {
		t.Errorf("S10: a Carla marcou o aviso do Bruno (erro = %v)", err)
	}
	if err := a.sino.MarcarTodasLidas(ctx, a.bruno); err != nil {
		t.Fatalf("marcar todas: %v", err)
	}
	if p := a.pagina(t, a.bruno); p.NaoLidas != 0 {
		t.Errorf("S10: %d não lida(s) depois de marcar todas", p.NaoLidas)
	}
	for _, e := range estadosDosEmails(t, a.bruno) {
		if e != "cancelado" {
			t.Errorf("S10: email %q depois de marcar todas, esperado cancelado", e)
		}
	}
	// Marcar de novo não é erro.
	if err := a.sino.MarcarLida(ctx, p.Itens[0].ID, a.bruno); err != nil {
		t.Errorf("marcar de novo: %v", err)
	}
}

func TestS11AFaxinaRespeitaARetencaoDeCadaQuadro(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeNotificacao(t)
	if _, err := a.quadros.DefinirRetencaoDeNotificacoes(ctx, a.boardID, a.ana, 7); err != nil {
		t.Fatalf("retenção: %v", err)
	}
	// Um segundo quadro, que nunca escolheu: vale o padrão.
	outroQuadro, outraColuna, dono := cenario(t)
	m, _ := membro.Novo(outroQuadro, a.bruno, membro.PapelEditor)
	_ = repository.NovoMembroPostgres(pool).Salvar(ctx, m)

	criar := func(boardID, colunaID string, idade time.Duration, lidaHa *time.Duration, valendo bool) string {
		card := cardDeTeste(t, colunaID, "Card "+uuid.NewString()[:8])
		if valendo {
			if err := repository.NovoResponsavelPostgres(pool).Atribuir(ctx, card, a.bruno); err != nil {
				t.Fatal(err)
			}
		}
		n := notificacao.NovaDeAtribuicao(uuid.NewString(), a.bruno, boardID, card, dono, time.Now().Add(-idade))
		if lidaHa != nil {
			l := time.Now().Add(-*lidaHa)
			n.LidaEm = &l
		}
		if _, err := a.repo.Criar(ctx, n); err != nil {
			t.Fatal(err)
		}
		return n.ID
	}
	dia := 24 * time.Hour
	d := func(n int) *time.Duration { v := time.Duration(n) * dia; return &v }

	fica := map[string]string{
		"lida há 6 dias, retenção 7":               criar(a.boardID, a.colunaID, 10*dia, d(6), true),
		"não lida e valendo, 100 dias":             criar(a.boardID, a.colunaID, 100*dia, nil, true),
		"lida há 20 dias, quadro no padrão (30)":   criar(outroQuadro, outraColuna, 25*dia, d(20), true),
		"não lida e encerrada, mais nova que 7 d.": criar(a.boardID, a.colunaID, 3*dia, nil, false),
	}
	sai := map[string]string{
		"lida há 8 dias, retenção 7":             criar(a.boardID, a.colunaID, 10*dia, d(8), true),
		"lida há 31 dias, quadro no padrão (30)": criar(outroQuadro, outraColuna, 40*dia, d(31), true),
		"não lida e encerrada, mais velha que 7": criar(a.boardID, a.colunaID, 8*dia, nil, false),
		"não lida e valendo, além de um ano":     criar(a.boardID, a.colunaID, 400*dia, nil, true),
	}

	if _, err := a.repo.LimparLote(ctx, 1000); err != nil {
		t.Fatalf("faxina: %v", err)
	}
	for caso, id := range fica {
		if contarNotificacoes(t, `id = $1`, id) != 1 {
			t.Errorf("S11: a faxina apagou o aviso %s", caso)
		}
	}
	for caso, id := range sai {
		if contarNotificacoes(t, `id = $1`, id) != 0 {
			t.Errorf("S11: a faxina deixou o aviso %s", caso)
		}
	}
}

func TestS12SoODonoTrocaARetencaoEElaFicaNoHistorico(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeNotificacao(t)

	if _, err := a.quadros.DefinirRetencaoDeNotificacoes(ctx, a.boardID, a.bruno, 7); !errors.Is(err, membro.ErrSemPermissao) {
		t.Errorf("S12: o editor trocou a retenção (erro = %v)", err)
	}
	if _, err := a.quadros.DefinirRetencaoDeNotificacoes(ctx, a.boardID, a.ana, 0); !errors.Is(err, notificacao.ErrRetencaoInvalida) {
		t.Errorf("S12: retenção zero aceita (erro = %v)", err)
	}
	if _, err := a.quadros.DefinirRetencaoDeNotificacoes(ctx, a.boardID, a.ana, 90); err != nil {
		t.Fatalf("definir 90 dias: %v", err)
	}
	b, _ := repository.NovoBoardPostgres(pool).BuscarPorID(ctx, a.boardID)
	if b.RetencaoDeNotificacoes() != 90 {
		t.Errorf("S12: retenção gravada = %d, esperado 90", b.RetencaoDeNotificacoes())
	}
	var tipo string
	var anterior, nova int
	if err := pool.QueryRow(ctx,
		`SELECT tipo, (payload->>'retencaoDiasAnterior')::int, (payload->>'retencaoDias')::int
		   FROM board_events WHERE board_id = $1 ORDER BY seq DESC LIMIT 1`, a.boardID,
	).Scan(&tipo, &anterior, &nova); err != nil {
		t.Fatalf("ler o histórico: %v", err)
	}
	if tipo != "quadro.retencao_notificacoes" || anterior != notificacao.RetencaoPadraoDias || nova != 90 {
		t.Errorf("S12: histórico = %s de %d para %d", tipo, anterior, nova)
	}
}

func TestS13OAvisoDeUmaPessoaNaoAbreParaOutra(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeNotificacao(t)
	a.atribuir(t, a.cardID, a.bruno, a.ana)
	id := a.pagina(t, a.bruno).Itens[0].ID

	if _, err := a.sino.Detalhar(ctx, id, a.carla); !errors.Is(err, notificacao.ErrNaoEncontrada) {
		t.Errorf("S13: a Carla abriu o aviso do Bruno (erro = %v)", err)
	}
	item, err := a.sino.Detalhar(ctx, id, a.bruno)
	if err != nil || item.CardID != a.cardID || item.BoardID != a.boardID {
		t.Errorf("S13: o Bruno não abre o próprio aviso: %+v, %v", item, err)
	}
}

func TestS14OCarregarMaisNaoRepeteNemPulaAvisos(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeNotificacao(t)
	total := notificacao.TamanhoDaPagina + 5
	for i := 0; i < total; i++ {
		a.atribuir(t, cardDeTeste(t, a.colunaID, "Card "+uuid.NewString()[:8]), a.bruno, a.ana)
	}

	primeira, err := a.sino.Listar(ctx, a.bruno, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(primeira.Itens) != notificacao.TamanhoDaPagina || !primeira.TemMais {
		t.Fatalf("S14: primeira página com %d avisos (temMais %v)", len(primeira.Itens), primeira.TemMais)
	}
	segunda, err := a.sino.Listar(ctx, a.bruno, primeira.Itens[len(primeira.Itens)-1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(segunda.Itens) != total-notificacao.TamanhoDaPagina || segunda.TemMais {
		t.Errorf("S14: segunda página com %d avisos (temMais %v), esperado %d e false",
			len(segunda.Itens), segunda.TemMais, total-notificacao.TamanhoDaPagina)
	}
	vistos := map[string]bool{}
	for _, item := range append(primeira.Itens, segunda.Itens...) {
		if vistos[item.ID] {
			t.Errorf("S14: o aviso %s apareceu nas duas páginas", item.ID)
		}
		vistos[item.ID] = true
	}
	if len(vistos) != total {
		t.Errorf("S14: as duas páginas somam %d avisos distintos, esperado %d", len(vistos), total)
	}
	// O cursor de outra pessoa não abre a lista do Bruno a partir dele.
	carlaVe, _ := a.sino.Listar(ctx, a.carla, primeira.Itens[0].ID)
	if len(carlaVe.Itens) != 0 {
		t.Errorf("S14: a Carla listou %d aviso(s) com o cursor do Bruno", len(carlaVe.Itens))
	}
}

func TestS15EmailDeNotificacaoNaoGastaACotaDeConvites(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeNotificacao(t)
	// Mais emails de notificação pedidos pela Ana do que a cota diária de
	// convites dela: o teto por quem RECEBE segura 20 para cada um.
	for _, alvo := range []string{a.bruno, a.carla} {
		for i := 0; i < notificacao.MaximoDeEmailsPorDia; i++ {
			a.atribuir(t, cardDeTeste(t, a.colunaID, "Card "+uuid.NewString()[:8]), alvo, a.ana)
		}
	}
	res, err := a.membros.Convidar(ctx, a.boardID, a.ana, emailNovo(), membro.PapelEditor)
	if err != nil {
		t.Fatalf("convidar: %v", err)
	}
	if res.Entrega != ucboard.EntregaPorEmail {
		t.Errorf("S15: depois de atribuir cards, o convite da Ana saiu como %q — a notificação gastou a cota de convites", res.Entrega)
	}
}
