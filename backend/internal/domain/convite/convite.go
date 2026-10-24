// Package convite modela o convite para participar de um quadro — a ÚNICA porta
// de entrada, tenha a pessoa conta ou não. Conhecer o email de alguém não põe
// ninguém num quadro: o acesso nasce do link do convite, apresentado por quem
// está logado na conta daquele email.
package convite

import (
	"errors"
	"net/url"
	"time"

	"stacktrack/internal/domain/membro"
	"stacktrack/internal/domain/usuario"
)

// TTL é a validade de um convite a partir da criação (ou do último reenvio).
//
// Sete dias: longo o bastante para a pessoa ver a mensagem depois de um fim de
// semana, curto o bastante para um link esquecido num histórico de conversa não
// virar porta aberta para sempre.
const TTL = 7 * 24 * time.Hour

// Os tetos de convite. Com o envio por email, convidar deixou de ser só gerar
// um link: virou um jeito de o servidor mandar mensagem a qualquer endereço. Sem
// teto, uma conta qualquer usaria o remetente do stacktrack para mandar spam — e
// a reputação queimada é a do domínio, que derruba a entrega de todo mundo.
//
// Os tetos de EMAIL não impedem o convite: estourados, ele nasce do mesmo jeito,
// só que com o link para o dono entregar. O que contém o spam é o email não
// sair; travar o convite só castigaria quem convida.
const (
	// MaximoDePendentesPorQuadro é quantos convites um quadro pode ter em aberto.
	MaximoDePendentesPorQuadro = 50
	// MaximoDeEmailsPorDia é quantos emails de convite uma conta pode pedir em
	// 24 horas, somando convites novos e reenvios.
	MaximoDeEmailsPorDia = 30
	// MaximoDeEmailsPorConvite é quantas vezes um mesmo convite pode ser mandado.
	MaximoDeEmailsPorConvite = 5
	// IntervaloEntreReenvios é o mínimo entre dois envios do mesmo convite.
	IntervaloEntreReenvios = 2 * time.Minute
)

// Link monta a URL que entrega o convite, a partir da origem pública do
// frontend. O token vai no caminho, e não em query string: query string vaza com
// mais facilidade em log de proxy e em cabeçalho Referer.
//
// Mora no domínio porque o link sai por dois caminhos — a resposta que o dono vê
// e o email que o convidado recebe —, e os dois precisam ser o MESMO link.
func Link(origem, tokenPuro string) string {
	return origem + "/convite/" + url.PathEscape(tokenPuro)
}

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
	// ErrPendentesDemais é retornado quando o quadro já tem
	// MaximoDePendentesPorQuadro convites em aberto.
	ErrPendentesDemais = errors.New("este quadro já tem convites demais em aberto — revogue os que não vão ser usados")
	// ErrReenvioCedoDemais é retornado quando o mesmo convite foi mandado há
	// menos de IntervaloEntreReenvios.
	ErrReenvioCedoDemais = errors.New("o convite acabou de ser enviado — espere alguns minutos para reenviar")
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

// Renovar troca o token e devolve ao convite a validade inteira, a partir de
// agora. É o que o reenvio faz: o link antigo deixa de valer no mesmo instante
// em que o novo passa a existir.
//
// Vale para o convite pendente e para o VENCIDO — reenviar é o caminho natural
// de quem descobre que o convite expirou sem ser visto. Aceito e revogado são
// fatos gravados, e devolvem ErrJaResolvido.
func (c *Convite) Renovar(tokenHash string, agora time.Time) error {
	switch c.EstadoEm(agora) {
	case EstadoPendente, EstadoExpirado:
		c.TokenHash = tokenHash
		c.ExpiraEm = agora.Add(TTL)
		return nil
	default:
		return ErrJaResolvido
	}
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
