package board

import (
	"context"
	"time"

	dcomentario "stacktrack/internal/domain/comentario"
	demail "stacktrack/internal/domain/email"
	"stacktrack/internal/domain/notificacao"

	"github.com/google/uuid"
)

// RepositorioNotificacao grava os avisos pessoais, do ponto de vista de quem os
// CAUSA: a atribuição, o comentário, o convite. Participa da mesma transação
// do que os causou — o aviso existe se, e só se, o fato existe.
//
// Quem lê, marca como lido e conta é outro caso de uso (usecase/notificacao),
// que enxerga a mesma tabela por outra porta.
type RepositorioNotificacao interface {
	// Criar grava o aviso. Devolve false, sem erro, quando a chave já existia:
	// o mesmo fato não vira dois avisos.
	Criar(ctx context.Context, n *notificacao.Notificacao) (bool, error)
	// ExisteNaoLida responde se a pessoa ainda tem, sem ler, um aviso daquele
	// tipo sobre o card.
	ExisteNaoLida(ctx context.Context, usuarioID string, tipo notificacao.Tipo, cardID string) (bool, error)
	// MencionadosNoComentario devolve quem tem aviso de menção no comentário.
	MencionadosNoComentario(ctx context.Context, comentarioID string) ([]string, error)
	// RemoverMencoes apaga os avisos de menção daquelas pessoas no comentário
	// — e, pelo CASCADE, o email deles que ainda não saiu.
	RemoverMencoes(ctx context.Context, comentarioID string, usuarioIDs []string) error
	// RemoverDoQuadro apaga os avisos de uma pessoa sobre o quadro. É o que
	// roda quando ela sai dele.
	RemoverDoQuadro(ctx context.Context, boardID, usuarioID string) error
	// DestinatariosDoQuadro devolve quem tem algum aviso sobre o quadro: quem
	// precisa saber que a lista mudou quando algo do quadro some por CASCADE.
	DestinatariosDoQuadro(ctx context.Context, boardID string) ([]string, error)
}

// Campainha avisa as telas abertas de uma pessoa de que a lista de
// notificações dela mudou. Quem a implementa é o adaptador de tempo real
// (adapter/realtime/campainha), que entrega por SSE.
//
// Tocar não carrega a notificação, e é de propósito: a lista mora no banco, e a
// tela relê. Um aviso perdido — a aba fechada, a conexão caída — não perde
// nada, porque não havia nada nele além de "olhe de novo".
type Campainha interface {
	Tocar(usuarioIDs ...string)
}

// Os nomes e as versões dos textos dos emails de notificação, como em
// ModeloDeConvite.
const (
	ModeloDeAtribuicao = "notificacao.atribuicao.v1"
	ModeloDeMencao     = "notificacao.mencao.v1"
	ModeloDePrazo      = "notificacao.prazo.v1"
)

// modeloDoEmail devolve o modelo do email de cada tipo de notificação.
func modeloDoEmail(t notificacao.Tipo) string {
	switch t {
	case notificacao.TipoMencao:
		return ModeloDeMencao
	case notificacao.TipoPrazo:
		return ModeloDePrazo
	default:
		return ModeloDeAtribuicao
	}
}

// RedatorDeNotificacao escreve o email de uma notificação. Quem o implementa é
// o adaptador de email.
type RedatorDeNotificacao interface {
	Notificacao(d DadosDoEmailDeNotificacao) (demail.Mensagem, error)
}

// DadosDoEmailDeNotificacao é TUDO o que o email de uma notificação pode dizer.
//
// Só títulos — do quadro e do card —, como no email do convite: o email sai da
// proteção do sistema, e o texto de um comentário ou a descrição de um card
// ficariam numa caixa de entrada que ninguém controla. Quem quer o conteúdo
// abre o link, que exige sessão.
type DadosDoEmailDeNotificacao struct {
	Tipo notificacao.Tipo
	Para string
	// Nome é de quem recebe.
	Nome string
	// Autor é quem causou; vazio no prazo.
	Autor        string
	TituloQuadro string
	TituloCard   string
	// Prazo só vem no aviso de prazo.
	Prazo time.Time
	Link  string
}

