package handler

import (
	"errors"
	"net/http"

	"kanbango/internal/adapter/http/dto"
	dboard "kanbango/internal/domain/board"
	dcard "kanbango/internal/domain/card"
	dcoluna "kanbango/internal/domain/coluna"
	"kanbango/internal/domain/membro"
	ucauth "kanbango/internal/usecase/auth"
	ucboard "kanbango/internal/usecase/board"

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
		ID: b.ID, Titulo: b.Titulo, Papel: string(membro.PapelDono), CriadoEm: b.CriadoEm,
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
			cards = append(cards, paraCardResponse(c.ID, c.ColunaID, c.Titulo, c.Descricao, c.Posicao, c.Version))
		}
		colunas = append(colunas, dto.ColunaResponse{
			ID: cc.Coluna.ID, BoardID: cc.Coluna.BoardID, Titulo: cc.Coluna.Titulo,
			Posicao: cc.Coluna.Posicao, Cards: cards,
		})
	}

	responderJSON(w, http.StatusOK, dto.BoardDetalhadoResponse{
		ID: detalhado.Board.ID, Titulo: detalhado.Board.Titulo,
		Papel: string(detalhado.Papel), Colunas: colunas,
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
		ID: b.ID, Titulo: b.Titulo, Papel: string(membro.PapelDono), CriadoEm: b.CriadoEm,
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

	c, err := h.colunas.Criar(chi.URLParam(r, "boardID"), usuarioID, req.Titulo)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao criar coluna", err)
		return
	}
	responderJSON(w, http.StatusCreated, dto.ColunaResponse{
		ID: c.ID, BoardID: c.BoardID, Titulo: c.Titulo, Posicao: c.Posicao, Cards: []dto.CardResponse{},
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

	c, err := h.colunas.Renomear(chi.URLParam(r, "colunaID"), usuarioID, req.Titulo)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao renomear coluna", err)
		return
	}
	responderJSON(w, http.StatusOK, dto.ColunaResponse{
		ID: c.ID, BoardID: c.BoardID, Titulo: c.Titulo, Posicao: c.Posicao, Cards: []dto.CardResponse{},
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

	c, err := h.cards.Criar(chi.URLParam(r, "colunaID"), usuarioID, req.Titulo, req.Descricao)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao criar card", err)
		return
	}
	responderJSON(w, http.StatusCreated, paraCardResponse(c.ID, c.ColunaID, c.Titulo, c.Descricao, c.Posicao, c.Version))
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

	c, err := h.cards.Editar(chi.URLParam(r, "cardID"), usuarioID, req.Titulo, req.Descricao)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao editar card", err)
		return
	}
	responderJSON(w, http.StatusOK, paraCardResponse(c.ID, c.ColunaID, c.Titulo, c.Descricao, c.Posicao, c.Version))
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

func paraCardResponse(id, colunaID, titulo, descricao string, posicao float64, version int) dto.CardResponse {
	return dto.CardResponse{
		ID: id, ColunaID: colunaID, Titulo: titulo, Descricao: descricao,
		Posicao: posicao, Version: version,
	}
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
		errors.Is(err, dcard.ErrNaoEncontrado):
		responderErro(w, http.StatusNotFound, err.Error())
	case errors.Is(err, membro.ErrSemPermissao):
		responderErro(w, http.StatusForbidden, err.Error())
	case errors.Is(err, dboard.ErrTituloObrigatorio),
		errors.Is(err, dboard.ErrTituloLongo),
		errors.Is(err, dcard.ErrTituloObrigatorio),
		errors.Is(err, dcard.ErrTituloLongo),
		errors.Is(err, dcard.ErrDescricaoLonga),
		errors.Is(err, membro.ErrPapelInvalido):
		responderErro(w, http.StatusBadRequest, err.Error())
	default:
		responderErroInterno(w, r, contexto, err)
	}
}
