package handler

import (
	"errors"
	"net/http"

	"stacktrack/internal/adapter/http/dto"
	"stacktrack/internal/domain/recuperacao"
	ucauth "stacktrack/internal/usecase/auth"

	"github.com/go-chi/chi/v5"
)

// mensagemDoPedido é a resposta do "esqueci minha senha", igual para email com
// conta e sem conta.
const mensagemDoPedido = "Se existe uma conta com esse email, mandamos um link para criar uma senha nova."

// RecuperacaoHandler expõe a recuperação de senha. É um handler à parte do de
// autenticação porque as três rotas são públicas e têm tetos próprios.
type RecuperacaoHandler struct {
	uc           *ucauth.RecuperarSenhaUseCase
	cookieSeguro bool
}

// NovoRecuperacaoHandler cria o handler.
func NovoRecuperacaoHandler(uc *ucauth.RecuperarSenhaUseCase, cookieSeguro bool) *RecuperacaoHandler {
	return &RecuperacaoHandler{uc: uc, cookieSeguro: cookieSeguro}
}

// Pedir manda o link de senha nova. Responde 202 com a mesma mensagem para
// email com conta, sem conta e com o teto de pedidos atingido — a diferença
// revelaria quem tem conta. Só o formato do email (400) e falha interna (500)
// mudam a resposta.
func (h *RecuperacaoHandler) Pedir(w http.ResponseWriter, r *http.Request) {
	req, ok := decodificarJSON[dto.PedidoDeRecuperacaoRequest](w, r)
	if !ok {
		return
	}
	if err := h.uc.Pedir(r.Context(), req.Email.String()); err != nil {
		responderErroInterno(w, r, "erro ao pedir a recuperação de senha", err)
		return
	}
	responderJSON(w, http.StatusAccepted, dto.PedidoDeRecuperacaoResponse{Mensagem: mensagemDoPedido})
}

// Consultar diz de qual conta é o link e até quando ele vale. Responde 404
// para link inexistente, vencido ou já usado — os três iguais.
func (h *RecuperacaoHandler) Consultar(w http.ResponseWriter, r *http.Request) {
	aberta, err := h.uc.Consultar(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		h.responderErro(w, r, err)
		return
	}
	responderJSON(w, http.StatusOK, dto.RecuperacaoResponse{EmailMascarado: aberta.EmailMascarado, ExpiraEm: aberta.ExpiraEm})
}

// Redefinir troca a senha e já abre a sessão (200 + cookie). Responde 400 para
// senha que o cadastro também recusaria — sem gastar o link — e 404 para link
// inexistente, vencido ou já usado.
func (h *RecuperacaoHandler) Redefinir(w http.ResponseWriter, r *http.Request) {
	req, ok := decodificarJSON[dto.RedefinirSenhaRequest](w, r)
	if !ok {
		return
	}
	out, err := h.uc.Redefinir(r.Context(), chi.URLParam(r, "token"), req.Senha)
	if err != nil {
		h.responderErro(w, r, err)
		return
	}
	http.SetCookie(w, novoCookieSessao(out.Token, out.ExpiraEm, h.cookieSeguro))
	responderJSON(w, http.StatusOK, dto.SessaoResponse{ID: out.UsuarioID, Nome: out.Nome, Email: out.Email, EmailConfirmado: out.EmailConfirmado})
}

func (h *RecuperacaoHandler) responderErro(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, recuperacao.ErrInvalida):
		responderErro(w, http.StatusNotFound, err.Error())
	case erroDeValidacaoDoDominio(err):
		responderErro(w, http.StatusBadRequest, err.Error())
	default:
		responderErroInterno(w, r, "erro na recuperação de senha", err)
	}
}
