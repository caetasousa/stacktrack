package usecase_test

import (
	"errors"
	"testing"
	"time"

	"stacktrack/internal/domain/session"
	"stacktrack/internal/domain/usuario"
	"stacktrack/internal/pkg/token"
	ucauth "stacktrack/internal/usecase/auth"
	"stacktrack/test/repository/memoria"
)

type ambiente struct {
	usuarios *memoria.Usuarios
	sessoes  *memoria.Sessoes
	hasher   *memoria.Hasher
	cadastro *ucauth.CadastrarUseCase
	login    *ucauth.LoginUseCase
	logout   *ucauth.LogoutUseCase
	validar  *ucauth.ValidarSessaoUseCase
	perfil   *ucauth.PerfilUseCase
}

func novoAmbiente() *ambiente {
	usuarios := memoria.NovosUsuarios()
	sessoes := memoria.NovasSessoes()
	hasher := &memoria.Hasher{}

	return &ambiente{
		usuarios: usuarios,
		sessoes:  sessoes,
		hasher:   hasher,
		cadastro: ucauth.NovoCadastrarUseCase(usuarios, sessoes, hasher),
		login:    ucauth.NovoLoginUseCase(usuarios, sessoes, hasher),
		logout:   ucauth.NovoLogoutUseCase(sessoes),
		validar:  ucauth.NovoValidarSessaoUseCase(sessoes),
		perfil:   ucauth.NovoPerfilUseCase(usuarios),
	}
}

func (a *ambiente) cadastrar(t *testing.T, nome, email, senha string) *ucauth.SessaoAberta {
	t.Helper()
	out, err := a.cadastro.Executar(ucauth.CadastroInput{Nome: nome, Email: email, Senha: senha})
	if err != nil {
		t.Fatalf("cadastro falhou: %v", err)
	}
	return out
}

func TestCadastroCriaContaEJaAbreSessao(t *testing.T) {
	a := novoAmbiente()

	out := a.cadastrar(t, "Ana", "ana@exemplo.com", "senha-boa-123")

	if out.Token == "" {
		t.Error("o cadastro devia devolver um token de sessão")
	}
	if out.Nome != "Ana" || out.Email != "ana@exemplo.com" {
		t.Errorf("saída = %+v", out)
	}
	if a.usuarios.Quantidade() != 1 {
		t.Errorf("contas gravadas = %d, esperado 1", a.usuarios.Quantidade())
	}
	if a.sessoes.Quantidade() != 1 {
		t.Errorf("sessões abertas = %d, esperado 1", a.sessoes.Quantidade())
	}
}

// O token puro nunca pode estar no que foi persistido: o banco guarda só o
// hash, para um vazamento não entregar sessões utilizáveis.
func TestCadastroPersisteApenasOHashDoToken(t *testing.T) {
	a := novoAmbiente()

	out := a.cadastrar(t, "Ana", "ana@exemplo.com", "senha-boa-123")

	if s, _ := a.sessoes.BuscarPorTokenHash(out.Token); s != nil {
		t.Fatal("a sessão foi encontrada pelo token puro — ele está sendo persistido")
	}
	s, _ := a.sessoes.BuscarPorTokenHash(token.Hash(out.Token))
	if s == nil {
		t.Fatal("a sessão devia ser encontrada pelo hash do token")
	}
	if s.UsuarioID != out.UsuarioID {
		t.Errorf("sessão aberta para %q, esperado %q", s.UsuarioID, out.UsuarioID)
	}
}

func TestCadastroRecusaEmailJaCadastrado(t *testing.T) {
	a := novoAmbiente()
	a.cadastrar(t, "Ana", "ana@exemplo.com", "senha-boa-123")

	_, err := a.cadastro.Executar(ucauth.CadastroInput{
		Nome: "Outra Ana", Email: "ana@exemplo.com", Senha: "outra-senha-123",
	})

	if !errors.Is(err, usuario.ErrEmailEmUso) {
		t.Errorf("erro = %v, esperado ErrEmailEmUso", err)
	}
	if a.usuarios.Quantidade() != 1 {
		t.Errorf("contas gravadas = %d, esperado 1", a.usuarios.Quantidade())
	}
}

// A caixa do email não pode criar uma segunda conta: quem digitou
// "Ana@Exemplo.com" no cadastro e "ana@exemplo.com" no login é a mesma pessoa.
func TestCadastroRecusaMesmoEmailComOutraCaixa(t *testing.T) {
	a := novoAmbiente()
	a.cadastrar(t, "Ana", "ana@exemplo.com", "senha-boa-123")

	_, err := a.cadastro.Executar(ucauth.CadastroInput{
		Nome: "Ana de novo", Email: "  ANA@Exemplo.COM ", Senha: "senha-boa-123",
	})

	if !errors.Is(err, usuario.ErrEmailEmUso) {
		t.Errorf("erro = %v, esperado ErrEmailEmUso", err)
	}
}

