//go:build tempo_real

// O teste que define a fase 5: duas conexões no mesmo quadro, uma age e a
// outra vê — sem F5.
//
// Fica atrás de build tag porque precisa da API NO AR: ele exercita o handshake
// de verdade, com cookie de sessão real e checagem de origem. Sem a tag, o
// `go test ./...` de quem não subiu a stack passaria a falhar.
//
//	make run                      # noutro terminal
//	make test-tempo-real
//
// É o equivalente sem navegador do teste de duas abas do Playwright, que chega
// junto com os testes de ponta a ponta da interface.
package realtime_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func baseAPI() string {
	if v := os.Getenv("API_URL"); v != "" {
		return v
	}
	return "http://localhost:8080"
}

func origemAceita() string {
	if v := os.Getenv("ORIGEM"); v != "" {
		return v
	}
	return "http://localhost:5173"
}

// pessoa é uma conta autenticada, com o cookie de sessão pronto para uso.
type pessoa struct {
	email  string
	cookie string
}

func criarPessoa(t *testing.T, nome string) pessoa {
	t.Helper()
	email := fmt.Sprintf("%s-%d@teste.dev", nome, time.Now().UnixNano())
	senha := "Senha!12345"

	corpo := fmt.Sprintf(`{"nome":%q,"email":%q,"senha":%q}`, nome, email, senha)
	if _, _, err := pedir(t, "POST", "/auth/cadastro", "", corpo); err != nil {
		t.Fatalf("cadastro de %s: %v", nome, err)
	}

	resp, err := http.Post(baseAPI()+"/auth/login", "application/json",
		bytes.NewReader([]byte(fmt.Sprintf(`{"email":%q,"senha":%q}`, email, senha))))
	if err != nil {
		t.Fatalf("login de %s: %v", nome, err)
	}
	defer resp.Body.Close()

	for _, c := range resp.Cookies() {
		if c.Name == "stacktrack_session" || c.Name == "__Host-stacktrack_session" {
			return pessoa{email: email, cookie: c.Name + "=" + c.Value}
		}
	}
	// 429 não é defeito do produto: é o teto por IP do rate limiter fazendo o
	// trabalho dele, e a suíte inteira divide o mesmo IP. Vira skip com a causa
	// escrita, porque um FAIL vermelho aqui manda procurar bug onde não há.
	if resp.StatusCode == http.StatusTooManyRequests {
		t.Skipf("teto por IP do rate limiter atingido ao criar %s — espere um minuto e rode de novo", nome)
	}
	corpoResp, _ := io.ReadAll(resp.Body)
	t.Fatalf("login de %s não devolveu cookie: status %d — %s", nome, resp.StatusCode, corpoResp)
	return pessoa{}
}

// pedir faz uma requisição autenticada e devolve status e corpo.
func pedir(t *testing.T, metodo, caminho, cookie, corpo string) (int, []byte, error) {
	t.Helper()
	var leitor io.Reader
	if corpo != "" {
		leitor = bytes.NewReader([]byte(corpo))
	}
	req, err := http.NewRequest(metodo, baseAPI()+caminho, leitor)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	dados, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, dados, nil
}

// O cenário é montado UMA vez para a suíte inteira.
//
// Não é só economia: cadastro e login têm teto por IP, e um cenário por teste
// derrubava a suíte no terceiro caso com 429 — um erro que parecia bug do
// WebSocket e era o rate limiter fazendo o trabalho dele.
var (
	umaVez  sync.Once
	cenario struct {
		ana, bruno pessoa
		boardID    string
	}
)

// quadroCompartilhado devolve um quadro em que ana é dona e bruno é editor.
func quadroCompartilhado(t *testing.T) (ana, bruno pessoa, boardID string) {
	t.Helper()
	umaVez.Do(func() { montarCenario(t) })
	if cenario.boardID == "" {
		t.Skip("o cenário compartilhado não pôde ser montado")
	}
	return cenario.ana, cenario.bruno, cenario.boardID
}

func montarCenario(t *testing.T) {
	t.Helper()
	ana := criarPessoa(t, "ana")
	bruno := criarPessoa(t, "bruno")

	status, corpo, err := pedir(t, "POST", "/boards", ana.cookie, `{"titulo":"Tempo real"}`)
	if err != nil || status != http.StatusCreated {
		t.Fatalf("criar quadro: %v (status %d)", err, status)
	}
	var quadro struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(corpo, &quadro); err != nil {
		t.Fatalf("resposta do quadro: %v", err)
	}

	status, _, err = pedir(t, "POST", "/boards/"+quadro.ID+"/membros", ana.cookie,
		fmt.Sprintf(`{"email":%q,"papel":"editor"}`, bruno.email))
	if err != nil || status != http.StatusCreated {
		t.Fatalf("convidar bruno: %v (status %d)", err, status)
	}

	cenario.ana, cenario.bruno, cenario.boardID = ana, bruno, quadro.ID
}

// intrusoCompartilhado é a conta que não participa de quadro nenhum.
var (
	umaVezIntruso sync.Once
	intruso       pessoa
)

