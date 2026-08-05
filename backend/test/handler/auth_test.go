// Testes da borda HTTP da autenticação: códigos de status, corpo e — o que
// mais importa aqui — os atributos do cookie de sessão.
package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kanbango/internal/adapter/http/handler"
	"kanbango/internal/adapter/http/middleware"
	ucauth "kanbango/internal/usecase/auth"
	"kanbango/test/repository/memoria"

	"github.com/go-chi/chi/v5"
)

// montarAPI sobe o roteador de autenticação sobre repositórios em memória,
// espelhando o wiring do main.go. cookieSeguro liga o modo produção (cookie
// __Host- com Secure).
func montarAPI(cookieSeguro bool) (http.Handler, *memoria.Usuarios) {
	usuarios := memoria.NovosUsuarios()
	sessoes := memoria.NovasSessoes()
	hasher := &memoria.Hasher{}

	autenticacao := middleware.NovoAuth(ucauth.NovoValidarSessaoUseCase(sessoes), cookieSeguro)
	h := handler.NovoAuthHandler(
		ucauth.NovoCadastrarUseCase(usuarios, sessoes, hasher),
		ucauth.NovoLoginUseCase(usuarios, sessoes, hasher),
		ucauth.NovoLogoutUseCase(sessoes),
		ucauth.NovoPerfilUseCase(usuarios),
		cookieSeguro,
		nil, // teto por conta desligado: quem o exercita é o teste de rate limit
		func(r *http.Request) (ucauth.Identidade, bool) {
			return middleware.IdentidadeDoContexto(r.Context())
		},
	)

	r := chi.NewRouter()
	r.Route("/auth", func(r chi.Router) {
		r.Post("/cadastro", h.Cadastrar)
		r.Post("/login", h.Login)
		r.Post("/logout", h.Logout)
		r.Group(func(r chi.Router) {
			r.Use(autenticacao.Autenticar)
			r.Get("/me", h.Me)
		})
	})
	return r, usuarios
}

func chamar(api http.Handler, metodo, caminho, corpo string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(metodo, caminho, strings.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	return rec
}

func cookieDeSessao(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if strings.Contains(c.Name, "kanbango_session") {
			return c
		}
	}
	t.Fatal("a resposta não trouxe cookie de sessão")
	return nil
}

const cadastroValido = `{"nome":"Ana","email":"ana@exemplo.com","senha":"senha-boa-123"}`

func TestCadastroResponde201ComCookieDeSessao(t *testing.T) {
	api, _ := montarAPI(false)

	rec := chamar(api, http.MethodPost, "/auth/cadastro", cadastroValido)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, esperado 201: %s", rec.Code, rec.Body)
	}

	var corpo map[string]string
	json.Unmarshal(rec.Body.Bytes(), &corpo)
	if corpo["email"] != "ana@exemplo.com" || corpo["nome"] != "Ana" || corpo["id"] == "" {
		t.Errorf("corpo = %v", corpo)
	}
	cookieDeSessao(t, rec)
}

// O token de sessão não pode aparecer no corpo: se o JavaScript conseguir
// lê-lo, o HttpOnly do cookie deixa de valer para alguma coisa.
func TestCadastroNaoDevolveOTokenNoCorpo(t *testing.T) {
	api, _ := montarAPI(false)

	rec := chamar(api, http.MethodPost, "/auth/cadastro", cadastroValido)
	cookie := cookieDeSessao(t, rec)

	if strings.Contains(rec.Body.String(), cookie.Value) {
		t.Error("o token de sessão vazou no corpo da resposta")
	}
	var corpo map[string]any
	json.Unmarshal(rec.Body.Bytes(), &corpo)
	for _, proibido := range []string{"token", "senha", "senhaHash"} {
		if _, existe := corpo[proibido]; existe {
			t.Errorf("o corpo não pode conter %q: %v", proibido, corpo)
		}
	}
}

