// Package email é o adaptador de email: redige as mensagens a partir dos
// modelos versionados em modelos/ e as entrega por SMTP.
package email

import (
	"bytes"
	"embed"
	"fmt"
	htemplate "html/template"
	ttemplate "text/template"
	"time"
	// O fuso de Brasília vem embutido no binário: a data de validade do
	// convite não pode depender de o container ter o tzdata instalado.
	_ "time/tzdata"
	"unicode/utf8"

	demail "stacktrack/internal/domain/email"
	"stacktrack/internal/domain/membro"
	"stacktrack/internal/domain/notificacao"
	ucauth "stacktrack/internal/usecase/auth"
	ucboard "stacktrack/internal/usecase/board"
)

//go:embed modelos/*
var modelos embed.FS

// maximoDoTituloNoAssunto é até onde o título do quadro entra no assunto. O
// título pode ter 120 caracteres; um assunto desse tamanho é cortado pelo
// leitor de email no lugar errado.
const maximoDoTituloNoAssunto = 60

var explicacaoDoPapel = map[membro.Papel]string{
	membro.PapelDono:   "poder total no quadro, inclusive apagá-lo",
	membro.PapelEditor: "criar e mexer em colunas e cards",
	membro.PapelLeitor: "ver o quadro, sem alterar nada",
}

// Redator escreve os emails do sistema a partir dos modelos versionados.
type Redator struct {
	textoDoConvite     *ttemplate.Template
	htmlDoConvite      *htemplate.Template
	textoDeRecuperacao *ttemplate.Template
	htmlDeRecuperacao  *htemplate.Template
	textoDeConfirmacao *ttemplate.Template
	htmlDeConfirmacao  *htemplate.Template
	textoDeNotificacao *ttemplate.Template
	htmlDeNotificacao  *htemplate.Template
	fuso               *time.Location
}

// NovoRedator carrega os modelos. Eles vêm embutidos no binário, então um
// modelo quebrado é erro de programação, e o processo não sobe.
func NovoRedator() *Redator {
	fuso, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		panic(fmt.Sprintf("email: fuso de Brasília indisponível: %v", err))
	}
	return &Redator{
		textoDoConvite:     ttemplate.Must(ttemplate.ParseFS(modelos, "modelos/convite.txt")),
		htmlDoConvite:      htemplate.Must(htemplate.ParseFS(modelos, "modelos/convite.html")),
		textoDeRecuperacao: ttemplate.Must(ttemplate.ParseFS(modelos, "modelos/recuperacao.txt")),
		htmlDeRecuperacao:  htemplate.Must(htemplate.ParseFS(modelos, "modelos/recuperacao.html")),
		textoDeConfirmacao: ttemplate.Must(ttemplate.ParseFS(modelos, "modelos/confirmacao.txt")),
		htmlDeConfirmacao:  htemplate.Must(htemplate.ParseFS(modelos, "modelos/confirmacao.html")),
		textoDeNotificacao: ttemplate.Must(ttemplate.ParseFS(modelos, "modelos/notificacao.txt")),
		htmlDeNotificacao:  htemplate.Must(htemplate.ParseFS(modelos, "modelos/notificacao.html")),
		fuso:               fuso,
	}
}

// Convite redige o email que entrega um convite.
//
// O nome de quem convida e o título do quadro são escritos por usuários: no
// assunto eles passam por LimparCabecalho, que tira quebra de linha, e no HTML
// pelo escape do html/template — um quadro chamado "<a href=…>" aparece como
// texto, e não como link.
func (r *Redator) Convite(d ucboard.DadosDoEmailDeConvite) (demail.Mensagem, error) {
	nome := demail.LimparCabecalho(d.ConvidadoPor)
	if nome == "" {
		nome = "Alguém"
	}
	titulo := demail.LimparCabecalho(d.TituloQuadro)

	dados := map[string]any{
		"Nome":       nome,
		"Titulo":     titulo,
		"Papel":      string(d.Papel),
		"Explicacao": explicacaoDoPapel[d.Papel],
		"Link":       d.Link,
		"Para":       d.Para,
		"ValeAte":    d.ExpiraEm.In(r.fuso).Format("02/01/2006 às 15h04"),
	}

	var texto, html bytes.Buffer
	if err := r.textoDoConvite.Execute(&texto, dados); err != nil {
		return demail.Mensagem{}, fmt.Errorf("redigir o texto do convite: %w", err)
	}
	// O link entra como texto comum, e não como template.URL: assim o
	// html/template continua filtrando esquema estranho no href — a origem é
	// validada no boot, e esta é a segunda barreira, não a primeira.
	if err := r.htmlDoConvite.Execute(&html, dados); err != nil {
		return demail.Mensagem{}, fmt.Errorf("redigir o HTML do convite: %w", err)
	}

	m := demail.Mensagem{
		Para:    d.Para,
		Assunto: fmt.Sprintf("%s convidou você para o quadro “%s” no stacktrack", nome, encurtar(titulo, maximoDoTituloNoAssunto)),
		Texto:   texto.String(),
		HTML:    html.String(),
	}
	// A resposta vai para quem convidou — mas só um endereço que seja
	// endereço: o campo vem da conta, e uma conta antiga com email torto não
	// pode derrubar o convite.
	if demail.EnderecoValido(d.ResponderPara) {
		m.ResponderPara = d.ResponderPara
	}
	return m, nil
}

