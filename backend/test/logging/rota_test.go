// O log de acesso não pode conter segredo. O token do convite viaja no CAMINHO
// da URL, então registrar o caminho registra a credencial — em texto puro, num
// arquivo guardado por semanas e enviado a um agregador.
package logging_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"stacktrack/internal/pkg/logging"

	"github.com/go-chi/chi/v5"
)

const tokenSecreto = "3f9a1c7e5b2d8046a1f3e9c7b5d2a840"

// apiComLog monta um roteador com as rotas que carregam token e devolve o que
// o log de acesso escreveu.
func apiComLog(t *testing.T, caminho string) string {
	t.Helper()

	var saida bytes.Buffer
	anterior := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&saida, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(anterior) })

	r := chi.NewRouter()
	r.Use(logging.Middleware)
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	r.Get("/convites/{token}", ok)
	r.Get("/publico/{token}", ok)
	r.Get("/boards/{boardID}", ok)
	logging.DefinirPrefixosConhecidos([]string{"boards", "convites", "publico"})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, caminho, nil))
	return saida.String()
}

// URL VÁLIDA: casa com a rota, e o que se registra é o padrão.
func TestCaminhoValidoComTokenRegistraOPadraoDaRota(t *testing.T) {
	log := apiComLog(t, "/convites/"+tokenSecreto)

	if strings.Contains(log, tokenSecreto) {
		t.Errorf("o token vazou no log: %s", log)
	}
	if !strings.Contains(log, "/convites/{token}") {
		t.Errorf("o log devia trazer o padrão da rota: %s", log)
	}
}

// URL INVÁLIDA: é o caso que vazava. Errar a rota por uma letra dava 404 — e o
// caminho inteiro, token incluído, ia para o log.
func TestCaminhoInvalidoComTokenNaoVazaOToken(t *testing.T) {
	for _, caminho := range []string{
		"/convitess/" + tokenSecreto,          // rota inexistente, prefixo desconhecido
		"/" + tokenSecreto,                    // segredo como caminho de primeiro nível
		"/boards/" + tokenSecreto + "/xyz",    // sob prefixo conhecido, mas sem casar
		"/publico/" + tokenSecreto + "/extra", // idem
	} {
		log := apiComLog(t, caminho)
		if strings.Contains(log, tokenSecreto) {
			t.Errorf("caminho %q vazou o token no log: %s", caminho, log)
		}
		if !strings.Contains(log, logging.SemRota) {
			t.Errorf("caminho %q devia registrar %q: %s", caminho, logging.SemRota, log)
		}
	}
}

// O 404 não pode virar cegueira total: sob um prefixo CONHECIDO, o prefixo
// continua no log — é o que permite ver que alguém está varrendo /boards.
func TestCaminhoInvalidoSobPrefixoConhecidoMantemOPrefixo(t *testing.T) {
	log := apiComLog(t, "/boards/"+tokenSecreto+"/xyz")
	if !strings.Contains(log, "/boards/") {
		t.Errorf("o prefixo conhecido devia aparecer: %s", log)
	}
}

// Prefixo desconhecido não entra no log nem como prefixo: um segredo tentado
// como caminho de primeiro nível apareceria exatamente aí.
func TestPrefixoDesconhecidoNaoEhRegistrado(t *testing.T) {
	log := apiComLog(t, "/"+tokenSecreto)
	// O valor sai entre aspas por conter espaço; o que importa é que ele seja o
	// marcador, e não o caminho.
	if !strings.Contains(log, `rota="`+logging.SemRota+`"`) {
		t.Errorf("esperado rota=%q, veio: %s", logging.SemRota, log)
	}
}
