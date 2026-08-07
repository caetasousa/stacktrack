// Testes da borda HTTP de participação e convites. O foco é o que a resposta
// entrega — e o que ela deixa de entregar — a cada tipo de solicitante.
package handler_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// convidar chama a rota de convite e devolve a resposta decodificada.
func convidar(t *testing.T, api *apiDeQuadro, cookie *http.Cookie, boardID, email, papel string) struct {
	Adicionado bool   `json:"adicionado"`
	Link       string `json:"link"`
	Membro     *struct {
		UsuarioID string `json:"usuarioId"`
		Papel     string `json:"papel"`
	} `json:"membro"`
	Convite *struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Papel string `json:"papel"`
	} `json:"convite"`
} {
	t.Helper()
	rec := chamar(api, http.MethodPost, "/boards/"+boardID+"/membros",
		`{"email":"`+email+`","papel":"`+papel+`"}`, cookie)
	if rec.Code != http.StatusCreated {
		t.Fatalf("convidar: status = %d, esperado 201: %s", rec.Code, rec.Body)
	}

	var corpo struct {
		Adicionado bool   `json:"adicionado"`
		Link       string `json:"link"`
		Membro     *struct {
			UsuarioID string `json:"usuarioId"`
			Papel     string `json:"papel"`
		} `json:"membro"`
		Convite *struct {
			ID    string `json:"id"`
			Email string `json:"email"`
			Papel string `json:"papel"`
		} `json:"convite"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("corpo não é JSON: %s", rec.Body)
	}
	return corpo
}

// tokenDoLink extrai o token da URL devolvida ao dono.
func tokenDoLink(t *testing.T, link string) string {
	t.Helper()
	partes := strings.Split(link, "/convite/")
	if len(partes) != 2 || partes[1] == "" {
		t.Fatalf("link em formato inesperado: %q", link)
	}
	return partes[1]
}

func TestConvidarQuemJaTemContaResponde201ComOMembro(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, _ := api.conta(t, "Ana", "ana@exemplo.com")
	_, bobID := api.conta(t, "Bob", "bob@exemplo.com")
	boardID := api.criarQuadro(t, cookieAna, "Estudos")

	corpo := convidar(t, api, cookieAna, boardID, "bob@exemplo.com", "editor")

	if !corpo.Adicionado {
		t.Fatal("quem já tem conta entra direto")
	}
	if corpo.Membro == nil || corpo.Membro.UsuarioID != bobID || corpo.Membro.Papel != "editor" {
		t.Errorf("membro = %+v", corpo.Membro)
	}
	if corpo.Link != "" {
		t.Error("não faz sentido devolver link para quem já entrou")
	}
}

func TestConvidarQuemNaoTemContaDevolveOLink(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, _ := api.conta(t, "Ana", "ana@exemplo.com")
	boardID := api.criarQuadro(t, cookieAna, "Estudos")

	corpo := convidar(t, api, cookieAna, boardID, "novo@exemplo.com", "leitor")

	if corpo.Adicionado {
		t.Fatal("não há conta para adicionar")
	}
	if !strings.HasPrefix(corpo.Link, "http://localhost:5173/convite/") {
		t.Errorf("link = %q, esperado apontando para o frontend", corpo.Link)
	}
	if corpo.Convite == nil || corpo.Convite.Email != "novo@exemplo.com" {
		t.Errorf("convite = %+v", corpo.Convite)
	}
}

// O token aparece uma vez só, no link. A listagem não pode devolvê-lo: quem
// abre a tela de membros depois não deve conseguir reconstruir um link antigo.
func TestListagemNaoDevolveOTokenDoConvite(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, _ := api.conta(t, "Ana", "ana@exemplo.com")
	boardID := api.criarQuadro(t, cookieAna, "Estudos")
	corpo := convidar(t, api, cookieAna, boardID, "novo@exemplo.com", "leitor")
	token := tokenDoLink(t, corpo.Link)

	rec := chamar(api, http.MethodGet, "/boards/"+boardID+"/membros", "", cookieAna)

	if strings.Contains(rec.Body.String(), token) {
		t.Errorf("o token vazou na listagem: %s", rec.Body)
	}
	if strings.Contains(rec.Body.String(), "tokenHash") {
		t.Error("o hash do token também não deve aparecer")
	}
}

func TestListagemMostraConvitesSoParaODono(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, _ := api.conta(t, "Ana", "ana@exemplo.com")
	cookieBob, _ := api.conta(t, "Bob", "bob@exemplo.com")
	boardID := api.criarQuadro(t, cookieAna, "Estudos")
	convidar(t, api, cookieAna, boardID, "bob@exemplo.com", "editor")
	convidar(t, api, cookieAna, boardID, "novo@exemplo.com", "leitor")

	var comoDono, comoEditor struct {
		Membros  []struct{ Nome, Papel string } `json:"membros"`
		Convites []struct{ Email string }       `json:"convites"`
	}
	json.Unmarshal(chamar(api, http.MethodGet, "/boards/"+boardID+"/membros", "", cookieAna).Body.Bytes(), &comoDono)
	json.Unmarshal(chamar(api, http.MethodGet, "/boards/"+boardID+"/membros", "", cookieBob).Body.Bytes(), &comoEditor)

	if len(comoDono.Convites) != 1 {
		t.Errorf("o dono devia ver 1 convite, viu %d", len(comoDono.Convites))
	}
	// O editor vê a mesma tela, só com menos: a lista de membros continua, e a
	// de convites vem vazia em vez de dar erro.
	if len(comoEditor.Convites) != 0 {
		t.Errorf("o editor não devia ver convite nenhum, viu %d", len(comoEditor.Convites))
	}
	if len(comoEditor.Membros) != 2 {
		t.Errorf("o editor devia ver os 2 membros, viu %d", len(comoEditor.Membros))
	}
}

func TestDetalheDoConviteNaoExigeSessao(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, _ := api.conta(t, "Ana", "ana@exemplo.com")
	boardID := api.criarQuadro(t, cookieAna, "Estudos")
	token := tokenDoLink(t, convidar(t, api, cookieAna, boardID, "novo@exemplo.com", "editor").Link)

	// sem cookie nenhum: quem foi convidado ainda nem tem conta
	rec := chamar(api, http.MethodGet, "/convites/"+token, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200: %s", rec.Code, rec.Body)
	}
	var detalhe struct{ Quadro, Email, Papel, ConvidadoPor string }
	json.Unmarshal(rec.Body.Bytes(), &detalhe)
	if detalhe.Quadro != "Estudos" || detalhe.Email != "novo@exemplo.com" || detalhe.ConvidadoPor != "Ana" {
		t.Errorf("detalhe = %+v", detalhe)
	}
}

func TestConviteDesconhecidoResponde404(t *testing.T) {
	api := montarAPIDeQuadro()

	if rec := chamar(api, http.MethodGet, "/convites/token-inventado", ""); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, esperado 404", rec.Code)
	}
}

func TestAceitarConviteColocaAPessoaNoQuadro(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, _ := api.conta(t, "Ana", "ana@exemplo.com")
	boardID := api.criarQuadro(t, cookieAna, "Estudos")
	token := tokenDoLink(t, convidar(t, api, cookieAna, boardID, "novo@exemplo.com", "editor").Link)

	cookieNovo, _ := api.conta(t, "Novo", "novo@exemplo.com")
	rec := chamar(api, http.MethodPost, "/convites/"+token+"/aceitar", "", cookieNovo)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200: %s", rec.Code, rec.Body)
	}
	if rec := chamar(api, http.MethodGet, "/boards/"+boardID, "", cookieNovo); rec.Code != http.StatusOK {
		t.Errorf("quem aceitou devia enxergar o quadro: %d", rec.Code)
	}
}

// O link encaminhado não coloca qualquer pessoa no quadro: ele está amarrado
// ao email convidado.
func TestOutraContaAceitandoOConviteResponde403(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, _ := api.conta(t, "Ana", "ana@exemplo.com")
	cookieIntruso, _ := api.conta(t, "Intruso", "intruso@exemplo.com")
	boardID := api.criarQuadro(t, cookieAna, "Estudos")
	token := tokenDoLink(t, convidar(t, api, cookieAna, boardID, "novo@exemplo.com", "editor").Link)

	rec := chamar(api, http.MethodPost, "/convites/"+token+"/aceitar", "", cookieIntruso)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, esperado 403", rec.Code)
	}
	if rec := chamar(api, http.MethodGet, "/boards/"+boardID, "", cookieIntruso); rec.Code != http.StatusNotFound {
		t.Errorf("o intruso não podia ter entrado: %d", rec.Code)
	}
}

func TestAceitarSemSessaoResponde401(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, _ := api.conta(t, "Ana", "ana@exemplo.com")
	boardID := api.criarQuadro(t, cookieAna, "Estudos")
	token := tokenDoLink(t, convidar(t, api, cookieAna, boardID, "novo@exemplo.com", "editor").Link)

	if rec := chamar(api, http.MethodPost, "/convites/"+token+"/aceitar", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, esperado 401", rec.Code)
	}
}

func TestConviteRepetidoResponde409(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, _ := api.conta(t, "Ana", "ana@exemplo.com")
	api.conta(t, "Bob", "bob@exemplo.com")
	boardID := api.criarQuadro(t, cookieAna, "Estudos")
	convidar(t, api, cookieAna, boardID, "bob@exemplo.com", "editor")

	casos := map[string]string{
		"quem já participa": `{"email":"bob@exemplo.com","papel":"leitor"}`,
		"a si mesmo":        `{"email":"ana@exemplo.com","papel":"leitor"}`,
	}
	for nome, corpo := range casos {
		t.Run(nome, func(t *testing.T) {
			rec := chamar(api, http.MethodPost, "/boards/"+boardID+"/membros", corpo, cookieAna)
			if rec.Code != http.StatusConflict {
				t.Errorf("status = %d, esperado 409: %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestPapelInvalidoNoConviteResponde400(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, _ := api.conta(t, "Ana", "ana@exemplo.com")
	boardID := api.criarQuadro(t, cookieAna, "Estudos")

	rec := chamar(api, http.MethodPost, "/boards/"+boardID+"/membros",
		`{"email":"novo@exemplo.com","papel":"chefe"}`, cookieAna)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, esperado 400: %s", rec.Code, rec.Body)
	}
}

func TestQuemNaoParticipaRecebe404NasRotasDeMembro(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, _ := api.conta(t, "Ana", "ana@exemplo.com")
	cookieBob, bobID := api.conta(t, "Bob", "bob@exemplo.com")
	boardID := api.criarQuadro(t, cookieAna, "Estudos")

	rotas := []struct{ nome, metodo, caminho, corpo string }{
		{"listar", http.MethodGet, "/boards/" + boardID + "/membros", ""},
		{"convidar", http.MethodPost, "/boards/" + boardID + "/membros", `{"email":"x@exemplo.com","papel":"leitor"}`},
		{"trocar papel", http.MethodPatch, "/boards/" + boardID + "/membros/" + bobID, `{"papel":"dono"}`},
		{"remover", http.MethodDelete, "/boards/" + boardID + "/membros/" + bobID, ""},
	}

	for _, rota := range rotas {
		t.Run(rota.nome, func(t *testing.T) {
			rec := chamar(api, rota.metodo, rota.caminho, rota.corpo, cookieBob)
			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, esperado 404 (403 confirmaria que o quadro existe)", rec.Code)
			}
		})
	}
}

func TestRemoverOUltimoDonoResponde400(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, anaID := api.conta(t, "Ana", "ana@exemplo.com")
	boardID := api.criarQuadro(t, cookieAna, "Estudos")

	rec := chamar(api, http.MethodDelete, "/boards/"+boardID+"/membros/"+anaID, "", cookieAna)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado 400: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "dono") {
		t.Errorf("a mensagem devia explicar o motivo: %s", rec.Body)
	}
}

func TestRevogarConviteInvalidaOLink(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, _ := api.conta(t, "Ana", "ana@exemplo.com")
	boardID := api.criarQuadro(t, cookieAna, "Estudos")
	corpo := convidar(t, api, cookieAna, boardID, "novo@exemplo.com", "editor")
	token := tokenDoLink(t, corpo.Link)

	rec := chamar(api, http.MethodDelete, "/boards/"+boardID+"/convites/"+corpo.Convite.ID, "", cookieAna)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, esperado 204", rec.Code)
	}

	if rec := chamar(api, http.MethodGet, "/convites/"+token, ""); rec.Code != http.StatusNotFound {
		t.Errorf("o link revogado devia responder 404, respondeu %d", rec.Code)
	}
}
