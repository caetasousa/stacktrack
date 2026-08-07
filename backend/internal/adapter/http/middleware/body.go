package middleware

import (
	"net/http"
	"strings"
)

// LimitarCorpo devolve um middleware que limita o tamanho do corpo de cada
// requisição. Sem teto, um corpo gigante ocuparia memória e conexão à toa
// (vetor de negação de serviço).
//
// São dois tetos porque são dois tipos de tráfego: a API troca JSONs pequenos,
// e o upload de anexo precisa de megabytes. O tipo do conteúdo decide qual
// vale — e não a rota, porque teto por rota deixa de valer no dia em que
// alguém cria uma rota nova e esquece de listá-la.
//
// O teto de upload é o teto do TRANSPORTE. Quem decide se o arquivo é aceitável
// continua sendo o domínio (anexo.TamanhoMaximoArquivo), que responde uma
// mensagem que faz sentido para quem enviou.
func LimitarCorpo(maxJSON, maxUpload int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limite := maxJSON
			if ehUpload(r) {
				limite = maxUpload
			}
			r.Body = http.MaxBytesReader(w, r.Body, limite)
			next.ServeHTTP(w, r)
		})
	}
}

// ehUpload informa se a requisição carrega um envio de arquivo.
func ehUpload(r *http.Request) bool {
	return strings.HasPrefix(
		strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))),
		"multipart/form-data",
	)
}
