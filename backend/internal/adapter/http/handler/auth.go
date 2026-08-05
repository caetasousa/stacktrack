// Package handler contém os handlers HTTP da aplicação: eles traduzem
// requisição em chamada de usecase e resultado em resposta, sem regra de
// negócio própria.
package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"kanbango/internal/adapter/http/dto"
	"kanbango/internal/domain/usuario"
	"kanbango/internal/pkg/logging"
	ucauth "kanbango/internal/usecase/auth"
)

// AuthHandler concentra os handlers de cadastro, login, logout e perfil.
//
// identidadeDoContexto extrai a identidade posta no contexto pelo middleware
// de autenticação — recebida como função para evitar um import cycle entre os
// pacotes handler e middleware.
type AuthHandler struct {
	cadastrar            *ucauth.CadastrarUseCase
	login                *ucauth.LoginUseCase
	logout               *ucauth.LogoutUseCase
	perfil               *ucauth.PerfilUseCase
	cookieSeguro         bool
	limitadorPorConta    *LimitadorPorConta
	identidadeDoContexto func(r *http.Request) (ucauth.Identidade, bool)
}

// NovoAuthHandler cria uma instância de AuthHandler com os usecases injetados.
// limitadorPorConta pode ser nil (teto por conta desligado).
func NovoAuthHandler(
	cadastrar *ucauth.CadastrarUseCase,
	login *ucauth.LoginUseCase,
	logout *ucauth.LogoutUseCase,
	perfil *ucauth.PerfilUseCase,
	cookieSeguro bool,
	limitadorPorConta *LimitadorPorConta,
	identidadeDoContexto func(r *http.Request) (ucauth.Identidade, bool),
) *AuthHandler {
	return &AuthHandler{
		cadastrar:            cadastrar,
		login:                login,
		logout:               logout,
		perfil:               perfil,
		cookieSeguro:         cookieSeguro,
		limitadorPorConta:    limitadorPorConta,
		identidadeDoContexto: identidadeDoContexto,
	}
}

// Cadastrar cria uma conta e já abre a sessão dela, respondendo 201 com o
// cookie de sessão. Responde 409 se o email já pertencer a alguém e 400 nos
// erros de validação do domínio.
func (h *AuthHandler) Cadastrar(w http.ResponseWriter, r *http.Request) {
	req, ok := decodificarJSON[dto.CadastroRequest](w, r)
	if !ok {
		return
	}

	out, err := h.cadastrar.Executar(ucauth.CadastroInput{
		Nome:  req.Nome,
		Email: req.Email.String(),
		Senha: req.Senha,
	})
	if err != nil {
		if errors.Is(err, usuario.ErrEmailEmUso) {
			responderErro(w, http.StatusConflict, err.Error())
			return
		}
		if erroDeValidacaoDoDominio(err) {
			responderErro(w, http.StatusBadRequest, err.Error())
			return
		}
		responderErroInterno(w, r, "erro ao cadastrar usuário", err)
		return
	}

	http.SetCookie(w, novoCookieSessao(out.Token, out.ExpiraEm, h.cookieSeguro))
	responderJSON(w, http.StatusCreated, dto.SessaoResponse{ID: out.UsuarioID, Nome: out.Nome, Email: out.Email})
}

// Login autentica e abre uma sessão, respondendo 200 com o cookie de sessão.
// Responde 401 para credenciais inválidas e 429 quando a conta estourou o teto
// de tentativas.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	req, ok := decodificarJSON[dto.LoginRequest](w, r)
	if !ok {
		return
	}

	chave := chaveDeConta("login", req.Email.String())
	if h.limitadorPorConta.Excedido(w, r, chave) {
		logging.RequisicaoLogger(r).Warn("login bloqueado: teto de tentativas da conta",
			slog.String("email", req.Email.String()), slog.String("ip", r.RemoteAddr))
		return
	}

	out, err := h.login.Executar(ucauth.LoginInput{Email: req.Email.String(), Senha: req.Senha})
	if err != nil {
		// só o fracasso conta para o teto — quem acerta a senha nunca fica
		// trancado fora da própria conta por excesso de logins
		h.limitadorPorConta.Registrar(w, r, chave)

		if errors.Is(err, ucauth.ErrCredenciaisInvalidas) {
			logging.RequisicaoLogger(r).Warn("login recusado",
				slog.String("email", req.Email.String()), slog.String("ip", r.RemoteAddr))
			responderErro(w, http.StatusUnauthorized, err.Error())
			return
		}
		responderErroInterno(w, r, "erro ao autenticar", err)
		return
	}

	http.SetCookie(w, novoCookieSessao(out.Token, out.ExpiraEm, h.cookieSeguro))
	responderJSON(w, http.StatusOK, dto.SessaoResponse{ID: out.UsuarioID, Nome: out.Nome, Email: out.Email})
}

// Logout encerra a sessão atual e apaga o cookie. Responde 204 mesmo sem
// cookie ou com token desconhecido: o resultado pretendido — ninguém
// autenticado — já vale, e distinguir os casos só ajudaria quem está sondando.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(NomeCookieSessao(h.cookieSeguro)); err == nil {
		if err := h.logout.Executar(cookie.Value); err != nil {
			logging.RequisicaoLogger(r).Error("erro ao encerrar sessão", slog.String("erro", err.Error()))
		}
	}
	http.SetCookie(w, cookieSessaoExpirado(h.cookieSeguro))
	w.WriteHeader(http.StatusNoContent)
}

// Me devolve a conta autenticada. Exige o middleware de autenticação antes.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	identidade, ok := h.identidadeDoContexto(r)
	if !ok {
		responderErro(w, http.StatusUnauthorized, "não autenticado")
		return
	}

	u, err := h.perfil.Executar(identidade.UsuarioID)
	if err != nil {
		if errors.Is(err, ucauth.ErrSessaoInvalida) {
			responderErro(w, http.StatusUnauthorized, "não autenticado")
			return
		}
		responderErroInterno(w, r, "erro ao carregar perfil", err)
		return
	}

	responderJSON(w, http.StatusOK, dto.SessaoResponse{ID: u.ID, Nome: u.Nome, Email: u.Email})
}

// erroDeValidacaoDoDominio informa se o erro veio das regras do domínio de
// usuário — os únicos casos em que a culpa é do que foi enviado, e não do
// servidor. A lista é explícita de propósito: tratar "qualquer erro" como 400
// esconderia falha de infraestrutura atrás de uma mensagem de formulário.
func erroDeValidacaoDoDominio(err error) bool {
	return errors.Is(err, usuario.ErrNomeObrigatorio) ||
		errors.Is(err, usuario.ErrEmailObrigatorio) ||
		errors.Is(err, usuario.ErrEmailInvalido) ||
		errors.Is(err, usuario.ErrSenhaObrigatoria) ||
		errors.Is(err, usuario.ErrSenhaCurta)
}
