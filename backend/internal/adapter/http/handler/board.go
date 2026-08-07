package handler

import (
	"errors"
	"net/http"
	"time"

	"stacktrack/internal/adapter/http/dto"
	danexo "stacktrack/internal/domain/anexo"
	dboard "stacktrack/internal/domain/board"
	dcard "stacktrack/internal/domain/card"
	dchecklist "stacktrack/internal/domain/checklist"
	dcoluna "stacktrack/internal/domain/coluna"
	dcor "stacktrack/internal/domain/cor"
	detiqueta "stacktrack/internal/domain/etiqueta"
	"stacktrack/internal/domain/membro"
	"stacktrack/internal/domain/ordem"
	ucauth "stacktrack/internal/usecase/auth"
	ucboard "stacktrack/internal/usecase/board"

	"github.com/go-chi/chi/v5"
)

// BoardHandler concentra os handlers de quadros, colunas e cards.
type BoardHandler struct {
	quadros              *ucboard.QuadroUseCase
	colunas              *ucboard.ColunaUseCase
	cards                *ucboard.CardUseCase
	identidadeDoContexto func(r *http.Request) (ucauth.Identidade, bool)
}

// NovoBoardHandler cria uma instância de BoardHandler com os usecases injetados.
func NovoBoardHandler(
	quadros *ucboard.QuadroUseCase,
	colunas *ucboard.ColunaUseCase,
	cards *ucboard.CardUseCase,
	identidadeDoContexto func(r *http.Request) (ucauth.Identidade, bool),
) *BoardHandler {
	return &BoardHandler{quadros: quadros, colunas: colunas, cards: cards, identidadeDoContexto: identidadeDoContexto}
}

// Listar devolve os quadros de que o usuário autenticado participa.
func (h *BoardHandler) Listar(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	resumos, err := h.quadros.Listar(usuarioID)
	if err != nil {
		responderErroInterno(w, r, "erro ao listar quadros", err)
		return
	}

	boards := make([]dto.BoardResponse, 0, len(resumos))
	for _, resumo := range resumos {
		boards = append(boards, dto.BoardResponse{
			ID:       resumo.Board.ID,
			Titulo:   resumo.Board.Titulo,
			Papel:    string(resumo.Papel),
			Fundo:    resumo.Board.FundoEfetivo(),
			CriadoEm: resumo.Board.CriadoEm,
		})
	}
	responderJSON(w, http.StatusOK, dto.ListaBoardsResponse{Boards: boards})
}

// Criar cria um quadro, com quem criou como dono.
func (h *BoardHandler) Criar(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	req, ok := decodificarJSON[dto.TituloRequest](w, r)
	if !ok {
		return
	}

	b, err := h.quadros.Criar(usuarioID, req.Titulo)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao criar quadro", err)
		return
	}
	responderJSON(w, http.StatusCreated, dto.BoardResponse{
		ID: b.ID, Titulo: b.Titulo, Papel: string(membro.PapelDono),
		Fundo: b.FundoEfetivo(), CriadoEm: b.CriadoEm,
	})
}

// Detalhar devolve o quadro com colunas e cards.
func (h *BoardHandler) Detalhar(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	detalhado, err := h.quadros.Detalhar(chi.URLParam(r, "boardID"), usuarioID)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao carregar quadro", err)
		return
	}

	colunas := make([]dto.ColunaResponse, 0, len(detalhado.Colunas))
	for _, cc := range detalhado.Colunas {
		cards := make([]dto.CardResponse, 0, len(cc.Cards))
		for _, c := range cc.Cards {
			cards = append(cards, paraCardNoQuadro(c))
		}
		colunas = append(colunas, dto.ColunaResponse{
			ID: cc.Coluna.ID, BoardID: cc.Coluna.BoardID, Titulo: cc.Coluna.Titulo,
			Cor: string(cc.Coluna.Cor), Posicao: cc.Coluna.Posicao, Cards: cards,
		})
	}

	responderJSON(w, http.StatusOK, dto.BoardDetalhadoResponse{
		ID: detalhado.Board.ID, Titulo: detalhado.Board.Titulo,
		Papel: string(detalhado.Papel), Fundo: detalhado.Board.FundoEfetivo(),
		Colunas: colunas, Etiquetas: paraEtiquetasResponse(detalhado.Etiquetas),
	})
}

