// O IP do cliente é a chave dos tetos por IP. Se o cliente puder escolhê-lo,
// os tetos deixam de existir: basta um cabeçalho novo por requisição para cada
// tentativa cair num balde vazio.
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"stacktrack/internal/adapter/http/middleware"
)

// ecoDoIP devolve o que o middleware deixou em r.RemoteAddr.
func ecoDoIP(confiaveis []netip.Prefix, remoteAddr string, cabecalhos map[string]string) (string, map[string]string) {
	visto := ""
	sobraram := map[string]string{}
	final := middleware.IPReal(confiaveis)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		visto = r.RemoteAddr
		for _, nome := range []string{"X-Real-IP", "X-Forwarded-For"} {
			sobraram[nome] = r.Header.Get(nome)
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	for nome, valor := range cabecalhos {
		req.Header.Set(nome, valor)
	}
	final.ServeHTTP(httptest.NewRecorder(), req)
	return visto, sobraram
}

func faixas(t *testing.T, brutas ...string) []netip.Prefix {
	t.Helper()
	var lista []netip.Prefix
	for _, b := range brutas {
		p, err := netip.ParsePrefix(b)
		if err != nil {
			t.Fatalf("faixa %q inválida: %v", b, err)
		}
		lista = append(lista, p)
	}
	return lista
}

// O caso que importa: a requisição NÃO veio de um proxy confiável, e mesmo
// assim traz o cabeçalho. Antes, ele era obedecido.
func TestCabecalhoDeIPVindoDeForaEhIgnorado(t *testing.T) {
	visto, sobraram := ecoDoIP(
		faixas(t, "10.0.0.0/8"),
		"203.0.113.9:5555",
		map[string]string{"X-Real-IP": "1.2.3.4", "X-Forwarded-For": "5.6.7.8"},
	)

	if visto != "203.0.113.9:5555" {
		t.Errorf("RemoteAddr = %q, esperado o peer direto — o cliente escolheu o próprio IP", visto)
	}
	// E os cabeçalhos são apagados: deixá-los passar entregaria a decisão a
	// qualquer handler que os lesse por conta própria mais adiante.
	if sobraram["X-Real-IP"] != "" || sobraram["X-Forwarded-For"] != "" {
		t.Errorf("cabeçalhos forjados sobreviveram: %+v", sobraram)
	}
}

// Sem lista de confiança (o padrão em desenvolvimento), nada é confiado.
func TestSemProxiesConfiaveisNadaEhConfiado(t *testing.T) {
	visto, _ := ecoDoIP(nil, "10.0.0.7:1111", map[string]string{"X-Real-IP": "1.2.3.4"})
	if visto != "10.0.0.7:1111" {
		t.Errorf("RemoteAddr = %q, esperado o peer direto", visto)
	}
}

func TestCabecalhoDeProxyConfiavelEhObedecido(t *testing.T) {
	visto, _ := ecoDoIP(faixas(t, "10.0.0.0/8"), "10.0.0.7:1111", map[string]string{"X-Real-IP": "203.0.113.9"})
	if visto != "203.0.113.9" {
		t.Errorf("RemoteAddr = %q, esperado o IP informado pelo proxy", visto)
	}
}

// Um peer IPv4 que chega por socket dual-stack aparece como ::ffff:10.0.0.7.
// Sem o Unmap na comparação, a confiança falharia justamente na topologia mais
// comum — e todo cliente cairia no mesmo balde de rate limit.
func TestProxyConfiavelEmFormatoMapeadoContinuaConfiavel(t *testing.T) {
	visto, _ := ecoDoIP(faixas(t, "10.0.0.0/8"), "[::ffff:10.0.0.7]:1111",
		map[string]string{"X-Real-IP": "203.0.113.9"})
	if visto != "203.0.113.9" {
		t.Errorf("RemoteAddr = %q, esperado o IP informado pelo proxy", visto)
	}
}

// Mesmo vindo de um proxy confiável, o valor é PARSEADO. Uma string arbitrária
// viraria uma chave arbitrária no contador de rate limit — memória sem teto
// alimentada de fora.
func TestValorNaoParseavelDeProxyConfiavelNaoViraChave(t *testing.T) {
	for _, lixo := range []string{
		"nao-e-um-ip",
		"'; DROP TABLE sessions; --",
		"999.999.999.999",
		"",
	} {
		visto, _ := ecoDoIP(faixas(t, "10.0.0.0/8"), "10.0.0.7:1111", map[string]string{"X-Real-IP": lixo})
		if visto != "10.0.0.7:1111" {
			t.Errorf("X-Real-IP=%q produziu RemoteAddr = %q, esperado o peer direto", lixo, visto)
		}
	}
}
