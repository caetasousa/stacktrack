package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"stacktrack/internal/pkg/limite"
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
// A contagem em si vive em pkg/limite, compartilhada com o teto de cookies de
// sessão desconhecidos; o que é próprio daqui é a RESPOSTA, que fala de conta.
type LimitadorPorConta struct {
	contador *limite.PorChave
}

// NovoLimitadorPorConta cria o limitador com o teto e a janela informados.
// limite <= 0 devolve nil — um limitador desligado, já que os métodos toleram
// o receptor nil.
func NovoLimitadorPorConta(teto int, janela time.Duration) *LimitadorPorConta {
	contador := limite.NovoPorChave(teto, janela)
	if contador == nil {
		return nil
	}
	return &LimitadorPorConta{contador: contador}
}

// Reservar ocupa atomicamente uma vaga antes da verificacao de senha. O
// chamador confirma a reserva somente se as credenciais forem invalidas e a
// cancela em sucesso ou falha de infraestrutura.
func (l *LimitadorPorConta) Reservar(w http.ResponseWriter, chave string) (*limite.Reserva, bool) {
	if l == nil {
		return nil, true
	}
	reserva, permitido := l.contador.Reservar(chave)
	if permitido {
		return reserva, true
	}
	w.Header().Set("Retry-After", strconv.Itoa(l.contador.SegundosDeEspera()))
	responderErro(w, http.StatusTooManyRequests, "muitas tentativas para esta conta; tente de novo em alguns minutos")
	return nil, false
}

// chaveDeConta monta a chave do limitador. O email vai em minúsculas para que
// variar a caixa não crie um balde novo a cada tentativa.
func chaveDeConta(prefixo, email string) string {
	return prefixo + ":" + strings.ToLower(strings.TrimSpace(email))
}
