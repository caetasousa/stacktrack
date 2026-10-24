//go:build integracao

// Confirmação de email e convites na conta, com os repositórios e as unidades de
// trabalho de verdade. O único dublê, de novo, é o da fronteira de fora: o
// servidor SMTP de mentira.
//
// Como isto quebra — cada teste abaixo cobre um item:
//
//	C1  o cadastro não manda a confirmação, guarda o token em claro, ou o link
//	    confirma mais de uma vez;
//	C2  reenviar a confirmação não tem teto, ou manda email para quem já
//	    confirmou;
//	C3  o painel mostra convites para conta NÃO confirmada — quem cadastrou o
//	    email de outra pessoa veria os convites dela —, ou mostra convite de
//	    outro email, revogado ou vencido;
//	C4  aceitar pelo painel vale para convite de outro email (basta saber o
//	    id), ou não põe a pessoa no quadro;
//	C5  recusar não encerra o convite, deixa o email dele na fila, não fica no
//	    histórico, ou prende a vaga e o dono não consegue convidar de novo;
//	C6  trocar a senha pelo link do email — que prova a caixa de entrada — não
//	    confirma a conta;
//	C7  aceitar e recusar ao mesmo tempo passam os dois.
package repository_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	aemail "stacktrack/internal/adapter/email"
	"stacktrack/internal/adapter/repository"
	"stacktrack/internal/adapter/security"
	dconfirmacao "stacktrack/internal/domain/confirmacao"
	dconvite "stacktrack/internal/domain/convite"
	"stacktrack/internal/domain/membro"
	"stacktrack/internal/pkg/token"
	ucauth "stacktrack/internal/usecase/auth"
	ucboard "stacktrack/internal/usecase/board"
	"stacktrack/internal/usecase/correio"
	"stacktrack/test/smtpfalso"

	"github.com/google/uuid"
)

// ambienteDeConta é uma dona com um quadro, as duas unidades de trabalho com o
// mesmo outbox cifrado, os casos de uso de conta e de membros, o worker e o
// SMTP de mentira.
type ambienteDeConta struct {
	cadastrar *ucauth.CadastrarUseCase
	confirmar *ucauth.ConfirmarEmailUseCase
	recuperar *ucauth.RecuperarSenhaUseCase
	membros   *ucboard.MembroUseCase
	carteiro  *correio.Carteiro
	smtp      *smtpfalso.Servidor
	boardID   string
	dona      string
}

