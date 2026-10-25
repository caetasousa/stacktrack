// Package email modela a mensagem que o sistema manda e o ciclo de vida do
// envio dela — independente de fornecedor, de SMTP e de quem redige o texto.
//
// O envio é assíncrono: o fluxo que pede uma mensagem grava a intenção no
// outbox, na mesma transação, e um worker a entrega depois do commit. As regras
// de quando tentar de novo e quando desistir moram aqui, e não no worker, para
// que a política seja uma decisão escrita e testada, não um detalhe de loop.
package email

import (
	"errors"
	"math/rand/v2"
	"net/mail"
	"regexp"
	"strings"
	"time"
)

// Estado é onde um envio está no seu ciclo de vida.
type Estado string

const (
	// EstadoPendente é o envio na fila, esperando a vez ou a próxima tentativa.
	EstadoPendente Estado = "pendente"
	// EstadoEnviado é terminal: o servidor SMTP aceitou a mensagem.
	EstadoEnviado Estado = "enviado"
	// EstadoFalhou é terminal: o envio desistiu, por recusa definitiva do
	// servidor ou por esgotar as tentativas. A linha fica para a operação ver.
	EstadoFalhou Estado = "falhou"
	// EstadoCancelado é terminal: a mensagem deixou de fazer sentido antes de
	// sair (o convite foi revogado, aceito ou reenviado com outro link).
	EstadoCancelado Estado = "cancelado"
	// EstadoExpirado é terminal: o prazo da mensagem passou sem ela sair, e o
	// link dentro dela já não abriria nada.
	EstadoExpirado Estado = "expirado"
)

// MaximoDeTentativas é quantas vezes o worker tenta antes de desistir.
//
// Oito, com a espera dobrando a partir de trinta segundos (30 s, 1, 2, 4, 8, 16
// e 32 min), cobre cerca de uma hora: o bastante para atravessar uma queda curta
// do provedor, e pouco o bastante para um convite não chegar no dia seguinte
// como se fosse novo. Para além disso, quem convidou vê "o email não saiu" na
// lista de convites e reenvia.
const MaximoDeTentativas = 8

const (
	esperaInicial = 30 * time.Second
	esperaMaxima  = time.Hour
)

var (
	// ErrDestinatarioInvalido é retornado quando o endereço de destino não é um
	// email utilizável.
	ErrDestinatarioInvalido = errors.New("destinatário de email inválido")
	// ErrMensagemIncompleta é retornado quando falta assunto ou corpo.
	ErrMensagemIncompleta = errors.New("mensagem de email sem assunto ou sem corpo")
	// ErrRecusaPermanente marca a falha que não melhora com nova tentativa — o
	// servidor recusou o destinatário ou a mensagem em definitivo (respostas
	// 5xx do SMTP). Quem envia a embrulha com errors.Join ou fmt.Errorf("%w").
	ErrRecusaPermanente = errors.New("o servidor de email recusou a mensagem em definitivo")
)

// Mensagem é um email pronto para sair: endereços, assunto e as duas versões
// do corpo. Quem a monta é o redator; quem a entrega é o adaptador de envio.
type Mensagem struct {
	Para string
	// ResponderPara é opcional: o endereço que recebe a resposta, quando ela
	// não deve ir para o remetente do sistema.
	ResponderPara string
	Assunto       string
	Texto         string
	HTML          string
}

// Validar confere o que o envio exige: um destinatário que seja endereço de
// verdade, assunto e corpo. Os campos de cabeçalho não podem carregar quebra de
// linha — é por ela que um valor vindo de usuário injetaria cabeçalhos.
func (m Mensagem) Validar() error {
	if !EnderecoValido(m.Para) {
		return ErrDestinatarioInvalido
	}
	if m.ResponderPara != "" && !EnderecoValido(m.ResponderPara) {
		return ErrDestinatarioInvalido
	}
	if strings.TrimSpace(m.Assunto) == "" || (strings.TrimSpace(m.Texto) == "" && strings.TrimSpace(m.HTML) == "") {
		return ErrMensagemIncompleta
	}
	if strings.ContainsAny(m.Assunto, "\r\n") {
		return ErrMensagemIncompleta
	}
	return nil
}