// Notificador é o que os casos de uso de quadro precisam para avisar alguém: a
// campainha das telas abertas e, quando o envio está ligado, o email.
//
// Entra por ComNotificador, e não pelo construtor, pela mesma razão do
// publicador de eventos: sem ele os casos de uso continuam funcionando, só não
// avisam ninguém — que é o que os testes de regra querem.
type Notificador struct {
	campainha Campainha
	// redator nil significa envio de email desligado: o aviso continua no
	// sino, que é o que importa. Nenhum fluxo depende de email.
	redator RedatorDeNotificacao
	origem  string
	espera  time.Duration
	// avisarCorreio acorda o worker do outbox. Com espera, ele pega o email
	// quando a hora chegar; o aviso só adianta quando a espera é zero.
	avisarCorreio func()
	// acordarVigia pede uma passada do vigia de prazos: quem passou a
	// responder por um card que vence amanhã precisa saber agora, e não na
	// próxima volta do relógio.
	acordarVigia func()
}

// NovoNotificador cria o notificador com a campainha das telas abertas. Nil é
// aceito: os avisos são gravados do mesmo jeito, e as telas os encontram ao
// recarregar.
func NovoNotificador(campainha Campainha) *Notificador {
	return &Notificador{campainha: campainha}
}

// ComEmail liga o email das notificações. espera é quanto cada email aguarda
// antes de sair — a janela em que ler no sistema o cancela.
func (n *Notificador) ComEmail(redator RedatorDeNotificacao, origem string, espera time.Duration, avisar func()) *Notificador {
	n.redator, n.origem, n.espera, n.avisarCorreio = redator, origem, espera, avisar
	return n
}

// ComVigiaDePrazos liga o pedido de passada do vigia de prazos.
func (n *Notificador) ComVigiaDePrazos(acordar func()) *Notificador {
	n.acordarVigia = acordar
	return n
}

// notificacoes é embutido em cada caso de uso que avisa alguém, como eventos.
type notificacoes struct {
	notificador *Notificador
}

// ComNotificador liga os avisos pessoais deste caso de uso.
func (x *notificacoes) ComNotificador(n *Notificador) {
	x.notificador = n
}

// avisando informa se este caso de uso grava avisos nesta escrita. Sem
// notificador, ou fora de uma unidade de trabalho (os testes de regra), não.
func (x *notificacoes) avisando(e Escrita) bool {
	return x.notificador != nil && e.Notificacoes != nil
}

// avisar grava o aviso na transação em curso e, quando o tipo tem email e a
// cota da pessoa deixa, põe o email na fila com a espera. Devolve se o aviso
// nasceu — false quando o mesmo fato já tinha virado aviso.
func (x *notificacoes) avisar(ctx context.Context, e Escrita, aviso *notificacao.Notificacao) (bool, error) {
	if !x.avisando(e) {
		return false, nil
	}
	criada, err := e.Notificacoes.Criar(ctx, aviso)
	if err != nil || !criada {
		return criada, err
	}
	if !aviso.Tipo.TemEmail() || e.Emails == nil || x.notificador.redator == nil {
		return true, nil
	}
	return true, x.notificador.enfileirarEmail(ctx, e, aviso)
}

