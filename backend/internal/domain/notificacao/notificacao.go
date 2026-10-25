// Package notificacao modela o aviso pessoal: algo aconteceu e precisa da
// atenção de UMA pessoa — o convite para um quadro, o card que passou a ser
// dela, a menção num comentário, o prazo chegando.
//
// A notificação é um fato gravado, e não uma mensagem em trânsito: quem estava
// fora encontra tudo ao voltar, e o tempo real só avisa que a lista mudou. O
// que NÃO é gravado é se ela ainda vale — a atribuição pode ter sido desfeita,
// o prazo trocado, o convite recusado. Essa resposta é lida do estado atual a
// cada consulta (ver adapter/repository/notificacao_postgres.go), porque uma
// marca de "não vale mais" teria de ser escrita em cada caminho que desfaz
// alguma coisa, e o caminho esquecido viraria um aviso falso.
package notificacao

import (
	"errors"
	"fmt"
	"net/url"
	"time"
)

// Link monta o endereço que o email de uma notificação leva.
//
// Aponta para a NOTIFICAÇÃO, e não direto para o card: é a página dela que
// confere, na hora do clique, se o aviso ainda vale — a atribuição pode ter
// sido desfeita depois de o email sair, e o link precisa dizer isso em vez de
// abrir o card como se nada tivesse mudado. O id não é segredo: a página exige
// a sessão de quem recebeu.
func Link(origem, id string) string {
	return origem + "/notificacoes/" + url.PathEscape(id)
}

// Tipo diz o que a notificação avisa. O valor viaja para a tela e é gravado,
// então mudar uma destas strings é mudar contrato e dado.
type Tipo string

const (
	// TipoConvite é o convite para participar de um quadro, para quem já tem
	// conta com aquele email.
	TipoConvite Tipo = "convite"
	// TipoAtribuicao é a pessoa posta como responsável por um card.
	TipoAtribuicao Tipo = "atribuicao"
	// TipoMencao é a pessoa chamada pelo nome num comentário.
	TipoMencao Tipo = "mencao"
	// TipoPrazo é o prazo de um card de que a pessoa é responsável chegando.
	TipoPrazo Tipo = "prazo"
)

// TemEmail informa se a notificação também sai por email.
//
// O convite não sai: ele já tem o email dele, com o link, e um segundo email
// dizendo a mesma coisa só ensinaria a ignorar os dois.
func (t Tipo) TemEmail() bool {
	switch t {
	case TipoAtribuicao, TipoMencao, TipoPrazo:
		return true
	default:
		return false
	}
}

// A retenção é escolhida pelo dono de cada quadro: por quantos dias uma
// notificação LIDA daquele quadro fica guardada antes de a faxina apagá-la.
const (
	// RetencaoPadraoDias vale para o quadro que nunca escolheu. Um mês cobre a
	// volta de férias de quem quer achar o aviso que já leu.
	RetencaoPadraoDias = 30
	RetencaoMinimaDias = 1
	// RetencaoMaximaDias é também a idade máxima de qualquer notificação, lida
	// ou não: um aviso de um ano atrás não pede atenção de ninguém, e sem teto a
	// tabela de quem nunca abre o sino cresceria para sempre.
	RetencaoMaximaDias = 365
)

// IdadeMaxima é quanto uma notificação pode existir, lida ou não.
const IdadeMaxima = RetencaoMaximaDias * 24 * time.Hour

const (
	// AntecedenciaDoPrazo é quanto antes do prazo a pessoa é avisada. O prazo
	// é uma data, gravada ao meio-dia: um dia antes é a véspera, que é quando
	// ainda dá para fazer alguma coisa.
	AntecedenciaDoPrazo = 24 * time.Hour

	// EsperaDoEmail é quanto o email espera antes de sair. Quem está com o
	// stacktrack aberto vê o sino na hora e lê — e a notificação lida cancela o
	// email que ainda não saiu. Sem a espera, todo aviso chegaria duas vezes a
	// quem já estava olhando.
	EsperaDoEmail = 2 * time.Minute

	// ValidadeDoEmail é até quando um email de notificação faz sentido. Um que
	// não saiu em um dia (provedor fora do ar) já chegaria velho, e a pessoa
	// provavelmente já viu o sino.
	ValidadeDoEmail = 24 * time.Hour

	// MaximoDeEmailsPorDia é quantos emails de notificação uma pessoa recebe em
	// 24 horas. Passando disso, os avisos continuam no sino e o email para:
	// alguém atribuindo trinta cards de uma vez não pode encher a caixa de
	// entrada de outra pessoa — nem gastar sozinho a cota diária do provedor.
	MaximoDeEmailsPorDia = 20

	// TamanhoDaPagina é quantas notificações a lista devolve por vez.
	TamanhoDaPagina = 30
)

var (
	// ErrRetencaoInvalida é retornado quando a retenção sai da faixa permitida.
	ErrRetencaoInvalida = fmt.Errorf("a retenção das notificações precisa ficar entre %d e %d dias",
		RetencaoMinimaDias, RetencaoMaximaDias)
	// ErrNaoEncontrada é retornado quando a notificação não existe — ou é de
	// outra pessoa, ou de um quadro de que ela já não participa. Os casos não se
	// distinguem: saber o id de uma notificação não vale nada.
	ErrNaoEncontrada = errors.New("notificação não encontrada")
)