// EnderecoValido informa se o texto é um endereço de email nu, sem nome de
// exibição e sem nada que um cabeçalho possa interpretar.
func EnderecoValido(endereco string) bool {
	if endereco == "" || strings.ContainsAny(endereco, "\r\n<>\" ,;") {
		return false
	}
	a, err := mail.ParseAddress(endereco)
	return err == nil && a.Address == endereco
}

// LimparCabecalho tira de um texto o que não pode ir num cabeçalho: quebras de
// linha e outros caracteres de controle. Serve para valores que vêm de usuário
// — o nome de quem convida, o título do quadro — e acabam no Assunto.
func LimparCabecalho(texto string) string {
	var b strings.Builder
	for _, r := range texto {
		if r < 0x20 || r == 0x7f {
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// Envio é uma linha do outbox: a intenção de mandar uma mensagem, e onde ela
// está. A mensagem em si não está aqui — ela viaja cifrada, e só quem entrega
// precisa dela.
type Envio struct {
	ID string
	// Modelo é o nome e a versão do texto que redigiu a mensagem.
	Modelo string
	// ChaveDeduplicacao impede a mesma intenção de virar dois emails.
	ChaveDeduplicacao string
	Estado            Estado
	Tentativas        int
	ProximaEm         time.Time
	CriadoEm          time.Time
	EnviadoEm         *time.Time
	ExpiraEm          time.Time
	UltimoErro        string
	AutorID           string
	// ConviteID é o convite que a mensagem entrega, quando é o caso.
	ConviteID string
	// NotificacaoID é a notificação que a mensagem avisa, quando é o caso. O
	// worker a confere na hora de entregar: lida ou desfeita, o email não sai.
	NotificacaoID string
}

// NovoEnvio cria a intenção de mandar uma mensagem agora, valendo até expiraEm.
func NovoEnvio(id, modelo, chave, autorID string, agora, expiraEm time.Time) *Envio {
	return &Envio{
		ID:                id,
		Modelo:            modelo,
		ChaveDeduplicacao: chave,
		Estado:            EstadoPendente,
		ProximaEm:         agora,
		CriadoEm:          agora,
		ExpiraEm:          expiraEm,
		AutorID:           autorID,
	}
}

// Desfecho é o que fazer com um envio depois de uma tentativa que falhou.
type Desfecho struct {
	// Desistir é true quando o envio vira EstadoFalhou.
	Desistir bool
	// ProximaEm é quando tentar de novo; só vale quando Desistir é false.
	ProximaEm time.Time
}

// AposFalha decide o que acontece depois da tentativa de número `tentativas`
// (contando a que acabou de falhar).
//
// Recusa permanente desiste na hora: insistir com um endereço que não existe
// só piora a reputação do remetente. O resto espera em dobro a cada tentativa,
// com um sorteio de ±20% — sem ele, um provedor que volta do ar recebe todas as
// mensagens represadas no mesmo segundo.
func AposFalha(tentativas int, err error, agora time.Time) Desfecho {
	if errors.Is(err, ErrRecusaPermanente) || tentativas >= MaximoDeTentativas {
		return Desfecho{Desistir: true}
	}
	return Desfecho{ProximaEm: agora.Add(Espera(tentativas, rand.Float64))}
}

// Espera é o intervalo antes da próxima tentativa, depois de `tentativas`
// falhas. `sorteio` devolve um número em [0, 1); recebê-lo de fora é o que
// deixa o teste fixar o resultado.
func Espera(tentativas int, sorteio func() float64) time.Duration {
	if tentativas < 1 {
		tentativas = 1
	}
	espera := esperaInicial
	for i := 1; i < tentativas && espera < esperaMaxima; i++ {
		espera *= 2
	}
	espera = min(espera, esperaMaxima)
	fator := 0.8 + 0.4*sorteio()
	return time.Duration(float64(espera) * fator)
}

// enderecoNoTexto acha endereços de email no meio de um texto livre.
var enderecoNoTexto = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)

// MascararEnderecos troca todo endereço de email de um texto pela forma
// "a***@exemplo.com".
//
// Existe para as mensagens de erro do servidor SMTP, que repetem o endereço do
// destinatário ("550 5.1.1 <ana@exemplo.com>: User unknown") e vão para o log e
// para o outbox — onde email completo não entra.
func MascararEnderecos(texto string) string {
	return enderecoNoTexto.ReplaceAllStringFunc(texto, func(endereco string) string {
		arroba := strings.LastIndex(endereco, "@")
		return endereco[:1] + "***" + endereco[arroba:]
	})
}
