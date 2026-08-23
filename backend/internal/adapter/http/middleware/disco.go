package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"stacktrack/internal/adapter/armazem"
)

// PorteiroDeDisco recusa operações que aumentam estado quando não há mais
// margem no volume, mas preserva os caminhos de recuperação.
//
// Só mutações, e essa é a decisão central: um quadro que não aceita card novo
// mas mostra o que já existe é um sistema degradado; um que não abre é um
// sistema fora do ar. A diferença importa justamente na hora em que alguém
// precisa consultar o que está lá para decidir o que apagar.
//
// O padrão é conservador: leituras e DELETE passam, as demais mutações são
// barradas. Login e logout são as duas exceções explícitas que permitem entrar
// para liberar espaço sem reabrir cadastro e criação de conteúdo.
//
// 507 Insufficient Storage, e não 503: o servidor está no ar e respondendo, e o
// que falta é espaço. O 503 diria "tente de novo já", e tentar de novo não
// resolve nada até alguém liberar disco.
func PorteiroDeDisco(guarda *armazem.Guarda, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if guarda == nil || ehLeituraOuRecuperacao(r) {
				next.ServeHTTP(w, r)
				return
			}

			podeEscrever, espaco, err := guarda.PodeEscrever(r.Context())
			if err != nil {
				// Falha ao MEDIR não bloqueia: é problema do instrumento, não
				// do espaço, e recusar tudo por causa dele derrubaria o produto
				// sem que faltasse disco nenhum.
				log.Warn("não foi possível medir o disco", slog.String("erro", err.Error()))
				next.ServeHTTP(w, r)
				return
			}
			if podeEscrever {
				next.ServeHTTP(w, r)
				return
			}

			log.Error("escrita recusada: disco sem margem",
				slog.Uint64("livre_bytes", espaco.LivreBytes),
				slog.Float64("livre_por_cem", espaco.LivrePorCem))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInsufficientStorage)
			json.NewEncoder(w).Encode(map[string]string{
				"erro": "o servidor está sem espaço em disco; a escrita foi suspensa temporariamente",
			})
		})
	}
}

// ehLeituraOuRecuperacao informa se a operação não aumenta armazenamento ou
// se é necessária para que alguém consiga entrar e liberar espaço.
//
// OPTIONS entra na lista porque é o preflight do CORS: recusá-lo faria o
// navegador esconder o 507 atrás de um erro genérico de CORS, e quem usa o
// produto veria "falha de rede" no lugar da mensagem que explica o problema.
func ehLeituraOuRecuperacao(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodDelete:
		return true
	}

	// Login cria uma sessão pequena, mas sem ele uma pessoa cuja sessão venceu
	// não conseguiria autenticar para apagar anexos. Logout remove uma sessão.
	return r.Method == http.MethodPost &&
		(r.URL.Path == "/auth/login" || r.URL.Path == "/auth/logout")
}
