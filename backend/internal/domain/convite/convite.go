// Package convite modela o convite para participar de um quadro — o caminho de
// quem ainda não tem conta. Quem já tem entra direto como membro, e nem passa
// por aqui.
package convite

import (
	"errors"
	"time"

	"stacktrack/internal/domain/membro"
	"stacktrack/internal/domain/usuario"
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
	// ErrJaResolvido é retornado quando se tenta aceitar ou revogar um convite
	// que já saiu do estado pendente — porque outra requisição chegou primeiro.
	//
	// É separado de ErrInvalido de propósito: ErrInvalido é a resposta genérica
	// dada a QUEM TEM O TOKEN, e precisa continuar não distinguindo "não existe"
	// de "já foi usado". Este aqui não vai para a borda pública: ele descreve
	// uma corrida entre duas escritas, e quem o lê é o usecase.
	ErrJaResolvido = errors.New("este convite já foi aceito ou revogado")
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
	// RevogadoEm é nil enquanto o convite não foi revogado.
	//
	// Revogar MARCA, e não apaga. O DELETE anterior levava junto a resposta
	// para "quem convidou quem, e quando" — e, pior, liberava a vaga do índice
	// de forma indistinguível de um convite que nunca existiu.
	RevogadoEm *time.Time
}

// Estado é o resultado terminal (ou não) de um convite.
type Estado string

const (
	// EstadoPendente é o convite que ainda pode ser aceito.
	EstadoPendente Estado = "pendente"
	// EstadoAceito é terminal.
	EstadoAceito Estado = "aceito"
	// EstadoRevogado é terminal.
	EstadoRevogado Estado = "revogado"
	// EstadoExpirado é o convite que passou da validade sem ser resolvido. Não
	// é terminal no banco — é uma leitura do relógio —, e é o domínio que o
	// transforma em revogado quando precisa liberar a vaga.
	EstadoExpirado Estado = "expirado"
)

// EstadoEm devolve em que situação o convite está em relação a agora.
//
// A ordem das checagens importa: aceito e revogado são fatos gravados e vencem
// o relógio. Um convite aceito ontem e "vencido" hoje continua sendo aceito —
// tratá-lo como expirado faria a auditoria mudar de resposta com o passar do
// tempo.
func (c *Convite) EstadoEm(agora time.Time) Estado {
	switch {
	case c.AceitoEm != nil:
		return EstadoAceito
	case c.RevogadoEm != nil:
		return EstadoRevogado
	case agora.After(c.ExpiraEm):
		return EstadoExpirado
	default:
		return EstadoPendente
	}
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
	return c.EstadoEm(agora) == EstadoPendente
}

// Aceitar marca o convite como usado. Retorna ErrInvalido se ele já tiver sido
// aceito, revogado ou já estiver vencido — um convite vale uma vez só, senão o
// link vazado continuaria valendo depois de a pessoa certa já ter entrado.
func (c *Convite) Aceitar(agora time.Time) error {
	if !c.Pendente(agora) {
		return ErrInvalido
	}
	c.AceitoEm = &agora
	return nil
}

// Revogar marca o convite como cancelado, invalidando o link já entregue.
//
// Revogar um convite VENCIDO é permitido, e é o caminho normal de quem convida
// de novo o mesmo email: a vaga do índice de pendência só é liberada por um
// fato gravado, e o vencimento não é um.
//
// Revogar o que já é terminal devolve ErrJaResolvido em vez de sobrescrever a
// data: quem chegou primeiro decidiu, e regravar apagaria o instante real de
// uma decisão que já vale.
func (c *Convite) Revogar(agora time.Time) error {
	switch c.EstadoEm(agora) {
	case EstadoPendente, EstadoExpirado:
		c.RevogadoEm = &agora
		return nil
	default:
		return ErrJaResolvido
	}
}
