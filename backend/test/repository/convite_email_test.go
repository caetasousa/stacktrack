//go:build integracao

// O convite por email, com os repositórios e a UnidadeDeTrabalho de verdade.
// O único dublê é o da fronteira de fora: um servidor SMTP de mentira (pacote
// smtpfalso) e o relógio do worker. Outbox, cifra, redator, cliente SMTP e
// worker rodam como em produção.
//
// Como isto quebra — cada teste abaixo cobre um item:
//
//	I1  o convite é gravado e o email não entra na fila; ou a transação cai
//	    depois de enfileirar, e a mensagem fica órfã, sem convite;
//	I2  o link do convite fica em claro no banco, ou continua lá depois de o
//	    email sair; ou o link que chega no email não abre o convite;
//	I3  o servidor SMTP falha por um instante e o email se perde, ou o convite
//	    é desfeito junto;
//	I4  o servidor recusa o endereço em definitivo e o worker insiste até
//	    esgotar as tentativas — queimando a reputação do remetente;
//	I5  revogar, ou aceitar pelo link manual, deixa sair depois um email com
//	    link morto;
//	I6  reenviar deixa o link antigo valendo, ou duas abas reenviando juntas
//	    mandam dois emails com links diferentes;
//	I7  uma conta manda emails de convite sem teto;
//	I8  a resposta de erro do servidor SMTP grava no banco o endereço completo
//	    de quem foi convidado;
//	I9  sem SMTP configurado, convidar quebra em vez de devolver o link manual;
//	I10 um quadro acumula convites em aberto sem limite;
//	I11 apagar o quadro deixa na fila o email de um convite que não existe mais;
//	I12 o mesmo email sai duas vezes numa operação normal;
//	I13 o email que nunca saiu continua com o link no banco depois de vencer.
package repository_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"time"

	aemail "stacktrack/internal/adapter/email"
	"stacktrack/internal/adapter/repository"
	"stacktrack/internal/adapter/security"
	dconvite "stacktrack/internal/domain/convite"
	"stacktrack/internal/domain/membro"
	ucboard "stacktrack/internal/usecase/board"
	"stacktrack/internal/usecase/correio"
	"stacktrack/test/smtpfalso"

	"github.com/google/uuid"
)

const origemDeTeste = "https://stacktrack.teste"

// ambienteDeConvite é um quadro com dona, o caso de uso de membros ligado ao
// outbox de verdade, o worker e o SMTP de mentira.
type ambienteDeConvite struct {
	uc       *ucboard.MembroUseCase
	emails   *repository.EmailPostgres
	carteiro *correio.Carteiro
	smtp     *smtpfalso.Servidor
	boardID  string
	dona     string
	// desvio é quanto o relógio do worker está à frente do real. Avançá-lo é
	// como o teste espera a próxima tentativa sem dormir.
	desvio time.Duration
}

// agora é o relógio do worker: o real, mais o que o teste já avançou.
func (a *ambienteDeConvite) agora() time.Time { return time.Now().Add(a.desvio) }

// avancar adianta o relógio do worker.
func (a *ambienteDeConvite) avancar(d time.Duration) { a.desvio += d }

func novoAmbienteDeConvite(t *testing.T) *ambienteDeConvite {
	t.Helper()
	limparFilaDeEmail(t)
	boardID, _, dona := cenario(t)

	cifra, err := security.NovaCifra(chaveAleatoria(t))
	if err != nil {
		t.Fatalf("cifra: %v", err)
	}
	emails := repository.NovoEmailPostgres(pool, cifra)

	uc := ucboard.NovoMembroUseCase(
		repository.NovoMembroPostgres(pool), repository.NovoConvitePostgres(pool),
		repository.NovoUsuarioPostgres(pool), repository.NovoBoardPostgres(pool),
		repository.NovoResponsavelPostgres(pool),
	)
	uc.ComEscritaAtomica(novaUnidade(3 * time.Second).ComCifraDeEmail(cifra))
	uc.ComCorreio(emails, aemail.NovoRedator(), origemDeTeste, nil)
	uc.ComIntervaloDeReenvio(0)

	smtp := smtpfalso.Iniciar(t)
	host, porta := smtp.Endereco()
	enviador := aemail.NovoEnviadorSMTP(aemail.ConfigSMTP{
		Host: host, Porta: porta,
		Remetente: "convites@stacktrack.teste", NomeDoRemetente: "stacktrack",
		Prazo: 5 * time.Second,
	})

	a := &ambienteDeConvite{uc: uc, emails: emails, smtp: smtp, boardID: boardID, dona: dona}
	a.carteiro = correio.NovoCarteiro(emails, enviador, slog.New(slog.NewTextHandler(io.Discard, nil))).
		ComRelogio(a.agora)
	return a
}

