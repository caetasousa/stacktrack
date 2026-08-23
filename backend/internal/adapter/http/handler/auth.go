// Package handler contém os handlers HTTP da aplicação: eles traduzem
// requisição em chamada de usecase e resultado em resposta, sem regra de
// negócio própria.
package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"stacktrack/internal/adapter/http/dto"
	"stacktrack/internal/domain/usuario"
	"stacktrack/internal/pkg/logging"
	ucauth "stacktrack/internal/usecase/auth"
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

	out, err := h.cadastrar.Executar(r.Context(), ucauth.CadastroInput{
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
	reserva, permitido := h.limitadorPorConta.Reservar(w, chave)
	if !permitido {
		logging.RequisicaoLogger(r).Warn("login bloqueado: teto de tentativas da conta",
			slog.String("email", usuario.MascararEmail(req.Email.String())),
			slog.String("ip", r.RemoteAddr))
		return
	}
	if reserva != nil {
		defer reserva.Cancelar()
	}

	out, err := h.login.Executar(r.Context(), ucauth.LoginInput{Email: req.Email.String(), Senha: req.Senha})
	if err != nil {
		if errors.Is(err, ucauth.ErrCredenciaisInvalidas) {
			// So credencial invalida conta. Erro de banco ou do hasher devolve a
			// reserva: indisponibilidade nao pode trancar a conta depois que o
			// servico se recuperar.
			if reserva != nil {
				reserva.Confirmar(w)
			}
			// O email vai MASCARADO. Um log de acesso guardado por trinta dias,
			// enviado a um agregador e lido por quem opera não pode conter a
			// lista de quem tem conta aqui — nem, pelas tentativas fracassadas,
			// a lista de endereços que alguém está testando contra o sistema.
			// A máscara mantém o que o log serve para responder ("está tendo
			// brute-force contra a mesma conta?") e descarta o resto.
			logging.RequisicaoLogger(r).Warn("login recusado",
				slog.String("email", usuario.MascararEmail(req.Email.String())),
				slog.String("ip", r.RemoteAddr))
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
		if err := h.logout.Executar(r.Context(), cookie.Value); err != nil {
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

	u, err := h.perfil.Executar(r.Context(), identidade.UsuarioID)
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
		errors.Is(err, usuario.ErrSenhaCurta) ||
		errors.Is(err, usuario.ErrSenhaComum)
}
