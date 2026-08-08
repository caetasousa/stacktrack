package handler

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"stacktrack/internal/adapter/http/dto"
	dconvite "stacktrack/internal/domain/convite"
	"stacktrack/internal/domain/membro"
	ucauth "stacktrack/internal/usecase/auth"
	ucboard "stacktrack/internal/usecase/board"

	"github.com/go-chi/chi/v5"
)

// MembroHandler concentra os handlers de quem participa do quadro e dos
// convites.
//
// origemFrontend é a base do link de convite: o backend não sabe em que
// endereço o frontend está publicado, e montar o link no cliente espalharia a
// mesma regra por toda tela que precisasse dele.
type MembroHandler struct {
	membros              *ucboard.MembroUseCase
	origemFrontend       string
	identidadeDoContexto func(r *http.Request) (ucauth.Identidade, bool)
}

// NovoMembroHandler cria uma instância de MembroHandler com o usecase injetado.
func NovoMembroHandler(
	membros *ucboard.MembroUseCase,
	origemFrontend string,
	identidadeDoContexto func(r *http.Request) (ucauth.Identidade, bool),
) *MembroHandler {
	return &MembroHandler{
		membros:              membros,
		origemFrontend:       origemFrontend,
		identidadeDoContexto: identidadeDoContexto,
	}
}

// Listar devolve quem participa do quadro e, para o dono, os convites
// pendentes.
func (h *MembroHandler) Listar(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	boardID := chi.URLParam(r, "boardID")

	participantes, err := h.membros.Listar(r.Context(), boardID, usuarioID)
	if err != nil {
		responderErroDeMembro(w, r, "erro ao listar membros", err)
		return
	}

	resposta := dto.MembrosResponse{
		Membros:  paraMembrosResponse(participantes),
		Convites: []dto.ConvitePendenteResponse{},
	}

	// Só o dono vê os convites. Para os demais o campo vem vazio, e não com
	// erro: a tela é a mesma para todo mundo, só mostra menos.
	if convites, err := h.membros.ListarConvites(r.Context(), boardID, usuarioID); err == nil {
		resposta.Convites = paraConvitesResponse(convites)
	} else if !errors.Is(err, membro.ErrSemPermissao) {
		responderErroDeMembro(w, r, "erro ao listar convites", err)
		return
	}

	responderJSON(w, http.StatusOK, resposta)
}

// Convidar acrescenta alguém ao quadro pelo email. Responde 201 com o membro
// criado (quando a pessoa já tinha conta) ou com o link do convite.
func (h *MembroHandler) Convidar(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	req, ok := decodificarJSON[dto.ConviteRequest](w, r)
	if !ok {
		return
	}

	resultado, err := h.membros.Convidar(r.Context(),
		chi.URLParam(r, "boardID"), usuarioID, req.Email.String(), membro.Papel(req.Papel),
	)
	if err != nil {
		responderErroDeMembro(w, r, "erro ao convidar", err)
		return
	}

	resposta := dto.ConviteCriadoResponse{Adicionado: resultado.Adicionado}
	if resultado.Adicionado {
		m := paraMembroResponse(*resultado.Participante)
		resposta.Membro = &m
	} else {
		c := paraConviteResponse(*resultado.Convite)
		resposta.Convite = &c
		resposta.Link = h.linkDoConvite(resultado.Token)
	}
	responderJSON(w, http.StatusCreated, resposta)
}

// AlterarPapel troca o papel de quem participa. Só o dono.
func (h *MembroHandler) AlterarPapel(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	req, ok := decodificarJSON[dto.PapelRequest](w, r)
	if !ok {
		return
	}

	p, err := h.membros.AlterarPapel(r.Context(),
		chi.URLParam(r, "boardID"), usuarioID, chi.URLParam(r, "usuarioID"), membro.Papel(req.Papel),
	)
	if err != nil {
		responderErroDeMembro(w, r, "erro ao trocar papel", err)
		return
	}
	responderJSON(w, http.StatusOK, paraMembroResponse(*p))
}

