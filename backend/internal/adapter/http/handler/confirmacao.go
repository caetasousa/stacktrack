package handler

import (
	"errors"
	"net/http"

	"stacktrack/internal/adapter/http/dto"
	"stacktrack/internal/domain/confirmacao"
	ucauth "stacktrack/internal/usecase/auth"

	"github.com/go-chi/chi/v5"
)

// ConfirmacaoHandler expõe a confirmação de email da conta.
type ConfirmacaoHandler struct {
	uc                   *ucauth.ConfirmarEmailUseCase
	identidadeDoContexto func(r *http.Request) (ucauth.Identidade, bool)
}

// NovoConfirmacaoHandler cria o handler.
func NovoConfirmacaoHandler(
	uc *ucauth.ConfirmarEmailUseCase,
	identidadeDoContexto func(r *http.Request) (ucauth.Identidade, bool),
) *ConfirmacaoHandler {
	return &ConfirmacaoHandler{uc: uc, identidadeDoContexto: identidadeDoContexto}
}

// Reenviar manda de novo o link de confirmação para a conta logada (202). A
// resposta é a mesma com o teto de pedidos atingido e para conta já confirmada.
func (h *ConfirmacaoHandler) Reenviar(w http.ResponseWriter, r *http.Request) {
	identidade, ok := h.identidadeDoContexto(r)
	if !ok {
		responderErro(w, http.StatusUnauthorized, "não autenticado")
		return
	}
	if err := h.uc.Enviar(r.Context(), identidade.UsuarioID); err != nil {
		responderErroInterno(w, r, "erro ao reenviar a confirmação", err)
		return
	}
	responderJSON(w, http.StatusAccepted, dto.ConfirmacaoPedidaResponse{
		Mensagem: "Mandamos um link para o seu email. Ele vale por sete dias.",
	})
}

// Confirmar confirma a conta do link (204), sem exigir sessão: quem abre o
// link prova a caixa de entrada. 404 para link inexistente, vencido ou usado.
func (h *ConfirmacaoHandler) Confirmar(w http.ResponseWriter, r *http.Request) {
	err := h.uc.Confirmar(r.Context(), chi.URLParam(r, "token"))
	switch {
	case errors.Is(err, confirmacao.ErrInvalida):
		responderErro(w, http.StatusNotFound, err.Error())
	case err != nil:
		responderErroInterno(w, r, "erro ao confirmar o email", err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
