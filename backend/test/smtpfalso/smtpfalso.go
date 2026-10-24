// Package smtpfalso é um servidor SMTP de mentira para os testes: fala o
// protocolo de verdade numa porta local, guarda as mensagens que recebe e pode
// ser mandado recusar ou cair.
//
// É o dublê da FRONTEIRA DE FORA — o provedor de email —, e só dela. O que é
// nosso (o outbox, o worker, o redator, o cliente SMTP) roda de verdade contra
// ele, como rodaria contra o Brevo.
package smtpfalso

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"strings"
	"sync"
	"testing"
	"time"
)

// Recebida é uma mensagem que o servidor aceitou.
type Recebida struct {
	De   string
	Para []string
	// Bruta é o conteúdo do DATA, cabeçalhos e corpo, como chegou.
	Bruta []byte
}

// Servidor é o SMTP de mentira. O zero value não serve: use Iniciar.
type Servidor struct {
	ouvinte net.Listener

	mu        sync.Mutex
	recebidas []Recebida
	// respostasDoRcpt são as respostas programadas para os próximos RCPT TO,
	// consumidas em ordem. Vazia, o RCPT é aceito.
	respostasDoRcpt []string
}

// Iniciar sobe o servidor numa porta livre de 127.0.0.1 e o derruba no fim do
// teste.
func Iniciar(t *testing.T) *Servidor {
	t.Helper()
	ouvinte, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("smtpfalso: %v", err)
	}
	s := &Servidor{ouvinte: ouvinte}
	go s.atender()
	t.Cleanup(func() { _ = ouvinte.Close() })
	return s
}

// Endereco devolve host e porta em que o servidor escuta.
func (s *Servidor) Endereco() (host string, porta int) {
	a := s.ouvinte.Addr().(*net.TCPAddr)
	return a.IP.String(), a.Port
}

// RecusarProximos faz os próximos `vezes` RCPT TO responderem `resposta` (ex.:
// "451 4.3.0 tente mais tarde" ou "550 5.1.1 <x@y>: usuário desconhecido").
func (s *Servidor) RecusarProximos(vezes int, resposta string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for range vezes {
		s.respostasDoRcpt = append(s.respostasDoRcpt, resposta)
	}
}

// Recebidas devolve uma cópia do que chegou até agora.
func (s *Servidor) Recebidas() []Recebida {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Recebida(nil), s.recebidas...)
}