// passada roda UMA volta do worker, como o laço dele faria.
func (a *ambienteDeConvite) passada(t *testing.T) {
	t.Helper()
	if _, err := a.carteiro.Passada(context.Background()); err != nil {
		t.Fatalf("passada do worker: %v", err)
	}
}

// linhaDeEmail é o que fica gravado no outbox, lido direto da tabela.
type linhaDeEmail struct {
	estado     string
	tentativas int
	proximaEm  time.Time
	payload    []byte
	ultimoErro string
}

// String resume a linha para a mensagem de falha — sem despejar o payload.
func (l linhaDeEmail) String() string {
	return fmt.Sprintf("{estado:%s tentativas:%d com_payload:%v erro:%q}",
		l.estado, l.tentativas, l.payload != nil, l.ultimoErro)
}

func linhasDoConvite(t *testing.T, conviteID string) []linhaDeEmail {
	t.Helper()
	linhas, err := pool.Query(context.Background(),
		`SELECT estado, tentativas, proxima_em, payload_cifrado, COALESCE(ultimo_erro, '')
		   FROM email_outbox WHERE convite_id = $1 ORDER BY criado_em`, conviteID)
	if err != nil {
		t.Fatalf("ler outbox: %v", err)
	}
	defer linhas.Close()
	var todas []linhaDeEmail
	for linhas.Next() {
		var l linhaDeEmail
		if err := linhas.Scan(&l.estado, &l.tentativas, &l.proximaEm, &l.payload, &l.ultimoErro); err != nil {
			t.Fatalf("ler outbox: %v", err)
		}
		todas = append(todas, l)
	}
	return todas
}

func limparFilaDeEmail(t *testing.T) {
	t.Helper()
	// Os testes deste pacote dividem o banco e rodam um de cada vez: sem isto, o
	// worker de um teste mandaria a mensagem que outro deixou na fila.
	if _, err := pool.Exec(context.Background(), `DELETE FROM email_outbox`); err != nil {
		t.Fatalf("limpar outbox: %v", err)
	}
}

func chaveAleatoria(t *testing.T) []byte {
	t.Helper()
	chave := make([]byte, 32)
	if _, err := rand.Read(chave); err != nil {
		t.Fatal(err)
	}
	return chave
}

func emailNovo() string { return "convidada-" + uuid.NewString() + "@teste.dev" }

var linkNoTexto = regexp.MustCompile(regexp.QuoteMeta(origemDeTeste) + `/convite/([A-Za-z0-9_-]+)`)

// tokenDoEmail tira do corpo de texto o token do link — o que a pessoa clica.
func tokenDoEmail(t *testing.T, texto string) string {
	t.Helper()
	m := linkNoTexto.FindStringSubmatch(texto)
	if m == nil {
		t.Fatalf("o email não traz o link do convite:\n%s", texto)
	}
	return m[1]
}

