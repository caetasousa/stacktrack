package auth

import (
	"context"
	"errors"
	"time"

	demail "stacktrack/internal/domain/email"
	"stacktrack/internal/domain/recuperacao"
	"stacktrack/internal/domain/usuario"
	"stacktrack/internal/pkg/token"

	"github.com/google/uuid"
)

// ModeloDeRecuperacao é o nome e a versão do texto do email de recuperação.
const ModeloDeRecuperacao = "recuperacao.v2"

// RedatorDeRecuperacao escreve o email que leva o link de senha nova. Quem o
// implementa é o adaptador de email, que conhece os modelos de texto e HTML.
type RedatorDeRecuperacao interface {
	Recuperacao(d DadosDoEmailDeRecuperacao) (demail.Mensagem, error)
}

// DadosDoEmailDeRecuperacao é TUDO o que o email de recuperação pode dizer:
// para quem, o link e até quando ele vale.
type DadosDoEmailDeRecuperacao struct {
	Para     string
	Nome     string
	Link     string
	ExpiraEm time.Time
}

// RecuperacaoAberta é o que a tela de senha nova mostra antes de a pessoa
// digitar: de qual conta é o link, sem o email inteiro, e até quando ele vale.
type RecuperacaoAberta struct {
	EmailMascarado string
	ExpiraEm       time.Time
}

// correioDeRecuperacao é o que o envio precisa. Nil no caso de uso significa
// envio de email desligado — e, sem email, não há como recuperar a senha.
type correioDeRecuperacao struct {
	redator RedatorDeRecuperacao
	// origem é a URL pública do frontend, base do link.
	origem string
	// avisar acorda o worker depois do commit. Pode ser nil.
	avisar func()
}

// RecuperarSenhaUseCase é o "esqueci minha senha": pedir o link, conferi-lo e
// trocar a senha com ele.
type RecuperarSenhaUseCase struct {
	usuarios BuscadorUsuario
	hasher   HasherSenha
	unidade  UnidadeDeAutenticacao
	correio  *correioDeRecuperacao
	agora    func() time.Time
}

// NovoRecuperarSenhaUseCase cria o caso de uso. O envio de email entra depois,
// por ComCorreio; sem ele, Pedir não manda nada.
func NovoRecuperarSenhaUseCase(usuarios BuscadorUsuario, hasher HasherSenha, unidade UnidadeDeAutenticacao) *RecuperarSenhaUseCase {
	return &RecuperarSenhaUseCase{usuarios: usuarios, hasher: hasher, unidade: unidade, agora: time.Now}
}

// ComCorreio liga o envio do email de recuperação.
func (uc *RecuperarSenhaUseCase) ComCorreio(redator RedatorDeRecuperacao, origem string, avisar func()) {
	uc.correio = &correioDeRecuperacao{redator: redator, origem: origem, avisar: avisar}
}

// ComRelogio troca o relógio. Existe para o teste atravessar a validade do link
// sem esperar uma hora.
func (uc *RecuperarSenhaUseCase) ComRelogio(agora func() time.Time) {
	uc.agora = agora
}

// EmailLigado informa se a recuperação consegue mandar o link.
func (uc *RecuperarSenhaUseCase) EmailLigado() bool { return uc.correio != nil }

// Pedir manda o link de recuperação para o email informado, se ele for de uma
// conta e ela não tiver estourado o teto de pedidos da hora.
//
// Devolve nil nos três casos — conta inexistente, teto atingido e email
// enviado. A diferença entre eles é exatamente o que não pode sair daqui: uma
// resposta diferente para "esse email não tem conta" transformaria a tela de
// recuperação num jeito de descobrir quem usa o stacktrack. Só falha de
// infraestrutura (banco, redator) volta como erro.
func (uc *RecuperarSenhaUseCase) Pedir(ctx context.Context, email string) error {
	if uc.correio == nil {
		return nil
	}
	u, err := uc.usuarios.BuscarPorEmail(ctx, usuario.NormalizarEmail(email))
	if err != nil || u == nil {
		return err
	}

	agora := uc.agora()
	r, tokenPuro, err := recuperacao.Nova(uuid.NewString(), u.ID, agora)
	if err != nil {
		return err
	}
	m, err := uc.correio.redator.Recuperacao(DadosDoEmailDeRecuperacao{
		Para: u.Email, Nome: u.Nome,
		Link: recuperacao.Link(uc.correio.origem, tokenPuro), ExpiraEm: r.ExpiraEm,
	})
	if err != nil {
		return err
	}
	if err := m.Validar(); err != nil {
		return err
	}
	envio := demail.NovoEnvio(uuid.NewString(), ModeloDeRecuperacao, "recuperacao:"+r.ID, u.ID, agora, r.ExpiraEm)

	enfileirado := false
	err = uc.unidade.Executar(ctx, func(e EscritaDeAuth) error {
		if e.Recuperacoes == nil || e.Emails == nil {
			return errSemCorreio
		}
		// O teto é conferido com a conta travada: conferido sem lock, dois
		// pedidos simultâneos passariam os dois pelo último lugar.
		if err := e.Recuperacoes.TravarConta(ctx, u.ID); err != nil {
			return err
		}
		feitos, err := e.Recuperacoes.ContarDoUsuarioDesde(ctx, u.ID, agora.Add(-time.Hour))
		if err != nil {
			return err
		}
		if feitos >= recuperacao.MaximoPorHora {
			return nil
		}
		// O pedido e o email no MESMO commit: não existe link gravado sem
		// email na fila, nem email com um link que não existe.
		if err := e.Recuperacoes.Salvar(ctx, r); err != nil {
			return err
		}
		if err := e.Emails.Enfileirar(ctx, envio, m); err != nil {
			return err
		}
		enfileirado = true
		return nil
	})
	if err != nil {
		return err
	}
	if enfileirado && uc.correio.avisar != nil {
		uc.correio.avisar()
	}
	return nil
}

