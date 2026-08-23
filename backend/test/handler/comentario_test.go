// A conversa pela borda HTTP.
//
// Estes testes existem porque as duas falhas que eles trancam passaram por
// domínio, usecase e testes de unidade sem nenhum sinal, e só apareceram
// exercitando a API de verdade: um erro de domínio sem tradução vira 500, e um
// campo que o DTO não copia simplesmente não chega à tela.
package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"stacktrack/internal/domain/membro"
)

// conversaMontada cria quadro, coluna e card da ana, e devolve o que os testes
// da conversa precisam.
func conversaMontada(t *testing.T, api *apiDeQuadro) (cookieAna *http.Cookie, boardID, cardID string) {
	t.Helper()
	cookieAna, _ = api.conta(t, "Ana", "ana@exemplo.com")
	boardID = api.criarQuadro(t, cookieAna, "Estudos")

	colunaRec := chamar(api, http.MethodPost, "/boards/"+boardID+"/colunas", `{"titulo":"A fazer"}`, cookieAna)
	if colunaRec.Code != http.StatusCreated {
		t.Fatalf("criar coluna: %d %s", colunaRec.Code, colunaRec.Body)
	}
	cardRec := chamar(api, http.MethodPost, "/colunas/"+idDoCorpo(t, colunaRec)+"/cards", `{"titulo":"Migração"}`, cookieAna)
	if cardRec.Code != http.StatusCreated {
		t.Fatalf("criar card: %d %s", cardRec.Code, cardRec.Body)
	}
	return cookieAna, boardID, idDoCorpo(t, cardRec)
}

// entra vincula alguém ao quadro com o papel informado.
func entra(t *testing.T, api *apiDeQuadro, boardID, usuarioID string, papel membro.Papel) {
	t.Helper()
	vinculo, err := membro.Novo(boardID, usuarioID, papel)
	if err != nil {
		t.Fatalf("vínculo inválido: %v", err)
	}
	api.membros.Salvar(context.Background(), vinculo)
}

// Comentar exige participação, não papel de edição: acompanhar e responder é
// ver, não mexer.
func TestLeitorComentaPelaAPI(t *testing.T) {
	api := montarAPIDeQuadro()
	_, boardID, cardID := conversaMontada(t, api)
	cookieBob, bobID := api.conta(t, "Bob", "bob@exemplo.com")
	entra(t, api, boardID, bobID, membro.PapelLeitor)

	rec := chamar(api, http.MethodPost, "/cards/"+cardID+"/comentarios", `{"texto":"Revisei"}`, cookieBob)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, esperado 201 — %s", rec.Code, rec.Body)
	}
}

// A falha que motivou este arquivo: ErrNaoEhAutor não tinha tradução para HTTP
// e a API respondia 500 — dizendo "erro interno" para uma recusa que é regra de
// negócio, e enchendo o log de erro por um caminho perfeitamente esperado.
func TestEditarComentarioAlheioResponde403(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, boardID, cardID := conversaMontada(t, api)
	cookieBob, bobID := api.conta(t, "Bob", "bob@exemplo.com")
	entra(t, api, boardID, bobID, membro.PapelEditor)

	criado := chamar(api, http.MethodPost, "/cards/"+cardID+"/comentarios", `{"texto":"do bob"}`, cookieBob)
	if criado.Code != http.StatusCreated {
		t.Fatalf("comentar: %d %s", criado.Code, criado.Body)
	}

	rec := chamar(api, http.MethodPatch, "/comentarios/"+idDoCorpo(t, criado), `{"texto":"reescrito pela ana"}`, cookieAna)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, esperado 403 — %s", rec.Code, rec.Body)
	}
}

// Mas apagar o dono pode: é dele a responsabilidade pelo que fica no quadro.
func TestDonoApagaComentarioAlheioPelaAPI(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, boardID, cardID := conversaMontada(t, api)
	cookieBob, bobID := api.conta(t, "Bob", "bob@exemplo.com")
	entra(t, api, boardID, bobID, membro.PapelEditor)

	criado := chamar(api, http.MethodPost, "/cards/"+cardID+"/comentarios", `{"texto":"do bob"}`, cookieBob)
	rec := chamar(api, http.MethodDelete, "/comentarios/"+idDoCorpo(t, criado), "", cookieAna)
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, esperado 204 — %s", rec.Code, rec.Body)
	}
}

func TestComentarioVazioResponde400(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, _, cardID := conversaMontada(t, api)

	rec := chamar(api, http.MethodPost, "/cards/"+cardID+"/comentarios", `{"texto":"   "}`, cookieAna)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, esperado 400 — %s", rec.Code, rec.Body)
	}
}

func TestConversaDeQuadroAlheioResponde404(t *testing.T) {
	api := montarAPIDeQuadro()
	_, _, cardID := conversaMontada(t, api)
	cookieEstranho, _ := api.conta(t, "Estranho", "estranho@exemplo.com")

	rec := chamar(api, http.MethodGet, "/cards/"+cardID+"/comentarios", "", cookieEstranho)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, esperado 404 — %s", rec.Code, rec.Body)
	}
}

