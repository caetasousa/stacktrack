// A borda HTTP da auditoria do quadro.
//
// A regra é de test/usecase. O que só a borda decide são os PARÂMETROS: eles
// vêm da query string, que é entrada de fora, e cada um tem um jeito próprio de
// dar errado — um recorte que se abre sozinho, um cursor com lixo, um filtro
// que não filtra.
package handler_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"stacktrack/internal/domain/membro"
)

type linhaDeAuditoria struct {
	Seq        int64  `json:"seq"`
	Tipo       string `json:"tipo"`
	AutorID    string `json:"autorId"`
	AutorNome  string `json:"autorNome"`
	AutorEmail string `json:"autorEmail"`
}

func auditar(t *testing.T, api *apiDeQuadro, cookie *http.Cookie, boardID, consulta string) []linhaDeAuditoria {
	t.Helper()
	rec := chamar(api, http.MethodGet, "/boards/"+boardID+"/atividade"+consulta, "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("auditoria = %d: %s", rec.Code, rec.Body)
	}
	var corpo struct {
		Atividade []linhaDeAuditoria `json:"atividade"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("corpo não é JSON: %s", rec.Body)
	}
	return corpo.Atividade
}

// cenarioMovimentado monta um quadro com duas colunas e um card já movido pelo
// bob, e devolve os cookies das duas contas e o id do quadro.
func cenarioMovimentado(t *testing.T, api *apiDeQuadro) (daAna, doBob *http.Cookie, bobID, boardID, cardID string) {
	t.Helper()
	daAna, _ = api.conta(t, "Ana", "ana@exemplo.com")
	doBob, bobID = api.conta(t, "Roberto Silva", "roberto@exemplo.com")
	boardID = api.criarQuadro(t, daAna, "Roadmap")
	entra(t, api, boardID, bobID, membro.PapelEditor)

	aFazer := idDoCorpo(t, chamar(api, http.MethodPost, "/boards/"+boardID+"/colunas", `{"titulo":"A fazer"}`, daAna))
	pronto := idDoCorpo(t, chamar(api, http.MethodPost, "/boards/"+boardID+"/colunas", `{"titulo":"Pronto"}`, daAna))
	cardID = idDoCorpo(t, chamar(api, http.MethodPost, "/colunas/"+aFazer+"/cards", `{"titulo":"Revisar o contrato"}`, daAna))

	if rec := chamar(api, http.MethodPatch, "/cards/"+cardID+"/mover", `{"colunaId":"`+pronto+`"}`, doBob); rec.Code != http.StatusOK {
		t.Fatalf("mover falhou: %d %s", rec.Code, rec.Body)
	}
	return
}

func TestAuditoriaDoQuadroTrazQuemMoveu(t *testing.T) {
	api := montarAPIDeQuadro()
	daAna, _, bobID, boardID, _ := cenarioMovimentado(t, api)

	linhas := auditar(t, api, daAna, boardID, "")

	if len(linhas) != 1 {
		t.Fatalf("movimentações = %d, esperada 1: %+v", len(linhas), linhas)
	}
	if linhas[0].AutorID != bobID || linhas[0].AutorNome != "Roberto Silva" {
		t.Errorf("autor = %q/%q, esperado o bob", linhas[0].AutorID, linhas[0].AutorNome)
	}
}

// O padrão é o recorte ESTREITO. Um parâmetro ausente, ou escrito errado, não
// pode abrir a resposta: a direção segura para entrada de fora é devolver
// menos, nunca a história inteira do quadro.
func TestSemFiltroOuComFiltroInvalidoVemSoMovimentacao(t *testing.T) {
	api := montarAPIDeQuadro()
	daAna, _, _, boardID, _ := cenarioMovimentado(t, api)

	for _, consulta := range []string{"", "?filtro=", "?filtro=TUDO", "?filtro=qualquer-coisa"} {
		linhas := auditar(t, api, daAna, boardID, consulta)
		for _, l := range linhas {
			if l.Tipo != "card.movido" {
				t.Errorf("consulta %q abriu o recorte e trouxe %q", consulta, l.Tipo)
			}
		}
	}
}

func TestFiltroTudoAbreORecorte(t *testing.T) {
	api := montarAPIDeQuadro()
	daAna, _, _, boardID, _ := cenarioMovimentado(t, api)

	estreito := auditar(t, api, daAna, boardID, "")
	tudo := auditar(t, api, daAna, boardID, "?filtro=tudo")

	if len(tudo) <= len(estreito) {
		t.Errorf("filtro=tudo trouxe %d linhas e o padrão trouxe %d — devia trazer mais", len(tudo), len(estreito))
	}
}

func TestAuditoriaFiltraPorAutorNaBorda(t *testing.T) {
	api := montarAPIDeQuadro()
	daAna, _, bobID, boardID, _ := cenarioMovimentado(t, api)

	doBob := auditar(t, api, daAna, boardID, "?filtro=tudo&autor="+bobID)
	for _, l := range doBob {
		if l.AutorID != bobID {
			t.Errorf("o filtro por autor deixou passar %q", l.AutorID)
		}
	}
	if len(doBob) == 0 {
		t.Error("o bob moveu um card e não apareceu no próprio filtro")
	}

	deNinguem := auditar(t, api, daAna, boardID, "?filtro=tudo&autor=nao-existe")
	if len(deNinguem) != 0 {
		t.Errorf("autor desconhecido devolveu %d linhas", len(deNinguem))
	}
}

// Cursor com lixo vira primeira página, e não erro: quem chama com um valor
// inválido receberia a primeira página de qualquer forma, e um 400 aqui só
// transformaria um link velho colado no navegador numa tela de erro.
func TestCursorInvalidoDevolveAPrimeiraPagina(t *testing.T) {
	api := montarAPIDeQuadro()
	daAna, _, _, boardID, _ := cenarioMovimentado(t, api)

	esperado := auditar(t, api, daAna, boardID, "")
	for _, cursor := range []string{"abc", "-5", "0", "9999999999999999999999"} {
		linhas := auditar(t, api, daAna, boardID, "?antesDe="+cursor)
		if len(linhas) != len(esperado) {
			t.Errorf("cursor %q devolveu %d linhas, esperadas %d", cursor, len(linhas), len(esperado))
		}
	}
}

func TestAuditoriaDeQuemNaoParticipaResponde404(t *testing.T) {
	api := montarAPIDeQuadro()
	_, _, _, boardID, _ := cenarioMovimentado(t, api)
	deTerceiro, _ := api.conta(t, "Carla", "carla@exemplo.com")

	rec := chamar(api, http.MethodGet, "/boards/"+boardID+"/atividade", "", deTerceiro)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, esperado 404", rec.Code)
	}
}

func TestAuditoriaExigeSessao(t *testing.T) {
	api := montarAPIDeQuadro()

	if rec := chamar(api, http.MethodGet, "/boards/qualquer/atividade", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, esperado 401", rec.Code)
	}
}

// O selo no card é o que a tela do quadro mostra sem abrir nada. Ele viaja no
// GET do quadro, junto com os outros resumos — não numa requisição por card.
func TestOQuadroTrazOSeloDeUltimaMovimentacaoNoCard(t *testing.T) {
	api := montarAPIDeQuadro()
	daAna, _, bobID, boardID, cardID := cenarioMovimentado(t, api)

	rec := chamar(api, http.MethodGet, "/boards/"+boardID, "", daAna)
	if rec.Code != http.StatusOK {
		t.Fatalf("detalhar = %d: %s", rec.Code, rec.Body)
	}

	var corpo struct {
		Colunas []struct {
			Cards []struct {
				ID                 string `json:"id"`
				UltimaMovimentacao *struct {
					AutorID   string `json:"autorId"`
					AutorNome string `json:"autorNome"`
					De        string `json:"de"`
					Para      string `json:"para"`
				} `json:"ultimaMovimentacao"`
			} `json:"cards"`
		} `json:"colunas"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("corpo não é JSON: %s", rec.Body)
	}

	achou := false
	for _, coluna := range corpo.Colunas {
		for _, c := range coluna.Cards {
			if c.ID != cardID {
				continue
			}
			achou = true
			if c.UltimaMovimentacao == nil {
				t.Fatal("o card movido veio sem selo")
			}
			if c.UltimaMovimentacao.AutorID != bobID || c.UltimaMovimentacao.AutorNome != "Roberto Silva" {
				t.Errorf("selo = %+v, esperado o bob", c.UltimaMovimentacao)
			}
			if c.UltimaMovimentacao.De != "A fazer" || c.UltimaMovimentacao.Para != "Pronto" {
				t.Errorf("selo = %q -> %q, esperado 'A fazer' -> 'Pronto'", c.UltimaMovimentacao.De, c.UltimaMovimentacao.Para)
			}
		}
	}
	if !achou {
		t.Fatal("o card não veio no quadro")
	}
}

