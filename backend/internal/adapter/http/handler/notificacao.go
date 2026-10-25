package handler

import (
	"errors"
	"net/http"

	"stacktrack/internal/adapter/http/dto"
	dnotificacao "stacktrack/internal/domain/notificacao"
	ucauth "stacktrack/internal/usecase/auth"
	ucnotificacao "stacktrack/internal/usecase/notificacao"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// NotificacaoHandler concentra as rotas do sino de cada pessoa. O stream de
// tempo real (SSE) é de outro adaptador, em adapter/http/sse.
type NotificacaoHandler struct {
	notificacoes         *ucnotificacao.UseCase
	identidadeDoContexto func(r *http.Request) (ucauth.Identidade, bool)
}

// NovoNotificacaoHandler cria o handler com o caso de uso injetado.
func NovoNotificacaoHandler(
	notificacoes *ucnotificacao.UseCase,
	identidadeDoContexto func(r *http.Request) (ucauth.Identidade, bool),
) *NotificacaoHandler {
	return &NotificacaoHandler{notificacoes: notificacoes, identidadeDoContexto: identidadeDoContexto}
}

// Listar devolve uma página dos avisos da pessoa e quantos ela não leu.
// `antesDe` é o id do último aviso da página anterior.
func (h *NotificacaoHandler) Listar(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	antesDe := r.URL.Query().Get("antesDe")
	if antesDe != "" {
		// A mesma régua dos ids da URL: um cursor que não é UUID é recusado
		// antes de virar consulta.
		if _, err := uuid.Parse(antesDe); err != nil {
			responderErro(w, http.StatusBadRequest, "antesDe precisa ser o id de uma notificação")
			return
		}
	}

	pagina, err := h.notificacoes.Listar(r.Context(), usuarioID, antesDe)
	if err != nil {
		responderErroInterno(w, r, "erro ao listar as notificações", err)
		return
	}
	lista := make([]dto.NotificacaoResponse, 0, len(pagina.Itens))
	for _, item := range pagina.Itens {
		lista = append(lista, paraNotificacaoResponse(item))
	}
	responderJSON(w, http.StatusOK, dto.NotificacoesResponse{
		Notificacoes: lista, NaoLidas: pagina.NaoLidas, TemMais: pagina.TemMais,
	})
}

// Detalhar devolve um aviso da pessoa — é o que a página do link do email
// consulta. 404 para aviso de outra pessoa, de quadro de que ela já não
// participa, ou que não existe mais.
func (h *NotificacaoHandler) Detalhar(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	item, err := h.notificacoes.Detalhar(r.Context(), chi.URLParam(r, "notificacaoID"), usuarioID)
	if err != nil {
		responderErroDeNotificacao(w, r, "erro ao ler a notificação", err)
		return
	}
	responderJSON(w, http.StatusOK, paraNotificacaoResponse(*item))
}

// MarcarLida marca o aviso como lido (204) e cancela o email dele que ainda não
// saiu. Marcar de novo não é erro.
func (h *NotificacaoHandler) MarcarLida(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	if err := h.notificacoes.MarcarLida(r.Context(), chi.URLParam(r, "notificacaoID"), usuarioID); err != nil {
		responderErroDeNotificacao(w, r, "erro ao marcar a notificação como lida", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MarcarTodasLidas marca todos os avisos não lidos da pessoa (204).
func (h *NotificacaoHandler) MarcarTodasLidas(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	if err := h.notificacoes.MarcarTodasLidas(r.Context(), usuarioID); err != nil {
		responderErroInterno(w, r, "erro ao marcar as notificações como lidas", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *NotificacaoHandler) usuario(w http.ResponseWriter, r *http.Request) (string, bool) {
	identidade, ok := h.identidadeDoContexto(r)
	if !ok {
		responderErro(w, http.StatusUnauthorized, "não autenticado")
		return "", false
	}
	return identidade.UsuarioID, true
}

func responderErroDeNotificacao(w http.ResponseWriter, r *http.Request, contexto string, err error) {
	if errors.Is(err, dnotificacao.ErrNaoEncontrada) {
		responderErro(w, http.StatusNotFound, err.Error())
		return
	}
	responderErroInterno(w, r, contexto, err)
}

func paraNotificacaoResponse(i ucnotificacao.Item) dto.NotificacaoResponse {
	return dto.NotificacaoResponse{
		ID: i.ID, Tipo: string(i.Tipo), QuadroID: i.BoardID, Quadro: i.TituloQuadro,
		CardID: i.CardID, Card: i.TituloCard, Autor: i.AutorNome, Trecho: i.Trecho,
		Prazo: i.Prazo, CriadaEm: i.CriadaEm, Lida: i.LidaEm != nil,
		Situacao: string(i.Situacao),
	}
}