// enfileirarEmail redige e enfileira o email do aviso, na mesma transação.
//
// Redige AQUI dentro, ao contrário do convite: quem recebe só é conhecido sob o
// lock (a menção é lida da lista de membros daquele instante), e redigir é
// trabalho de CPU, sem rede. A transação continua curta.
//
// A cota é conferida aqui pelo mesmo motivo das do convite. Sem lock por
// pessoa, duas escritas simultâneas em quadros diferentes podem passar dela por
// um — o teto é contra caixa de entrada lotada, não contabilidade.
func (n *Notificador) enfileirarEmail(ctx context.Context, e Escrita, aviso *notificacao.Notificacao) error {
	enviados, err := e.Emails.ContarDeNotificacaoDesde(ctx, aviso.UsuarioID, aviso.CriadaEm.Add(-24*time.Hour))
	if err != nil {
		return err
	}
	if enviados >= notificacao.MaximoDeEmailsPorDia {
		return nil
	}

	destinatario, err := e.Usuarios.BuscarPorID(ctx, aviso.UsuarioID)
	if err != nil || destinatario == nil {
		return err
	}
	dados := DadosDoEmailDeNotificacao{
		Tipo: aviso.Tipo, Para: destinatario.Email, Nome: destinatario.Nome,
		Link: notificacao.Link(n.origem, aviso.ID),
	}
	if aviso.Prazo != nil {
		dados.Prazo = *aviso.Prazo
	}
	if aviso.AutorID != "" {
		if autor, err := e.Usuarios.BuscarPorID(ctx, aviso.AutorID); err != nil {
			return err
		} else if autor != nil {
			dados.Autor = autor.Nome
		}
	}
	if b, err := e.Boards.BuscarPorID(ctx, aviso.BoardID); err != nil {
		return err
	} else if b != nil {
		dados.TituloQuadro = b.Titulo
	}
	if aviso.CardID != "" {
		if c, err := e.Cards.BuscarPorID(ctx, aviso.CardID); err != nil {
			return err
		} else if c != nil {
			dados.TituloCard = c.Titulo
		}
	}

	m, err := n.redator.Notificacao(dados)
	if err != nil {
		return err
	}
	// Uma conta com email torto não pode desfazer a atribuição de quem a fez:
	// o aviso fica no sino, e o email simplesmente não sai.
	if m.Validar() != nil {
		return nil
	}

	// O envio é atribuído a quem causou o aviso. O prazo não tem ninguém por
	// trás, e aí o envio fica com a própria pessoa avisada: a coluna é
	// obrigatória, e um id inventado mentiria sobre quem pediu.
	autorDoEnvio := aviso.AutorID
	if autorDoEnvio == "" {
		autorDoEnvio = aviso.UsuarioID
	}
	envio := demail.NovoEnvio(uuid.NewString(), modeloDoEmail(aviso.Tipo), "notificacao:"+aviso.ID,
		autorDoEnvio, aviso.CriadaEm, aviso.CriadaEm.Add(notificacao.ValidadeDoEmail))
	envio.ProximaEm = aviso.CriadaEm.Add(n.espera)
	envio.NotificacaoID = aviso.ID
	return e.Emails.Enfileirar(ctx, envio, m)
}

// depois toca a campainha de quem foi afetado e acorda o worker de email. Roda
// só DEPOIS do commit: tocar antes avisaria de um aviso que o rollback ainda
// pode desfazer.
func (x *notificacoes) depois(usuarioIDs ...string) {
	if x.notificador == nil {
		return
	}
	ids := semRepeticao(usuarioIDs)
	if len(ids) == 0 {
		return
	}
	if x.notificador.campainha != nil {
		x.notificador.campainha.Tocar(ids...)
	}
	if x.notificador.avisarCorreio != nil {
		x.notificador.avisarCorreio()
	}
}

// acordarVigia pede uma passada ao vigia de prazos, depois do commit.
func (x *notificacoes) acordarVigia() {
	if x.notificador != nil && x.notificador.acordarVigia != nil {
		x.notificador.acordarVigia()
	}
}

// destinatariosDoQuadro devolve quem tem aviso sobre o quadro, quando este caso
// de uso avisa alguém. Chamado antes de uma exclusão em cascata: depois dela
// não há mais de onde tirar a lista.
func (x *notificacoes) destinatariosDoQuadro(ctx context.Context, e Escrita, boardID string) ([]string, error) {
	if !x.avisando(e) {
		return nil, nil
	}
	return e.Notificacoes.DestinatariosDoQuadro(ctx, boardID)
}

// mencionaveis transforma quem participa do quadro em quem pode ser chamado num
// comentário — menos o próprio autor, que não se avisa de nada.
func mencionaveis(participantes []Participante, autorID string) []dcomentario.Mencionavel {
	lista := make([]dcomentario.Mencionavel, 0, len(participantes))
	for _, p := range participantes {
		if p.UsuarioID == autorID {
			continue
		}
		lista = append(lista, dcomentario.Mencionavel{ID: p.UsuarioID, Nome: p.Nome})
	}
	return lista
}

func semRepeticao(ids []string) []string {
	vistos := make(map[string]bool, len(ids))
	unicos := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" || vistos[id] {
			continue
		}
		vistos[id] = true
		unicos = append(unicos, id)
	}
	return unicos
}
