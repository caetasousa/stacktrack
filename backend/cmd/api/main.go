// Package main é o entrypoint do servidor HTTP.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"stacktrack/config"
	"stacktrack/internal/adapter/armazem"
	"stacktrack/internal/adapter/http/handler"
	"stacktrack/internal/adapter/http/middleware"
	"stacktrack/internal/adapter/repository"
	"stacktrack/internal/adapter/security"
	"stacktrack/internal/pkg/logging"
	ucauth "stacktrack/internal/usecase/auth"
	ucboard "stacktrack/internal/usecase/board"

	"stacktrack/internal/adapter/http/ws"
	"stacktrack/internal/adapter/realtime/hub"

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
	conviteRepo := repository.NovoConvitePostgres(pool)
	etiquetaRepo := repository.NovoEtiquetaPostgres(pool)
	checklistRepo := repository.NovoChecklistPostgres(pool)
	anexoRepo := repository.NovoAnexoPostgres(pool)

	// armazém de anexos: disco, num volume próprio. O binário não vai para o
	// banco — incharia backup e restore de um schema que guarda texto curto no
	// resto todo.
	armazemAnexos, err := armazem.NovoDisco(config.DiretorioDeAnexos())
	if err != nil {
		slog.Error("erro ao preparar o diretório de anexos", slog.String("erro", err.Error()))
		os.Exit(1)
	}

	// segurança
	hasher := security.NovoHasherArgon2id()

	// usecases
	cadastrarUC := ucauth.NovoCadastrarUseCase(usuarioRepo, sessionRepo, hasher)
	loginUC := ucauth.NovoLoginUseCase(usuarioRepo, sessionRepo, hasher)
	logoutUC := ucauth.NovoLogoutUseCase(sessionRepo)
	validarSessaoUC := ucauth.NovoValidarSessaoUseCase(sessionRepo)
	perfilUC := ucauth.NovoPerfilUseCase(usuarioRepo)
	quadroUC := ucboard.NovoQuadroUseCase(boardRepo, membroRepo, colunaRepo, cardRepo, etiquetaRepo, checklistRepo, anexoRepo)
	colunaUC := ucboard.NovoColunaUseCase(membroRepo, colunaRepo)
	cardUC := ucboard.NovoCardUseCase(membroRepo, colunaRepo, cardRepo, etiquetaRepo, checklistRepo, anexoRepo)
	etiquetaUC := ucboard.NovoEtiquetaUseCase(membroRepo, colunaRepo, cardRepo, etiquetaRepo)
	checklistUC := ucboard.NovoChecklistUseCase(membroRepo, colunaRepo, cardRepo, checklistRepo)
	anexoUC := ucboard.NovoAnexoUseCase(membroRepo, colunaRepo, cardRepo, anexoRepo, armazemAnexos)
	membroUC := ucboard.NovoMembroUseCase(membroRepo, conviteRepo, usuarioRepo, boardRepo)

	// O hub é o adaptador que implementa a porta Publicador. Ligá-lo aqui, e
	// não no construtor de cada usecase, é o que mantém os testes construindo
	// os mesmos usecases sem saber que tempo real existe.
	salaDeEventos := hub.Novo()
	logDeEventos := repository.NovoEventoPostgres(pool)
	// A unidade de trabalho é o que faz o dado e o evento caírem no MESMO
	// commit. Sem ela ligada, o usecase ainda funciona — grava numa transação e
	// registra noutra —, e é assim que os testes de regra rodam, sem banco.
	// Em produção, essa separação deixaria buraco invisível no log do quadro.
	unidadeDeTrabalho := repository.NovaUnidadeDeTrabalho(pool)
	for _, uc := range []interface {
		ComPublicador(ucboard.Publicador)
		ComRegistro(ucboard.RegistroDeEventos)
		ComEscritaAtomica(ucboard.EscritaAtomica)
	}{
		quadroUC, colunaUC, cardUC, etiquetaUC, checklistUC, anexoUC,
	} {
		uc.ComPublicador(salaDeEventos)
		uc.ComRegistro(logDeEventos)
		uc.ComEscritaAtomica(unidadeDeTrabalho)
	}

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
	membroHandler := handler.NovoMembroHandler(membroUC, config.OrigemFrontend(), identidadeDoContexto)
	extrasHandler := handler.NovoExtrasHandler(etiquetaUC, checklistUC, anexoUC, identidadeDoContexto)

	// OriginPatterns com a origem do frontend, e nada além: WebSocket NÃO
	// obedece CORS, então sem esta lista qualquer site que a vítima visitar
	// abre uma conexão autenticada com o cookie dela e lê o quadro inteiro em
	// tempo real (Cross-Site WebSocket Hijacking).
	wsHandler := ws.NovoHandler(
		salaDeEventos,
		logDeEventos,
		quadroUC,
		func(ctx context.Context) (string, bool) {
			id, ok := middleware.IdentidadeDoContexto(ctx)
			return id.UsuarioID, ok
		},
		// O nome é resolvido uma vez por conexão, no handshake — e não a cada
		// evento. Se a consulta falhar, a presença mostra um rótulo genérico em
		// vez de derrubar a conexão: ver quem está no quadro é acessório, e
		// perder o tempo real por causa disso seria trocar o essencial pelo
		// secundário.
		func(ctx context.Context, usuarioID string) string {
			u, err := perfilUC.Executar(ctx, usuarioID)
			if err != nil || u == nil {
				return "alguém"
			}
			return u.Nome
		},
		// A sessão é reconferida enquanto a conexão vive. O middleware autentica
		// uma vez, no handshake, e a conexão dura horas: sem isto, o logout
		// apaga a sessão no banco e o socket continua transmitindo o quadro.
		func(ctx context.Context, token string) bool {
			_, err := validarSessaoUC.Executar(ctx, token)
			return err == nil
		},
		handler.NomeCookieSessao(config.CookieSeguro()),
		[]string{origemSemEsquema(config.OrigemFrontend())},
		slog.Default(),
	)

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

	// Detalhe do convite é PÚBLICO de propósito: quem foi convidado costuma
	// ainda não ter conta, e precisa ver de que quadro se trata antes de criar
	// uma. O token é a credencial — quem não o tem recebe o mesmo 404 de um
	// convite vencido. Aceitar, esse sim, exige sessão.
	r.Group(func(r chi.Router) {
		r.Use(limitePorIP(config.RateLimitPublicoPorMinuto()))
		r.Get("/convites/{token}", membroHandler.DetalharConvite)
	})

	// Tudo daqui para baixo exige sessão. O teto por sessão fica no grupo, e
	// não em cada rota, pela mesma razão dos tetos de /auth: rota nova nasce
	// coberta em vez de depender de alguém lembrar.
	r.Group(func(r chi.Router) {
		r.Use(autenticacao.Autenticar)
		r.Use(limitePorSessao(config.RateLimitAutenticadoPorMinuto(), config.CookieSeguro()))

		// Tempo real. Fica FORA do teto por sessão de propósito: o limite conta
		// requisições por minuto, e esta é uma requisição só que dura horas —
		// sujeitá-la ao teto não protegeria nada e derrubaria a reconexão de
		// quem estivesse ativo.
		r.Get("/ws", wsHandler.Acompanhar)

		r.Route("/boards", func(r chi.Router) {
			r.Get("/", boardHandler.Listar)
			r.Post("/", boardHandler.Criar)
			r.Get("/{boardID}", boardHandler.Detalhar)
			r.Patch("/{boardID}", boardHandler.Renomear)
			r.Delete("/{boardID}", boardHandler.Apagar)
			r.Post("/{boardID}/colunas", boardHandler.CriarColuna)

			r.Get("/{boardID}/membros", membroHandler.Listar)
			r.Post("/{boardID}/membros", membroHandler.Convidar)
			r.Patch("/{boardID}/membros/{usuarioID}", membroHandler.AlterarPapel)
			r.Delete("/{boardID}/membros/{usuarioID}", membroHandler.Remover)
			r.Delete("/{boardID}/convites/{conviteID}", membroHandler.RevogarConvite)

			r.Patch("/{boardID}/fundo", boardHandler.DefinirFundo)
			r.Get("/{boardID}/etiquetas", extrasHandler.ListarEtiquetas)
			r.Post("/{boardID}/etiquetas", extrasHandler.CriarEtiqueta)
		})

		r.Route("/etiquetas/{etiquetaID}", func(r chi.Router) {
			r.Patch("/", extrasHandler.EditarEtiqueta)
			r.Delete("/", extrasHandler.ApagarEtiqueta)
		})

		r.Route("/checklists/{checklistID}", func(r chi.Router) {
			r.Patch("/", extrasHandler.RenomearChecklist)
			r.Delete("/", extrasHandler.ApagarChecklist)
			r.Post("/itens", extrasHandler.CriarItem)
		})

		r.Route("/itens/{itemID}", func(r chi.Router) {
			r.Patch("/", extrasHandler.EditarItem)
			r.Delete("/", extrasHandler.ApagarItem)
		})

		r.Route("/anexos/{anexoID}", func(r chi.Router) {
			r.Get("/", extrasHandler.BaixarAnexo)
			r.Delete("/", extrasHandler.ApagarAnexo)
		})

		r.Post("/convites/{token}/aceitar", membroHandler.AceitarConvite)

		// Coluna e card são endereçados pelo próprio id, e não sob o caminho
		// do quadro: o servidor descobre a que quadro pertencem para autorizar
		// (card → coluna → quadro). Aceitar o quadro pela URL deixaria alguém
		// mexer em coluna alheia informando o id de um quadro próprio.
		r.Route("/colunas/{colunaID}", func(r chi.Router) {
			r.Patch("/", boardHandler.RenomearColuna)
			r.Patch("/mover", boardHandler.MoverColuna)
			r.Delete("/", boardHandler.ApagarColuna)
			r.Post("/cards", boardHandler.CriarCard)
		})

		r.Route("/cards/{cardID}", func(r chi.Router) {
			r.Get("/", boardHandler.DetalharCard)
			r.Patch("/", boardHandler.EditarCard)
			r.Delete("/", boardHandler.ApagarCard)
			r.Patch("/prazo", boardHandler.DefinirPrazo)
			r.Patch("/mover", boardHandler.MoverCard)
			r.Put("/etiquetas/{etiquetaID}", extrasHandler.AplicarEtiqueta)
			r.Delete("/etiquetas/{etiquetaID}", extrasHandler.RemoverEtiqueta)
			r.Post("/checklists", extrasHandler.CriarChecklist)
			r.Post("/anexos/link", extrasHandler.AnexarLink)
			r.Post("/anexos/arquivo", extrasHandler.AnexarArquivo)
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

	// O hub fecha ANTES do Shutdown, e a ordem é o ponto: conexão de tempo real
	// não termina sozinha, então o Shutdown esperaria o timeout inteiro com
	// elas abertas e só então as cortaria no meio. Fechando aqui, cada conexão
	// encerra de forma limpa e o Shutdown só espera as requisições HTTP comuns.
	slog.Info("encerrando: fechando conexões de tempo real")
	salaDeEventos.Fechar()

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
	desligados := make([]string, 0, 5)
	for nome, limite := range map[string]int{
		"RATE_LIMIT_LOGIN_POR_MINUTO":       config.RateLimitLoginPorMinuto(),
		"RATE_LIMIT_CADASTRO_POR_MINUTO":    config.RateLimitCadastroPorMinuto(),
		"RATE_LIMIT_LOGIN_POR_CONTA":        config.RateLimitLoginPorConta(),
		"RATE_LIMIT_AUTENTICADO_POR_MINUTO": config.RateLimitAutenticadoPorMinuto(),
		"RATE_LIMIT_PUBLICO_POR_MINUTO":     config.RateLimitPublicoPorMinuto(),
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

// origemSemEsquema devolve só o host da origem, que é o formato do
// OriginPatterns do coder/websocket — ele compara contra o host do cabeçalho
// Origin, não contra a URL inteira.
func origemSemEsquema(origem string) string {
	u, err := url.Parse(origem)
	if err != nil || u.Host == "" {
		return origem
	}
	return u.Host
}