func quemNaoParticipa(t *testing.T) pessoa {
	t.Helper()
	umaVezIntruso.Do(func() { intruso = criarPessoa(t, "intruso") })
	return intruso
}

func abrirSocket(t *testing.T, ctx context.Context, cookie, boardID, origem string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	return websocket.Dial(ctx, wsURL(boardID), &websocket.DialOptions{
		HTTPHeader: http.Header{"Cookie": {cookie}, "Origin": {origem}},
	})
}

func wsURL(boardID string) string {
	base := baseAPI()
	if len(base) > 4 && base[:5] == "https" {
		return "wss" + base[5:] + "/ws?board=" + boardID
	}
	return "ws" + base[4:] + "/ws?board=" + boardID
}

// ---------------------------------------------------------------------------

func TestUmAgeEOOutroVe(t *testing.T) {
	ana, bruno, boardID := quadroCompartilhado(t)

	ctx, cancelar := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelar()

	conexao, _, err := abrirSocket(t, ctx, bruno.cookie, boardID, origemAceita())
	if err != nil {
		t.Fatalf("bruno não conseguiu abrir o WebSocket: %v", err)
	}
	defer conexao.CloseNow()

	// A assinatura acontece depois do handshake; sem esta folga, o evento pode
	// ser publicado numa sala que ainda está vazia.
	time.Sleep(300 * time.Millisecond)

	status, _, err := pedir(t, "POST", "/boards/"+boardID+"/colunas", ana.cookie,
		`{"titulo":"Feito pela ana","cor":"verde"}`)
	if err != nil || status != http.StatusCreated {
		t.Fatalf("ana não criou a coluna: %v (status %d)", err, status)
	}

	_, dados, err := conexao.Read(ctx)
	if err != nil {
		t.Fatalf("bruno não recebeu nada: %v", err)
	}

	var m struct {
		Tipo    string `json:"tipo"`
		BoardID string `json:"boardId"`
		Dados   struct {
			Titulo string
			Cor    string
		} `json:"dados"`
	}
	if err := json.Unmarshal(dados, &m); err != nil {
		t.Fatalf("evento ilegível: %v — %s", err, dados)
	}
	if m.Tipo != "coluna.criada" {
		t.Errorf("tipo = %q, esperado coluna.criada", m.Tipo)
	}
	if m.BoardID != boardID {
		t.Errorf("evento de outro quadro: %q", m.BoardID)
	}
	if m.Dados.Titulo != "Feito pela ana" || m.Dados.Cor != "verde" {
		t.Errorf("o payload não chegou inteiro: %+v", m.Dados)
	}
}

// Quem age não recebe o próprio eco: a tela dele já mudou quando ele agiu, e
// reaplicar causaria um solavanco no que ele acabou de fazer.
func TestOAutorNaoRecebeOProprioEco(t *testing.T) {
	ana, _, boardID := quadroCompartilhado(t)

	ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()

	conexao, _, err := abrirSocket(t, ctx, ana.cookie, boardID, origemAceita())
	if err != nil {
		t.Fatalf("ana não conectou: %v", err)
	}
	defer conexao.CloseNow()
	time.Sleep(300 * time.Millisecond)

	if _, _, err := pedir(t, "POST", "/boards/"+boardID+"/colunas", ana.cookie, `{"titulo":"So da ana"}`); err != nil {
		t.Fatalf("ana não criou: %v", err)
	}

	curto, cancelarCurto := context.WithTimeout(ctx, 2*time.Second)
	defer cancelarCurto()
	if _, dados, err := conexao.Read(curto); err == nil {
		t.Errorf("ana recebeu o próprio eco: %s", dados)
	}
}

// A sala é fronteira de acesso: quem não participa do quadro nem chega a
// assinar. E o 404 — em vez de 403 — é o mesmo cuidado do resto da API: um 403
// confirmaria que o quadro existe.
func TestQuemNaoParticipaRecebe404(t *testing.T) {
	_, _, boardID := quadroCompartilhado(t)
	intruso := quemNaoParticipa(t)

	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()

	_, resp, err := abrirSocket(t, ctx, intruso.cookie, boardID, origemAceita())
	if err == nil {
		t.Fatal("um não-membro conseguiu assinar o quadro")
	}
	if resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %v, esperado 404", resp)
	}
}

// Cross-Site WebSocket Hijacking: WebSocket NÃO obedece CORS, então sem a
// checagem de origem qualquer site que a vítima visitasse abriria uma conexão
// autenticada com o cookie dela e leria o quadro inteiro em tempo real.
func TestOrigemDeOutroSiteERecusada(t *testing.T) {
	ana, _, boardID := quadroCompartilhado(t)

	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()

	_, resp, err := abrirSocket(t, ctx, ana.cookie, boardID, "https://malicioso.example")
	if err == nil {
		t.Fatal("aceitou conexão vinda de outra origem")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %v, esperado 403", resp)
	}
}

func TestSemSessaoNaoConecta(t *testing.T) {
	_, _, boardID := quadroCompartilhado(t)

	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()

	_, resp, err := abrirSocket(t, ctx, "", boardID, origemAceita())
	if err == nil {
		t.Fatal("aceitou conexão sem sessão")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %v, esperado 401", resp)
	}
}
