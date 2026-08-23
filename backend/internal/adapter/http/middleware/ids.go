package middleware

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// sufixoDeID é o que marca um parâmetro de URL como identificador.
//
// A convenção do projeto é `{boardID}`, `{cardID}`, `{usuarioID}` — sempre
// terminando em "ID". Amarrar a checagem à convenção, em vez de a uma lista de
// nomes, é o que faz uma rota nova nascer validada: quem acrescentar
// `{etiquetaID}` amanhã não precisa lembrar de cadastrá-la em lugar nenhum.
//
// Parâmetros que NÃO são id ficam de fora pelo mesmo critério: `{token}` é
// base64url de 256 bits, e exigir formato de UUID dele quebraria o convite.
const sufixoDeID = "ID"

// IDsDaURLValidos recusa, com 400, requisição cujo parâmetro de id não seja um
// UUID.
//
// Sem isto, `/cards/nao-e-um-uuid` chegava ao repositório e o PostgreSQL
// respondia `invalid input syntax for type uuid`. Isso vira 500 — "erro
// interno" para uma entrada malformada que nunca teve chance —, enche o log de
// erro com tráfego de varredura e gasta uma conexão do pool por tentativa. É
// entrada externa mal formada, e o lugar de recusá-la é a borda.
//
// ⚠️ Precisa ser registrado num roteador que JÁ tenha resolvido o parâmetro:
// `r.Route("/cards/{cardID}", func(r chi.Router) { r.Use(IDsDaURLValidos) })`
// funciona; um `r.Use` no grupo acima dele roda antes do casamento da rota e
// veria a lista de parâmetros vazia. O teste em test/middleware cobre os dois
// arranjos justamente para essa diferença não passar despercebida.
func IDsDaURLValidos(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		if rctx == nil {
			next.ServeHTTP(w, r)
			return
		}

		for i, nome := range rctx.URLParams.Keys {
			if !strings.HasSuffix(nome, sufixoDeID) {
				continue
			}
			valor := rctx.URLParams.Values[i]
			if valor == "" {
				continue
			}
			if _, err := uuid.Parse(valor); err != nil {
				// O VALOR não vai na mensagem: ele veio de fora, e ecoá-lo
				// devolveria conteúdo controlado pelo cliente numa resposta que
				// o navegador pode renderizar.
				responderIDInvalido(w, nome)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func responderIDInvalido(w http.ResponseWriter, nome string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{
		"erro": "o identificador informado em " + nome + " não é válido",
	})
}