// O selo é do quadro AUTENTICADO. Na página pública ele não pode aparecer: é o
// nome de uma pessoa, e quem publica o quadro não publica o colega junto.
func TestOSeloNaoSaiPelaPaginaPublica(t *testing.T) {
	api := montarAPIDeQuadro()
	daAna, _, _, boardID, _ := cenarioMovimentado(t, api)
	token := tokenDe(t, api.publicar(t, daAna, boardID))

	rec := chamar(api, http.MethodGet, "/publico/"+token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}

	corpo := rec.Body.String()
	for _, proibido := range []string{"Roberto Silva", "ultimaMovimentacao", "autorNome"} {
		if strings.Contains(corpo, proibido) {
			t.Errorf("a página pública vazou %q.\nCorpo: %s", proibido, corpo)
		}
	}
}

// O email desempata homônimos, e sem ele a auditoria fica inútil justamente
// quando é necessária: dois "Ana Silva" no mesmo quadro são indistinguíveis
// olhando só o nome, e não há como saber se são duas pessoas ou a mesma.
//
// Não é exposição nova — qualquer membro já lê o email de todos na tela de
// membros. O teste existe para que TIRÁ-LO seja uma decisão, e não o efeito
// colateral de mexer no SELECT.
func TestAAuditoriaIdentificaQuemAgiuPeloEmail(t *testing.T) {
	api := montarAPIDeQuadro()
	daAna, _, _, boardID, _ := cenarioMovimentado(t, api)

	linhas := auditar(t, api, daAna, boardID, "")

	if len(linhas) != 1 {
		t.Fatalf("movimentações = %d, esperada 1", len(linhas))
	}
	if linhas[0].AutorEmail != "roberto@exemplo.com" {
		t.Errorf("autorEmail = %q, esperado o email de quem moveu", linhas[0].AutorEmail)
	}
}

