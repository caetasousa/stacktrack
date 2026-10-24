//go:build integracao

// Recuperar a senha, com os repositórios e a unidade de autenticação de
// verdade. Como no convite, o único dublê é o da fronteira de fora: o servidor
// SMTP de mentira e o relógio. Outbox, cifra, redator, cliente SMTP e worker
// rodam como em produção.
//
// Como isto quebra — cada teste abaixo cobre um item:
//
//	R1  o pedido grava o token em claro, ou o email não entra na fila no mesmo
//	    commit; ou o link que chega no email não abre a tela de senha nova;
//	R2  pedir com um email sem conta grava alguma coisa ou manda email — e a
//	    diferença revela quem tem conta;
//	R3  uma conta (ou quem digita o email dela) dispara emails de recuperação
//	    sem teto;
//	R4  o link continua valendo depois de uma hora;
//	R5  redefinir não troca a senha, deixa a antiga entrando, deixa outro link
//	    da mesma conta valendo, ou mantém as sessões abertas com a senha antiga;
//	R6  duas redefinições simultâneas com o mesmo link passam as duas;
//	R7  a senha nova fraca é recusada só depois de gastar o link;
//	R8  o link vencido fica guardado para sempre.
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
	drecuperacao "stacktrack/internal/domain/recuperacao"
	"stacktrack/internal/domain/usuario"
	"stacktrack/internal/pkg/token"
	ucauth "stacktrack/internal/usecase/auth"
	"stacktrack/internal/usecase/correio"
	"stacktrack/test/smtpfalso"

	"github.com/google/uuid"
)

const senhaDeAntes = "a senha de antes, que a Carla esqueceu"

// ambienteDeRecuperacao é uma conta com senha conhecida, o caso de uso ligado
// ao outbox de verdade, o worker e o SMTP de mentira.
type ambienteDeRecuperacao struct {
	uc       *ucauth.RecuperarSenhaUseCase
	login    *ucauth.LoginUseCase
	carteiro *correio.Carteiro
	smtp     *smtpfalso.Servidor
	conta    *ucauth.SessaoAberta
	// desvio adianta o relógio do caso de uso e do worker juntos.
	desvio time.Duration
}

func (a *ambienteDeRecuperacao) agora() time.Time { return time.Now().Add(a.desvio) }

func novoAmbienteDeRecuperacao(t *testing.T) *ambienteDeRecuperacao {
	t.Helper()
	limparFilaDeEmail(t)

	cifra, err := security.NovaCifra(chaveAleatoria(t))
	if err != nil {
		t.Fatalf("cifra: %v", err)
	}
	usuarios := repository.NovoUsuarioPostgres(pool)
	sessoes := repository.NovoSessionPostgres(pool)
	hasher := security.NovoHasherArgon2id()
	unidade := repository.NovaUnidadeDeAutenticacao(pool).ComCifraDeEmail(cifra)

	cadastrar := ucauth.NovoCadastrarUseCase(usuarios, sessoes, hasher)
	cadastrar.ComUnidadeDeTrabalho(unidade)
	conta, err := cadastrar.Executar(context.Background(), ucauth.CadastroInput{
		Nome: "Carla", Email: "carla-" + uuid.NewString() + "@teste.dev", Senha: senhaDeAntes,
	})
	if err != nil {
		t.Fatalf("cadastrar: %v", err)
	}

	a := &ambienteDeRecuperacao{conta: conta, login: ucauth.NovoLoginUseCase(usuarios, sessoes, hasher)}
	a.uc = ucauth.NovoRecuperarSenhaUseCase(usuarios, hasher, unidade)
	a.uc.ComCorreio(aemail.NovoRedator(), origemDeTeste, nil)
	a.uc.ComRelogio(a.agora)

	a.smtp = smtpfalso.Iniciar(t)
	host, porta := a.smtp.Endereco()
	enviador := aemail.NovoEnviadorSMTP(aemail.ConfigSMTP{
		Host: host, Porta: porta,
		Remetente: "contas@stacktrack.teste", NomeDoRemetente: "stacktrack",
		Prazo: 5 * time.Second,
	})
	a.carteiro = correio.NovoCarteiro(repository.NovoEmailPostgres(pool, cifra), enviador,
		slog.New(slog.NewTextHandler(io.Discard, nil))).ComRelogio(a.agora)
	return a
}