func TestCadastroRecusaSenhaCurtaAntesDeGravar(t *testing.T) {
	a := novoAmbiente()

	_, err := a.cadastro.Executar(ucauth.CadastroInput{
		Nome: "Ana", Email: "ana@exemplo.com", Senha: "curta",
	})

	if !errors.Is(err, usuario.ErrSenhaCurta) {
		t.Errorf("erro = %v, esperado ErrSenhaCurta", err)
	}
	if a.usuarios.Quantidade() != 0 {
		t.Error("nada podia ter sido gravado com senha inválida")
	}
}

func TestLoginAceitaCredenciaisCorretas(t *testing.T) {
	a := novoAmbiente()
	cadastro := a.cadastrar(t, "Ana", "ana@exemplo.com", "senha-boa-123")

	out, err := a.login.Executar(ucauth.LoginInput{Email: "ana@exemplo.com", Senha: "senha-boa-123"})
	if err != nil {
		t.Fatalf("login falhou: %v", err)
	}

	if out.UsuarioID != cadastro.UsuarioID {
		t.Errorf("login abriu sessão para %q, esperado %q", out.UsuarioID, cadastro.UsuarioID)
	}
	if out.Token == cadastro.Token {
		t.Error("cada login precisa gerar um token novo")
	}
	if a.sessoes.Quantidade() != 2 {
		t.Errorf("sessões = %d — o login não pode derrubar a sessão anterior (outro dispositivo)", a.sessoes.Quantidade())
	}
}

func TestLoginAceitaEmailComOutraCaixa(t *testing.T) {
	a := novoAmbiente()
	a.cadastrar(t, "Ana", "ana@exemplo.com", "senha-boa-123")

	if _, err := a.login.Executar(ucauth.LoginInput{Email: " Ana@EXEMPLO.com ", Senha: "senha-boa-123"}); err != nil {
		t.Errorf("login com email em outra caixa devia funcionar: %v", err)
	}
}

func TestLoginRecusaSenhaErrada(t *testing.T) {
	a := novoAmbiente()
	a.cadastrar(t, "Ana", "ana@exemplo.com", "senha-boa-123")

	_, err := a.login.Executar(ucauth.LoginInput{Email: "ana@exemplo.com", Senha: "senha-errada"})

	if !errors.Is(err, ucauth.ErrCredenciaisInvalidas) {
		t.Errorf("erro = %v, esperado ErrCredenciaisInvalidas", err)
	}
}

// Email inexistente e senha errada precisam ser indistinguíveis — na mensagem
// e no TEMPO. Por isso o login verifica um hash dummy quando não acha a conta:
// sem essa verificação, a resposta instantânea denunciaria quais emails têm
// cadastro aqui.
func TestLoginComEmailInexistenteGastaOMesmoCustoDeVerificacao(t *testing.T) {
	a := novoAmbiente()
	a.cadastrar(t, "Ana", "ana@exemplo.com", "senha-boa-123")

	a.hasher.Chamadas = 0
	_, errInexistente := a.login.Executar(ucauth.LoginInput{Email: "ninguem@exemplo.com", Senha: "chute"})
	chamadasInexistente := a.hasher.Chamadas

	a.hasher.Chamadas = 0
	_, errSenhaErrada := a.login.Executar(ucauth.LoginInput{Email: "ana@exemplo.com", Senha: "chute"})
	chamadasSenhaErrada := a.hasher.Chamadas

	if chamadasInexistente != chamadasSenhaErrada {
		t.Errorf("verificações: email inexistente = %d, senha errada = %d — o tempo de resposta denuncia quais emails existem",
			chamadasInexistente, chamadasSenhaErrada)
	}
	if errInexistente.Error() != errSenhaErrada.Error() {
		t.Errorf("mensagens diferentes: %q vs %q", errInexistente, errSenhaErrada)
	}
}

// Falha de infraestrutura não pode virar "credenciais inválidas": quem tem a
// senha certa merece saber que o problema é do servidor, e o log precisa
// registrar um 500, não um login recusado.
func TestLoginPropagaFalhaDeInfraestrutura(t *testing.T) {
	a := novoAmbiente()
	falha := errors.New("conexão recusada")
	a.usuarios.ErroForcado = falha

	_, err := a.login.Executar(ucauth.LoginInput{Email: "ana@exemplo.com", Senha: "senha-boa-123"})

	if !errors.Is(err, falha) {
		t.Errorf("erro = %v, esperado a falha de infraestrutura", err)
	}
	if errors.Is(err, ucauth.ErrCredenciaisInvalidas) {
		t.Error("falha de infraestrutura não pode ser reportada como credencial inválida")
	}
}

