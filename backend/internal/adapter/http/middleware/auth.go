// Package middleware contém os middlewares HTTP da aplicação.
package middleware

import (
	"context"
	"encoding/json"
	"net/http"

	"stacktrack/internal/adapter/http/handler"
	ucauth "stacktrack/internal/usecase/auth"
)

type identidadeContextKey struct{}

// Auth valida a sessão do cookie e injeta a identidade do usuário no contexto
// da requisição. cookieSeguro decide o nome do cookie procurado — em produção
// ele leva o prefixo __Host- (ver handler.NomeCookieSessao).
type Auth struct {
	validar      *ucauth.ValidarSessaoUseCase
	cookieSeguro bool
}

// NovoAuth cria uma instância de Auth com o usecase de validação de sessão injetado.
func NovoAuth(validar *ucauth.ValidarSessaoUseCase, cookieSeguro bool) *Auth {
	return &Auth{validar: validar, cookieSeguro: cookieSeguro}
}

// Autenticar responde 401 se o cookie de sessão estiver ausente, ou se a
// sessão for inválida ou já tiver expirado. Caso contrário, injeta a
// identidade do usuário no contexto e segue para o próximo handler.
func (a *Auth) Autenticar(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(handler.NomeCookieSessao(a.cookieSeguro))
		if err != nil {
			responderNaoAutenticado(w)
			return
		}

		id, err := a.validar.Executar(r.Context(), cookie.Value)
		if err != nil {
			responderNaoAutenticado(w)
			return
		}

		ctx := context.WithValue(r.Context(), identidadeContextKey{}, *id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// IdentidadeDoContexto recupera a identidade injetada pelo middleware Autenticar.
//
// A chave é um tipo privado deste pacote, e não uma string: assim nenhum outro
// pacote consegue sobrescrever a identidade no contexto por acidente ou de
// propósito.
func IdentidadeDoContexto(ctx context.Context) (ucauth.Identidade, bool) {
	id, ok := ctx.Value(identidadeContextKey{}).(ucauth.Identidade)
	return id, ok
}

func responderNaoAutenticado(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"erro": "não autenticado"})
}