// A outra falha: o selo 💬 existia no domínio e no usecase, mas o conversor do
// DTO não copiava o campo — e a contagem simplesmente não chegava à tela. É o
// tipo de defeito que só a borda pega, porque as camadas de dentro estavam
// todas certas.
func TestQuadroDevolveAContagemDeComentarios(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, boardID, cardID := conversaMontada(t, api)

	for _, texto := range []string{`{"texto":"um"}`, `{"texto":"dois"}`} {
		if rec := chamar(api, http.MethodPost, "/cards/"+cardID+"/comentarios", texto, cookieAna); rec.Code != http.StatusCreated {
			t.Fatalf("comentar: %d %s", rec.Code, rec.Body)
		}
	}

	rec := chamar(api, http.MethodGet, "/boards/"+boardID, "", cookieAna)
	var detalhe struct {
		Colunas []struct {
			Cards []struct {
				QtdComentarios int `json:"qtdComentarios"`
			} `json:"cards"`
		} `json:"colunas"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detalhe); err != nil {
		t.Fatalf("corpo ilegível: %v", err)
	}
	if len(detalhe.Colunas) != 1 || len(detalhe.Colunas[0].Cards) != 1 {
		t.Fatalf("estrutura = %+v", detalhe)
	}
	if got := detalhe.Colunas[0].Cards[0].QtdComentarios; got != 2 {
		t.Errorf("qtdComentarios = %d, esperado 2 — o selo não chega à tela", got)
	}
}

// A conversa volta com o nome de quem falou: um comentário com id cru não diz
// nada a ninguém.
func TestConversaVoltaComNomeDoAutorPelaAPI(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, _, cardID := conversaMontada(t, api)

	chamar(api, http.MethodPost, "/cards/"+cardID+"/comentarios", `{"texto":"primeiro"}`, cookieAna)

	rec := chamar(api, http.MethodGet, "/cards/"+cardID+"/comentarios", "", cookieAna)
	var corpo struct {
		Comentarios []struct {
			Texto     string  `json:"texto"`
			AutorNome string  `json:"autorNome"`
			EditadoEm *string `json:"editadoEm"`
		} `json:"comentarios"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("corpo ilegível: %v", err)
	}
	if len(corpo.Comentarios) != 1 {
		t.Fatalf("comentários = %d", len(corpo.Comentarios))
	}
	if corpo.Comentarios[0].AutorNome != "Ana" {
		t.Errorf("autorNome = %q, esperado Ana", corpo.Comentarios[0].AutorNome)
	}
	if corpo.Comentarios[0].EditadoEm != nil {
		t.Errorf("editadoEm = %v, esperado nulo num comentário recém-criado", *corpo.Comentarios[0].EditadoEm)
	}
}

// O modal lê a conversa do PRÓPRIO card detalhado, e não de uma segunda
// requisição. Este teste existe porque o campo chegou a ficar sem conversão no
// handler: o DTO tinha `comentarios`, o usecase devolvia a lista, e a resposta
// saía com `null` — o que estourava na tela em tempo de execução, num lugar
// onde as três camadas de dentro estavam corretas.
//
// A garantia que ele trava é o `[]` em vez de `null`: uma lista ausente força
// toda a tela a tratar dois casos onde só deveria existir um.
func TestCardDetalhadoTrazAConversa(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, _, cardID := conversaMontada(t, api)

	rec := chamar(api, http.MethodGet, "/cards/"+cardID, "", cookieAna)
	if rec.Code != http.StatusOK {
		t.Fatalf("detalhar card: %d %s", rec.Code, rec.Body)
	}

	// Sem comentário nenhum a lista precisa ser [], nunca null.
	var cru map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cru); err != nil {
		t.Fatalf("corpo ilegível: %v", err)
	}
	lista, presente := cru["comentarios"]
	if !presente || lista == nil {
		t.Fatalf("comentarios = %v, esperado lista vazia — null quebra a tela", lista)
	}
	if revisao, presente := cru["revisao"]; !presente {
		t.Fatal("resposta do modal não trouxe o campo revisao")
	} else if _, numero := revisao.(float64); !numero {
		t.Fatalf("revisao = %T, esperado número JSON", revisao)
	}

	chamar(api, http.MethodPost, "/cards/"+cardID+"/comentarios", `{"texto":"primeiro"}`, cookieAna)

	rec = chamar(api, http.MethodGet, "/cards/"+cardID, "", cookieAna)
	var detalhe struct {
		Comentarios []struct {
			Texto     string `json:"texto"`
			AutorNome string `json:"autorNome"`
		} `json:"comentarios"`
	}
	json.Unmarshal(rec.Body.Bytes(), &detalhe)
	if len(detalhe.Comentarios) != 1 || detalhe.Comentarios[0].Texto != "primeiro" {
		t.Fatalf("conversa no card detalhado = %+v", detalhe.Comentarios)
	}
	if detalhe.Comentarios[0].AutorNome != "Ana" {
		t.Errorf("autorNome = %q, esperado Ana", detalhe.Comentarios[0].AutorNome)
	}
}