// Esperar aguarda até `quantidade` mensagens terem chegado, e falha o teste se
// não chegarem no prazo.
func (s *Servidor) Esperar(t *testing.T, quantidade int, prazo time.Duration) []Recebida {
	t.Helper()
	fim := time.Now().Add(prazo)
	for {
		if r := s.Recebidas(); len(r) >= quantidade {
			return r
		}
		if time.Now().After(fim) {
			t.Fatalf("smtpfalso: esperava %d mensagem(ns), chegaram %d", quantidade, len(s.Recebidas()))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (s *Servidor) atender() {
	for {
		conexao, err := s.ouvinte.Accept()
		if err != nil {
			return
		}
		go s.sessao(conexao)
	}
}

// sessao conduz uma conversa SMTP. Implementa o mínimo que um cliente
// net/smtp usa: EHLO, MAIL, RCPT, DATA, RSET, NOOP e QUIT — sem STARTTLS e sem
// AUTH, porque os testes falam com ele em texto puro, em loopback.
func (s *Servidor) sessao(conexao net.Conn) {
	defer conexao.Close() //nolint:errcheck // fim da sessão de mentira: não há o que fazer com a falha
	_ = conexao.SetDeadline(time.Now().Add(30 * time.Second))
	leitor := bufio.NewReader(conexao)
	// Escrever para um cliente que já desligou não é falha do servidor de
	// mentira: o teste que o usa é que vai reclamar do que não recebeu.
	responder := func(linha string) { fmt.Fprintf(conexao, "%s\r\n", linha) } //nolint:errcheck

	responder("220 smtpfalso pronto")
	var atual Recebida
	for {
		linha, err := leitor.ReadString('\n')
		if err != nil {
			return
		}
		comando := strings.ToUpper(strings.TrimSpace(linha))
		switch {
		case strings.HasPrefix(comando, "EHLO"), strings.HasPrefix(comando, "HELO"):
			responder("250-smtpfalso")
			responder("250 8BITMIME")
		case strings.HasPrefix(comando, "MAIL FROM:"):
			atual = Recebida{De: endereco(linha)}
			responder("250 ok")
		case strings.HasPrefix(comando, "RCPT TO:"):
			if resposta, recusou := s.proximaRecusa(); recusou {
				responder(resposta)
				continue
			}
			atual.Para = append(atual.Para, endereco(linha))
			responder("250 ok")
		case comando == "DATA":
			responder("354 manda")
			var corpo bytes.Buffer
			for {
				l, err := leitor.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" || l == ".\n" {
					break
				}
				corpo.WriteString(strings.TrimPrefix(l, "."))
			}
			atual.Bruta = corpo.Bytes()
			s.mu.Lock()
			s.recebidas = append(s.recebidas, atual)
			s.mu.Unlock()
			responder("250 aceita")
		case comando == "RSET", comando == "NOOP":
			responder("250 ok")
		case comando == "QUIT":
			responder("221 tchau")
			return
		default:
			responder("502 não implementado")
		}
	}
}

func (s *Servidor) proximaRecusa() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.respostasDoRcpt) == 0 {
		return "", false
	}
	resposta := s.respostasDoRcpt[0]
	s.respostasDoRcpt = s.respostasDoRcpt[1:]
	return resposta, true
}

func endereco(linha string) string {
	_, depois, _ := strings.Cut(linha, ":")
	return strings.Trim(strings.TrimSpace(depois), "<>")
}

// Lida é uma mensagem já decodificada: cabeçalhos e as duas versões do corpo,
// como o leitor de email as mostraria.
type Lida struct {
	Cabecalho mail.Header
	Assunto   string
	Texto     string
	HTML      string
}

// Ler decodifica a mensagem: assunto em MIME word, partes multipart e
// quoted-printable. É o que o teste precisa para conferir o que a pessoa lê.
func (r Recebida) Ler(t *testing.T) Lida {
	t.Helper()
	m, err := mail.ReadMessage(bytes.NewReader(r.Bruta))
	if err != nil {
		t.Fatalf("smtpfalso: mensagem ilegível: %v", err)
	}
	var decodificador mime.WordDecoder
	assunto, err := decodificador.DecodeHeader(m.Header.Get("Subject"))
	if err != nil {
		t.Fatalf("smtpfalso: assunto ilegível: %v", err)
	}
	lida := Lida{Cabecalho: m.Header, Assunto: assunto}

	tipo, parametros, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(tipo, "multipart/") {
		corpo, _ := io.ReadAll(decodificar(m.Header.Get("Content-Transfer-Encoding"), m.Body))
		lida.Texto = string(corpo)
		return lida
	}
	partes := multipart.NewReader(m.Body, parametros["boundary"])
	for {
		parte, err := partes.NextPart()
		if err != nil {
			break
		}
		corpo, _ := io.ReadAll(decodificar(parte.Header.Get("Content-Transfer-Encoding"), parte))
		switch tipoDaParte, _, _ := mime.ParseMediaType(parte.Header.Get("Content-Type")); tipoDaParte {
		case "text/plain":
			lida.Texto = string(corpo)
		case "text/html":
			lida.HTML = string(corpo)
		}
	}
	return lida
}

// decodificar trata o quoted-printable explicitamente: o multipart.Reader da
// biblioteca padrão só o faz quando a parte declara o cabeçalho, e o leitor de
// fora do multipart não o faz nunca.
func decodificar(codificacao string, r io.Reader) io.Reader {
	if strings.EqualFold(strings.TrimSpace(codificacao), "quoted-printable") {
		return quotedprintable.NewReader(r)
	}
	return r
}