// pedirELerOLink pede a recuperação, deixa o worker mandar e devolve o token
// que chegou no email.
func (a *ambienteDeRecuperacao) pedirELerOLink(t *testing.T) string {
	t.Helper()
	antes := len(a.smtp.Recebidas())
	if err := a.uc.Pedir(context.Background(), a.conta.Email); err != nil {
		t.Fatalf("pedir: %v", err)
	}
	if _, err := a.carteiro.Passada(context.Background()); err != nil {
		t.Fatalf("passada do worker: %v", err)
	}
	recebidas := a.smtp.Esperar(t, antes+1, 5*time.Second)
	lida := recebidas[len(recebidas)-1].Ler(t)
	achado := regexp.MustCompile(regexp.QuoteMeta(origemDeTeste) + `/redefinir-senha/([A-Za-z0-9_-]+)`).
		FindStringSubmatch(lida.Texto)
	if achado == nil {
		t.Fatalf("o email não traz o link de recuperação:\n%s", lida.Texto)
	}
	return achado[1]
}

func recuperacoesDa(t *testing.T, usuarioID string) (total, abertas int) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`SELECT count(*), count(*) FILTER (WHERE encerrada_em IS NULL)
		   FROM recuperacoes_senha WHERE usuario_id = $1`, usuarioID).Scan(&total, &abertas)
	if err != nil {
		t.Fatalf("ler recuperações: %v", err)
	}
	return total, abertas
}

func emailsNaFilaDa(t *testing.T, usuarioID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM email_outbox WHERE autor_id = $1 AND modelo LIKE 'recuperacao%'`,
		usuarioID).Scan(&n); err != nil {
		t.Fatalf("ler outbox: %v", err)
	}
	return n
}

func TestR1OPedidoGuardaSoOHashEOLinkDoEmailAbreATela(t *testing.T) {
	a := novoAmbienteDeRecuperacao(t)
	link := a.pedirELerOLink(t)

	var hash string
	if err := pool.QueryRow(context.Background(),
		`SELECT token_hash FROM recuperacoes_senha WHERE usuario_id = $1`, a.conta.UsuarioID).Scan(&hash); err != nil {
		t.Fatalf("ler a recuperação: %v", err)
	}
	if hash == link || strings.Contains(hash, link) {
		t.Fatal("o token do link está em claro no banco")
	}

	// A mensagem saiu e o conteúdo foi apagado do outbox: o link não fica lá.
	var comPayload int
	_ = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM email_outbox WHERE autor_id = $1 AND payload_cifrado IS NOT NULL`,
		a.conta.UsuarioID).Scan(&comPayload)
	if comPayload != 0 {
		t.Errorf("%d mensagem(ns) ainda com o link no outbox depois de sair", comPayload)
	}

	aberta, err := a.uc.Consultar(context.Background(), link)
	if err != nil {
		t.Fatalf("o link do email não abre: %v", err)
	}
	if !strings.HasPrefix(aberta.EmailMascarado, "c***@") {
		t.Errorf("email mascarado = %q", aberta.EmailMascarado)
	}
}

