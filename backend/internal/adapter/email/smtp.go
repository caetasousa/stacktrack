package email

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	demail "stacktrack/internal/domain/email"
)

// ConfigSMTP é como falar com o provedor de email.
type ConfigSMTP struct {
	Host    string
	Porta   int
	Usuario string
	Senha   string
	// STARTTLS sobe a conexão para TLS antes de autenticar. Obrigatório em
	// produção (config.Validar); desligado só contra o Mailpit local.
	STARTTLS bool
	// Remetente é o endereço do From; NomeDoRemetente, o nome que aparece.
	Remetente       string
	NomeDoRemetente string
	// Prazo é o teto da conversa inteira com o servidor, da conexão ao QUIT.
	Prazo time.Duration
}

// EnviadorSMTP entrega mensagens por SMTP. Uma conexão por mensagem: o volume
// é de convites, não de newsletter, e uma conexão longa precisaria de
// reconexão, keepalive e limite de mensagens — complexidade sem ganho.
type EnviadorSMTP struct {
	cfg ConfigSMTP
}

// NovoEnviadorSMTP cria o enviador.
func NovoEnviadorSMTP(cfg ConfigSMTP) *EnviadorSMTP {
	if cfg.Prazo <= 0 {
		cfg.Prazo = 30 * time.Second
	}
	return &EnviadorSMTP{cfg: cfg}
}

// Enviar entrega a mensagem. A falha que não melhora com nova tentativa — o
// servidor respondeu 5xx, ou a mensagem é inválida — vem embrulhada em
// email.ErrRecusaPermanente; o resto é passageiro, e o worker tenta de novo.
func (e *EnviadorSMTP) Enviar(ctx context.Context, m demail.Mensagem) error {
	if err := m.Validar(); err != nil {
		return errors.Join(demail.ErrRecusaPermanente, err)
	}
	bruta, err := e.montar(m)
	if err != nil {
		return errors.Join(demail.ErrRecusaPermanente, err)
	}

	prazo := time.Now().Add(e.cfg.Prazo)
	if limite, ok := ctx.Deadline(); ok && limite.Before(prazo) {
		prazo = limite
	}
	endereco := net.JoinHostPort(e.cfg.Host, strconv.Itoa(e.cfg.Porta))
	conexao, err := (&net.Dialer{Deadline: prazo}).DialContext(ctx, "tcp", endereco)
	if err != nil {
		return fmt.Errorf("conectar ao servidor de email: %w", err)
	}
	// O prazo vale para a conversa inteira: sem ele, um servidor que aceita a
	// conexão e não responde prenderia o worker para sempre.
	_ = conexao.SetDeadline(prazo)

	cliente, err := smtp.NewClient(conexao, e.cfg.Host)
	if err != nil {
		_ = conexao.Close()
		return classificar("abrir a conversa SMTP", err)
	}
	defer cliente.Close() //nolint:errcheck // depois do QUIT a conexão já acabou; a falha não muda a entrega

	if e.cfg.STARTTLS {
		if ok, _ := cliente.Extension("STARTTLS"); !ok {
			return errors.New("o servidor de email não oferece STARTTLS, e a configuração o exige")
		}
		if err := cliente.StartTLS(&tls.Config{ServerName: e.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return classificar("subir a conexão para TLS", err)
		}
	}
	if e.cfg.Usuario != "" {
		// PlainAuth recusa mandar a senha por conexão sem TLS a um host que
		// não seja local — a mesma regra que a configuração aplica, repetida
		// pela biblioteca.
		if err := cliente.Auth(smtp.PlainAuth("", e.cfg.Usuario, e.cfg.Senha, e.cfg.Host)); err != nil {
			return classificar("autenticar no servidor de email", err)
		}
	}
	if err := cliente.Mail(e.cfg.Remetente); err != nil {
		return classificar("informar o remetente", err)
	}
	if err := cliente.Rcpt(m.Para); err != nil {
		return classificar("informar o destinatário", err)
	}
	escritor, err := cliente.Data()
	if err != nil {
		return classificar("abrir o corpo da mensagem", err)
	}
	if _, err := escritor.Write(bruta); err != nil {
		return classificar("mandar o corpo da mensagem", err)
	}
	if err := escritor.Close(); err != nil {
		return classificar("concluir a mensagem", err)
	}
	// O servidor já aceitou: uma falha no QUIT não desfaz a entrega, e tratá-la
	// como erro faria o worker mandar a mensagem de novo.
	_ = cliente.Quit()
	return nil
}

// classificar marca como permanente a resposta 5xx do servidor. O texto do
// erro é o do servidor, que costuma citar o destinatário — quem o grava ou
// loga mascara antes (email.MascararEnderecos).
func classificar(etapa string, err error) error {
	var resposta *textproto.Error
	if errors.As(err, &resposta) && resposta.Code >= 500 {
		return errors.Join(demail.ErrRecusaPermanente, fmt.Errorf("%s: %w", etapa, err))
	}
	return fmt.Errorf("%s: %w", etapa, err)
}

// montar escreve a mensagem em MIME: cabeçalhos com acento em encoded-word, e
// as duas versões do corpo como multipart/alternative em quoted-printable — o
// leitor escolhe o HTML, e o que não sabe ler HTML mostra o texto.
func (e *EnviadorSMTP) montar(m demail.Mensagem) ([]byte, error) {
	var corpo bytes.Buffer
	partes := multipart.NewWriter(&corpo)
	for _, parte := range []struct{ tipo, conteudo string }{
		{"text/plain; charset=utf-8", m.Texto},
		{"text/html; charset=utf-8", m.HTML},
	} {
		if strings.TrimSpace(parte.conteudo) == "" {
			continue
		}
		escritor, err := partes.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {parte.tipo},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, err
		}
		qp := quotedprintable.NewWriter(escritor)
		if _, err := qp.Write([]byte(parte.conteudo)); err != nil {
			return nil, err
		}
		if err := qp.Close(); err != nil {
			return nil, err
		}
	}
	if err := partes.Close(); err != nil {
		return nil, err
	}

	de := (&mail.Address{Name: demail.LimparCabecalho(e.cfg.NomeDoRemetente), Address: e.cfg.Remetente}).String()
	cabecalhos := []string{
		"From: " + de,
		"To: " + (&mail.Address{Address: m.Para}).String(),
		"Subject: " + mime.QEncoding.Encode("utf-8", demail.LimparCabecalho(m.Assunto)),
		"Date: " + time.Now().Format(time.RFC1123Z),
		"Message-ID: " + idDaMensagem(e.cfg.Remetente),
		"MIME-Version: 1.0",
		"Content-Type: multipart/alternative; boundary=" + partes.Boundary(),
	}
	if m.ResponderPara != "" {
		cabecalhos = append(cabecalhos, "Reply-To: "+(&mail.Address{Address: m.ResponderPara}).String())
	}

	var bruta bytes.Buffer
	bruta.WriteString(strings.Join(cabecalhos, "\r\n"))
	bruta.WriteString("\r\n\r\n")
	bruta.Write(corpo.Bytes())
	return bruta.Bytes(), nil
}

// idDaMensagem gera um Message-ID no domínio do remetente. Sem ele, alguns
// provedores o inventam, e outros contam a ausência contra a reputação.
func idDaMensagem(remetente string) string {
	dominio := "stacktrack"
	if _, depois, ok := strings.Cut(remetente, "@"); ok && depois != "" {
		dominio = depois
	}
	aleatorio := make([]byte, 16)
	_, _ = rand.Read(aleatorio)
	return "<" + hex.EncodeToString(aleatorio) + "@" + dominio + ">"
}