func novoAmbienteDeConta(t *testing.T) *ambienteDeConta {
	t.Helper()
	limparFilaDeEmail(t)
	boardID, _, dona := cenario(t)

	cifra, err := security.NovaCifra(chaveAleatoria(t))
	if err != nil {
		t.Fatalf("cifra: %v", err)
	}
	usuarios := repository.NovoUsuarioPostgres(pool)
	sessoes := repository.NovoSessionPostgres(pool)
	hasher := security.NovoHasherArgon2id()
	unidadeDeAuth := repository.NovaUnidadeDeAutenticacao(pool).ComCifraDeEmail(cifra)
	redator := aemail.NovoRedator()

	a := &ambienteDeConta{boardID: boardID, dona: dona}
	a.confirmar = ucauth.NovoConfirmarEmailUseCase(usuarios, unidadeDeAuth)
	a.confirmar.ComCorreio(redator, origemDeTeste, nil)
	a.cadastrar = ucauth.NovoCadastrarUseCase(usuarios, sessoes, hasher)
	a.cadastrar.ComUnidadeDeTrabalho(unidadeDeAuth)
	a.cadastrar.AoCadastrar(func(ctx context.Context, usuarioID string) {
		if err := a.confirmar.Enviar(ctx, usuarioID); err != nil {
			t.Errorf("enviar a confirmação: %v", err)
		}
	})
	a.recuperar = ucauth.NovoRecuperarSenhaUseCase(usuarios, hasher, unidadeDeAuth)
	a.recuperar.ComCorreio(redator, origemDeTeste, nil)

	emails := repository.NovoEmailPostgres(pool, cifra)
	a.membros = ucboard.NovoMembroUseCase(
		repository.NovoMembroPostgres(pool), repository.NovoConvitePostgres(pool),
		usuarios, repository.NovoBoardPostgres(pool), repository.NovoResponsavelPostgres(pool),
	)
	a.membros.ComEscritaAtomica(novaUnidade(3 * time.Second).ComCifraDeEmail(cifra))
	a.membros.ComRegistro(repository.NovoEventoPostgres(pool))
	a.membros.ComCorreio(emails, redator, origemDeTeste, nil)
	a.membros.ComIntervaloDeReenvio(0)

	a.smtp = smtpfalso.Iniciar(t)
	host, porta := a.smtp.Endereco()
	enviador := aemail.NovoEnviadorSMTP(aemail.ConfigSMTP{
		Host: host, Porta: porta, Remetente: "contas@stacktrack.teste", NomeDoRemetente: "stacktrack",
		Prazo: 5 * time.Second,
	})
	a.carteiro = correio.NovoCarteiro(emails, enviador, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return a
}

// conta cadastra alguém, como a tela faria.
func (a *ambienteDeConta) conta(t *testing.T, nome string) *ucauth.SessaoAberta {
	t.Helper()
	s, err := a.cadastrar.Executar(context.Background(), ucauth.CadastroInput{
		Nome: nome, Email: strings.ToLower(nome) + "-" + uuid.NewString() + "@teste.dev",
		Senha: "uma senha de teste bem comprida",
	})
	if err != nil {
		t.Fatalf("cadastrar %s: %v", nome, err)
	}
	return s
}

// linkDoEmailPara deixa o worker mandar o que está na fila e devolve o token do
// email mais recente para o endereço com o link do caminho informado. Procura
// pelo destinatário porque o worker manda a fila inteira: o último email que
// chegou pode ser de outra conta, ou de outro tipo.
func (a *ambienteDeConta) linkDoEmailPara(t *testing.T, para, caminho string) string {
	t.Helper()
	if _, err := a.carteiro.Passada(context.Background()); err != nil {
		t.Fatalf("passada do worker: %v", err)
	}
	padrao := regexp.MustCompile(regexp.QuoteMeta(origemDeTeste) + caminho + `([A-Za-z0-9_-]+)`)
	fim := time.Now().Add(5 * time.Second)
	for time.Now().Before(fim) {
		recebidas := a.smtp.Recebidas()
		for i := len(recebidas) - 1; i >= 0; i-- {
			if len(recebidas[i].Para) == 0 || recebidas[i].Para[0] != para {
				continue
			}
			if achado := padrao.FindStringSubmatch(recebidas[i].Ler(t).Texto); achado != nil {
				return achado[1]
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("nenhum email para %s com o link %s", para, caminho)
	return ""
}

// confirmada cadastra alguém e confirma o email pelo link que chegou.
func (a *ambienteDeConta) confirmada(t *testing.T, nome string) *ucauth.SessaoAberta {
	t.Helper()
	s := a.conta(t, nome)
	if err := a.confirmar.Confirmar(context.Background(), a.linkDoEmailPara(t, s.Email, "/confirmar-email/")); err != nil {
		t.Fatalf("confirmar %s: %v", nome, err)
	}
	return s
}

func (a *ambienteDeConta) convidar(t *testing.T, email string) *ucboard.ResultadoConvite {
	t.Helper()
	r, err := a.membros.Convidar(context.Background(), a.boardID, a.dona, email, membro.PapelEditor)
	if err != nil {
		t.Fatalf("convidar: %v", err)
	}
	return r
}

func confirmadaNoBanco(t *testing.T, usuarioID string) bool {
	t.Helper()
	var confirmada bool
	if err := pool.QueryRow(context.Background(),
		`SELECT email_confirmado_em IS NOT NULL FROM usuarios WHERE id = $1`, usuarioID).Scan(&confirmada); err != nil {
		t.Fatalf("ler a conta: %v", err)
	}
	return confirmada
}

func TestC1OCadastroMandaAConfirmacaoEOLinkConfirmaUmaVez(t *testing.T) {
	a := novoAmbienteDeConta(t)
	s := a.conta(t, "Helena")
	if confirmadaNoBanco(t, s.UsuarioID) {
		t.Fatal("a conta nasceu confirmada")
	}

	link := a.linkDoEmailPara(t, s.Email, "/confirmar-email/")
	var hash string
	if err := pool.QueryRow(context.Background(),
		`SELECT token_hash FROM confirmacoes_email WHERE usuario_id = $1`, s.UsuarioID).Scan(&hash); err != nil {
		t.Fatalf("ler a confirmação: %v", err)
	}
	if hash != token.Hash(link) {
		t.Error("o banco não guarda o hash do token do link")
	}

	if err := a.confirmar.Confirmar(context.Background(), link); err != nil {
		t.Fatalf("confirmar: %v", err)
	}
	if !confirmadaNoBanco(t, s.UsuarioID) {
		t.Error("o link não confirmou a conta")
	}
	if err := a.confirmar.Confirmar(context.Background(), link); !errors.Is(err, dconfirmacao.ErrInvalida) {
		t.Errorf("o link valeu uma segunda vez (erro = %v)", err)
	}
}

func TestC2AConfirmacaoTemTetoENaoReenviaParaQuemJaConfirmou(t *testing.T) {
	a := novoAmbienteDeConta(t)
	s := a.conta(t, "Igor") // o cadastro já mandou um
	for i := 0; i < dconfirmacao.MaximoPorHora+2; i++ {
		if err := a.confirmar.Enviar(context.Background(), s.UsuarioID); err != nil {
			t.Fatalf("reenvio %d: %v", i+1, err)
		}
	}
	var n int
	_ = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM confirmacoes_email WHERE usuario_id = $1`, s.UsuarioID).Scan(&n)
	if n != dconfirmacao.MaximoPorHora {
		t.Errorf("%d confirmações numa hora, esperado o teto de %d", n, dconfirmacao.MaximoPorHora)
	}

	c := a.confirmada(t, "Julia")
	limparFilaDeEmail(t)
	if err := a.confirmar.Enviar(context.Background(), c.UsuarioID); err != nil {
		t.Fatalf("reenviar para quem já confirmou: %v", err)
	}
	var fila int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM email_outbox`).Scan(&fila)
	if fila != 0 {
		t.Errorf("%d email(s) de confirmação para uma conta já confirmada", fila)
	}
}

func TestC3OPainelSoMostraConvitesParaContaConfirmadaEDoProprioEmail(t *testing.T) {
	a := novoAmbienteDeConta(t)
	ctx := context.Background()

	// Sem confirmar: nada, mesmo com convite para o email dela.
	semConfirmar := a.conta(t, "Kleber")
	a.convidar(t, semConfirmar.Email)
	if _, err := a.membros.ListarRecebidos(ctx, semConfirmar.UsuarioID); !errors.Is(err, dconvite.ErrEmailNaoConfirmado) {
		t.Errorf("conta sem confirmar: erro = %v, esperado ErrEmailNaoConfirmado", err)
	}

	lia := a.confirmada(t, "Lia")
	valendo := a.convidar(t, lia.Email)
	a.convidar(t, "outra-"+uuid.NewString()+"@teste.dev")

	// Um revogado e um vencido, de outros quadros, para o mesmo email.
	outroQuadro, _, outraDona := cenario(t)
	revogado, err := a.membros.Convidar(ctx, outroQuadro, outraDona, lia.Email, membro.PapelLeitor)
	if err != nil {
		t.Fatalf("convidar: %v", err)
	}
	if err := a.membros.RevogarConvite(ctx, revogado.Convite.ID, outraDona); err != nil {
		t.Fatalf("revogar: %v", err)
	}
	terceiro, _, terceiraDona := cenario(t)
	vencido, err := a.membros.Convidar(ctx, terceiro, terceiraDona, lia.Email, membro.PapelLeitor)
	if err != nil {
		t.Fatalf("convidar: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE convites_board SET expira_em = now() - interval '1 hour' WHERE id = $1`,
		vencido.Convite.ID); err != nil {
		t.Fatalf("vencer o convite: %v", err)
	}

	recebidos, err := a.membros.ListarRecebidos(ctx, lia.UsuarioID)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(recebidos) != 1 || recebidos[0].ID != valendo.Convite.ID {
		t.Fatalf("recebidos = %+v, esperado só o convite que vale", recebidos)
	}
	if recebidos[0].TituloQuadro == "" || recebidos[0].ConvidadoPor != "Ana" {
		t.Errorf("o convite não diz de que quadro e de quem: %+v", recebidos[0])
	}
}

func TestC4AceitarPeloPainelExigeOProprioEmailEPoeNoQuadro(t *testing.T) {
	a := novoAmbienteDeConta(t)
	ctx := context.Background()
	mario := a.confirmada(t, "Mario")

	alheio := a.convidar(t, "nina-"+uuid.NewString()+"@teste.dev")
	if _, _, err := a.membros.AceitarRecebido(ctx, alheio.Convite.ID, mario.UsuarioID); !errors.Is(err, dconvite.ErrInvalido) {
		t.Errorf("aceitar o convite de outro email: erro = %v, esperado ErrInvalido", err)
	}

	proprio := a.convidar(t, mario.Email)
	b, papel, err := a.membros.AceitarRecebido(ctx, proprio.Convite.ID, mario.UsuarioID)
	if err != nil {
		t.Fatalf("aceitar: %v", err)
	}
	if b.ID != a.boardID || papel != membro.PapelEditor {
		t.Errorf("aceitou o quadro %s como %s", b.ID, papel)
	}
	if m, _ := repository.NovoMembroPostgres(pool).Buscar(ctx, a.boardID, mario.UsuarioID); m == nil {
		t.Fatal("aceitou e não entrou no quadro")
	}
	if _, err := a.membros.DetalharConvite(ctx, proprio.Token); !errors.Is(err, dconvite.ErrInvalido) {
		t.Errorf("o link do email continua valendo depois do aceite (erro = %v)", err)
	}
}

func TestC5RecusarEncerraOConviteCancelaOEmailERegistraOEvento(t *testing.T) {
	a := novoAmbienteDeConta(t)
	ctx := context.Background()
	olga := a.confirmada(t, "Olga")
	r := a.convidar(t, olga.Email) // o email fica na fila: o worker não passa

	if err := a.membros.RecusarRecebido(ctx, r.Convite.ID, olga.UsuarioID); err != nil {
		t.Fatalf("recusar: %v", err)
	}
	var recusado bool
	_ = pool.QueryRow(ctx, `SELECT recusado_em IS NOT NULL FROM convites_board WHERE id = $1`, r.Convite.ID).Scan(&recusado)
	if !recusado {
		t.Error("o convite não ficou marcado como recusado")
	}
	if linhas := linhasDoConvite(t, r.Convite.ID); len(linhas) != 1 || linhas[0].estado != "cancelado" || linhas[0].payload != nil {
		t.Errorf("o email do convite recusado continua na fila: %v", linhas)
	}
	var eventos int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM board_events WHERE board_id = $1 AND tipo = 'convite.recusado'`,
		a.boardID).Scan(&eventos)
	if eventos != 1 {
		t.Errorf("%d evento(s) convite.recusado, esperado 1", eventos)
	}
	if _, err := a.membros.DetalharConvite(ctx, r.Token); !errors.Is(err, dconvite.ErrInvalido) {
		t.Errorf("o link do email vale depois da recusa (erro = %v)", err)
	}
	if recebidos, _ := a.membros.ListarRecebidos(ctx, olga.UsuarioID); len(recebidos) != 0 {
		t.Errorf("o convite recusado continua no painel: %+v", recebidos)
	}
	// A vaga ficou livre: a dona pode convidar de novo.
	if _, err := a.membros.Convidar(ctx, a.boardID, a.dona, olga.Email, membro.PapelLeitor); err != nil {
		t.Errorf("convidar de novo quem recusou: %v", err)
	}
}

func TestC6TrocarASenhaPeloEmailConfirmaAConta(t *testing.T) {
	a := novoAmbienteDeConta(t)
	s := a.conta(t, "Paulo")
	if err := a.recuperar.Pedir(context.Background(), s.Email); err != nil {
		t.Fatalf("pedir: %v", err)
	}
	link := a.linkDoEmailPara(t, s.Email, "/redefinir-senha/")
	if _, err := a.recuperar.Redefinir(context.Background(), link, "a senha nova do Paulo, comprida"); err != nil {
		t.Fatalf("redefinir: %v", err)
	}
	if !confirmadaNoBanco(t, s.UsuarioID) {
		t.Error("a troca de senha pelo email não confirmou a conta")
	}
}

func TestC7AceitarERecusarAoMesmoTempoUmSoVence(t *testing.T) {
	a := novoAmbienteDeConta(t)
	ctx := context.Background()
	rita := a.confirmada(t, "Rita")
	r := a.convidar(t, rita.Email)

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		certo int
	)
	for _, acao := range []func() error{
		func() error { _, _, err := a.membros.AceitarRecebido(ctx, r.Convite.ID, rita.UsuarioID); return err },
		func() error { return a.membros.RecusarRecebido(ctx, r.Convite.ID, rita.UsuarioID) },
	} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if acao() == nil {
				mu.Lock()
				certo++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if certo != 1 {
		t.Errorf("%d das duas ações passaram, esperado uma", certo)
	}
}
