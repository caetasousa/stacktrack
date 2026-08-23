// Testes da borda HTTP do quadro. O foco é a tradução de erro em código: 404
// para o que não se pode nem enxergar, 403 para o que se enxerga mas não se
// pode mexer.
package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"stacktrack/internal/adapter/http/handler"
	"stacktrack/internal/adapter/http/middleware"
	"stacktrack/internal/domain/membro"
	ucauth "stacktrack/internal/usecase/auth"
	ucboard "stacktrack/internal/usecase/board"
	"stacktrack/test/repository/memoria"

	"github.com/go-chi/chi/v5"
)

type apiDeQuadro struct {
	http.Handler
	membros  *memoria.Membros
	convites *memoria.Convites
}

// armazemDeTeste é a porta do armazém vista pelos testes — a mesma que os
// usecases consomem, declarada aqui porque a do pacote de domínio não é
// exportada.
type armazemDeTeste interface {
	Receber(conteudo io.Reader, limite int64) (ucboard.ArquivoRecebido, error)
	Publicar(recebido ucboard.ArquivoRecebido, extensao string) (string, error)
	Descartar(recebido ucboard.ArquivoRecebido) error
	Abrir(caminho string) (io.ReadCloser, error)
	Remover(caminho string) error
}

// montarAPIDeQuadro sobe autenticação e quadros juntos, espelhando o wiring do
// main.go — inclusive o fato de as rotas de quadro viverem dentro do grupo
// autenticado.
// montarAPIDeQuadro monta a API com o armazém EM MEMÓRIA, que é o certo para os
// testes de rota: eles falam de status e corpo, não de bytes em disco.
func montarAPIDeQuadro() *apiDeQuadro {
	return montarAPIDeQuadroCom(memoria.NovoArmazem())
}