// I1
func TestConviteEEmailNascemECaemJuntos(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeConvite(t)

	res, err := a.uc.Convidar(ctx, a.boardID, a.dona, emailNovo(), membro.PapelEditor)
	if err != nil {
		t.Fatalf("convidar: %v", err)
	}
	if res.Entrega != ucboard.EntregaPorEmail {
		t.Fatalf("entrega = %q, esperado email", res.Entrega)
	}
	if l := linhasDoConvite(t, res.Convite.ID); len(l) != 1 || l[0].estado != "pendente" || l[0].payload == nil {
		t.Fatalf("outbox do convite = %v, esperado um envio pendente com a mensagem", l)
	}

	// A falha é provocada no próprio banco, DEPOIS de o convite e o email terem
	// sido gravados: o evento é a última escrita da transação.
	// O trigger só recusa eventos DESTE quadro: os outros testes do pacote
	// dividem o banco e não podem sentir a falha.
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

	outro := emailNovo()
	// A falha precisa ser A PROVOCADA: falhar por outro motivo (uma cota, um
	// lock) também deixaria o banco vazio, e o teste passaria sem provar nada.
	if _, err := a.uc.Convidar(ctx, a.boardID, a.dona, outro, membro.PapelEditor); err == nil ||
		!strings.Contains(err.Error(), "falha provocada pelo teste") {
		t.Fatalf("o convite devia ter falhado pela escrita do evento recusada, veio: %v", err)
	}
	var convites, mensagens int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM convites_board WHERE email = $1`, outro).Scan(&convites)
	_ = pool.QueryRow(ctx,
		`SELECT count(*) FROM email_outbox o JOIN convites_board c ON c.id = o.convite_id WHERE c.email = $1`,
		outro).Scan(&mensagens)
	var orfas int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM email_outbox WHERE convite_id IS NULL`).Scan(&orfas)
	if convites != 0 || mensagens != 0 || orfas != 0 {
		t.Fatalf("depois do rollback: %d convite(s), %d mensagem(ns), %d órfã(s) — esperado nada", convites, mensagens, orfas)
	}
}

// I2 e I12
func TestOLinkViajaCifradoChegaInteiroESomeDoBanco(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeConvite(t)
	destino := emailNovo()

	res, err := a.uc.Convidar(ctx, a.boardID, a.dona, destino, membro.PapelLeitor)
	if err != nil {
		t.Fatalf("convidar: %v", err)
	}

	// Antes de sair: nem o token nem o endereço aparecem em claro no banco.
	linhas := linhasDoConvite(t, res.Convite.ID)
	if len(linhas) != 1 {
		t.Fatalf("outbox = %v", linhas)
	}
	if bytes.Contains(linhas[0].payload, []byte(res.Token)) || bytes.Contains(linhas[0].payload, []byte(destino)) {
		t.Fatal("o payload do outbox guarda o link ou o endereço em claro")
	}

	a.passada(t)
	recebidas := a.smtp.Esperar(t, 1, 5*time.Second)
	if recebidas[0].Para[0] != destino {
		t.Fatalf("destinatário = %v, esperado %s", recebidas[0].Para, destino)
	}
	lida := recebidas[0].Ler(t)
	if !strings.Contains(lida.Assunto, "Quadro de teste") || !strings.Contains(lida.Assunto, "Ana") {
		t.Errorf("assunto = %q, esperado o quadro e quem convidou", lida.Assunto)
	}

	// O link que chegou abre o convite: é o mesmo token que o banco conhece pelo hash.
	token := tokenDoEmail(t, lida.Texto)
	detalhe, err := a.uc.DetalharConvite(ctx, token)
	if err != nil {
		t.Fatalf("o link do email não abre o convite: %v", err)
	}
	if detalhe.TituloQuadro != "Quadro de teste" {
		t.Errorf("quadro do convite = %q", detalhe.TituloQuadro)
	}

	// Depois de sair: a linha fica como registro, sem a mensagem.
	linhas = linhasDoConvite(t, res.Convite.ID)
	if linhas[0].estado != "enviado" || linhas[0].payload != nil {
		t.Fatalf("depois do envio: %v, esperado enviado e sem payload", linhas[0])
	}

	// I12: uma segunda volta do worker não manda de novo.
	a.avancar(time.Hour)
	a.passada(t)
	if n := len(a.smtp.Recebidas()); n != 1 {
		t.Fatalf("%d emails recebidos, esperado 1", n)
	}
}

