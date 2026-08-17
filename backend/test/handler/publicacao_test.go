// A borda HTTP do link público.
//
// O que estes testes cercam não é a regra — essa é de test/usecase — e sim a
// única coisa que só a borda decide: se a rota está mesmo FORA do grupo
// autenticado, e se a resposta que sai por ela sem cookie nenhum leva o que
// devia levar e nada além.
//
// A checagem de vazamento é feita sobre o CORPO CRU da resposta, e não sobre um
// struct decodificado. Decodificar num tipo esconderia exatamente o defeito
// procurado: um campo a mais no JSON simplesmente não teria onde cair, e o
// teste passaria enquanto o dado sai pelo fio.
package handler_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"stacktrack/internal/domain/membro"
)

// publicar liga o link do quadro e devolve a URL pública.
func (a *apiDeQuadro) publicar(t *testing.T, cookie *http.Cookie, boardID string) string {
	t.Helper()
	rec := chamar(a, http.MethodPut, "/boards/"+boardID+"/publicacao", "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("publicar falhou: %d %s", rec.Code, rec.Body)
	}
	var corpo struct {
		Publicado bool   `json:"publicado"`
		URL       string `json:"url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("corpo não é JSON: %s", rec.Body)
	}
	if !corpo.Publicado || corpo.URL == "" {
		t.Fatalf("publicação sem link: %s", rec.Body)
	}
	return corpo.URL
}

// tokenDe extrai o token do fim da URL pública.
func tokenDe(t *testing.T, url string) string {
	t.Helper()
	partes := strings.Split(url, "/publico/")
	if len(partes) != 2 || partes[1] == "" {
		t.Fatalf("URL pública em formato inesperado: %q", url)
	}
	return partes[1]
}

func TestQuadroPublicoAbreSemSessao(t *testing.T) {
	api := montarAPIDeQuadro()
	cookie, _ := api.conta(t, "Ana", "ana@exemplo.com")
	boardID := api.criarQuadro(t, cookie, "Roadmap")
	token := tokenDe(t, api.publicar(t, cookie, boardID))

	// Sem cookie nenhum: é o navegador de quem recebeu o link e não tem conta.
	rec := chamar(api, http.MethodGet, "/publico/"+token, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200 sem sessão. Corpo: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "Roadmap") {
		t.Errorf("a resposta não traz o quadro: %s", rec.Body)
	}
}

// Token inventado, revogado e de quadro apagado respondem os três 404, e nunca
// 401: um 401 diria "existe, faça login", que é informação sobre um quadro que
// quem pergunta não deveria nem saber que existe.
func TestTokenInvalidoResponde404(t *testing.T) {
	api := montarAPIDeQuadro()

	rec := chamar(api, http.MethodGet, "/publico/nao-existe", "")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, esperado 404. Corpo: %s", rec.Code, rec.Body)
	}
}

func TestRevogarDerrubaOLinkNaBorda(t *testing.T) {
	api := montarAPIDeQuadro()
	cookie, _ := api.conta(t, "Ana", "ana@exemplo.com")
	boardID := api.criarQuadro(t, cookie, "Roadmap")
	token := tokenDe(t, api.publicar(t, cookie, boardID))

	if rec := chamar(api, http.MethodDelete, "/boards/"+boardID+"/publicacao", "", cookie); rec.Code != http.StatusNoContent {
		t.Fatalf("revogar = %d, esperado 204. Corpo: %s", rec.Code, rec.Body)
	}

	if rec := chamar(api, http.MethodGet, "/publico/"+token, ""); rec.Code != http.StatusNotFound {
		t.Errorf("o link revogado ainda responde %d", rec.Code)
	}
}

// Revogar sem nunca ter publicado é 204, não 404: o resultado pretendido — o
// quadro não estar público — já vale. Um 404 aqui faria a tela mostrar erro
// para quem clicou em desligar o que já estava desligado.
func TestRevogarOQueNaoEstaPublicadoResponde204(t *testing.T) {
	api := montarAPIDeQuadro()
	cookie, _ := api.conta(t, "Ana", "ana@exemplo.com")
	boardID := api.criarQuadro(t, cookie, "Roadmap")

	if rec := chamar(api, http.MethodDelete, "/boards/"+boardID+"/publicacao", "", cookie); rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, esperado 204", rec.Code)
	}
}

func TestRotasDePublicacaoExigemSessao(t *testing.T) {
	api := montarAPIDeQuadro()

	for _, caso := range []struct{ metodo, caminho string }{
		{http.MethodGet, "/boards/qualquer/publicacao"},
		{http.MethodPut, "/boards/qualquer/publicacao"},
		{http.MethodDelete, "/boards/qualquer/publicacao"},
	} {
		rec := chamar(api, caso.metodo, caso.caminho, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, esperado 401", caso.metodo, caso.caminho, rec.Code)
		}
	}
}

// Quem não é dono não recebe o token. É a rota, e não a tela, que precisa
// recusar: a tela que esconde o botão é conveniência, não proteção.
func TestQuemNaoEDonoNaoRecebeOToken(t *testing.T) {
	api := montarAPIDeQuadro()
	daAna, _ := api.conta(t, "Ana", "ana@exemplo.com")
	doBob, bobID := api.conta(t, "Bob", "bob@exemplo.com")
	boardID := api.criarQuadro(t, daAna, "Roadmap")
	api.publicar(t, daAna, boardID)
	entra(t, api, boardID, bobID, membro.PapelEditor)

	rec := chamar(api, http.MethodGet, "/boards/"+boardID+"/publicacao", "", doBob)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, esperado 403. Corpo: %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "/publico/") {
		t.Errorf("a recusa vazou o link: %s", rec.Body)
	}
}

// Quem não participa do quadro recebe 404, e não 403: dizer "proibido"
// confirmaria que o quadro existe.
func TestQuemNaoParticipaRecebe404NaPublicacao(t *testing.T) {
	api := montarAPIDeQuadro()
	daAna, _ := api.conta(t, "Ana", "ana@exemplo.com")
	doBob, _ := api.conta(t, "Bob", "bob@exemplo.com")
	boardID := api.criarQuadro(t, daAna, "Roadmap")

	if rec := chamar(api, http.MethodGet, "/boards/"+boardID+"/publicacao", "", doBob); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, esperado 404", rec.Code)
	}
}

// O corpo que sai sem sessão não pode ter pessoa nem id dentro. Ver o cabeçalho
// deste arquivo para por que a busca é no texto cru.
func TestOCorpoPublicoNaoLevaPessoaNemID(t *testing.T) {
	api := montarAPIDeQuadro()
	daAna, _ := api.conta(t, "Ana", "ana@exemplo.com")
	doBob, bobID := api.conta(t, "Roberto Silva", "roberto@exemplo.com")
	boardID := api.criarQuadro(t, daAna, "Roadmap")
	entra(t, api, boardID, bobID, membro.PapelEditor)

	colunaID := idDoCorpo(t, chamar(api, http.MethodPost, "/boards/"+boardID+"/colunas", `{"titulo":"A fazer"}`, daAna))
	cardID := idDoCorpo(t, chamar(api, http.MethodPost, "/colunas/"+colunaID+"/cards", `{"titulo":"Revisar o contrato"}`, daAna))
	if rec := chamar(api, http.MethodPost, "/cards/"+cardID+"/comentarios", `{"texto":"isto aqui é confidencial"}`, doBob); rec.Code != http.StatusCreated {
		t.Fatalf("comentar falhou: %d %s", rec.Code, rec.Body)
	}

	token := tokenDe(t, api.publicar(t, daAna, boardID))
	rec := chamar(api, http.MethodGet, "/publico/"+token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200. Corpo: %s", rec.Code, rec.Body)
	}

	corpo := rec.Body.String()
	for _, proibido := range []string{
		"Roberto Silva", "roberto@exemplo.com", bobID,
		"isto aqui é confidencial",
		boardID, colunaID, cardID,
	} {
		if strings.Contains(corpo, proibido) {
			t.Errorf("a resposta pública vazou %q.\nCorpo: %s", proibido, corpo)
		}
	}
}

// Cache-Control: no-store é o que faz a revogação valer na hora. Sem ele, um
// intermediário — o proxy reverso, o cache de uma rede corporativa — continuaria
// servindo uma cópia do quadro depois de o dono desligar o link, e ele não teria
// como saber.
func TestARespostaPublicaNaoPodeSerGuardada(t *testing.T) {
	api := montarAPIDeQuadro()
	cookie, _ := api.conta(t, "Ana", "ana@exemplo.com")
	boardID := api.criarQuadro(t, cookie, "Roadmap")
	token := tokenDe(t, api.publicar(t, cookie, boardID))

	rec := chamar(api, http.MethodGet, "/publico/"+token, "")

	if cache := rec.Header().Get("Cache-Control"); !strings.Contains(cache, "no-store") {
		t.Errorf("Cache-Control = %q, esperado no-store", cache)
	}
	if robots := rec.Header().Get("X-Robots-Tag"); !strings.Contains(robots, "noindex") {
		t.Errorf("X-Robots-Tag = %q, esperado noindex — o link é para quem o dono mandou", robots)
	}
}

// O quadro autenticado avisa quem edita de que ele está à vista de fora.
func TestOQuadroAvisaQueEstaPublico(t *testing.T) {
	api := montarAPIDeQuadro()
	cookie, _ := api.conta(t, "Ana", "ana@exemplo.com")
	boardID := api.criarQuadro(t, cookie, "Roadmap")

	if publico := publicoDoQuadro(t, api, cookie, boardID); publico {
		t.Error("quadro nunca publicado veio como público")
	}
	api.publicar(t, cookie, boardID)
	if publico := publicoDoQuadro(t, api, cookie, boardID); !publico {
		t.Error("quadro publicado não veio marcado como público")
	}
}

func publicoDoQuadro(t *testing.T, api *apiDeQuadro, cookie *http.Cookie, boardID string) bool {
	t.Helper()
	rec := chamar(api, http.MethodGet, "/boards/"+boardID, "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("detalhar = %d: %s", rec.Code, rec.Body)
	}
	var corpo struct {
		Publico bool `json:"publico"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("corpo não é JSON: %s", rec.Body)
	}
	return corpo.Publico
}
