// Testes do adaptador de SSE das notificações — o protocolo, sem navegador e
// sem stack no ar, como os do WebSocket.
//
// O handler roda sobre httptest e o cliente lê a resposta linha a linha, que é
// exatamente o que o EventSource do navegador faz. A contagem vem de um dublê:
// o que ela conta, contra o banco de verdade, está em
// test/repository/notificacao_test.go e no teste do servidor inteiro.
//
// Como isto quebra — cada teste cobre um item:
//
//	P1 sem sessão, o stream abre mesmo assim;
//	P2 a resposta sai sem text/event-stream, sem no-store ou com buffer na
//	   borda — e o navegador recebe tudo de uma vez, ou nunca;
//	P3 a tela só descobre a contagem no primeiro toque, e abre sem número;
//	P4 o toque da campainha não chega à tela aberta;
//	P5 a conexão fica muda tempo demais e a Cloudflare (100 s) ou o nginx a
//	   derrubam como ociosa;
//	P6 o logout não derruba o stream, e a aba continua recebendo as
//	   notificações de uma sessão encerrada; ou derruba sem dizer por quê, e o
//	   navegador reconecta para sempre;
//	P7 o que muda só com o relógio (o convite que venceu) nunca chega à tela,
//	   porque ninguém toca a campainha;
//	P8 uma conta abre streams sem limite e consome a capacidade de todos;
//	P9 o prazo de dez segundos das requisições comuns derruba o stream;
//	P10 o desligamento do servidor espera o prazo inteiro por streams que não
//	    terminam sozinhos.
package realtime_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	httpmiddleware "stacktrack/internal/adapter/http/middleware"
	"stacktrack/internal/adapter/http/sse"
	"stacktrack/internal/adapter/realtime/campainha"
)

// --- montagem ---------------------------------------------------------------

type cenarioSSE struct {
	servidor  *httptest.Server
	campainha *campainha.Campainha
	handler   *sse.Handler
	// naoLidas é o que o contador devolve; o teste muda no meio.
	naoLidas    atomic.Int64
	sessaoOk    atomic.Bool
	autenticado bool
	usuario     string
	porConta    int
	prazoHTTP   time.Duration
}

func montarSSE(t *testing.T, ajustar func(*cenarioSSE)) *cenarioSSE {
	t.Helper()
	c := &cenarioSSE{campainha: campainha.Nova(), autenticado: true, usuario: "bruno"}
	c.sessaoOk.Store(true)
	if ajustar != nil {
		ajustar(c)
	}

	c.handler = sse.NovoHandler(
		c.campainha,
		func(context.Context, string) (int, error) { return int(c.naoLidas.Load()), nil },
		func(context.Context) (string, bool) { return c.usuario, c.autenticado },
		func(context.Context, string) bool { return c.sessaoOk.Load() },
		"sessao",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).ComIntervalos(80*time.Millisecond, 60*time.Millisecond)
	if c.porConta > 0 {
		c.handler.ComLimites(c.porConta, 100, 0, time.Minute)
	}

	var endpoint http.Handler = http.HandlerFunc(c.handler.Acompanhar)
	if c.prazoHTTP > 0 {
		endpoint = httpmiddleware.Prazo(c.prazoHTTP, c.prazoHTTP)(endpoint)
	}
	c.servidor = httptest.NewServer(endpoint)
	t.Cleanup(func() {
		c.campainha.Fechar()
		c.servidor.Close()
	})
	return c
}

// mensagemSSE é um bloco do stream: o evento e os dados, ou um comentário.
type mensagemSSE struct {
	evento     string
	dados      string
	retry      string
	comentario bool
}

type streamSSE struct {
	resposta  *http.Response
	mensagens chan mensagemSSE
}