// I3
func TestFalhaPassageiraDoSMTPTentaDeNovoSemDesfazerOConvite(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeConvite(t)
	res, err := a.uc.Convidar(ctx, a.boardID, a.dona, emailNovo(), membro.PapelEditor)
	if err != nil {
		t.Fatalf("convidar: %v", err)
	}

	a.smtp.RecusarProximos(1, "451 4.3.0 sistema ocupado, tente mais tarde")
	momentoDaFalha := a.agora()
	a.passada(t)

	linha := linhasDoConvite(t, res.Convite.ID)[0]
	if linha.estado != "pendente" || linha.tentativas != 1 || linha.payload == nil {
		t.Fatalf("depois da falha passageira: %v, esperado pendente com 1 tentativa", linha)
	}
	if espera := linha.proximaEm.Sub(momentoDaFalha); espera < 20*time.Second || espera > 40*time.Second {
		t.Errorf("próxima tentativa em %v, esperado ~30s", espera)
	}
	if !strings.Contains(linha.ultimoErro, "451") {
		t.Errorf("último erro = %q, esperado a resposta do servidor", linha.ultimoErro)
	}
	if _, err := a.uc.DetalharConvite(ctx, res.Token); err != nil {
		t.Fatalf("a falha do SMTP desfez o convite: %v", err)
	}

	// Antes da hora, nada sai; na hora, sai.
	a.passada(t)
	if len(a.smtp.Recebidas()) != 0 {
		t.Fatal("o worker tentou de novo antes da hora")
	}
	a.avancar(time.Minute)
	a.passada(t)
	a.smtp.Esperar(t, 1, 5*time.Second)
	if l := linhasDoConvite(t, res.Convite.ID)[0]; l.estado != "enviado" {
		t.Fatalf("depois da nova tentativa: %v", l)
	}
}

// I4 e I8
func TestRecusaDefinitivaDesisteNaHoraSemGravarOEndereco(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeConvite(t)
	destino := emailNovo()
	res, err := a.uc.Convidar(ctx, a.boardID, a.dona, destino, membro.PapelEditor)
	if err != nil {
		t.Fatalf("convidar: %v", err)
	}

	a.smtp.RecusarProximos(1, "550 5.1.1 <"+destino+">: destinatário desconhecido")
	a.passada(t)

	linha := linhasDoConvite(t, res.Convite.ID)[0]
	if linha.estado != "falhou" || linha.tentativas != 1 {
		t.Fatalf("depois da recusa definitiva: %v, esperado falhou na primeira", linha)
	}
	if strings.Contains(linha.ultimoErro, destino) {
		t.Fatalf("o erro gravado traz o endereço completo: %q", linha.ultimoErro)
	}
	if !strings.Contains(linha.ultimoErro, "550") {
		t.Errorf("último erro = %q, esperado o código da recusa", linha.ultimoErro)
	}

	a.avancar(24 * time.Hour)
	a.passada(t)
	if len(a.smtp.Recebidas()) != 0 {
		t.Fatal("o worker insistiu depois de uma recusa definitiva")
	}
}

// I5
func TestConviteQueDeixouDeValerNaoMandaEmail(t *testing.T) {
	ctx := context.Background()

	t.Run("revogado", func(t *testing.T) {
		a := novoAmbienteDeConvite(t)
		res, err := a.uc.Convidar(ctx, a.boardID, a.dona, emailNovo(), membro.PapelEditor)
		if err != nil {
			t.Fatalf("convidar: %v", err)
		}
		if err := a.uc.RevogarConvite(ctx, res.Convite.ID, a.dona); err != nil {
			t.Fatalf("revogar: %v", err)
		}
		a.passada(t)
		if len(a.smtp.Recebidas()) != 0 {
			t.Fatal("o email do convite revogado saiu")
		}
		if l := linhasDoConvite(t, res.Convite.ID)[0]; l.estado != "cancelado" || l.payload != nil {
			t.Fatalf("outbox do convite revogado: %v, esperado cancelado e sem payload", l)
		}
	})

	t.Run("aceito pelo link manual antes do email sair", func(t *testing.T) {
		a := novoAmbienteDeConvite(t)
		convidadaID, convidadaEmail := contaDeTeste(t, "Bia")
		res, err := a.uc.Convidar(ctx, a.boardID, a.dona, convidadaEmail, membro.PapelEditor)
		if err != nil {
			t.Fatalf("convidar: %v", err)
		}
		if _, _, err := a.uc.Aceitar(ctx, res.Token, convidadaID); err != nil {
			t.Fatalf("aceitar: %v", err)
		}
		a.passada(t)
		if len(a.smtp.Recebidas()) != 0 {
			t.Fatal("o email de um convite já aceito saiu")
		}
		if l := linhasDoConvite(t, res.Convite.ID)[0]; l.estado != "cancelado" {
			t.Fatalf("outbox do convite aceito: %v, esperado cancelado", l)
		}
	})
}

