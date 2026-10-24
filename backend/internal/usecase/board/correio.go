package board

import (
	"context"
	"fmt"
	"time"

	dconvite "stacktrack/internal/domain/convite"
	demail "stacktrack/internal/domain/email"
	"stacktrack/internal/domain/membro"

	"github.com/google/uuid"
)

// ModeloDeConvite é o nome e a versão do texto do email de convite. Muda quando
// o texto muda de um jeito que importe saber, olhando a fila, qual versão saiu.
const ModeloDeConvite = "convite.v1"

// RedatorDeConvite escreve o email que entrega um convite. Quem o implementa é
// o adaptador de email, que conhece os modelos de texto e de HTML; o caso de
// uso só sabe o que a mensagem precisa dizer.
type RedatorDeConvite interface {
	Convite(d DadosDoEmailDeConvite) (demail.Mensagem, error)
}

// DadosDoEmailDeConvite é TUDO o que o email de convite pode dizer.
//
// A lista é curta de propósito: o email sai da proteção do sistema — fica na
// caixa de entrada, é encaminhado, é lido por quem não foi convidado. Nada do
// conteúdo do quadro entra nele; só o que a pessoa precisa para decidir se abre
// o link.
type DadosDoEmailDeConvite struct {
	Para         string
	ConvidadoPor string
	// ResponderPara é o email de quem convidou: a resposta ao convite vai para
	// a pessoa, e não para o remetente automático do sistema.
	ResponderPara string
	TituloQuadro  string
	Papel         membro.Papel
	Link          string
	ExpiraEm      time.Time
}

// Entrega diz como o convite chegou, ou vai chegar, a quem foi convidado.
type Entrega string

const (
	// EntregaPorEmail é o convite com email na fila: o worker o envia logo após
	// o commit.
	EntregaPorEmail Entrega = "email"
	// EntregaManual é o convite sem email porque o envio está desligado neste
	// ambiente. O link vai na resposta para o dono entregar.
	EntregaManual Entrega = "manual"
	// EntregaCotaEsgotada é o convite sem email porque a conta (ou o convite)
	// atingiu o teto de envios. O convite vale e o link vai na resposta — o
	// teto existe para conter spam, não para impedir o dono de convidar.
	EntregaCotaEsgotada Entrega = "cota"
)

// correioDeConvite é o que o envio por email precisa. Nil no MembroUseCase
// significa envio desligado.
type correioDeConvite struct {
	fila    RepositorioEmail
	redator RedatorDeConvite
	// origem é a URL pública do frontend, base do link que vai no email.
	origem string
	// avisar acorda o worker depois do commit, para o email sair em segundos e
	// não no próximo ciclo da fila. Pode ser nil: o ciclo da fila entrega mesmo
	// assim, só que mais tarde.
	avisar func()
}

// ComCorreio liga a entrega de convites por email.
//
// Entra por aqui, e não pelo construtor, pela mesma razão do publicador de
// eventos: sem ele o caso de uso continua funcionando — com o link manual, que
// é exatamente o comportamento de um ambiente sem SMTP.
func (uc *MembroUseCase) ComCorreio(fila RepositorioEmail, redator RedatorDeConvite, origem string, avisar func()) {
	uc.correio = &correioDeConvite{fila: fila, redator: redator, origem: origem, avisar: avisar}
}

// ConviteComEntrega é um convite pendente e o último envio dele. Entrega é nil
// quando o envio está desligado ou quando o convite nunca foi mandado por email.
type ConviteComEntrega struct {
	Convite dconvite.Convite
	Entrega *demail.Envio
}

// emailDoConvite é a mensagem redigida e o envio que a leva, prontos para
// entrar na transação.
type emailDoConvite struct {
	envio    *demail.Envio
	mensagem demail.Mensagem
}

// redigirConvite monta a mensagem e o envio de um convite. Roda FORA da
// transação: redigir não depende de lock, e a transação do quadro precisa ser
// curta.
func (uc *MembroUseCase) redigirConvite(
	ctx context.Context, c *dconvite.Convite, tokenPuro, autorID string, agora time.Time,
) (*emailDoConvite, error) {
	b, err := uc.boards.BuscarPorID(ctx, c.BoardID)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, dconvite.ErrInvalido
	}

	dados := DadosDoEmailDeConvite{
		Para:         c.Email,
		TituloQuadro: b.Titulo,
		Papel:        c.Papel,
		Link:         dconvite.Link(uc.correio.origem, tokenPuro),
		ExpiraEm:     c.ExpiraEm,
	}
	if autor, err := uc.usuarios.BuscarPorID(ctx, autorID); err == nil && autor != nil {
		dados.ConvidadoPor, dados.ResponderPara = autor.Nome, autor.Email
	}

	m, err := uc.correio.redator.Convite(dados)
	if err != nil {
		return nil, err
	}
	if err := m.Validar(); err != nil {
		return nil, err
	}

	// A intenção é "mandar ESTE link deste convite": a chave leva o começo do
	// hash do token, e não um número de ordem. Uma transação refeita reencontra
	// a mesma chave; um reenvio, que troca o token, ganha outra — e duas abas
	// contando envios ao mesmo tempo não chegam ao mesmo número.
	e := demail.NovoEnvio(uuid.NewString(), ModeloDeConvite,
		fmt.Sprintf("convite:%s:%s", c.ID, c.TokenHash[:16]), autorID, agora, c.ExpiraEm)
	e.ConviteID = c.ID
	return &emailDoConvite{envio: e, mensagem: m}, nil
}

// enfileirarNaTransacao grava o email do convite na transação em curso, se o
// envio estiver ligado e a cota permitir. Devolve como o convite vai chegar.
//
// As cotas são conferidas AQUI, sob o lock do quadro, e não antes da
// transação: conferidas fora, duas requisições simultâneas passariam as duas
// pelo último lugar da cota.
func enfileirarNaTransacao(ctx context.Context, e Escrita, email *emailDoConvite, autorID string, agora time.Time) (Entrega, error) {
	if e.Emails == nil || email == nil {
		return EntregaManual, nil
	}

	doAutor, err := e.Emails.ContarDoAutorDesde(ctx, autorID, agora.Add(-24*time.Hour))
	if err != nil {
		return "", err
	}
	if doAutor >= dconvite.MaximoDeEmailsPorDia {
		return EntregaCotaEsgotada, nil
	}

	if email.envio.ConviteID != "" {
		doConvite, err := e.Emails.DoConvite(ctx, email.envio.ConviteID)
		if err != nil {
			return "", err
		}
		if len(doConvite) >= dconvite.MaximoDeEmailsPorConvite {
			return EntregaCotaEsgotada, nil
		}
	}

	if err := e.Emails.Enfileirar(ctx, email.envio, email.mensagem); err != nil {
		return "", err
	}
	return EntregaPorEmail, nil
}

// cancelarEmailsDoConvite tira da fila o que ainda não saiu de um convite que
// deixou de valer — revogado, aceito ou com link trocado. Sem isso o worker
// mandaria, minutos depois, um email com link morto.
func cancelarEmailsDoConvite(ctx context.Context, e Escrita, conviteID string, agora time.Time) error {
	if e.Emails == nil {
		return nil
	}
	return e.Emails.CancelarDoConvite(ctx, conviteID, agora)
}

// avisarCorreio acorda o worker depois do commit, se houver email na fila.
func (uc *MembroUseCase) avisarCorreio(entrega Entrega) {
	if entrega == EntregaPorEmail && uc.correio != nil && uc.correio.avisar != nil {
		uc.correio.avisar()
	}
}
