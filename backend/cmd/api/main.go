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
	"stacktrack/internal/adapter/repository"
	"stacktrack/internal/pkg/logging"

	"stacktrack/internal/adapter/realtime/despachante"
	"stacktrack/internal/adapter/realtime/hub"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"
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

	// A configuração é conferida ANTES do banco, do disco e da porta. Subir
	// primeiro e validar depois deixaria o processo atendendo requisições com
	// uma proteção desligada durante a janela entre um e outro — e, se o erro
	// aparecesse só no primeiro acesso, ele apareceria para um usuário.
	if err := config.Validar(); err != nil {
		slog.Error("configuração recusada", slog.String("erro", err.Error()))
		os.Exit(1)
	}

	app, err := montar(ctx)
	if err != nil {
		slog.Error("não foi possível montar a aplicação", slog.String("erro", err.Error()))
		os.Exit(1)
	}
	defer app.fechar()
	app.rodarTrabalhos(ctx)

	srv := config.NovoServidor(app.roteador)

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
	app.fecharTempoReal()

	slog.Info("encerrando: aguardando requisições em andamento")
	ctxDesligamento, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	if err := srv.Shutdown(ctxDesligamento); err != nil {
		slog.Error("desligamento forçado", slog.String("erro", err.Error()))
	}

	slog.Info("servidor encerrado")
}

// prefixosDeRota devolve o primeiro segmento de cada rota registrada, sem
// repetição.
func prefixosDeRota(r chi.Routes) []string {
	vistos := make(map[string]struct{})
	_ = chi.Walk(r, func(_ string, rota string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		primeiro, _, _ := strings.Cut(strings.TrimPrefix(rota, "/"), "/")
		// Um segmento de PARÂMETRO como primeiro nível ("/{token}") não é
		// prefixo conhecido: ele casa com qualquer coisa, inclusive com um
		// segredo, e registrá-lo devolveria ao log exatamente o que esta lista
		// existe para manter fora dele.
		if primeiro == "" || strings.HasPrefix(primeiro, "{") || strings.HasPrefix(primeiro, "*") {
			return nil
		}
		vistos[primeiro] = struct{}{}
		return nil
	})

	lista := make([]string, 0, len(vistos))
	for p := range vistos {
		lista = append(lista, p)
	}
	sort.Strings(lista)
	return lista
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

// ready informa se a API consegue atender de fato, e DISTINGUE os motivos.
//
// A distinção é o ponto. Um readiness booleano obriga a escolher entre duas
// respostas erradas quando o disco enche: dizer "pronto" mantém o tráfego indo
// para uma API que falha em toda escrita, e dizer "não pronto" tira do ar
// também a LEITURA, que continuava funcionando perfeitamente. Nenhuma das duas
// é o que se quer.
//
// Aqui:
//
//   - banco fora do ar é 503, porque sem ele nem leitura existe;
//   - disco sem margem é 200 com `escrita: false`. O orquestrador mantém a
//     instância recebendo tráfego, quem lê continua lendo, e quem escreve
//     recebe um erro claro em vez de uma falha no meio do upload.
//
// O readiness usa o pool COM TETO de espera de propósito: sob saturação, um
// ping que fica dez segundos na fila faz a sonda expirar e a instância ser
// marcada como fora do ar por um problema de fila, não de banco. Com o teto, a
// resposta chega rápido e diz a verdade.
func ready(
	pool *repository.PoolComEspera,
	guarda *armazem.Guarda,
	entregador *despachante.Despachante,
	salas *hub.Hub,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancelar := context.WithTimeout(r.Context(), timeoutReady)
		defer cancelar()

		w.Header().Set("Content-Type", "application/json")

		if err := pool.Ping(ctx); err != nil {
			slog.Error("readiness: banco indisponível", slog.String("erro", err.Error()))
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]any{
				"status": "degradado", "banco": false, "escrita": false,
				"erro": "banco indisponível",
			})
			return
		}

		estadoDaEntrega := entregador.Estado()
		derrubados := salas.DerrubadosPorLentidao()

		podeEscrever, espaco, err := guarda.PodeEscrever(ctx)
		if err != nil {
			// Falha ao MEDIR não é falha de espaço: transformar "não consegui
			// olhar" em "recuso tudo" derrubaria o produto por causa do
			// instrumento.
			slog.Warn("readiness: não foi possível medir o disco", slog.String("erro", err.Error()))
		}

		resposta := map[string]any{
			"status": "ok", "banco": true, "escrita": podeEscrever,
			"discoLivreBytes":  espaco.LivreBytes,
			"discoLivrePorCem": espaco.LivrePorCem,
			// Sinais de DEGRADAÇÃO do tempo real. Não mudam o status — o
			// serviço está no ar e correto —, mas precisam ser visíveis de
			// fora: sem eles, "o quadro está lento" é um relato sem número.
			//
			// A8 transforma isto em métrica e alerta; aqui eles apenas deixam
			// de ser invisíveis.
			"tempoReal": map[string]any{
				"quadrosObservados":     estadoDaEntrega.QuadrosObservados,
				"atrasoMaximoRevisoes":  estadoDaEntrega.AtrasoMaximo,
				"derrubadosPorLentidao": derrubados,
			},
		}
		if !podeEscrever {
			resposta["status"] = "degradado"
			resposta["erro"] = "sem margem de disco: escrita suspensa"
			slog.Error("readiness: disco sem margem, escrita suspensa",
				slog.Uint64("livre_bytes", espaco.LivreBytes),
				slog.Float64("livre_por_cem", espaco.LivrePorCem))
		}
		json.NewEncoder(w).Encode(resposta)
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