// Recuperacao redige o email que leva o link de senha nova.
//
// Sem Reply-To: do outro lado não há pessoa, e uma resposta iria para o
// remetente automático, que é o certo. O nome da conta passa pelo escape do
// html/template no corpo; o assunto é fixo e não leva nada vindo de usuário.
func (r *Redator) Recuperacao(d ucauth.DadosDoEmailDeRecuperacao) (demail.Mensagem, error) {
	dados := map[string]any{
		"Nome":    demail.LimparCabecalho(d.Nome),
		"Link":    d.Link,
		"Para":    d.Para,
		"ValeAte": d.ExpiraEm.In(r.fuso).Format("02/01/2006 às 15h04"),
	}

	var texto, html bytes.Buffer
	if err := r.textoDeRecuperacao.Execute(&texto, dados); err != nil {
		return demail.Mensagem{}, fmt.Errorf("redigir o texto da recuperação: %w", err)
	}
	if err := r.htmlDeRecuperacao.Execute(&html, dados); err != nil {
		return demail.Mensagem{}, fmt.Errorf("redigir o HTML da recuperação: %w", err)
	}
	return demail.Mensagem{
		Para:    d.Para,
		Assunto: "Crie uma senha nova no stacktrack",
		Texto:   texto.String(),
		HTML:    html.String(),
	}, nil
}

// Confirmacao redige o email que leva o link de confirmação da conta. Como o da
// recuperação, sem Reply-To e com assunto fixo.
func (r *Redator) Confirmacao(d ucauth.DadosDoEmailDeConfirmacao) (demail.Mensagem, error) {
	dados := map[string]any{
		"Nome":    demail.LimparCabecalho(d.Nome),
		"Link":    d.Link,
		"Para":    d.Para,
		"ValeAte": d.ExpiraEm.In(r.fuso).Format("02/01/2006 às 15h04"),
	}
	var texto, html bytes.Buffer
	if err := r.textoDeConfirmacao.Execute(&texto, dados); err != nil {
		return demail.Mensagem{}, fmt.Errorf("redigir o texto da confirmação: %w", err)
	}
	if err := r.htmlDeConfirmacao.Execute(&html, dados); err != nil {
		return demail.Mensagem{}, fmt.Errorf("redigir o HTML da confirmação: %w", err)
	}
	return demail.Mensagem{
		Para:    d.Para,
		Assunto: "Confirme seu email no stacktrack",
		Texto:   texto.String(),
		HTML:    html.String(),
	}, nil
}

// Notificacao redige o email de uma notificação: atribuição, menção ou prazo.
//
// Um modelo só para os três, com a frase montada aqui: o que muda entre eles é
// uma frase e o convite à ação, e três pares de arquivos quase iguais
// divergiriam no primeiro ajuste de redação. A frase leva títulos escritos por
// usuários — no assunto eles passam por LimparCabecalho, e no HTML pelo escape
// do html/template, como no convite.
//
// Só títulos, nunca o texto do comentário nem a descrição do card: ver
// ucboard.DadosDoEmailDeNotificacao.
func (r *Redator) Notificacao(d ucboard.DadosDoEmailDeNotificacao) (demail.Mensagem, error) {
	autor := demail.LimparCabecalho(d.Autor)
	if autor == "" {
		autor = "Alguém"
	}
	card := demail.LimparCabecalho(d.TituloCard)
	quadro := demail.LimparCabecalho(d.TituloQuadro)
	curto := encurtar(card, maximoDoTituloNoAssunto)

	var assunto, frase, acao string
	switch d.Tipo {
	case notificacao.TipoAtribuicao:
		assunto = fmt.Sprintf("%s pôs você como responsável por “%s”", autor, curto)
		frase = fmt.Sprintf("%s pôs você como responsável pelo card “%s”, no quadro “%s”.", autor, card, quadro)
		acao = "Para abrir o card:"
	case notificacao.TipoMencao:
		assunto = fmt.Sprintf("%s mencionou você em “%s”", autor, curto)
		frase = fmt.Sprintf("%s mencionou você num comentário do card “%s”, no quadro “%s”.", autor, card, quadro)
		acao = "Para ler o comentário:"
	case notificacao.TipoPrazo:
		// A data no fuso de Brasília: o prazo é gravado ao meio-dia daqui, e em
		// UTC ele cairia no dia seguinte sempre que fosse gravado à noite.
		dia := d.Prazo.In(r.fuso)
		assunto = fmt.Sprintf("O prazo de “%s” vence em %s", curto, dia.Format("02/01"))
		frase = fmt.Sprintf("O prazo do card “%s”, no quadro “%s”, vence em %s — e você é uma das pessoas responsáveis por ele.",
			card, quadro, dia.Format("02/01/2006"))
		acao = "Para abrir o card:"
	default:
		return demail.Mensagem{}, fmt.Errorf("a notificação do tipo %q não tem email", d.Tipo)
	}

	dados := map[string]any{
		"Assunto": assunto, "Frase": frase, "Acao": acao, "Link": d.Link, "Quadro": quadro,
	}
	var texto, html bytes.Buffer
	if err := r.textoDeNotificacao.Execute(&texto, dados); err != nil {
		return demail.Mensagem{}, fmt.Errorf("redigir o texto da notificação: %w", err)
	}
	if err := r.htmlDeNotificacao.Execute(&html, dados); err != nil {
		return demail.Mensagem{}, fmt.Errorf("redigir o HTML da notificação: %w", err)
	}
	return demail.Mensagem{Para: d.Para, Assunto: assunto, Texto: texto.String(), HTML: html.String()}, nil
}

func encurtar(texto string, maximo int) string {
	if utf8.RuneCountInString(texto) <= maximo {
		return texto
	}
	runas := []rune(texto)
	return string(runas[:maximo-1]) + "…"
}