// I6
func TestReenviarTrocaOLinkEMandaUmEmailSo(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeConvite(t)
	res, err := a.uc.Convidar(ctx, a.boardID, a.dona, emailNovo(), membro.PapelEditor)
	if err != nil {
		t.Fatalf("convidar: %v", err)
	}
	a.passada(t)
	primeiro := a.smtp.Esperar(t, 1, 5*time.Second)[0].Ler(t)

	novo, err := a.uc.ReenviarConvite(ctx, a.boardID, res.Convite.ID, a.dona)
	if err != nil {
		t.Fatalf("reenviar: %v", err)
	}
	if novo.Entrega != ucboard.EntregaPorEmail {
		t.Fatalf("entrega do reenvio = %q", novo.Entrega)
	}
	a.passada(t)
	segundo := a.smtp.Esperar(t, 2, 5*time.Second)[1].Ler(t)

	tokenAntigo, tokenNovo := tokenDoEmail(t, primeiro.Texto), tokenDoEmail(t, segundo.Texto)
	if tokenAntigo == tokenNovo {
		t.Fatal("o reenvio mandou o mesmo link")
	}
	if _, err := a.uc.DetalharConvite(ctx, tokenAntigo); !errors.Is(err, dconvite.ErrInvalido) {
		t.Fatalf("o link anterior ainda abre o convite: %v", err)
	}
	if _, err := a.uc.DetalharConvite(ctx, tokenNovo); err != nil {
		t.Fatalf("o link novo não abre o convite: %v", err)
	}

	// Duas abas reenviando ao mesmo tempo, com o intervalo mínimo já vencido
	// desde o último envio: uma passa e a outra é recusada — e só um email novo
	// entra na fila. O intervalo é curto porque o relógio do caso de uso é o de
	// verdade.
	a.uc.ComIntervaloDeReenvio(2 * time.Second)
	time.Sleep(2500 * time.Millisecond)
	errA, errB := emParalelo(
		func() error { _, err := a.uc.ReenviarConvite(ctx, a.boardID, res.Convite.ID, a.dona); return err },
		func() error { _, err := a.uc.ReenviarConvite(ctx, a.boardID, res.Convite.ID, a.dona); return err },
	)
	if (errA == nil) == (errB == nil) {
		t.Fatalf("esperado exatamente um reenvio aceito: %v / %v", errA, errB)
	}
	if recusado := errors.Join(errA, errB); !errors.Is(recusado, dconvite.ErrReenvioCedoDemais) {
		t.Fatalf("o reenvio recusado devia ser por intervalo: %v", recusado)
	}
	if l := linhasDoConvite(t, res.Convite.ID); len(l) != 3 || l[2].estado != "pendente" {
		t.Fatalf("outbox depois da corrida: %v, esperado 2 enviados e 1 pendente", l)
	}
}

// I7
func TestAContaTemTetoDiarioDeEmailsDeConvite(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeConvite(t)

	for i := range dconvite.MaximoDeEmailsPorDia {
		res, err := a.uc.Convidar(ctx, a.boardID, a.dona, emailNovo(), membro.PapelLeitor)
		if err != nil || res.Entrega != ucboard.EntregaPorEmail {
			t.Fatalf("convite %d: entrega %v, erro %v", i+1, res, err)
		}
	}

	excedente, err := a.uc.Convidar(ctx, a.boardID, a.dona, emailNovo(), membro.PapelLeitor)
	if err != nil {
		t.Fatalf("o convite além do teto devia nascer com o link manual: %v", err)
	}
	if excedente.Entrega != ucboard.EntregaCotaEsgotada {
		t.Fatalf("entrega além do teto = %q, esperado cota", excedente.Entrega)
	}
	if l := linhasDoConvite(t, excedente.Convite.ID); len(l) != 0 {
		t.Fatalf("o convite além do teto enfileirou email: %v", l)
	}
	if _, err := a.uc.DetalharConvite(ctx, excedente.Token); err != nil {
		t.Fatalf("o convite além do teto não vale: %v", err)
	}
}