// montarAPIDeQuadroCom permite trocar o armazém.
//
// Existe para o teste de upload, que precisa do armazém de DISCO: o fake em
// memória guarda o arquivo inteiro por definição, e mediria exatamente o
// oposto do que se quer provar.
func montarAPIDeQuadroCom(armazem armazemDeTeste) *apiDeQuadro {
	usuarios := memoria.NovosUsuarios()
	sessoes := memoria.NovasSessoes()
	hasher := &memoria.Hasher{}
	membros := memoria.NovosMembros()
	convites := memoria.NovosConvites()
	cards := memoria.NovosCards()
	colunas := memoria.NovasColunas(cards)
	cards.LigarColunas(colunas)
	boards := memoria.NovosBoards(membros)
	membros.LigarUsuarios(usuarios)
	etiquetas := memoria.NovasEtiquetas()
	checklists := memoria.NovasChecklists()
	anexos := memoria.NovosAnexos()
	etiquetas.LigarQuadro(colunas, cards)
	checklists.LigarQuadro(colunas, cards)
	anexos.LigarQuadro(colunas, cards)
	responsaveis := memoria.NovosResponsaveis()
	responsaveis.LigarQuadro(colunas, cards)
	comentarios := memoria.NovosComentarios()
	comentarios.LigarQuadro(colunas, cards)
	comentarios.LigarUsuarios(usuarios)
	atividades := memoria.NovasAtividades()
	atividades.LigarUsuarios(usuarios)
	publicacoes := memoria.NovasPublicacoes()
	boards.LigarPublicacoes(publicacoes)

	// Teto de cookies desconhecidos desligado: estes testes exercitam rotas de
	// quadro, e um teto por IP aqui reprovaria a suíte inteira pelo IP comum do
	// httptest.
	autenticacao := middleware.NovoAuth(ucauth.NovoValidarSessaoUseCase(sessoes), false, 0, time.Minute)
	identidade := func(r *http.Request) (ucauth.Identidade, bool) {
		return middleware.IdentidadeDoContexto(r.Context())
	}
	authHandler := handler.NovoAuthHandler(
		ucauth.NovoCadastrarUseCase(usuarios, sessoes, hasher),
		ucauth.NovoLoginUseCase(usuarios, sessoes, hasher),
		ucauth.NovoLogoutUseCase(sessoes),
		ucauth.NovoPerfilUseCase(usuarios),
		false, nil, identidade,
	)
	quadroUC := ucboard.NovoQuadroUseCase(boards, membros, colunas, cards, etiquetas, checklists, anexos, responsaveis, comentarios, armazem)
	quadroUC.ComPublicacoes(publicacoes)
	publicacaoHandler := handler.NovoPublicacaoHandler(
		ucboard.NovoPublicacaoUseCase(publicacoes, membros, boards, colunas, cards, etiquetas, checklists),
		"http://localhost:5173",
		identidade,
	)
	quadroUC.ComAtividades(atividades)
	colunaUC := ucboard.NovoColunaUseCase(membros, colunas, anexos, armazem)
	cardUC := ucboard.NovoCardUseCase(boards, membros, colunas, cards, etiquetas, checklists, anexos, responsaveis, comentarios, armazem)
	// O log RECEBE as escritas, como em produção. Sem isto, mover um card pela
	// API não gravaria evento, e o teste da auditoria passaria por não haver o
	// que auditar — verde pelo motivo errado.
	for _, uc := range []interface {
		ComRegistro(ucboard.RegistroDeEventos)
	}{quadroUC, colunaUC, cardUC} {
		uc.ComRegistro(atividades)
	}
	boardHandler := handler.NovoBoardHandler(
		quadroUC,
		colunaUC,
		cardUC,
		identidade,
	)
	extrasHandler := handler.NovoExtrasHandler(
		ucboard.NovoEtiquetaUseCase(membros, colunas, cards, etiquetas),
		ucboard.NovoChecklistUseCase(membros, colunas, cards, checklists),
		// O armazém era `nil` aqui porque nenhum teste de rota exercitava
		// upload. O teste de streaming exercita, e um nil vira panic na
		// primeira leitura — passar o armazém de verdade é o que torna a rota
		// utilizável.
		ucboard.NovoAnexoUseCase(membros, colunas, cards, anexos, armazem),
		ucboard.NovoResponsavelUseCase(membros, colunas, cards, responsaveis),
		ucboard.NovoComentarioUseCase(membros, colunas, cards, comentarios),
		ucboard.NovoAtividadeUseCase(membros, colunas, cards, atividades),
		identidade,
	)
	membroHandler := handler.NovoMembroHandler(
		ucboard.NovoMembroUseCase(membros, convites, usuarios, boards, responsaveis),
		"http://localhost:5173",
		identidade,
	)

	r := chi.NewRouter()
	r.Post("/auth/cadastro", authHandler.Cadastrar)
	r.Get("/convites/{token}", membroHandler.DetalharConvite)
	// Fora do grupo autenticado, como no main.go: é justamente isso que estes
	// testes precisam poder exercitar.
	r.Get("/publico/{token}", publicacaoHandler.Ver)
	r.Group(func(r chi.Router) {
		r.Use(autenticacao.Autenticar)
		r.Post("/convites/{token}/aceitar", membroHandler.AceitarConvite)
		r.Route("/boards", func(r chi.Router) {
			r.Get("/", boardHandler.Listar)
			r.Post("/", boardHandler.Criar)
			r.Get("/{boardID}", boardHandler.Detalhar)
			r.Patch("/{boardID}", boardHandler.Renomear)
			r.Delete("/{boardID}", boardHandler.Apagar)
			r.Post("/{boardID}/colunas", boardHandler.CriarColuna)

			r.Get("/{boardID}/membros", membroHandler.Listar)
			r.Post("/{boardID}/membros", membroHandler.Convidar)
			r.Patch("/{boardID}/membros/{usuarioID}", membroHandler.AlterarPapel)
			r.Delete("/{boardID}/membros/{usuarioID}", membroHandler.Remover)
			r.Delete("/{boardID}/convites/{conviteID}", membroHandler.RevogarConvite)

			r.Get("/{boardID}/atividade", extrasHandler.AtividadeDoQuadro)

			r.Get("/{boardID}/publicacao", publicacaoHandler.Consultar)
			r.Put("/{boardID}/publicacao", publicacaoHandler.Publicar)
			r.Delete("/{boardID}/publicacao", publicacaoHandler.Revogar)
		})
		r.Route("/colunas/{colunaID}", func(r chi.Router) {
			r.Patch("/", boardHandler.RenomearColuna)
			r.Delete("/", boardHandler.ApagarColuna)
			r.Post("/cards", boardHandler.CriarCard)
		})
		r.Route("/cards/{cardID}", func(r chi.Router) {
			r.Patch("/", boardHandler.EditarCard)
			r.Delete("/", boardHandler.ApagarCard)
			r.Get("/", boardHandler.DetalharCard)
			r.Patch("/mover", boardHandler.MoverCard)
			r.Get("/atividade", extrasHandler.Atividade)
			r.Get("/comentarios", extrasHandler.ListarComentarios)
			r.Post("/comentarios", extrasHandler.Comentar)
			// O envio de anexo entra aqui porque o upload em streaming é
			// exercitado pela borda: o que se mede é o que o handler aloca ao
			// receber o multipart.
			r.Post("/anexos/arquivo", extrasHandler.AnexarArquivo)
			r.Post("/anexos/link", extrasHandler.AnexarLink)
		})
		r.Route("/comentarios/{comentarioID}", func(r chi.Router) {
			r.Patch("/", extrasHandler.EditarComentario)
			r.Delete("/", extrasHandler.ApagarComentario)
		})
	})

	return &apiDeQuadro{Handler: r, membros: membros, convites: convites}
}

