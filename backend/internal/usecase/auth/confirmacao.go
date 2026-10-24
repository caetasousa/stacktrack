package auth

import (
	"context"
	"time"

	"stacktrack/internal/domain/confirmacao"
	demail "stacktrack/internal/domain/email"
	"stacktrack/internal/pkg/token"

	"github.com/google/uuid"
)

// ModeloDeConfirmacao é o nome e a versão do texto do email de confirmação.
const ModeloDeConfirmacao = "confirmacao.v1"

// RedatorDeConfirmacao escreve o email com o link de confirmação.
type RedatorDeConfirmacao interface {
	Confirmacao(d DadosDoEmailDeConfirmacao) (demail.Mensagem, error)
}

// DadosDoEmailDeConfirmacao é tudo o que o email de confirmação pode dizer.
type DadosDoEmailDeConfirmacao struct {
	Para     string
	Nome     string
	Link     string
	ExpiraEm time.Time
}

// ConfirmarEmailUseCase manda o link de confirmação e confirma a conta com ele.
type ConfirmarEmailUseCase struct {
	usuarios BuscadorUsuario
	unidade  UnidadeDeAutenticacao
	redator  RedatorDeConfirmacao
	origem   string
	avisar   func()
	agora    func() time.Time
}

// NovoConfirmarEmailUseCase cria o caso de uso. Sem ComCorreio, Enviar não
// manda nada — num ambiente sem SMTP não há como provar o email.
func NovoConfirmarEmailUseCase(usuarios BuscadorUsuario, unidade UnidadeDeAutenticacao) *ConfirmarEmailUseCase {
	return &ConfirmarEmailUseCase{usuarios: usuarios, unidade: unidade, agora: time.Now}
}

// ComCorreio liga o envio do email de confirmação.
func (uc *ConfirmarEmailUseCase) ComCorreio(redator RedatorDeConfirmacao, origem string, avisar func()) {
	uc.redator, uc.origem, uc.avisar = redator, origem, avisar
}

// ComRelogio troca o relógio, para o teste atravessar a validade.
func (uc *ConfirmarEmailUseCase) ComRelogio(agora func() time.Time) { uc.agora = agora }

// Enviar manda um link de confirmação para o email da conta.
//
// Não faz nada — e não é erro — para conta já confirmada, para conta que
// estourou o teto de pedidos da hora e com o envio desligado. Só falha de
// infraestrutura volta como erro.
func (uc *ConfirmarEmailUseCase) Enviar(ctx context.Context, usuarioID string) error {
	if uc.redator == nil {
		return nil
	}
	u, err := uc.usuarios.BuscarPorID(ctx, usuarioID)
	if err != nil || u == nil || u.EmailConfirmado() {
		return err
	}

	agora := uc.agora()
	c, tokenPuro, err := confirmacao.Nova(uuid.NewString(), u.ID, agora)
	if err != nil {
		return err
	}
	m, err := uc.redator.Confirmacao(DadosDoEmailDeConfirmacao{
		Para: u.Email, Nome: u.Nome, Link: confirmacao.Link(uc.origem, tokenPuro), ExpiraEm: c.ExpiraEm,
	})
	if err != nil {
		return err
	}
	if err := m.Validar(); err != nil {
		return err
	}
	envio := demail.NovoEnvio(uuid.NewString(), ModeloDeConfirmacao, "confirmacao:"+c.ID, u.ID, agora, c.ExpiraEm)

	enfileirado := false
	err = uc.unidade.Executar(ctx, func(e EscritaDeAuth) error {
		if e.Confirmacoes == nil || e.Emails == nil || e.Recuperacoes == nil {
			return errSemCorreio
		}
		// A mesma trava de conta da recuperação: dois pedidos simultâneos não
		// passam os dois pelo último lugar do teto.
		if err := e.Recuperacoes.TravarConta(ctx, u.ID); err != nil {
			return err
		}
		feitos, err := e.Confirmacoes.ContarDoUsuarioDesde(ctx, u.ID, agora.Add(-time.Hour))
		if err != nil {
			return err
		}
		if feitos >= confirmacao.MaximoPorHora {
			return nil
		}
		if err := e.Confirmacoes.Salvar(ctx, c); err != nil {
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
	if enfileirado && uc.avisar != nil {
		uc.avisar()
	}
	return nil
}

// Confirmar marca a conta do link como dona do email e encerra os outros links
// abertos dela. Não exige sessão: quem abre o link prova a caixa de entrada,
// esteja logado ou não. Responde confirmacao.ErrInvalida para link
// inexistente, vencido ou já usado.
func (uc *ConfirmarEmailUseCase) Confirmar(ctx context.Context, tokenPuro string) error {
	if tokenPuro == "" {
		return confirmacao.ErrInvalida
	}
	return uc.unidade.Executar(ctx, func(e EscritaDeAuth) error {
		if e.Confirmacoes == nil {
			return confirmacao.ErrInvalida
		}
		c, err := e.Confirmacoes.BuscarPorTokenHash(ctx, token.Hash(tokenPuro))
		if err != nil {
			return err
		}
		agora := uc.agora()
		if c == nil || !c.Vale(agora) {
			return confirmacao.ErrInvalida
		}
		if err := e.Contas.ConfirmarEmail(ctx, c.UsuarioID, agora); err != nil {
			return err
		}
		return e.Confirmacoes.EncerrarDoUsuario(ctx, c.UsuarioID, agora)
	})
}
