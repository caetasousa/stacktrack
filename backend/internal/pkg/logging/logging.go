// Package logging centraliza a configuração do logger estruturado (log/slog) e
// a correlação de logs por requisição. Em produção emite JSON (parseável por
// agregadores como Loki/CloudWatch/Datadog); em desenvolvimento, texto legível
// no terminal.
package logging

import (
	"log/slog"
	"net/http"
	"os"
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

// Rota devolve o padrão da rota casada (ex.: "/boards/{id}"), não o caminho
// real — assim tokens e ids que viajam no path não vão parar nos logs. Cai
// para o caminho bruto quando nenhuma rota casou (ex.: 404).
func Rota(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if p := rctx.RoutePattern(); p != "" {
			return p
		}
	}
	return r.URL.Path
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