// Remover tira alguém do quadro. Só o dono, e nunca o último dono.
func (h *MembroHandler) Remover(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	err := h.membros.Remover(r.Context(), chi.URLParam(r, "boardID"), usuarioID, chi.URLParam(r, "usuarioID"))
	if err != nil {
		responderErroDeMembro(w, r, "erro ao remover membro", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RevogarConvite apaga um convite pendente, invalidando o link entregue.
func (h *MembroHandler) RevogarConvite(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	if err := h.membros.RevogarConvite(r.Context(), chi.URLParam(r, "conviteID"), usuarioID); err != nil {
		responderErroDeMembro(w, r, "erro ao revogar convite", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DetalharConvite descreve o convite para quem abriu o link. Rota pública: quem
// foi convidado costuma ainda não ter conta, e precisa ver do que se trata antes
// de criar uma.
func (h *MembroHandler) DetalharConvite(w http.ResponseWriter, r *http.Request) {
	detalhe, err := h.membros.DetalharConvite(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		responderErroDeMembro(w, r, "erro ao carregar convite", err)
		return
	}
	responderJSON(w, http.StatusOK, dto.ConviteDetalheResponse{
		Quadro:       detalhe.TituloQuadro,
		Email:        detalhe.Email,
		Papel:        string(detalhe.Papel),
		ConvidadoPor: detalhe.ConvidadoPor,
	})
}

// AceitarConvite transforma o convite em participação. Exige sessão da conta
// com o email convidado.
func (h *MembroHandler) AceitarConvite(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	b, papel, err := h.membros.Aceitar(r.Context(), chi.URLParam(r, "token"), usuarioID)
	if err != nil {
		responderErroDeMembro(w, r, "erro ao aceitar convite", err)
		return
	}
	responderJSON(w, http.StatusOK, dto.BoardResponse{
		ID: b.ID, Titulo: b.Titulo, Papel: string(papel), CriadoEm: b.CriadoEm,
	})
}

func (h *MembroHandler) usuario(w http.ResponseWriter, r *http.Request) (string, bool) {
	identidade, ok := h.identidadeDoContexto(r)
	if !ok {
		responderErro(w, http.StatusUnauthorized, "não autenticado")
		return "", false
	}
	return identidade.UsuarioID, true
}

// linkDoConvite monta a URL que o dono vai entregar. O token vai no caminho, e
// não em query string: query string vaza com mais facilidade em log de proxy e
// em cabeçalho Referer.
func (h *MembroHandler) linkDoConvite(tokenPuro string) string {
	return h.origemFrontend + "/convite/" + url.PathEscape(tokenPuro)
}

func paraMembroResponse(p ucboard.Participante) dto.MembroResponse {
	return dto.MembroResponse{
		UsuarioID: p.UsuarioID, Nome: p.Nome, Email: p.Email,
		Papel: string(p.Papel), DesdeEm: p.CriadoEm,
	}
}

func paraMembrosResponse(participantes []ucboard.Participante) []dto.MembroResponse {
	lista := make([]dto.MembroResponse, 0, len(participantes))
	for _, p := range participantes {
		lista = append(lista, paraMembroResponse(p))
	}
	return lista
}

func paraConviteResponse(c dconvite.Convite) dto.ConvitePendenteResponse {
	return dto.ConvitePendenteResponse{
		ID: c.ID, Email: c.Email, Papel: string(c.Papel),
		ExpiraEm: c.ExpiraEm, Expirado: !c.Pendente(time.Now()),
	}
}

func paraConvitesResponse(convites []dconvite.Convite) []dto.ConvitePendenteResponse {
	lista := make([]dto.ConvitePendenteResponse, 0, len(convites))
	for _, c := range convites {
		lista = append(lista, paraConviteResponse(c))
	}
	return lista
}

// responderErroDeMembro traduz os erros de participação em códigos HTTP.
//
// Convite inválido, vencido ou já aceito respondem todos 404 com a mesma
// mensagem: distinguir os casos ajudaria quem estivesse testando links.
func responderErroDeMembro(w http.ResponseWriter, r *http.Request, contexto string, err error) {
	switch {
	case errors.Is(err, dconvite.ErrInvalido):
		responderErro(w, http.StatusNotFound, err.Error())
	case errors.Is(err, membro.ErrNaoEMembro):
		responderErro(w, http.StatusNotFound, err.Error())
	case errors.Is(err, dconvite.ErrOutroDestinatario):
		responderErro(w, http.StatusForbidden, err.Error())
	case errors.Is(err, dconvite.ErrJaEMembro),
		errors.Is(err, dconvite.ErrJaConvidado),
		errors.Is(err, dconvite.ErrNaoConvidaODono):
		responderErro(w, http.StatusConflict, err.Error())
	case errors.Is(err, membro.ErrSemDono),
		errors.Is(err, dconvite.ErrEmailObrigatorio):
		responderErro(w, http.StatusBadRequest, err.Error())
	default:
		// Quadro não encontrado, sem permissão e papel inválido já são
		// traduzidos pelo tratamento das rotas de quadro.
		responderErroDeQuadro(w, r, contexto, err)
	}
}
