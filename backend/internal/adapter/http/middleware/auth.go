// Package middleware contém os middlewares HTTP da aplicação.
package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"stacktrack/internal/adapter/http/handler"
	"stacktrack/internal/pkg/limite"
	"stacktrack/internal/pkg/logging"
	ucauth "stacktrack/internal/usecase/auth"

	"github.com/go-chi/httprate"
)

type identidadeContextKey struct{}

// Auth valida a sessão do cookie e injeta a identidade do usuário no contexto
// da requisição. cookieSeguro decide o nome do cookie procurado — em produção
// ele leva o prefixo __Host- (ver handler.NomeCookieSessao).
type Auth struct {
	validar      *ucauth.ValidarSessaoUseCase
	cookieSeguro bool
	// desconhecidas conta, por IP, os cookies que NÃO corresponderam a sessão
	// nenhuma. Ver Autenticar.
	desconhecidas *limite.PorChave
}

// NovoAuth cria uma instância de Auth com o usecase de validação de sessão
// injetado.
//
// tetoDeCookiesDesconhecidos é quantos cookies inválidos um mesmo IP pode
// apresentar dentro de janela antes de levar 429. Zero desliga o teto.
func NovoAuth(
	validar *ucauth.ValidarSessaoUseCase,
	cookieSeguro bool,
	tetoDeCookiesDesconhecidos int,
	janela time.Duration,
) *Auth {
	return &Auth{
		validar:       validar,
		cookieSeguro:  cookieSeguro,
		desconhecidas: limite.NovoPorChave(tetoDeCookiesDesconhecidos, janela),
	}
}

// Autenticar responde 401 se o cookie de sessão estiver ausente, ou se a
// sessão for inválida ou já tiver expirado. Caso contrário, injeta a
// identidade do usuário no contexto e segue para o próximo handler.
//
// Antes de consultar o banco, confere um teto POR IP de cookies desconhecidos.
// A ordem é a razão de ser deste teto: validar sessão é um SELECT indexado por
// hash, barato por unidade e caro em rajada — um laço mandando cookies
// aleatórios consome uma conexão do pool por requisição e compete com o
// tráfego real, sem nunca autenticar nada. Contar DEPOIS da consulta protegeria
// tarde demais; contar toda requisição faria a sessão legítima gastar a mesma
// cota do abuso. Por isso só a falha conta, e a checagem vem antes.
//
// O teto por sessão que existe no roteador não cobre este caso: ele chaveia
// pelo valor do cookie, e um cookie novo a cada tentativa cria um balde novo a
// cada tentativa. Aqui a chave é o IP, que o cliente não escolhe (ver IPReal).
func (a *Auth) Autenticar(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(handler.NomeCookieSessao(a.cookieSeguro))
		if err != nil {
			// Sem cookie não há consulta ao banco e não há o que contar: é o
			// caso de quem nunca entrou, e não de quem está testando token.
			responderNaoAutenticado(w)
			return
		}

		chave := chaveDeIP(r)
		reserva, permitido := a.desconhecidas.Reservar(chave)
		if !permitido {
			responderExcessoDeTentativas(w, a.desconhecidas.SegundosDeEspera())
			return
		}
		defer reserva.Cancelar()

		id, err := a.validar.Executar(r.Context(), cookie.Value)
		if err != nil {
			if errors.Is(err, ucauth.ErrSessaoInvalida) {
				reserva.Confirmar(w)
				responderNaoAutenticado(w)
				return
			}

			// Banco indisponivel e sessao invalida sao estados diferentes. Dizer
			// 401 apagaria a sessao no cliente e ainda contabilizaria a falha como
			// abuso; 503 preserva a credencial e orienta a repeticao.
			logging.RequisicaoLogger(r).Error("erro ao validar sessao",
				slog.String("erro", err.Error()))
			w.Header().Set("Retry-After", "1")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{"erro": "autenticacao temporariamente indisponivel"})
			return
		}

		ctx := context.WithValue(r.Context(), identidadeContextKey{}, *id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// chaveDeIP monta a chave do teto. Usa a mesma normalização do httprate para
// que /64 de IPv6 conte como um endereço só — senão um cliente com prefixo
// IPv6 delegado trocaria de endereço a cada tentativa e o teto não veria nada.
//
// r.RemoteAddr já passou por IPReal, então é o peer direto ou o IP que um proxy
// CONFIÁVEL informou; um cabeçalho forjado não chega até aqui.
func chaveDeIP(r *http.Request) string {
	chave, err := httprate.KeyByIP(r)
	if err != nil {
		return r.RemoteAddr
	}
	return chave
}

func responderNaoAutenticado(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"erro": "não autenticado"})
}

func responderExcessoDeTentativas(w http.ResponseWriter, segundos int) {
	w.Header().Set("Retry-After", strconv.Itoa(segundos))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	json.NewEncoder(w).Encode(map[string]string{"erro": "muitas tentativas; tente de novo em alguns minutos"})
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