func TestR2EmailSemContaNaoGravaNemMandaNada(t *testing.T) {
	a := novoAmbienteDeRecuperacao(t)
	if err := a.uc.Pedir(context.Background(), "ninguem-"+uuid.NewString()+"@teste.dev"); err != nil {
		t.Fatalf("pedir com email sem conta devolveu erro: %v — a diferença revela quem tem conta", err)
	}
	var fila int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM email_outbox`).Scan(&fila)
	if fila != 0 {
		t.Errorf("%d email(s) na fila para um endereço sem conta", fila)
	}
}

func TestR3OsPedidosDaMesmaContaTemTeto(t *testing.T) {
	a := novoAmbienteDeRecuperacao(t)
	for i := 0; i < drecuperacao.MaximoPorHora+2; i++ {
		if err := a.uc.Pedir(context.Background(), a.conta.Email); err != nil {
			t.Fatalf("pedido %d: %v — o teto não pode virar erro, ou revela que a conta existe", i+1, err)
		}
	}
	if total, _ := recuperacoesDa(t, a.conta.UsuarioID); total != drecuperacao.MaximoPorHora {
		t.Errorf("%d recuperações abertas numa hora, esperado o teto de %d", total, drecuperacao.MaximoPorHora)
	}
	if n := emailsNaFilaDa(t, a.conta.UsuarioID); n != drecuperacao.MaximoPorHora {
		t.Errorf("%d emails na fila, esperado %d", n, drecuperacao.MaximoPorHora)
	}

	// Passada a hora, o teto libera de novo.
	a.desvio = drecuperacao.Validade + time.Minute
	if err := a.uc.Pedir(context.Background(), a.conta.Email); err != nil {
		t.Fatalf("pedido depois da hora: %v", err)
	}
	if total, _ := recuperacoesDa(t, a.conta.UsuarioID); total != drecuperacao.MaximoPorHora+1 {
		t.Errorf("depois da hora: %d recuperações, esperado %d", total, drecuperacao.MaximoPorHora+1)
	}
}

func TestR4OLinkVenceEmUmaHora(t *testing.T) {
	a := novoAmbienteDeRecuperacao(t)
	link := a.pedirELerOLink(t)

	a.desvio = drecuperacao.Validade + time.Second
	if _, err := a.uc.Consultar(context.Background(), link); !errors.Is(err, drecuperacao.ErrInvalida) {
		t.Errorf("consultar o link vencido: erro = %v, esperado ErrInvalida", err)
	}
	if _, err := a.uc.Redefinir(context.Background(), link, "uma senha nova bem comprida"); !errors.Is(err, drecuperacao.ErrInvalida) {
		t.Errorf("redefinir com o link vencido: erro = %v, esperado ErrInvalida", err)
	}
}

func TestR5RedefinirTrocaASenhaEncerraOsLinksEDerrubaAsSessoes(t *testing.T) {
	a := novoAmbienteDeRecuperacao(t)
	primeiro := a.pedirELerOLink(t)
	segundo := a.pedirELerOLink(t)

	// Uma sessão aberta com a senha antiga, noutro aparelho.
	antiga, err := a.login.Executar(context.Background(), ucauth.LoginInput{Email: a.conta.Email, Senha: senhaDeAntes})
	if err != nil {
		t.Fatalf("login com a senha antiga: %v", err)
	}

	const nova = "a senha nova que a Carla escolheu"
	sessao, err := a.uc.Redefinir(context.Background(), segundo, nova)
	if err != nil {
		t.Fatalf("redefinir: %v", err)
	}
	if sessao.Token == "" || sessao.UsuarioID != a.conta.UsuarioID {
		t.Errorf("redefinir não abriu sessão para a conta: %+v", sessao)
	}

	if _, err := a.login.Executar(context.Background(), ucauth.LoginInput{Email: a.conta.Email, Senha: senhaDeAntes}); !errors.Is(err, ucauth.ErrCredenciaisInvalidas) {
		t.Errorf("a senha antiga ainda entra (erro = %v)", err)
	}
	if _, err := a.login.Executar(context.Background(), ucauth.LoginInput{Email: a.conta.Email, Senha: nova}); err != nil {
		t.Errorf("a senha nova não entra: %v", err)
	}

	// O outro link da mesma conta deixou de valer.
	if _, err := a.uc.Consultar(context.Background(), primeiro); !errors.Is(err, drecuperacao.ErrInvalida) {
		t.Errorf("o outro link aberto continua valendo (erro = %v)", err)
	}
	if _, abertas := recuperacoesDa(t, a.conta.UsuarioID); abertas != 0 {
		t.Errorf("%d recuperação(ões) ainda abertas", abertas)
	}

	// As sessões de antes caíram — a do cadastro e a do outro aparelho —, e só a
	// nova (e a do login acima, aberta depois) existem.
	sessoes := repository.NovoSessionPostgres(pool)
	for nome, tk := range map[string]string{"cadastro": a.conta.Token, "outro aparelho": antiga.Token} {
		s, err := sessoes.BuscarPorTokenHash(context.Background(), token.Hash(tk))
		if err != nil {
			t.Fatalf("buscar sessão: %v", err)
		}
		if s != nil {
			t.Errorf("a sessão do %s sobreviveu à troca de senha", nome)
		}
	}
	if s, _ := sessoes.BuscarPorTokenHash(context.Background(), token.Hash(sessao.Token)); s == nil {
		t.Error("a sessão aberta pela redefinição não foi gravada")
	}
}

func TestR6DuasRedefinicoesSimultaneasComOMesmoLinkPassamUmaSo(t *testing.T) {
	a := novoAmbienteDeRecuperacao(t)
	link := a.pedirELerOLink(t)

	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		certos int
		erros  []error
	)
	for _, senha := range []string{"primeira senha nova bem comprida", "segunda senha nova bem comprida"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := a.uc.Redefinir(context.Background(), link, senha)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				certos++
			} else {
				erros = append(erros, err)
			}
		}()
	}
	wg.Wait()

	if certos != 1 {
		t.Fatalf("%d redefinições passaram com o mesmo link (erros: %v)", certos, erros)
	}
	if !errors.Is(erros[0], drecuperacao.ErrInvalida) {
		t.Errorf("a perdedora recebeu %v, esperado ErrInvalida", erros[0])
	}
}

func TestR7SenhaFracaERecusadaSemGastarOLink(t *testing.T) {
	a := novoAmbienteDeRecuperacao(t)
	link := a.pedirELerOLink(t)

	if _, err := a.uc.Redefinir(context.Background(), link, "curta"); !errors.Is(err, usuario.ErrSenhaCurta) {
		t.Fatalf("senha curta: erro = %v, esperado ErrSenhaCurta", err)
	}
	if _, err := a.uc.Consultar(context.Background(), link); err != nil {
		t.Fatalf("a recusa gastou o link: %v", err)
	}
	if _, err := a.uc.Redefinir(context.Background(), link, "agora sim uma senha comprida"); err != nil {
		t.Errorf("o link não serviu depois da recusa: %v", err)
	}
}

func TestR8AFaxinaApagaOsLinksVencidos(t *testing.T) {
	a := novoAmbienteDeRecuperacao(t)
	// Um link que venceu há dois dias, gravado como o caso de uso gravaria.
	a.desvio = -48 * time.Hour
	if err := a.uc.Pedir(context.Background(), a.conta.Email); err != nil {
		t.Fatalf("pedir: %v", err)
	}
	a.desvio = 0
	if err := a.uc.Pedir(context.Background(), a.conta.Email); err != nil {
		t.Fatalf("pedir: %v", err)
	}

	faxineiro := repository.NovoRecuperacaoPostgres(pool)
	if _, err := faxineiro.LimparLote(context.Background(), 100); err != nil {
		t.Fatalf("faxina: %v", err)
	}
	if total, abertas := recuperacoesDa(t, a.conta.UsuarioID); total != 1 || abertas != 1 {
		t.Errorf("depois da faxina: %d recuperações (%d abertas), esperado só a de hoje", total, abertas)
	}
}