// Dois homônimos precisam sair distinguíveis. É o caso que motivou o campo, e
// o único em que o nome sozinho falha de forma invisível.
func TestHomonimosSaemDistinguiveis(t *testing.T) {
	api := montarAPIDeQuadro()
	daAna, _ := api.conta(t, "Ana Silva", "ana.silva@exemplo.com")
	daOutra, outraID := api.conta(t, "Ana Silva", "ana.s@exemplo.com")
	boardID := api.criarQuadro(t, daAna, "Roadmap")
	entra(t, api, boardID, outraID, membro.PapelEditor)

	aFazer := idDoCorpo(t, chamar(api, http.MethodPost, "/boards/"+boardID+"/colunas", `{"titulo":"A fazer"}`, daAna))
	pronto := idDoCorpo(t, chamar(api, http.MethodPost, "/boards/"+boardID+"/colunas", `{"titulo":"Pronto"}`, daAna))
	cardID := idDoCorpo(t, chamar(api, http.MethodPost, "/colunas/"+aFazer+"/cards", `{"titulo":"Card"}`, daAna))

	if rec := chamar(api, http.MethodPatch, "/cards/"+cardID+"/mover", `{"colunaId":"`+pronto+`"}`, daOutra); rec.Code != http.StatusOK {
		t.Fatalf("mover: %d %s", rec.Code, rec.Body)
	}

	linhas := auditar(t, api, daAna, boardID, "")
	if len(linhas) != 1 {
		t.Fatalf("movimentações = %d, esperada 1", len(linhas))
	}
	if linhas[0].AutorNome != "Ana Silva" {
		t.Fatalf("o cenário não tem homônimo: %q", linhas[0].AutorNome)
	}
	if linhas[0].AutorEmail != "ana.s@exemplo.com" {
		t.Errorf("autorEmail = %q — sem ele as duas Anas seriam a mesma pessoa na tela", linhas[0].AutorEmail)
	}
}
