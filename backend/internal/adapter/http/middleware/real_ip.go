package middleware

import "net/http"

// IPReal reescreve r.RemoteAddr a partir do cabeçalho X-Real-IP, definido pelo
// proxy reverso (Caddy, na fase 8) com o IP real do cliente. Em produção a API
// não é exposta diretamente — só o proxy fala com ela, e ele sobrescreve
// qualquer X-Real-IP que o cliente tente enviar — então confiar nesse cabeçalho
// é seguro. Sem isto, o log de acesso e o rate limit por IP veriam sempre o IP
// do container do proxy, não o do cliente (todos os clientes cairiam no mesmo
// balde de rate limit). Em desenvolvimento (sem proxy) o cabeçalho não existe e
// r.RemoteAddr fica inalterado.
func IPReal(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip := r.Header.Get("X-Real-IP"); ip != "" {
			r.RemoteAddr = ip
		}
		next.ServeHTTP(w, r)
	})
}
