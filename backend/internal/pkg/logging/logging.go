// Package logging centraliza a configuração do logger estruturado (log/slog) e
// a correlação de logs por requisição. Em produção emite JSON (parseável por
// agregadores como Loki/CloudWatch/Datadog); em desenvolvimento, texto legível
// no terminal.
package logging

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Configurar define o logger padrão do processo (slog.Default). Quando
// producao é true, emite JSON em stdout; caso contrário, texto legível. Deve
// ser chamada uma vez, no início do main, antes de qualquer log.
func Configurar(producao bool) {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	var h slog.Handler
	if producao {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(h))
}

// SemRota é o que se registra no lugar de um caminho que não casou com rota
// nenhuma.
const SemRota = "(sem rota)"

var (
	prefixosMu         sync.RWMutex
	prefixosConhecidos = map[string]struct{}{}
)

// DefinirPrefixosConhecidos informa quais primeiros segmentos de caminho
// pertencem a rotas registradas (ex.: "boards", "convites"). Chamar uma vez no
// boot, antes de servir.
//
// Serve só ao log de 404: com a lista, "alguém bateu em algo sob /boards"
// continua aparecendo, e é a informação que faz um 404 valer a pena. Sem ela,
// todo caminho não casado vira SemRota — seguro, e cego.
func DefinirPrefixosConhecidos(prefixos []string) {
	prefixosMu.Lock()
	defer prefixosMu.Unlock()
	prefixosConhecidos = make(map[string]struct{}, len(prefixos))
	for _, p := range prefixos {
		prefixosConhecidos[p] = struct{}{}
	}
}

// Rota devolve o padrão da rota casada (ex.: "/boards/{boardID}"), NUNCA o
// caminho real.
//
// O padrão é o ponto: `/convites/{token}` é o que se registra de uma
// requisição a `/convites/8f3a…`, e o token não vai para o log.
//
// Quando nada casa (404), o caminho bruto NÃO é registrado. Antes ele era, e
// era um vazamento silencioso: quem digita `/convitess/<token>` erra a rota por
// uma letra, leva 404, e o token inteiro fica escrito no log de acesso em texto
// puro — junto com todo caminho secreto que alguém tente adivinhar. O que sobra
// é o primeiro segmento, e só quando ele pertence a uma rota conhecida; um
// segredo aparece como caminho de primeiro nível e por isso nunca está nessa
// lista.
func Rota(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if p := rctx.RoutePattern(); p != "" {
			return p
		}
	}
	return rotaDesconhecida(r.URL.Path)
}

func rotaDesconhecida(caminho string) string {
	primeiro, _, _ := strings.Cut(strings.TrimPrefix(caminho, "/"), "/")
	if primeiro == "" {
		return SemRota
	}
	prefixosMu.RLock()
	_, conhecido := prefixosConhecidos[primeiro]
	prefixosMu.RUnlock()
	if !conhecido {
		return SemRota
	}
	return "/" + primeiro + "/" + SemRota
}

// RequisicaoLogger devolve um logger com o request_id e a rota da requisição
// já anexados, para correlacionar todos os logs de uma mesma requisição —
// inclusive os assíncronos que ela dispara. Use nos handlers:
// logging.RequisicaoLogger(r).Error("...", slog.String("erro", err.Error())).
func RequisicaoLogger(r *http.Request) *slog.Logger {
	return slog.Default().With(
		slog.String("request_id", middleware.GetReqID(r.Context())),
		slog.String("rota", Rota(r)),
	)
}

// Middleware emite um log estruturado (nível INFO) por requisição HTTP — o log
// de acesso do sistema — com método, rota, status, duração, bytes, IP e
// request_id. A rota é o padrão casado, não o caminho, para não registrar
// tokens. Requisições a /health e /ready (sondas do container e do monitor
// externo, de alta frequência) são omitidas para não poluir o log — a falha do
// /ready já se registra sozinha, com nível ERROR.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.URL.Path == "/ready" {
			next.ServeHTTP(w, r)
			return
		}

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		inicio := time.Now()
		defer func() {
			slog.LogAttrs(r.Context(), slog.LevelInfo, "requisição http",
				slog.String("request_id", middleware.GetReqID(r.Context())),
				slog.String("metodo", r.Method),
				slog.String("rota", Rota(r)),
				slog.Int("status", ww.Status()),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Duration("duracao", time.Since(inicio)),
				slog.String("ip", r.RemoteAddr),
			)
		}()
		next.ServeHTTP(ww, r)
	})
}