// conta cadastra alguém e devolve o cookie de sessão e o id.
func (a *apiDeQuadro) conta(t *testing.T, nome, email string) (*http.Cookie, string) {
	t.Helper()
	rec := chamar(a, http.MethodPost, "/auth/cadastro", `{"nome":"`+nome+`","email":"`+email+`","senha":"senha-boa-de-teste-123"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("cadastro de %s falhou: %d %s", nome, rec.Code, rec.Body)
	}
	var corpo map[string]string
	json.Unmarshal(rec.Body.Bytes(), &corpo)
	return cookieDeSessao(t, rec), corpo["id"]
}

// criarQuadro cria um quadro e devolve o id.
func (a *apiDeQuadro) criarQuadro(t *testing.T, cookie *http.Cookie, titulo string) string {
	t.Helper()
	rec := chamar(a, http.MethodPost, "/boards", `{"titulo":"`+titulo+`"}`, cookie)
	if rec.Code != http.StatusCreated {
		t.Fatalf("criar quadro falhou: %d %s", rec.Code, rec.Body)
	}
	return idDoCorpo(t, rec)
}

func idDoCorpo(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var corpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("corpo não é JSON: %s", rec.Body)
	}
	id, _ := corpo["id"].(string)
	if id == "" {
		t.Fatalf("corpo sem id: %s", rec.Body)
	}
	return id
}

func TestRotasDeQuadroExigemSessao(t *testing.T) {
	api := montarAPIDeQuadro()

	rotas := []struct{ metodo, caminho string }{
		{http.MethodGet, "/boards"},
		{http.MethodPost, "/boards"},
		{http.MethodGet, "/boards/qualquer"},
		{http.MethodPatch, "/boards/qualquer"},
		{http.MethodDelete, "/boards/qualquer"},
		{http.MethodPost, "/boards/qualquer/colunas"},
		{http.MethodPatch, "/colunas/qualquer"},
		{http.MethodDelete, "/colunas/qualquer"},
		{http.MethodPost, "/colunas/qualquer/cards"},
		{http.MethodPatch, "/cards/qualquer"},
		{http.MethodDelete, "/cards/qualquer"},
	}

	for _, rota := range rotas {
		t.Run(rota.metodo+" "+rota.caminho, func(t *testing.T) {
			rec := chamar(api, rota.metodo, rota.caminho, `{"titulo":"x"}`)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, esperado 401", rec.Code)
			}
		})
	}
}

func TestCriarEListarQuadros(t *testing.T) {
	api := montarAPIDeQuadro()
	cookie, _ := api.conta(t, "Ana", "ana@exemplo.com")

	api.criarQuadro(t, cookie, "Estudos")

	rec := chamar(api, http.MethodGet, "/boards", "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}

	var corpo struct {
		Boards []struct{ Titulo, Papel string } `json:"boards"`
	}
	json.Unmarshal(rec.Body.Bytes(), &corpo)
	if len(corpo.Boards) != 1 {
		t.Fatalf("quadros = %d, esperado 1: %s", len(corpo.Boards), rec.Body)
	}
	if corpo.Boards[0].Titulo != "Estudos" || corpo.Boards[0].Papel != string(membro.PapelDono) {
		t.Errorf("quadro = %+v", corpo.Boards[0])
	}
}

// Sem quadro nenhum, a listagem precisa sair como [] e não null — senão o
// frontend quebra ao iterar.
func TestListagemVaziaVemComoArrayVazio(t *testing.T) {
	api := montarAPIDeQuadro()
	cookie, _ := api.conta(t, "Ana", "ana@exemplo.com")

	rec := chamar(api, http.MethodGet, "/boards", "", cookie)

	if rec.Body.String() != `{"boards":[]}`+"\n" {
		t.Errorf("corpo = %q, esperado boards vazio como array", rec.Body.String())
	}
}

func TestQuadroDeTerceiroResponde404(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, _ := api.conta(t, "Ana", "ana@exemplo.com")
	cookieBob, _ := api.conta(t, "Bob", "bob@exemplo.com")
	boardID := api.criarQuadro(t, cookieAna, "Da Ana")

	for _, caso := range []struct {
		nome, metodo, caminho, corpo string
	}{
		{"ver", http.MethodGet, "/boards/" + boardID, ""},
		{"renomear", http.MethodPatch, "/boards/" + boardID, `{"titulo":"Invadido"}`},
		{"apagar", http.MethodDelete, "/boards/" + boardID, ""},
		{"criar coluna", http.MethodPost, "/boards/" + boardID + "/colunas", `{"titulo":"Invasora"}`},
	} {
		t.Run(caso.nome, func(t *testing.T) {
			rec := chamar(api, caso.metodo, caso.caminho, caso.corpo, cookieBob)
			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, esperado 404 (403 confirmaria que o quadro existe)", rec.Code)
			}
		})
	}
}

// O 403 é reservado para quem JÁ enxerga o quadro: aí a recusa não revela nada
// que a pessoa não saiba.
func TestLeitorRecebe403EmVezDe404(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, _ := api.conta(t, "Ana", "ana@exemplo.com")
	cookieBob, bobID := api.conta(t, "Bob", "bob@exemplo.com")
	boardID := api.criarQuadro(t, cookieAna, "Da Ana")

	vinculo, err := membro.Novo(boardID, bobID, membro.PapelLeitor)
	if err != nil {
		t.Fatalf("vínculo inválido: %v", err)
	}
	api.membros.Salvar(context.Background(), vinculo)

	if rec := chamar(api, http.MethodGet, "/boards/"+boardID, "", cookieBob); rec.Code != http.StatusOK {
		t.Fatalf("o leitor devia enxergar o quadro: %d %s", rec.Code, rec.Body)
	}

	rec := chamar(api, http.MethodPost, "/boards/"+boardID+"/colunas", `{"titulo":"Nova"}`, cookieBob)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, esperado 403", rec.Code)
	}
}

func TestMontarQuadroInteiroEDetalhar(t *testing.T) {
	api := montarAPIDeQuadro()
	cookie, _ := api.conta(t, "Ana", "ana@exemplo.com")
	boardID := api.criarQuadro(t, cookie, "Estudos")

	colunaRec := chamar(api, http.MethodPost, "/boards/"+boardID+"/colunas", `{"titulo":"A fazer"}`, cookie)
	if colunaRec.Code != http.StatusCreated {
		t.Fatalf("criar coluna: %d %s", colunaRec.Code, colunaRec.Body)
	}
	colunaID := idDoCorpo(t, colunaRec)

	cardRec := chamar(api, http.MethodPost, "/colunas/"+colunaID+"/cards",
		`{"titulo":"Migração","descricao":"com rollback"}`, cookie)
	if cardRec.Code != http.StatusCreated {
		t.Fatalf("criar card: %d %s", cardRec.Code, cardRec.Body)
	}

	rec := chamar(api, http.MethodGet, "/boards/"+boardID, "", cookie)
	var detalhe struct {
		Titulo  string `json:"titulo"`
		Papel   string `json:"papel"`
		Colunas []struct {
			Titulo string `json:"titulo"`
			Cards  []struct {
				Titulo    string `json:"titulo"`
				Descricao string `json:"descricao"`
				Version   int    `json:"version"`
			} `json:"cards"`
		} `json:"colunas"`
	}
	json.Unmarshal(rec.Body.Bytes(), &detalhe)

	if detalhe.Titulo != "Estudos" || detalhe.Papel != "dono" {
		t.Errorf("quadro = %+v", detalhe)
	}
	if len(detalhe.Colunas) != 1 || len(detalhe.Colunas[0].Cards) != 1 {
		t.Fatalf("estrutura = %+v", detalhe)
	}
	if detalhe.Colunas[0].Cards[0].Titulo != "Migração" || detalhe.Colunas[0].Cards[0].Version != 1 {
		t.Errorf("card = %+v", detalhe.Colunas[0].Cards[0])
	}
}

func TestCardDeQuadroAlheioResponde404(t *testing.T) {
	api := montarAPIDeQuadro()
	cookieAna, _ := api.conta(t, "Ana", "ana@exemplo.com")
	cookieBob, _ := api.conta(t, "Bob", "bob@exemplo.com")
	boardID := api.criarQuadro(t, cookieAna, "Da Ana")

	colunaID := idDoCorpo(t, chamar(api, http.MethodPost, "/boards/"+boardID+"/colunas", `{"titulo":"A fazer"}`, cookieAna))
	cardID := idDoCorpo(t, chamar(api, http.MethodPost, "/colunas/"+colunaID+"/cards", `{"titulo":"Segredo"}`, cookieAna))

	if rec := chamar(api, http.MethodPatch, "/cards/"+cardID, `{"titulo":"Invadido"}`, cookieBob); rec.Code != http.StatusNotFound {
		t.Errorf("editar: status = %d, esperado 404", rec.Code)
	}
	if rec := chamar(api, http.MethodDelete, "/cards/"+cardID, "", cookieBob); rec.Code != http.StatusNotFound {
		t.Errorf("apagar: status = %d, esperado 404", rec.Code)
	}
}

func TestTituloVazioResponde400(t *testing.T) {
	api := montarAPIDeQuadro()
	cookie, _ := api.conta(t, "Ana", "ana@exemplo.com")

	for nome, corpo := range map[string]string{
		"vazio":      `{"titulo":""}`,
		"só espaços": `{"titulo":"   "}`,
		"sem campo":  `{}`,
	} {
		t.Run(nome, func(t *testing.T) {
			if rec := chamar(api, http.MethodPost, "/boards", corpo, cookie); rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, esperado 400: %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestApagarQuadroResponde204ESomeDaListagem(t *testing.T) {
	api := montarAPIDeQuadro()
	cookie, _ := api.conta(t, "Ana", "ana@exemplo.com")
	boardID := api.criarQuadro(t, cookie, "Estudos")

	if rec := chamar(api, http.MethodDelete, "/boards/"+boardID, "", cookie); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, esperado 204", rec.Code)
	}

	if rec := chamar(api, http.MethodGet, "/boards/"+boardID, "", cookie); rec.Code != http.StatusNotFound {
		t.Errorf("depois de apagar: status = %d, esperado 404", rec.Code)
	}
}

func TestEditarCardDevolveVersaoIncrementada(t *testing.T) {
	api := montarAPIDeQuadro()
	cookie, _ := api.conta(t, "Ana", "ana@exemplo.com")
	boardID := api.criarQuadro(t, cookie, "Estudos")
	colunaID := idDoCorpo(t, chamar(api, http.MethodPost, "/boards/"+boardID+"/colunas", `{"titulo":"A fazer"}`, cookie))
	cardID := idDoCorpo(t, chamar(api, http.MethodPost, "/colunas/"+colunaID+"/cards", `{"titulo":"Rascunho"}`, cookie))

	rec := chamar(api, http.MethodPatch, "/cards/"+cardID, `{"titulo":"Definitivo","descricao":"pronto"}`, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}

	var c struct {
		Titulo  string `json:"titulo"`
		Version int    `json:"version"`
	}
	json.Unmarshal(rec.Body.Bytes(), &c)
	if c.Titulo != "Definitivo" || c.Version != 2 {
		t.Errorf("card = %+v, esperado version 2", c)
	}
}
