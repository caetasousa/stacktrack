// A validação na BORDA: o que a API recusa antes de o caso de uso existir.
package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// corpoCom monta uma requisição de cadastro com o Content-Type informado.
func corpoCom(t *testing.T, api http.Handler, tipo, corpo string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/auth/cadastro", strings.NewReader(corpo))
	if tipo == "" {
		req.Header.Del("Content-Type")
	} else {
		req.Header.Set("Content-Type", tipo)
	}
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	return rec.Code
}

// Content-Type não-JSON é recusado com 415.
//
// Não é preciosismo: `text/plain` e `multipart/form-data` são "requisições
// simples" para o navegador, e um formulário HTML em outro site consegue
// POSTá-las para a API SEM preflight de CORS. Exigir application/json força o
// preflight, e o CORS volta a ser a primeira defesa de CSRF em vez de o
// SameSite=Lax do cookie ser a única.
func TestCorpoComContentTypeErradoResponde415(t *testing.T) {
	api, _ := montarAPI(false)
	corpo := `{"nome":"Ana","email":"ana@exemplo.com","senha":"senha-boa-de-teste-123"}`

	for _, tipo := range []string{"text/plain", "multipart/form-data; boundary=x", "application/xml"} {
		if codigo := corpoCom(t, api, tipo, corpo); codigo != http.StatusUnsupportedMediaType {
			t.Errorf("Content-Type %q: status = %d, esperado 415", tipo, codigo)
		}
	}
}

func TestCorpoComContentTypeJSONEhAceito(t *testing.T) {
	// Um email por caso: o segundo cadastro com o mesmo email daria 409 e
	// esconderia o que está sendo testado.
	for i, tipo := range []string{"application/json", "application/json; charset=utf-8"} {
		api, _ := montarAPI(false)
		corpo := `{"nome":"Ana","email":"ana` + string(rune('a'+i)) + `@exemplo.com","senha":"senha-boa-de-teste-123"}`
		if codigo := corpoCom(t, api, tipo, corpo); codigo != http.StatusCreated {
			t.Errorf("Content-Type %q: status = %d, esperado 201", tipo, codigo)
		}
	}
}

// Campo desconhecido é recusado, e a mensagem DIZ QUAL.
//
// Aceitá-lo em silêncio transforma um erro de digitação do cliente em "o
// servidor ignorou o que mandei" — diagnosticado por eliminação, muito depois.
func TestCampoDesconhecidoNoCorpoResponde400ComONome(t *testing.T) {
	api, _ := montarAPI(false)

	rec := chamar(api, http.MethodPost, "/auth/cadastro",
		`{"nome":"Ana","email":"ana@exemplo.com","senha":"senha-boa-de-teste-123","admin":true}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "admin") {
		t.Errorf("a mensagem devia citar o campo desconhecido: %s", rec.Body)
	}
}

// Lixo depois do objeto é recusado.
//
// Antes, `{"a":1}{"b":2}` era lido como o primeiro documento e o resto sumia:
// dois pedidos entravam como um, e o segundo desaparecia sem erro nenhum.
func TestVariosDocumentosNoMesmoCorpoResponde400(t *testing.T) {
	api, _ := montarAPI(false)

	casos := []string{
		`{"nome":"Ana","email":"a@e.com","senha":"senha-boa-de-teste-123"}{"nome":"Bob"}`,
		`{"nome":"Ana","email":"a@e.com","senha":"senha-boa-de-teste-123"} lixo`,
		`{"nome":"Ana","email":"a@e.com","senha":"senha-boa-de-teste-123"}[]`,
	}
	for _, corpo := range casos {
		rec := chamar(api, http.MethodPost, "/auth/cadastro", corpo)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("corpo %q: status = %d, esperado 400", corpo, rec.Code)
		}
	}
}

// A mensagem de erro de entrada NÃO devolve o corpo enviado.
//
// O corpo de um cadastro carrega a senha em texto puro; ecoá-lo numa mensagem
// de erro a colocaria no console do navegador, no log de quem integra e em
// qualquer relatório de erro do cliente.
func TestErroDeEntradaNaoEcoaOCorpoEnviado(t *testing.T) {
	api, _ := montarAPI(false)

	rec := chamar(api, http.MethodPost, "/auth/cadastro",
		`{"nome":"Ana","email":"a@e.com","senha":"minha-senha-secreta-123","surpresa":1}`)

	if strings.Contains(rec.Body.String(), "minha-senha-secreta-123") {
		t.Errorf("a senha enviada vazou na resposta de erro: %s", rec.Body)
	}
}