// abrir faz o GET e passa a ler o stream numa goroutine, bloco a bloco.
func (c *cenarioSSE) abrir(t *testing.T) *streamSSE {
	t.Helper()
	resposta, err := http.Get(c.servidor.URL)
	if err != nil {
		t.Fatalf("GET do stream: %v", err)
	}
	t.Cleanup(func() { resposta.Body.Close() })
	s := &streamSSE{resposta: resposta, mensagens: make(chan mensagemSSE, 64)}
	if resposta.StatusCode != http.StatusOK {
		return s
	}
	go func() {
		defer close(s.mensagens)
		leitor := bufio.NewReader(resposta.Body)
		var atual mensagemSSE
		for {
			linha, err := leitor.ReadString('\n')
			if err != nil {
				return
			}
			linha = strings.TrimRight(linha, "\r\n")
			switch {
			case linha == "":
				s.mensagens <- atual
				atual = mensagemSSE{}
			case strings.HasPrefix(linha, ":"):
				atual.comentario = true
			case strings.HasPrefix(linha, "event: "):
				atual.evento = strings.TrimPrefix(linha, "event: ")
			case strings.HasPrefix(linha, "data: "):
				atual.dados = strings.TrimPrefix(linha, "data: ")
			case strings.HasPrefix(linha, "retry: "):
				atual.retry = strings.TrimPrefix(linha, "retry: ")
			}
		}
	}()
	return s
}

// proximoEvento devolve o próximo bloco com evento, pulando comentários e o
// `retry:` do começo.
func (s *streamSSE) proximoEvento(t *testing.T) mensagemSSE {
	t.Helper()
	prazo := time.After(2 * time.Second)
	for {
		select {
		case m, aberto := <-s.mensagens:
			if !aberto {
				t.Fatal("o stream terminou antes do evento")
			}
			if m.evento != "" {
				return m
			}
		case <-prazo:
			t.Fatal("nenhum evento em 2 s")
		}
	}
}

func naoLidasDe(t *testing.T, m mensagemSSE) int {
	t.Helper()
	if m.evento != "notificacoes" {
		t.Fatalf("evento %q, esperado notificacoes", m.evento)
	}
	var corpo struct {
		Versao   int `json:"versao"`
		NaoLidas int `json:"naoLidas"`
	}
	if err := json.Unmarshal([]byte(m.dados), &corpo); err != nil {
		t.Fatalf("dados ilegíveis %q: %v", m.dados, err)
	}
	if corpo.Versao != 1 {
		t.Errorf("versão do envelope = %d, esperado 1", corpo.Versao)
	}
	return corpo.NaoLidas
}

// --- os casos ---------------------------------------------------------------

func TestSSEP1RecusaQuemNaoEstaAutenticado(t *testing.T) {
	c := montarSSE(t, func(c *cenarioSSE) { c.autenticado = false })
	s := c.abrir(t)
	if s.resposta.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, esperado 401", s.resposta.StatusCode)
	}
}

func TestSSEP2P3AbreComOsCabecalhosCertosEJaComAContagem(t *testing.T) {
	c := montarSSE(t, nil)
	c.naoLidas.Store(3)
	s := c.abrir(t)

	if s.resposta.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", s.resposta.StatusCode)
	}
	cabecalhos := map[string]string{
		"Content-Type":      "text/event-stream",
		"Cache-Control":     "no-store",
		"X-Accel-Buffering": "no",
	}
	for nome, espera := range cabecalhos {
		if got := s.resposta.Header.Get(nome); !strings.HasPrefix(got, espera) {
			t.Errorf("P2: %s = %q, esperado %q", nome, got, espera)
		}
	}

	primeira := <-s.mensagens
	if primeira.retry == "" {
		t.Error("P2: o stream não começa dizendo ao navegador quanto esperar para reconectar (retry:)")
	}
	if got := naoLidasDe(t, s.proximoEvento(t)); got != 3 {
		t.Errorf("P3: contagem inicial = %d, esperado 3", got)
	}
}

