// O teto de cookies de sessão desconhecidos. O que este arquivo prova não é só
// que o 429 acontece: é que ele acontece ANTES da consulta ao banco, e que um
// cabeçalho de IP forjado não abre um balde novo.
package middleware_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"stacktrack/internal/adapter/http/handler"
	"stacktrack/internal/adapter/http/middleware"
	"stacktrack/internal/domain/session"
	ucauth "stacktrack/internal/usecase/auth"
)

// sessoesContadas é um repositório de sessões que só conta consultas e nunca
// encontra nada — o cenário de quem varre cookies aleatórios.
//
// Contar é o ponto do teste: o custo que o teto existe para evitar é
// exatamente uma ida ao banco por tentativa, e a única forma de provar que ela
// deixou de acontecer é contá-la.
type sessoesContadas struct {
	consultas atomic.Int64
	erro      error
}

func (s *sessoesContadas) Salvar(context.Context, *session.Session) error { return nil }
func (s *sessoesContadas) BuscarPorTokenHash(context.Context, string) (*session.Session, error) {
	s.consultas.Add(1)
	return nil, s.erro
}

// Falha do repositorio nao significa cookie invalido. Responder 401 faria o
// frontend apagar uma credencial boa, e contabilizar a falha manteria o IP
// bloqueado mesmo depois de o banco voltar.
func TestFalhaDoBancoNaoVira401NemConsomeCota(t *testing.T) {
	sessoes := &sessoesContadas{erro: errors.New("banco fora do ar")}
	auth := middleware.NovoAuth(ucauth.NovoValidarSessaoUseCase(sessoes), false, 2, time.Minute)
	api := auth.Autenticar(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 10; i++ {
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, comCookie("cookie-possivelmente-valido", "203.0.113.9:5555"))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("tentativa %d: status = %d, esperado 503", i, rec.Code)
		}
	}
	if sessoes.consultas.Load() != 10 {
		t.Errorf("consultas = %d, esperado 10: falha de infraestrutura consumiu a cota", sessoes.consultas.Load())
	}
}
func (s *sessoesContadas) Remover(context.Context, string) error  { return nil }
func (s *sessoesContadas) RemoverExpiradas(context.Context) error { return nil }

// protegida monta o middleware sobre o repositório contado.
func protegida(teto int) (http.Handler, *sessoesContadas) {
	sessoes := &sessoesContadas{}
	auth := middleware.NovoAuth(ucauth.NovoValidarSessaoUseCase(sessoes), false, teto, time.Minute)
	return auth.Autenticar(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})), sessoes
}

// comCookie monta uma requisição com o cookie de sessão informado, vinda do IP
// informado.
func comCookie(valor, ip string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/boards", nil)
	req.RemoteAddr = ip
	req.AddCookie(&http.Cookie{Name: handler.NomeCookieSessao(false), Value: valor})
	return req
}

func TestCookiesAleatoriosLevam429AntesDeConsultarOBanco(t *testing.T) {
	const teto = 10
	api, sessoes := protegida(teto)

	var status429 int
	for i := 0; i < 100; i++ {
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, comCookie(fmt.Sprintf("cookie-aleatorio-%d", i), "203.0.113.9:5555"))
		if rec.Code == http.StatusTooManyRequests {
			status429++
		}
	}

	if status429 == 0 {
		t.Fatal("cem cookies desconhecidos do mesmo IP não produziram nenhum 429")
	}
	// O teto é sobre a CONSULTA, não sobre a resposta: passado o limite, o
	// banco não deve mais ser tocado. Uma margem pequena cobre a janela
	// deslizante do contador.
	if sessoes.consultas.Load() > teto+2 {
		t.Errorf("consultas ao banco = %d, esperado no máximo ~%d: o teto não está barrando antes da consulta",
			sessoes.consultas.Load(), teto)
	}
}

// O 429 precisa dizer quando tentar de novo — sem isso o cliente legítimo que
// caiu no teto fica sem instrução e o retry vira laço.
func TestRespostaDeExcessoTrazRetryAfter(t *testing.T) {
	api, _ := protegida(1)
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, comCookie(fmt.Sprintf("c-%d", i), "203.0.113.9:5555"))
		if rec.Code == http.StatusTooManyRequests {
			if rec.Header().Get("Retry-After") == "" {
				t.Error("429 sem Retry-After")
			}
			return
		}
	}
	t.Fatal("o teto de 1 nunca disparou")
}

// O ataque que o teto por sessão do roteador não cobre: trocar o cabeçalho de
// IP a cada tentativa. Como IPReal só obedece proxy confiável, o balde continua
// sendo o do peer direto.
func TestIPForjadoNaoAbreBaldeNovo(t *testing.T) {
	const teto = 5
	autenticada, sessoes := protegida(teto)
	// Sem proxies confiáveis: é a configuração de quem expôs a API por engano.
	api := middleware.IPReal(nil)(autenticada)

	for i := 0; i < 100; i++ {
		rec := httptest.NewRecorder()
		req := comCookie(fmt.Sprintf("cookie-%d", i), "203.0.113.9:5555")
		// Um IP diferente a cada volta: se fosse obedecido, cada tentativa
		// cairia num balde vazio e o teto nunca disparava.
		req.Header.Set("X-Real-IP", fmt.Sprintf("198.51.100.%d", i%256))
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i%256))
		api.ServeHTTP(rec, req)
	}

	if sessoes.consultas.Load() > teto+2 {
		t.Errorf("consultas = %d: o cabeçalho forjado contornou o teto", sessoes.consultas.Load())
	}
}

// E o contrário: vindo de um proxy CONFIÁVEL, IPs diferentes são clientes
// diferentes e cada um tem o seu balde. Sem isto, o teto puniria o prédio
// inteiro pelo abuso de um.
func TestClientesDistintosAtrasDoProxyTemBaldesDistintos(t *testing.T) {
	autenticada, sessoes := protegida(3)
	confiavel := netip.MustParsePrefix("10.0.0.0/8")
	api := middleware.IPReal([]netip.Prefix{confiavel})(autenticada)

	for i := 0; i < 20; i++ {
		rec := httptest.NewRecorder()
		req := comCookie(fmt.Sprintf("cookie-%d", i), "10.0.0.7:1111")
		req.Header.Set("X-Real-IP", fmt.Sprintf("198.51.100.%d", i))
		api.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("cliente %d recebeu %d, esperado 401 — cada IP tem o próprio balde", i, rec.Code)
		}
	}
	if sessoes.consultas.Load() != 20 {
		t.Errorf("consultas = %d, esperado 20: um cliente foi punido pelo balde de outro", sessoes.consultas.Load())
	}
}

// Requisição SEM cookie não consome cota: é o caso de quem nunca entrou, e não
// de quem está testando token. Contá-la faria a home pública gastar o teto.
func TestRequisicaoSemCookieNaoConsomeCota(t *testing.T) {
	api, sessoes := protegida(2)
	for i := 0; i < 50; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/boards", nil)
		req.RemoteAddr = "203.0.113.9:5555"
		api.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, esperado 401", rec.Code)
		}
	}
	if sessoes.consultas.Load() != 0 {
		t.Errorf("consultas = %d, esperado 0: sem cookie não há o que consultar", sessoes.consultas.Load())
	}
}