func TestCookieDeSessaoEHttpOnlyESameSiteLax(t *testing.T) {
	api, _ := montarAPI(false)

	cookie := cookieDeSessao(t, chamar(api, http.MethodPost, "/auth/cadastro", cadastroValido))

	if !cookie.HttpOnly {
		t.Error("o cookie precisa ser HttpOnly, senão um XSS rouba a sessão")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, esperado Lax (defesa de CSRF)", cookie.SameSite)
	}
	if cookie.Path != "/" {
		t.Errorf("Path = %q, esperado /", cookie.Path)
	}
	if cookie.Secure {
		t.Error("em desenvolvimento o cookie não pode ser Secure: o navegador não o entrega em http://localhost")
	}
	if cookie.Name != "kanbango_session" {
		t.Errorf("nome = %q, esperado kanbango_session fora de produção", cookie.Name)
	}
}

// Em produção o cookie leva o prefixo __Host-, que o navegador só aceita com
// Secure, Path=/ e sem Domain — o que amarra a sessão a esta origem exata.
func TestEmProducaoOCookieUsaPrefixoHostESecure(t *testing.T) {
	api, _ := montarAPI(true)

	cookie := cookieDeSessao(t, chamar(api, http.MethodPost, "/auth/cadastro", cadastroValido))

	if cookie.Name != "__Host-kanbango_session" {
		t.Errorf("nome = %q, esperado com prefixo __Host-", cookie.Name)
	}
	if !cookie.Secure {
		t.Error("o prefixo __Host- exige Secure — sem ele o navegador ignora o cookie inteiro")
	}
	if cookie.Domain != "" {
		t.Errorf("Domain = %q, esperado vazio (exigência do __Host-)", cookie.Domain)
	}
}

func TestCadastroComEmailRepetidoResponde409(t *testing.T) {
	api, _ := montarAPI(false)
	chamar(api, http.MethodPost, "/auth/cadastro", cadastroValido)

	rec := chamar(api, http.MethodPost, "/auth/cadastro", cadastroValido)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, esperado 409", rec.Code)
	}
}