func TestSSEP4OToqueDaCampainhaChegaComAContagemNova(t *testing.T) {
	c := montarSSE(t, nil)
	s := c.abrir(t)
	if got := naoLidasDe(t, s.proximoEvento(t)); got != 0 {
		t.Fatalf("contagem inicial = %d", got)
	}

	c.naoLidas.Store(1)
	c.campainha.Tocar("outra-pessoa")
	c.campainha.Tocar("bruno")
	if got := naoLidasDe(t, s.proximoEvento(t)); got != 1 {
		t.Errorf("P4: depois do toque = %d, esperado 1", got)
	}
}

func TestSSEP5MandaBatimentoParaNaoParecerOcioso(t *testing.T) {
	c := montarSSE(t, nil)
	s := c.abrir(t)
	prazo := time.After(time.Second)
	for {
		select {
		case m := <-s.mensagens:
			if m.comentario && m.evento == "" {
				return
			}
		case <-prazo:
			t.Fatal("P5: nenhum batimento em 1 s com intervalo de 80 ms")
		}
	}
}

func TestSSEP6OLogoutEncerraOStreamDizendoPorQue(t *testing.T) {
	c := montarSSE(t, nil)
	s := c.abrir(t)
	s.proximoEvento(t)

	c.sessaoOk.Store(false)
	m := s.proximoEvento(t)
	if m.evento != "encerrada" || !strings.Contains(m.dados, `"sessao"`) {
		t.Fatalf("P6: veio %q %s, esperado encerrada por sessão", m.evento, m.dados)
	}
	// Um batimento ainda pode estar no caminho; o que importa é que termina.
	prazo := time.After(time.Second)
	for {
		select {
		case _, aberto := <-s.mensagens:
			if !aberto {
				return
			}
		case <-prazo:
			t.Fatal("P6: o stream continuou aberto depois de encerrado")
		}
	}
}

func TestSSEP7AContagemQueMudaSemToqueChegaNaRevalidacao(t *testing.T) {
	c := montarSSE(t, nil)
	c.naoLidas.Store(2)
	s := c.abrir(t)
	s.proximoEvento(t)

	// O convite venceu: ninguém toca a campainha, e a contagem cai.
	c.naoLidas.Store(1)
	if got := naoLidasDe(t, s.proximoEvento(t)); got != 1 {
		t.Errorf("P7: contagem = %d, esperado 1", got)
	}
}

func TestSSEP8UmaContaNaoAbreStreamsSemLimite(t *testing.T) {
	c := montarSSE(t, func(c *cenarioSSE) { c.porConta = 2 })
	c.abrir(t).proximoEvento(t)
	c.abrir(t).proximoEvento(t)

	terceiro := c.abrir(t)
	if terceiro.resposta.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("P8: terceiro stream com status %d, esperado 503", terceiro.resposta.StatusCode)
	}
	if terceiro.resposta.Header.Get("Retry-After") == "" {
		t.Error("P8: o 503 não diz quando tentar de novo")
	}
}

func TestSSEP9OPrazoDasRequisicoesComunsNaoDerrubaOStream(t *testing.T) {
	c := montarSSE(t, func(c *cenarioSSE) { c.prazoHTTP = 100 * time.Millisecond })
	s := c.abrir(t)
	s.proximoEvento(t)

	time.Sleep(400 * time.Millisecond)
	c.naoLidas.Store(5)
	c.campainha.Tocar("bruno")
	if got := naoLidasDe(t, s.proximoEvento(t)); got != 5 {
		t.Errorf("P9: depois do prazo das requisições comuns = %d, esperado 5", got)
	}
}

func TestSSEP10DesligarACampainhaEncerraOsStreams(t *testing.T) {
	c := montarSSE(t, nil)
	s := c.abrir(t)
	s.proximoEvento(t)

	c.campainha.Fechar()
	prazo := time.After(time.Second)
	for {
		select {
		case _, aberto := <-s.mensagens:
			if !aberto {
				return
			}
		case <-prazo:
			t.Fatal("P10: o stream não terminou com a campainha desligada")
		}
	}
}
