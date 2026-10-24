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
	textoDoConvite *ttemplate.Template
	htmlDoConvite  *htemplate.Template
	fuso           *time.Location
}

// NovoRedator carrega os modelos. Eles vêm embutidos no binário, então um
// modelo quebrado é erro de programação, e o processo não sobe.
func NovoRedator() *Redator {
	fuso, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		panic(fmt.Sprintf("email: fuso de Brasília indisponível: %v", err))
	}
	return &Redator{
		textoDoConvite: ttemplate.Must(ttemplate.ParseFS(modelos, "modelos/convite.txt")),
		htmlDoConvite:  htemplate.Must(htemplate.ParseFS(modelos, "modelos/convite.html")),
		fuso:           fuso,
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

func encurtar(texto string, maximo int) string {
	if utf8.RuneCountInString(texto) <= maximo {
		return texto
	}
	runas := []rune(texto)
	return string(runas[:maximo-1]) + "…"
}
