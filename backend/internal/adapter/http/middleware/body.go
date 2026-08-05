package middleware

import "net/http"

// LimitarCorpo devolve um middleware que limita o tamanho do corpo de cada
// requisição. A API só troca JSONs pequenos — sem o teto, um corpo gigante
// ocuparia memória e conexão à toa (vetor de negação de serviço).
func LimitarCorpo(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
