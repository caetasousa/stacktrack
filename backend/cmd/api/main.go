// Package main é o entrypoint do servidor HTTP.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"kanbango/config"
	"kanbango/internal/adapter/http/handler"
	"kanbango/internal/adapter/http/middleware"
	"kanbango/internal/adapter/repository"
	"kanbango/internal/adapter/security"
	"kanbango/internal/pkg/logging"
	ucauth "kanbango/internal/usecase/auth"
	ucboard "kanbango/internal/usecase/board"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	// logger estruturado (slog): JSON em produção, texto legível em dev.
	// Configurado antes de tudo, para todo log já sair no formato certo.
	logging.Configurar(config.EhProducao())

	// contexto de vida da aplicação: cancelado em SIGINT/SIGTERM, usado pelo
	// desligamento gracioso do servidor HTTP — e, da fase 5 em diante, pelo hub
	// de WebSocket, que precisa fechar as conexões abertas antes de o processo
	// morrer.
	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()

	pool, err := config.NovoPool(context.Background())
	if err != nil {
		slog.Error("erro ao conectar no banco", slog.String("erro", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()

	avisarSobreTetosDesligados()

	// repositórios
	usuarioRepo := repository.NovoUsuarioPostgres(pool)
	sessionRepo := repository.NovoSessionPostgres(pool)
	boardRepo := repository.NovoBoardPostgres(pool)
	membroRepo := repository.NovoMembroPostgres(pool)
	colunaRepo := repository.NovoColunaPostgres(pool)
	cardRepo := repository.NovoCardPostgres(pool)

	// segurança
	hasher := security.NovoHasherArgon2id()

	// usecases
	cadastrarUC := ucauth.NovoCadastrarUseCase(usuarioRepo, sessionRepo, hasher)
	loginUC := ucauth.NovoLoginUseCase(usuarioRepo, sessionRepo, hasher)
	logoutUC := ucauth.NovoLogoutUseCase(sessionRepo)
	validarSessaoUC := ucauth.NovoValidarSessaoUseCase(sessionRepo)
	perfilUC := ucauth.NovoPerfilUseCase(usuarioRepo)
	quadroUC := ucboard.NovoQuadroUseCase(boardRepo, membroRepo, colunaRepo, cardRepo)
	colunaUC := ucboard.NovoColunaUseCase(membroRepo, colunaRepo)
	cardUC := ucboard.NovoCardUseCase(membroRepo, colunaRepo, cardRepo)

	// handlers e middlewares
	autenticacao := middleware.NovoAuth(validarSessaoUC, config.CookieSeguro())
	identidadeDoContexto := func(r *http.Request) (ucauth.Identidade, bool) {
		return middleware.IdentidadeDoContexto(r.Context())
	}
	authHandler := handler.NovoAuthHandler(
		cadastrarUC, loginUC, logoutUC, perfilUC,
		config.CookieSeguro(),
		handler.NovoLimitadorPorConta(config.RateLimitLoginPorConta(), config.JanelaLimitePorConta),
		identidadeDoContexto,
	)
	boardHandler := handler.NovoBoardHandler(quadroUC, colunaUC, cardUC, identidadeDoContexto)

	r := config.NovoRouter()
	r.Get("/health", health)
	r.Get("/ready", ready(pool))

	r.Route("/auth", func(r chi.Router) {
		// Os tetos por IP entram por grupo, e não na rota: assim uma rota nova
		// de autenticação nasce protegida em vez de depender de alguém lembrar.
		r.Group(func(r chi.Router) {
			r.Use(limitePorIP(config.RateLimitCadastroPorMinuto()))
			r.Post("/cadastro", authHandler.Cadastrar)
		})
		r.Group(func(r chi.Router) {
			r.Use(limitePorIP(config.RateLimitLoginPorMinuto()))
			r.Post("/login", authHandler.Login)
		})

		r.Post("/logout", authHandler.Logout)

		r.Group(func(r chi.Router) {
			r.Use(autenticacao.Autenticar)
			r.Get("/me", authHandler.Me)
		})
	})

	// Tudo daqui para baixo exige sessão. O teto por sessão fica no grupo, e
	// não em cada rota, pela mesma razão dos tetos de /auth: rota nova nasce
	// coberta em vez de depender de alguém lembrar.
	r.Group(func(r chi.Router) {
		r.Use(autenticacao.Autenticar)
		r.Use(limitePorSessao(config.RateLimitAutenticadoPorMinuto(), config.CookieSeguro()))

		r.Route("/boards", func(r chi.Router) {
			r.Get("/", boardHandler.Listar)
			r.Post("/", boardHandler.Criar)
			r.Get("/{boardID}", boardHandler.Detalhar)
			r.Patch("/{boardID}", boardHandler.Renomear)
			r.Delete("/{boardID}", boardHandler.Apagar)
			r.Post("/{boardID}/colunas", boardHandler.CriarColuna)
		})

		// Coluna e card são endereçados pelo próprio id, e não sob o caminho
		// do quadro: o servidor descobre a que quadro pertencem para autorizar
		// (card → coluna → quadro). Aceitar o quadro pela URL deixaria alguém
		// mexer em coluna alheia informando o id de um quadro próprio.
		r.Route("/colunas/{colunaID}", func(r chi.Router) {
			r.Patch("/", boardHandler.RenomearColuna)
			r.Delete("/", boardHandler.ApagarColuna)
			r.Post("/cards", boardHandler.CriarCard)
		})

		r.Route("/cards/{cardID}", func(r chi.Router) {
			r.Patch("/", boardHandler.EditarCard)
			r.Delete("/", boardHandler.ApagarCard)
		})
	})

	srv := config.NovoServidor(r)

	go func() {
		slog.Info("servidor no ar", slog.String("endereco", config.Porta()))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("erro ao iniciar servidor", slog.String("erro", err.Error()))
			os.Exit(1)
		}
	}()

	<-ctx.Done()

	slog.Info("encerrando: aguardando requisições em andamento")
	ctxDesligamento, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	if err := srv.Shutdown(ctxDesligamento); err != nil {
		slog.Error("desligamento forçado", slog.String("erro", err.Error()))
	}

	slog.Info("servidor encerrado")
}

// limitePorIP devolve o middleware de teto por IP, ou um middleware neutro
// quando o limite é 0 (desligado). Sem o caso neutro, desligar o teto exigiria
// montar a rota de outro jeito.
func limitePorIP(porMinuto int) func(http.Handler) http.Handler {
	if porMinuto <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return httprate.LimitByIP(porMinuto, time.Minute)
}

// limitePorSessao devolve o middleware de teto por sessão, chaveado pelo
// cookie e não pelo IP: depois do login, o IP deixa de identificar quem abusa
// (um escritório inteiro sai pelo mesmo endereço, e a mesma conta troca de
// rede). Sem cookie, cai no IP — nesse ponto a requisição já foi autenticada,
// então isso só cobre um caminho que não deveria existir.
func limitePorSessao(porMinuto int, cookieSeguro bool) func(http.Handler) http.Handler {
	if porMinuto <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	nome := handler.NomeCookieSessao(cookieSeguro)
	return httprate.Limit(porMinuto, time.Minute, httprate.WithKeyFuncs(
		func(r *http.Request) (string, error) {
			if c, err := r.Cookie(nome); err == nil {
				return c.Value, nil
			}
			return httprate.KeyByIP(r)
		},
	))
}

// avisarSobreTetosDesligados registra um WARN quando algum teto de requisições
// está em zero. É fácil demais copiar um .env de desenvolvimento (onde os
// tetos atrapalham os testes) para um ambiente exposto e não perceber que o
// login ficou sem proteção nenhuma.
func avisarSobreTetosDesligados() {
	desligados := make([]string, 0, 4)
	for nome, limite := range map[string]int{
		"RATE_LIMIT_LOGIN_POR_MINUTO":       config.RateLimitLoginPorMinuto(),
		"RATE_LIMIT_CADASTRO_POR_MINUTO":    config.RateLimitCadastroPorMinuto(),
		"RATE_LIMIT_LOGIN_POR_CONTA":        config.RateLimitLoginPorConta(),
		"RATE_LIMIT_AUTENTICADO_POR_MINUTO": config.RateLimitAutenticadoPorMinuto(),
	} {
		if limite == 0 {
			desligados = append(desligados, nome)
		}
	}
	if len(desligados) > 0 {
		sort.Strings(desligados)
		slog.Warn("rate limiting parcialmente desligado: as rotas cobertas ficam sem teto",
			slog.String("variaveis_em_zero", strings.Join(desligados, ", ")))
	}
}

// health informa que o processo está no ar. Não toca em dependência nenhuma —
// use /ready para saber se a API consegue atender.
func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// timeoutReady limita o ping de readiness: sem teto, um banco travado (em vez de
// fora do ar) seguraria a checagem até o timeout do cliente, e o orquestrador
// ficaria sem resposta justamente quando ela mais importa.
const timeoutReady = 2 * time.Second

// ready informa se a API consegue atender de fato: faz ping no pool do banco.
// Responde 503 quando o banco está indisponível.
func ready(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancelar := context.WithTimeout(r.Context(), timeoutReady)
		defer cancelar()

		w.Header().Set("Content-Type", "application/json")
		if err := pool.Ping(ctx); err != nil {
			slog.Error("readiness: banco indisponível", slog.String("erro", err.Error()))
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{"status": "degradado", "erro": "banco indisponível"})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