// ValidarRetencao confere a retenção escolhida pelo dono do quadro.
func ValidarRetencao(dias int) error {
	if dias < RetencaoMinimaDias || dias > RetencaoMaximaDias {
		return ErrRetencaoInvalida
	}
	return nil
}

// Situacao é se a notificação ainda vale, e, quando não vale, por quê.
type Situacao string

const (
	// SituacaoVigente é a notificação que ainda pede atenção.
	SituacaoVigente Situacao = "vigente"
	// SituacaoEncerrada é o fato que deixou de valer: a atribuição desfeita, o
	// prazo trocado ou a pessoa que não responde mais pelo card.
	SituacaoEncerrada Situacao = "encerrada"
	// As do convite dizem o desfecho, porque cada um pede uma frase diferente:
	// "você aceitou" não é "o convite foi cancelado".
	SituacaoConviteAceito   Situacao = "aceito"
	SituacaoConviteRecusado Situacao = "recusado"
	SituacaoConviteRevogado Situacao = "revogado"
	SituacaoConviteVencido  Situacao = "vencido"
)

// Notificacao é um aviso para uma pessoa.
type Notificacao struct {
	ID string
	// UsuarioID é quem recebe.
	UsuarioID string
	Tipo      Tipo
	// BoardID é o quadro de que a notificação fala. Todo tipo tem um: é por
	// ele que a retenção é escolhida e que a saída do quadro esconde o aviso.
	BoardID string
	// CardID vem na atribuição, na menção e no prazo.
	CardID string
	// ComentarioID vem só na menção.
	ComentarioID string
	// ConviteID vem só no convite.
	ConviteID string
	// AutorID é quem causou o aviso. Vazio no prazo, que não tem pessoa por
	// trás.
	AutorID string
	// Prazo é a data que motivou o aviso de prazo. Guardada porque o prazo do
	// card pode mudar depois, e o aviso de uma data que já não vale precisa
	// deixar de contar.
	Prazo *time.Time
	// Chave impede que o mesmo fato vire dois avisos — o mesmo prazo visto
	// por duas passadas do vigia, o mesmo convite reenviado.
	Chave    string
	CriadaEm time.Time
	// LidaEm é nil enquanto a pessoa não leu.
	LidaEm *time.Time
}

// NovaDeConvite avisa quem tem conta com o email convidado. Uma por convite e
// pessoa: reenviar o convite não cria outra.
func NovaDeConvite(id, usuarioID, boardID, conviteID, autorID string, em time.Time) *Notificacao {
	return &Notificacao{
		ID: id, UsuarioID: usuarioID, Tipo: TipoConvite, BoardID: boardID,
		ConviteID: conviteID, AutorID: autorID,
		Chave:    fmt.Sprintf("convite:%s:%s", conviteID, usuarioID),
		CriadaEm: em,
	}
}

// NovaDeAtribuicao avisa quem foi posto como responsável por um card.
//
// A chave é a própria notificação: atribuir de novo, depois de desatribuir, é
// um fato novo. Quem evita o aviso repetido enquanto o primeiro não foi lido é
// o caso de uso, que confere isso sob o lock do quadro.
func NovaDeAtribuicao(id, usuarioID, boardID, cardID, autorID string, em time.Time) *Notificacao {
	return &Notificacao{
		ID: id, UsuarioID: usuarioID, Tipo: TipoAtribuicao, BoardID: boardID,
		CardID: cardID, AutorID: autorID,
		Chave:    "atribuicao:" + id,
		CriadaEm: em,
	}
}

// NovaDeMencao avisa quem foi chamado pelo nome num comentário. Uma por
// comentário e pessoa: editar o comentário sem mexer na menção não avisa de
// novo.
func NovaDeMencao(id, usuarioID, boardID, cardID, comentarioID, autorID string, em time.Time) *Notificacao {
	return &Notificacao{
		ID: id, UsuarioID: usuarioID, Tipo: TipoMencao, BoardID: boardID,
		CardID: cardID, ComentarioID: comentarioID, AutorID: autorID,
		Chave:    fmt.Sprintf("mencao:%s:%s", comentarioID, usuarioID),
		CriadaEm: em,
	}
}

// NovaDePrazo avisa quem responde por um card que o prazo está chegando. Uma
// por card, pessoa e DATA: o vigia passa várias vezes pelo mesmo prazo e avisa
// uma só; adiar o prazo é uma data nova, e avisa de novo quando ela chegar.
//
// A data entra na chave em microssegundos, que é a precisão em que o banco a
// guarda: com segundos, duas datas que o banco distingue cairiam na mesma
// chave.
func NovaDePrazo(id, usuarioID, boardID, cardID string, prazo, em time.Time) *Notificacao {
	p := prazo
	return &Notificacao{
		ID: id, UsuarioID: usuarioID, Tipo: TipoPrazo, BoardID: boardID,
		CardID: cardID, Prazo: &p,
		Chave:    fmt.Sprintf("prazo:%s:%s:%d", cardID, usuarioID, prazo.UnixMicro()),
		CriadaEm: em,
	}
}