func TestCadastroComSenhaCurtaResponde400ComMotivo(t *testing.T) {
	api, _ := montarAPI(false)

	rec := chamar(api, http.MethodPost, "/auth/cadastro",
		`{"nome":"Ana","email":"ana@exemplo.com","senha":"curta"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "8") {
		t.Errorf("a mensagem devia dizer qual é o mínimo: %s", rec.Body)
	}
}

func TestCadastroComCorpoQuebradoResponde400(t *testing.T) {
	api, _ := montarAPI(false)

	casos := map[string]string{
		"json inválido":   `{"nome":`,
		"email sem forma": `{"nome":"Ana","email":"nao-e-email","senha":"senha-boa-123"}`,
		"sem nome":        `{"email":"ana@exemplo.com","senha":"senha-boa-123"}`,
	}

	for nome, corpo := range casos {
		t.Run(nome, func(t *testing.T) {
			if rec := chamar(api, http.MethodPost, "/auth/cadastro", corpo); rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, esperado 400", rec.Code)
			}
		})
	}
}

// Teclado de celular e copiar-e-colar produzem espaço nas pontas o tempo todo,
// e o espaço é invisível no campo — quem o recebe de volta como "email
// inválido" não tem o que corrigir na tela. A validação de formato roda antes
// de o domínio normalizar, então o corte precisa acontecer na decodificação
// (dto.EmailEntrada).
func TestEmailComEspacosNasPontasEAceito(t *testing.T) {
	api, _ := montarAPI(false)

	rec := chamar(api, http.MethodPost, "/auth/cadastro",
		`{"nome":"Ana","email":"  Ana@Exemplo.COM  ","senha":"senha-boa-123"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("cadastro: status = %d, esperado 201: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"email":"ana@exemplo.com"`) {
		t.Errorf("o email devia ter sido normalizado: %s", rec.Body)
	}

	login := chamar(api, http.MethodPost, "/auth/login",
		`{"email":" ANA@exemplo.com ","senha":"senha-boa-123"}`)
	if login.Code != http.StatusOK {
		t.Errorf("login: status = %d, esperado 200: %s", login.Code, login.Body)
	}
}

func TestLoginComSenhaErradaResponde401(t *testing.T) {
	api, _ := montarAPI(false)
	chamar(api, http.MethodPost, "/auth/cadastro", cadastroValido)

	rec := chamar(api, http.MethodPost, "/auth/login",
		`{"email":"ana@exemplo.com","senha":"errada-mas-longa"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, esperado 401", rec.Code)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Error("login recusado não pode emitir cookie")
	}
}

// A resposta para email inexistente tem de ser idêntica à de senha errada —
// status e corpo — senão ela vira um oráculo de quais emails têm conta.
func TestLoginNaoRevelaSeOEmailExiste(t *testing.T) {
	api, _ := montarAPI(false)
	chamar(api, http.MethodPost, "/auth/cadastro", cadastroValido)

	senhaErrada := chamar(api, http.MethodPost, "/auth/login",
		`{"email":"ana@exemplo.com","senha":"errada-mas-longa"}`)
	inexistente := chamar(api, http.MethodPost, "/auth/login",
		`{"email":"ninguem@exemplo.com","senha":"errada-mas-longa"}`)

	if senhaErrada.Code != inexistente.Code {
		t.Errorf("status diferentes: %d vs %d", senhaErrada.Code, inexistente.Code)
	}
	if senhaErrada.Body.String() != inexistente.Body.String() {
		t.Errorf("corpos diferentes: %q vs %q", senhaErrada.Body, inexistente.Body)
	}
}

func TestMeSemCookieResponde401(t *testing.T) {
	api, _ := montarAPI(false)

	if rec := chamar(api, http.MethodGet, "/auth/me", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, esperado 401", rec.Code)
	}
}

func TestMeComCookieInventadoResponde401(t *testing.T) {
	api, _ := montarAPI(false)

	rec := chamar(api, http.MethodGet, "/auth/me", "",
		&http.Cookie{Name: "kanbango_session", Value: "token-inventado"})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, esperado 401", rec.Code)
	}
}

func TestMeComSessaoValidaDevolveAConta(t *testing.T) {
	api, _ := montarAPI(false)
	cookie := cookieDeSessao(t, chamar(api, http.MethodPost, "/auth/cadastro", cadastroValido))

	rec := chamar(api, http.MethodGet, "/auth/me", "", cookie)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200: %s", rec.Code, rec.Body)
	}
	var corpo map[string]string
	json.Unmarshal(rec.Body.Bytes(), &corpo)
	if corpo["email"] != "ana@exemplo.com" {
		t.Errorf("corpo = %v", corpo)
	}
}

func TestLogoutApagaOCookieEInvalidaASessao(t *testing.T) {
	api, _ := montarAPI(false)
	cookie := cookieDeSessao(t, chamar(api, http.MethodPost, "/auth/cadastro", cadastroValido))

	rec := chamar(api, http.MethodPost, "/auth/logout", "", cookie)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, esperado 204", rec.Code)
	}
	apagado := cookieDeSessao(t, rec)
	if apagado.Value != "" || apagado.MaxAge >= 0 {
		t.Errorf("o cookie devia vir vazio e com Max-Age negativo: valor=%q maxAge=%d", apagado.Value, apagado.MaxAge)
	}

	// o token antigo não pode continuar autenticando
	if rec := chamar(api, http.MethodGet, "/auth/me", "", cookie); rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d depois do logout, esperado 401", rec.Code)
	}
}

func TestLogoutSemCookieResponde204(t *testing.T) {
	api, _ := montarAPI(false)

	if rec := chamar(api, http.MethodPost, "/auth/logout", ""); rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, esperado 204", rec.Code)
	}
}