// Renomear troca o título do quadro. Só o dono.
func (h *BoardHandler) Renomear(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	req, ok := decodificarJSON[dto.TituloRequest](w, r)
	if !ok {
		return
	}

	b, err := h.quadros.Renomear(chi.URLParam(r, "boardID"), usuarioID, req.Titulo)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao renomear quadro", err)
		return
	}
	responderJSON(w, http.StatusOK, dto.BoardResponse{
		ID: b.ID, Titulo: b.Titulo, Papel: string(membro.PapelDono),
		Fundo: b.FundoEfetivo(), CriadoEm: b.CriadoEm,
	})
}

// Apagar remove o quadro e todo o conteúdo dele. Só o dono.
func (h *BoardHandler) Apagar(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	if err := h.quadros.Apagar(chi.URLParam(r, "boardID"), usuarioID); err != nil {
		responderErroDeQuadro(w, r, "erro ao apagar quadro", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CriarColuna acrescenta uma coluna no fim do quadro.
func (h *BoardHandler) CriarColuna(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	req, ok := decodificarJSON[dto.TituloRequest](w, r)
	if !ok {
		return
	}

	c, err := h.colunas.Criar(chi.URLParam(r, "boardID"), usuarioID, req.Titulo, dcor.Cor(req.Cor))
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao criar coluna", err)
		return
	}
	responderJSON(w, http.StatusCreated, dto.ColunaResponse{
		ID: c.ID, BoardID: c.BoardID, Titulo: c.Titulo, Cor: string(c.Cor),
		Posicao: c.Posicao, Cards: []dto.CardResponse{},
	})
}

// RenomearColuna troca o título da coluna.
func (h *BoardHandler) RenomearColuna(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	req, ok := decodificarJSON[dto.TituloRequest](w, r)
	if !ok {
		return
	}

	c, err := h.colunas.Renomear(chi.URLParam(r, "colunaID"), usuarioID, req.Titulo, dcor.Cor(req.Cor))
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao renomear coluna", err)
		return
	}
	responderJSON(w, http.StatusOK, dto.ColunaResponse{
		ID: c.ID, BoardID: c.BoardID, Titulo: c.Titulo, Cor: string(c.Cor),
		Posicao: c.Posicao, Cards: []dto.CardResponse{},
	})
}

// ApagarColuna remove a coluna e os cards dela.
func (h *BoardHandler) ApagarColuna(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	if err := h.colunas.Apagar(chi.URLParam(r, "colunaID"), usuarioID); err != nil {
		responderErroDeQuadro(w, r, "erro ao apagar coluna", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CriarCard acrescenta um card no fim da coluna.
func (h *BoardHandler) CriarCard(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	req, ok := decodificarJSON[dto.CardRequest](w, r)
	if !ok {
		return
	}

	c, err := h.cards.Criar(chi.URLParam(r, "colunaID"), usuarioID, req.Titulo, req.Descricao, dcor.Cor(req.Cor))
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao criar card", err)
		return
	}
	responderJSON(w, http.StatusCreated, paraCardResponse(*c))
}

// EditarCard troca título e descrição do card.
func (h *BoardHandler) EditarCard(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	req, ok := decodificarJSON[dto.CardRequest](w, r)
	if !ok {
		return
	}

	c, err := h.cards.Editar(chi.URLParam(r, "cardID"), usuarioID, req.Titulo, req.Descricao, dcor.Cor(req.Cor))
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao editar card", err)
		return
	}
	responderJSON(w, http.StatusOK, paraCardResponse(*c))
}

// ApagarCard remove o card.
func (h *BoardHandler) ApagarCard(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	if err := h.cards.Apagar(chi.URLParam(r, "cardID"), usuarioID); err != nil {
		responderErroDeQuadro(w, r, "erro ao apagar card", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// usuario extrai o id de quem está autenticado. Responde 401 e devolve false
// quando o middleware de autenticação não pôs identidade no contexto — o que
// só acontece se a rota for montada fora dele.
func (h *BoardHandler) usuario(w http.ResponseWriter, r *http.Request) (string, bool) {
	identidade, ok := h.identidadeDoContexto(r)
	if !ok {
		responderErro(w, http.StatusUnauthorized, "não autenticado")
		return "", false
	}
	return identidade.UsuarioID, true
}

// paraCardResponse converte o card do domínio, sem os resumos — usado onde a
// resposta é de um card só (criar, editar), e a tela já sabe o resto.
func paraCardResponse(c dcard.Card) dto.CardResponse {
	return dto.CardResponse{
		ID: c.ID, ColunaID: c.ColunaID, Titulo: c.Titulo, Descricao: c.Descricao,
		Cor: string(c.Cor), Posicao: c.Posicao, Version: c.Version, Prazo: c.Prazo,
		Vencido:   c.Vencido(time.Now()),
		Etiquetas: []string{},
	}
}

// paraCardNoQuadro converte o card com os selos que a tela do quadro mostra.
func paraCardNoQuadro(c ucboard.CardNoQuadro) dto.CardResponse {
	resposta := paraCardResponse(c.Card)
	resposta.Etiquetas = c.Etiquetas
	resposta.Checklist = dto.ProgressoResponse{Concluidos: c.Checklist.Concluidos, Total: c.Checklist.Total}
	resposta.QtdAnexos = c.QtdAnexos
	return resposta
}

func paraEtiquetaResponse(e detiqueta.Etiqueta) dto.EtiquetaResponse {
	return dto.EtiquetaResponse{ID: e.ID, Nome: e.Nome, Cor: string(e.Cor), Posicao: e.Posicao}
}

func paraEtiquetasResponse(etiquetas []detiqueta.Etiqueta) []dto.EtiquetaResponse {
	lista := make([]dto.EtiquetaResponse, 0, len(etiquetas))
	for _, e := range etiquetas {
		lista = append(lista, paraEtiquetaResponse(e))
	}
	return lista
}

// responderErroDeQuadro traduz os erros do domínio em códigos HTTP.
//
// "Não encontrado" cobre dois casos diferentes de propósito: o recurso não
// existe, ou quem pediu não participa do quadro. Responder 403 no segundo
// confirmaria a existência do quadro, e isso basta para varrer ids e mapear o
// que os outros têm. O 403 fica só para quem JÁ enxerga o quadro e esbarra no
// próprio papel — aí a recusa não revela nada de novo.
func responderErroDeQuadro(w http.ResponseWriter, r *http.Request, contexto string, err error) {
	switch {
	case errors.Is(err, dboard.ErrNaoEncontrado),
		errors.Is(err, dcoluna.ErrNaoEncontrada),
		errors.Is(err, dcard.ErrNaoEncontrado),
		errors.Is(err, detiqueta.ErrNaoEncontrada),
		errors.Is(err, dchecklist.ErrNaoEncontrada),
		errors.Is(err, dchecklist.ErrItemNaoEncontrado),
		errors.Is(err, danexo.ErrNaoEncontrado):
		responderErro(w, http.StatusNotFound, err.Error())
	case errors.Is(err, membro.ErrSemPermissao):
		responderErro(w, http.StatusForbidden, err.Error())
	// 413 e não 400: o corpo em si é válido, o que não serve é o tamanho — e é
	// o código que o navegador e os proxies entendem como "arquivo grande".
	// 409: a entrada está correta, o estado do quadro é que não comporta o
	// movimento. Ver ordem.ErrSemEspaco — a fase 9 remove essa possibilidade.
	case errors.Is(err, ordem.ErrSemEspaco):
		responderErro(w, http.StatusConflict, err.Error())
	case errors.Is(err, danexo.ErrArquivoGrande):
		responderErro(w, http.StatusRequestEntityTooLarge, err.Error())
	// 415: o tipo do conteúdo é que não é aceito.
	case errors.Is(err, danexo.ErrTipoNaoPermitido):
		responderErro(w, http.StatusUnsupportedMediaType, err.Error())
	case errors.Is(err, dboard.ErrTituloObrigatorio),
		errors.Is(err, dboard.ErrTituloLongo),
		errors.Is(err, dboard.ErrFundoInvalido),
		errors.Is(err, dcard.ErrTituloObrigatorio),
		errors.Is(err, dcard.ErrTituloLongo),
		errors.Is(err, dcard.ErrDescricaoLonga),
		errors.Is(err, membro.ErrPapelInvalido),
		errors.Is(err, dcor.ErrInvalida),
		errors.Is(err, detiqueta.ErrNomeObrigatorio),
		errors.Is(err, detiqueta.ErrNomeLongo),
		errors.Is(err, detiqueta.ErrCorInvalida),
		errors.Is(err, dchecklist.ErrTituloObrigatorio),
		errors.Is(err, dchecklist.ErrTituloLongo),
		errors.Is(err, dchecklist.ErrTextoObrigatorio),
		errors.Is(err, dchecklist.ErrTextoLongo),
		errors.Is(err, danexo.ErrNomeObrigatorio),
		errors.Is(err, danexo.ErrNomeLongo),
		errors.Is(err, danexo.ErrURLInvalida),
		errors.Is(err, danexo.ErrArquivoVazio):
		responderErro(w, http.StatusBadRequest, err.Error())
	default:
		responderErroInterno(w, r, contexto, err)
	}
}

// DefinirPrazo marca ou limpa a data de entrega do card.
func (h *BoardHandler) DefinirPrazo(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	req, ok := decodificarJSON[dto.PrazoRequest](w, r)
	if !ok {
		return
	}

	c, err := h.cards.DefinirPrazo(chi.URLParam(r, "cardID"), usuarioID, req.Prazo)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao definir prazo", err)
		return
	}
	responderJSON(w, http.StatusOK, paraCardResponse(*c))
}

// DetalharCard devolve o card com etiquetas, checklists e anexos — é o que o
// modal mostra, numa requisição só.
func (h *BoardHandler) DetalharCard(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	detalhe, err := h.cards.Detalhar(chi.URLParam(r, "cardID"), usuarioID)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao carregar card", err)
		return
	}

	checklists := make([]dto.ChecklistResponse, 0, len(detalhe.Checklists))
	for _, lista := range detalhe.Checklists {
		itens := make([]dto.ChecklistItemResponse, 0, len(lista.Itens))
		for _, item := range lista.Itens {
			itens = append(itens, paraItemResponse(item))
		}
		checklists = append(checklists, dto.ChecklistResponse{
			ID: lista.Checklist.ID, CardID: lista.Checklist.CardID,
			Titulo: lista.Checklist.Titulo, Posicao: lista.Checklist.Posicao, Itens: itens,
		})
	}

	responderJSON(w, http.StatusOK, dto.CardDetalhadoResponse{
		CardResponse:    paraCardResponse(detalhe.Card),
		BoardID:         detalhe.BoardID,
		EtiquetasDoCard: paraEtiquetasResponse(detalhe.Etiquetas),
		Checklists:      checklists,
		Anexos:          paraAnexosResponse(detalhe.Anexos),
	})
}

// DefinirFundo troca o fundo do quadro. Só o dono.
func (h *BoardHandler) DefinirFundo(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	req, ok := decodificarJSON[dto.FundoRequest](w, r)
	if !ok {
		return
	}

	b, err := h.quadros.DefinirFundo(chi.URLParam(r, "boardID"), usuarioID, req.Fundo)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao trocar o fundo", err)
		return
	}
	responderJSON(w, http.StatusOK, dto.BoardResponse{
		ID: b.ID, Titulo: b.Titulo, Papel: string(membro.PapelDono),
		Fundo: b.FundoEfetivo(), CriadoEm: b.CriadoEm,
	})
}

func paraItemResponse(i dchecklist.Item) dto.ChecklistItemResponse {
	return dto.ChecklistItemResponse{
		ID: i.ID, ChecklistID: i.ChecklistID, Texto: i.Texto,
		Concluido: i.Concluido, Posicao: i.Posicao,
	}
}

func paraAnexoResponse(a danexo.Anexo) dto.AnexoResponse {
	return dto.AnexoResponse{
		ID: a.ID, CardID: a.CardID, Tipo: string(a.Tipo), Nome: a.Nome,
		URL: a.URL, Tamanho: a.Tamanho, MIME: a.MIME, CriadoEm: a.CriadoEm,
	}
}

func paraAnexosResponse(anexos []danexo.Anexo) []dto.AnexoResponse {
	lista := make([]dto.AnexoResponse, 0, len(anexos))
	for _, a := range anexos {
		lista = append(lista, paraAnexoResponse(a))
	}
	return lista
}

// MoverCard leva o card para outra coluna e/ou outra posição.
func (h *BoardHandler) MoverCard(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	req, ok := decodificarJSON[dto.MoverRequest](w, r)
	if !ok {
		return
	}

	c, err := h.cards.Mover(chi.URLParam(r, "cardID"), usuarioID, req.ColunaID,
		ucboard.Vizinhos{AnteriorID: req.AnteriorID, ProximoID: req.ProximoID})
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao mover card", err)
		return
	}
	responderJSON(w, http.StatusOK, paraCardResponse(*c))
}

// MoverColuna reposiciona a coluna dentro do quadro.
func (h *BoardHandler) MoverColuna(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	req, ok := decodificarJSON[dto.MoverRequest](w, r)
	if !ok {
		return
	}

	c, err := h.colunas.Mover(chi.URLParam(r, "colunaID"), usuarioID,
		ucboard.Vizinhos{AnteriorID: req.AnteriorID, ProximoID: req.ProximoID})
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao mover coluna", err)
		return
	}
	responderJSON(w, http.StatusOK, dto.ColunaResponse{
		ID: c.ID, BoardID: c.BoardID, Titulo: c.Titulo, Posicao: c.Posicao,
		Cards: []dto.CardResponse{},
	})
}