func TestLoginVarreSessoesVencidas(t *testing.T) {
	a := novoAmbiente()
	a.cadastrar(t, "Ana", "ana@exemplo.com", "senha-boa-123")
	a.sessoes.Salvar(session.Nova("hash-velho", "outro", -time.Hour))

	if _, err := a.login.Executar(ucauth.LoginInput{Email: "ana@exemplo.com", Senha: "senha-boa-123"}); err != nil {
		t.Fatalf("login falhou: %v", err)
	}

	if s, _ := a.sessoes.BuscarPorTokenHash("hash-velho"); s != nil {
		t.Error("a sessão vencida devia ter sido varrida no login")
	}
}

func TestValidarSessaoDevolveIdentidade(t *testing.T) {
	a := novoAmbiente()
	out := a.cadastrar(t, "Ana", "ana@exemplo.com", "senha-boa-123")

	id, err := a.validar.Executar(out.Token)
	if err != nil {
		t.Fatalf("validação falhou: %v", err)
	}
	if id.UsuarioID != out.UsuarioID {
		t.Errorf("identidade = %q, esperado %q", id.UsuarioID, out.UsuarioID)
	}
}

func TestValidarSessaoRecusaTokenDesconhecido(t *testing.T) {
	a := novoAmbiente()

	if _, err := a.validar.Executar("token-inventado"); !errors.Is(err, ucauth.ErrSessaoInvalida) {
		t.Errorf("erro = %v, esperado ErrSessaoInvalida", err)
	}
}

// A expiração é decidida pelo servidor. O cookie do navegador pode ser mantido
// além do prazo — só a checagem daqui encerra a sessão de fato.
func TestValidarSessaoRecusaSessaoVencida(t *testing.T) {
	a := novoAmbiente()
	vencida := session.Nova(token.Hash("token-vencido"), "usuario-1", -time.Minute)
	a.sessoes.Salvar(vencida)

	if _, err := a.validar.Executar("token-vencido"); !errors.Is(err, ucauth.ErrSessaoInvalida) {
		t.Errorf("erro = %v, esperado ErrSessaoInvalida", err)
	}
}

func TestLogoutInvalidaASessaoNoServidor(t *testing.T) {
	a := novoAmbiente()
	out := a.cadastrar(t, "Ana", "ana@exemplo.com", "senha-boa-123")

	if err := a.logout.Executar(out.Token); err != nil {
		t.Fatalf("logout falhou: %v", err)
	}

	if _, err := a.validar.Executar(out.Token); !errors.Is(err, ucauth.ErrSessaoInvalida) {
		t.Error("o token devia ter deixado de valer depois do logout")
	}
}

func TestLogoutDeTokenDesconhecidoNaoEErro(t *testing.T) {
	a := novoAmbiente()

	if err := a.logout.Executar("token-inventado"); err != nil {
		t.Errorf("erro = %v, esperado nenhum", err)
	}
}

// Sair de um dispositivo não pode derrubar os outros.
func TestLogoutNaoDerrubaAsOutrasSessoesDoUsuario(t *testing.T) {
	a := novoAmbiente()
	primeira := a.cadastrar(t, "Ana", "ana@exemplo.com", "senha-boa-123")
	segunda, err := a.login.Executar(ucauth.LoginInput{Email: "ana@exemplo.com", Senha: "senha-boa-123"})
	if err != nil {
		t.Fatalf("login falhou: %v", err)
	}

	if err := a.logout.Executar(primeira.Token); err != nil {
		t.Fatalf("logout falhou: %v", err)
	}

	if _, err := a.validar.Executar(segunda.Token); err != nil {
		t.Errorf("a outra sessão devia continuar válida: %v", err)
	}
}

func TestPerfilDevolveAContaAutenticada(t *testing.T) {
	a := novoAmbiente()
	out := a.cadastrar(t, "Ana", "ana@exemplo.com", "senha-boa-123")

	u, err := a.perfil.Executar(out.UsuarioID)
	if err != nil {
		t.Fatalf("perfil falhou: %v", err)
	}
	if u.Email != "ana@exemplo.com" || u.Nome != "Ana" {
		t.Errorf("perfil = %+v", u)
	}
}

func TestPerfilDeUsuarioInexistenteNaoAutentica(t *testing.T) {
	a := novoAmbiente()

	if _, err := a.perfil.Executar("usuario-que-nao-existe"); !errors.Is(err, ucauth.ErrSessaoInvalida) {
		t.Errorf("erro = %v, esperado ErrSessaoInvalida", err)
	}
}