// errSemCorreio é a montagem com o caso de uso ligado ao correio e a unidade
// sem o outbox — erro de programação, e não de quem pediu.
var errSemCorreio = errors.New("recuperação de senha sem o outbox de email na transação")

// Consultar confere o link e diz de qual conta ele é. Responde
// recuperacao.ErrInvalida para link inexistente, vencido ou já usado.
func (uc *RecuperarSenhaUseCase) Consultar(ctx context.Context, tokenPuro string) (*RecuperacaoAberta, error) {
	var aberta *RecuperacaoAberta
	err := uc.unidade.Executar(ctx, func(e EscritaDeAuth) error {
		r, u, err := uc.abrir(ctx, e, tokenPuro)
		if err != nil {
			return err
		}
		aberta = &RecuperacaoAberta{EmailMascarado: usuario.MascararEmail(u.Email), ExpiraEm: r.ExpiraEm}
		return nil
	})
	return aberta, err
}

// Redefinir troca a senha da conta do link e abre uma sessão nova.
//
// Na mesma transação: grava a senha, confirma o email (o link chegou por ele),
// encerra TODOS os links abertos da conta e apaga TODAS as sessões dela. Quem pede recuperação pode estar fazendo isso
// porque a senha vazou — e aí o que importa é que quem entrou com a senha
// antiga saia junto, em qualquer aparelho.
//
// A senha nova passa pelas mesmas regras do cadastro, e é conferida ANTES da
// transação: uma senha recusada não gasta o link.
func (uc *RecuperarSenhaUseCase) Redefinir(ctx context.Context, tokenPuro, senha string) (*SessaoAberta, error) {
	if err := usuario.ValidarSenha(senha); err != nil {
		return nil, err
	}
	// O Argon2id custa dezenas de milissegundos: fora da transação, para ela
	// não segurar o lock da linha enquanto o hash é calculado.
	hash, err := uc.hasher.Gerar(senha)
	if err != nil {
		return nil, err
	}

	var aberta *SessaoAberta
	err = uc.unidade.Executar(ctx, func(e EscritaDeAuth) error {
		_, u, err := uc.abrir(ctx, e, tokenPuro)
		if err != nil {
			return err
		}
		agora := uc.agora()
		if err := e.Contas.DefinirSenha(ctx, u.ID, hash, agora); err != nil {
			return err
		}
		// O link chegou pelo email da conta: trocar a senha por ele prova a
		// caixa de entrada tanto quanto o link de confirmação.
		if err := e.Contas.ConfirmarEmail(ctx, u.ID, agora); err != nil {
			return err
		}
		if err := e.Recuperacoes.EncerrarDoUsuario(ctx, u.ID, agora); err != nil {
			return err
		}
		if err := e.SessoesDaConta.RemoverDoUsuario(ctx, u.ID); err != nil {
			return err
		}
		s, t, err := novaSessao(u)
		if err != nil {
			return err
		}
		if err := e.Sessoes.Salvar(ctx, s); err != nil {
			return err
		}
		aberta = sessaoAberta(s, t, u)
		return nil
	})
	return aberta, err
}

// abrir busca o link, trava a linha e confere que ele ainda vale.
func (uc *RecuperarSenhaUseCase) abrir(ctx context.Context, e EscritaDeAuth, tokenPuro string) (*recuperacao.Recuperacao, *usuario.Usuario, error) {
	if e.Recuperacoes == nil || tokenPuro == "" {
		return nil, nil, recuperacao.ErrInvalida
	}
	r, err := e.Recuperacoes.BuscarPorTokenHash(ctx, token.Hash(tokenPuro))
	if err != nil {
		return nil, nil, err
	}
	if r == nil || !r.Vale(uc.agora()) {
		return nil, nil, recuperacao.ErrInvalida
	}
	u, err := e.Usuarios.BuscarPorID(ctx, r.UsuarioID)
	if err != nil {
		return nil, nil, err
	}
	if u == nil {
		return nil, nil, recuperacao.ErrInvalida
	}
	return r, u, nil
}
