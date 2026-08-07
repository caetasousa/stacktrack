// Package convite modela o convite para participar de um quadro — o caminho de
// quem ainda não tem conta. Quem já tem entra direto como membro, e nem passa
// por aqui.
package convite

import (
	"errors"
	"time"

	"kanbango/internal/domain/membro"
	"kanbango/internal/domain/usuario"
)

// TTL é a validade de um convite a partir da criação.
//
// Sete dias: longo o bastante para a pessoa ver a mensagem depois de um fim de
// semana, curto o bastante para um link esquecido num histórico de conversa não
// virar porta aberta para sempre.
const TTL = 7 * 24 * time.Hour

var (
	// ErrEmailObrigatorio é retornado quando o email do convidado está vazio ou é inválido.
	ErrEmailObrigatorio = errors.New("email do convidado é obrigatório")
	// ErrNaoConvidaODono é retornado quando alguém tenta convidar a si mesmo.
	ErrNaoConvidaODono = errors.New("você já participa deste quadro")
	// ErrJaEMembro é retornado quando o email convidado já participa do quadro.
	ErrJaEMembro = errors.New("esta pessoa já participa do quadro")
	// ErrJaConvidado é retornado quando já existe convite pendente para o email.
	ErrJaConvidado = errors.New("já existe um convite pendente para este email")
	// ErrInvalido é retornado tanto para token inexistente quanto para convite
	// expirado ou já aceito — resposta genérica, para não confirmar a
	// existência de um convite a quem está testando links.
	ErrInvalido = errors.New("convite inválido ou expirado")
	// ErrOutroDestinatario é retornado quando quem abre o convite está logado
	// com uma conta de email diferente do convidado.
	ErrOutroDestinatario = errors.New("este convite é para outro email")
)

// Convite é a permissão pendente de alguém entrar em um quadro.
type Convite struct {
	ID        string
	BoardID   string
	Email     string
	Papel     membro.Papel
	TokenHash string
	CriadoPor string
	CriadoEm  time.Time
	ExpiraEm  time.Time
	// AceitoEm é nil enquanto o convite está pendente.
	AceitoEm *time.Time
}

// Novo cria um convite pendente com validade de TTL. O email é normalizado
// aqui, com a mesma régua da conta — senão "Ana@x.com" convidada e
// "ana@x.com" cadastrada não se reconheceriam.
func Novo(id, boardID, email string, papel membro.Papel, tokenHash, criadoPor string) (*Convite, error) {
	email = usuario.NormalizarEmail(email)
	if email == "" {
		return nil, ErrEmailObrigatorio
	}
	if !membro.PapelValido(papel) {
		return nil, membro.ErrPapelInvalido
	}

	agora := time.Now()
	return &Convite{
		ID:        id,
		BoardID:   boardID,
		Email:     email,
		Papel:     papel,
		TokenHash: tokenHash,
		CriadoPor: criadoPor,
		CriadoEm:  agora,
		ExpiraEm:  agora.Add(TTL),
	}, nil
}

// Pendente informa se o convite ainda pode ser aceito em relação a agora.
func (c *Convite) Pendente(agora time.Time) bool {
	return c.AceitoEm == nil && !agora.After(c.ExpiraEm)
}

// Aceitar marca o convite como usado. Retorna ErrInvalido se ele já tiver sido
// aceito ou já estiver vencido — um convite vale uma vez só, senão o link
// vazado continuaria valendo depois de a pessoa certa já ter entrado.
func (c *Convite) Aceitar(agora time.Time) error {
	if !c.Pendente(agora) {
		return ErrInvalido
	}
	c.AceitoEm = &agora
	return nil
}