// I9
func TestSemSMTPOConviteVoltaComOLinkManual(t *testing.T) {
	ctx := context.Background()
	limparFilaDeEmail(t)
	boardID, _, dona := cenario(t)
	uc := ucboard.NovoMembroUseCase(
		repository.NovoMembroPostgres(pool), repository.NovoConvitePostgres(pool),
		repository.NovoUsuarioPostgres(pool), repository.NovoBoardPostgres(pool),
		repository.NovoResponsavelPostgres(pool),
	)
	// A montagem de um ambiente sem SMTP: unidade de trabalho sem cifra, caso de
	// uso sem correio.
	uc.ComEscritaAtomica(novaUnidade(3 * time.Second))

	res, err := uc.Convidar(ctx, boardID, dona, emailNovo(), membro.PapelEditor)
	if err != nil {
		t.Fatalf("convidar sem SMTP: %v", err)
	}
	if res.Entrega != ucboard.EntregaManual || res.Token == "" {
		t.Fatalf("resultado sem SMTP = %v, esperado entrega manual com token", res)
	}
	var mensagens int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM email_outbox`).Scan(&mensagens)
	if mensagens != 0 {
		t.Fatalf("%d mensagem(ns) na fila sem SMTP configurado", mensagens)
	}
}

// I10
func TestOQuadroTemTetoDeConvitesEmAberto(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeConvite(t)
	for i := range dconvite.MaximoDePendentesPorQuadro {
		if _, err := a.uc.Convidar(ctx, a.boardID, a.dona, emailNovo(), membro.PapelLeitor); err != nil {
			t.Fatalf("convite %d: %v", i+1, err)
		}
	}
	if _, err := a.uc.Convidar(ctx, a.boardID, a.dona, emailNovo(), membro.PapelLeitor); !errors.Is(err, dconvite.ErrPendentesDemais) {
		t.Fatalf("convite além do teto do quadro: %v, esperado ErrPendentesDemais", err)
	}
}

// I11
func TestApagarOQuadroLevaOEmailPendente(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeConvite(t)
	res, err := a.uc.Convidar(ctx, a.boardID, a.dona, emailNovo(), membro.PapelEditor)
	if err != nil {
		t.Fatalf("convidar: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM boards WHERE id = $1`, a.boardID); err != nil {
		t.Fatalf("apagar quadro: %v", err)
	}
	if l := linhasDoConvite(t, res.Convite.ID); len(l) != 0 {
		t.Fatalf("o email do quadro apagado ficou na fila: %v", l)
	}
	a.passada(t)
	if len(a.smtp.Recebidas()) != 0 {
		t.Fatal("saiu email de um quadro que não existe mais")
	}
}

// I13
func TestOEmailQueVenceuSemSairPerdeOLink(t *testing.T) {
	ctx := context.Background()
	a := novoAmbienteDeConvite(t)
	res, err := a.uc.Convidar(ctx, a.boardID, a.dona, emailNovo(), membro.PapelEditor)
	if err != nil {
		t.Fatalf("convidar: %v", err)
	}
	// O relógio do banco não é controlável; o vencimento é: a linha passa a
	// valer até ontem, como um envio que ficou na fila além da validade.
	if _, err := pool.Exec(ctx,
		`UPDATE email_outbox SET expira_em = now() - interval '1 day' WHERE convite_id = $1`,
		res.Convite.ID); err != nil {
		t.Fatalf("vencer: %v", err)
	}

	a.passada(t)
	if len(a.smtp.Recebidas()) != 0 {
		t.Fatal("saiu um email vencido")
	}
	if _, err := a.emails.LimparLote(ctx, 100); err != nil {
		t.Fatalf("faxina do outbox: %v", err)
	}
	if l := linhasDoConvite(t, res.Convite.ID)[0]; l.estado != "expirado" || l.payload != nil {
		t.Fatalf("depois da faxina: %v, esperado expirado e sem payload", l)
	}
}
