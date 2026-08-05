package handler

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/httprate"
)

// LimitadorPorConta limita tentativas por conta (o email informado), e não por
// IP. É o teto que falta quando o atacante tem endereços de sobra — uma botnet
// ou uma faixa IPv6 inteira faz brute-force sem nunca encostar no limite por
// IP, porque cada tentativa chega de um endereço novo.
//
// Só tentativa fracassada é contabilizada (ver Excedido e Registrar): acertar a
// senha não consome cota, então o uso normal nunca aproxima a conta do teto.
// Estourado o teto, porém, a janela vale para todo mundo — inclusive para quem
// digita a senha certa em seguida. É de propósito: barrar só senha errada não
// para o atacante que, na enésima tentativa, acerta. O preço é conhecido: quem
// sabe o email de alguém consegue mantê-lo trancado enquanto insistir. Como a
// janela é curta e se renova sozinha, o estrago máximo é um atraso de minutos —
// bem menor que o de uma conta tomada.
//
// O contador é em memória, servindo a um processo só — que é a topologia do
// projeto. Com mais de uma réplica, cada uma teria o seu contador e o teto
// efetivo seria multiplicado pelo número de réplicas; nesse dia, um contador
// compartilhado (httprate.WithLimitCounter).
type LimitadorPorConta struct {
	limitador *httprate.RateLimiter
	limite    int
	janela    time.Duration
}

// NovoLimitadorPorConta cria o limitador com o teto e a janela informados.
// limite <= 0 devolve nil — um limitador desligado, já que os métodos toleram
// o receptor nil.
func NovoLimitadorPorConta(limite int, janela time.Duration) *LimitadorPorConta {
	if limite <= 0 {
		return nil
	}
	return &LimitadorPorConta{
		limitador: httprate.NewRateLimiter(limite, janela),
		limite:    limite,
		janela:    janela,
	}
}

// Excedido responde 429 e devolve true quando a conta já estourou o teto. Não
// contabiliza a tentativa atual — isso é papel de Registrar. Falha do contador
// nunca barra a requisição: um erro de infraestrutura não pode virar porta
// fechada para quem tem a senha certa.
func (l *LimitadorPorConta) Excedido(w http.ResponseWriter, r *http.Request, chave string) bool {
	if l == nil {
		return false
	}
	// Status devolve a taxa acumulada na janela sem incrementá-la; a conta
	// estoura quando ESTA tentativa passaria do teto — o mesmo critério que o
	// httprate usa internamente (rate + 1 > limite).
	_, taxa, err := l.limitador.Status(chave)
	if err != nil || int(math.Round(taxa))+1 <= l.limite {
		return false
	}
	w.Header().Set("Retry-After", strconv.Itoa(int(l.janela.Seconds())))
	responderErro(w, http.StatusTooManyRequests, "muitas tentativas para esta conta; tente de novo em alguns minutos")
	return true
}

// Registrar contabiliza uma tentativa na conta. Chamar antes de escrever a
// resposta, para os cabeçalhos X-RateLimit-* saírem junto.
func (l *LimitadorPorConta) Registrar(w http.ResponseWriter, r *http.Request, chave string) {
	if l == nil {
		return
	}
	l.limitador.OnLimit(w, r, chave)
}

// chaveDeConta monta a chave do limitador. O email vai em minúsculas para que
// variar a caixa não crie um balde novo a cada tentativa.
func chaveDeConta(prefixo, email string) string {
	return prefixo + ":" + strings.ToLower(strings.TrimSpace(email))
}
