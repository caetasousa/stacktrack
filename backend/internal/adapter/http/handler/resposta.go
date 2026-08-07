// Helpers de resposta HTTP compartilhados pelos handlers: serialização,
// decodificação com validação e o tratamento de erro inesperado.

package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"stacktrack/internal/pkg/logging"

	"github.com/go-playground/validator/v10"
)

func responderJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func responderErro(w http.ResponseWriter, status int, msg string) {
	responderJSON(w, status, map[string]string{"erro": msg})
}

// responderErroInterno loga o erro real (com request_id e rota, para
// correlação) e responde 500 genérico. O erro nunca vai para o corpo da
// resposta — mensagem de erro de banco expõe nome de tabela, de coluna e às
// vezes o próprio dado; quem precisa dela é quem lê o log, não quem chamou.
func responderErroInterno(w http.ResponseWriter, r *http.Request, msg string, err error) {
	logging.RequisicaoLogger(r).Error(msg, slog.String("erro", err.Error()))
	responderErro(w, http.StatusInternalServerError, "erro interno")
}

// validavel é satisfeito pelos DTOs de entrada.
type validavel interface {
	Validar() error
}

// decodificarJSON lê o corpo da requisição em T e valida os campos. Responde
// 400 e devolve false quando o corpo não é JSON válido ou não passa na
// validação — o handler só precisa retornar.
func decodificarJSON[T validavel](w http.ResponseWriter, r *http.Request) (T, bool) {
	var req T
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		responderErro(w, http.StatusBadRequest, "corpo da requisição inválido")
		return req, false
	}
	if err := req.Validar(); err != nil {
		responderErro(w, http.StatusBadRequest, mensagemDeValidacao(err))
		return req, false
	}
	return req, true
}

// mensagemDeValidacao traduz o primeiro erro do validator para uma frase em
// português. Só o primeiro: a tela destaca um campo por vez, e devolver a
// lista inteira faria a mensagem crescer sem ajudar ninguém.
func mensagemDeValidacao(err error) string {
	var erros validator.ValidationErrors
	if !errors.As(err, &erros) || len(erros) == 0 {
		return "dados inválidos"
	}

	campo := erros[0].Field()
	switch erros[0].Tag() {
	case "required":
		return campo + " é obrigatório"
	case "email":
		return "email inválido"
	case "max":
		return campo + " é longo demais"
	default:
		return campo + " é inválido"
	}
}

// copiarParaResposta transmite o conteúdo de um anexo para o cliente.
func copiarParaResposta(w http.ResponseWriter, leitura io.Reader) (int64, error) {
	return io.Copy(w, leitura)
}

// logarFalhaDeDownload registra uma entrega interrompida no meio. Não há
// resposta a corrigir: o cabeçalho já foi enviado quando isso acontece.
func logarFalhaDeDownload(r *http.Request, anexoID string, err error) {
	logging.RequisicaoLogger(r).Warn("download de anexo interrompido",
		slog.String("anexo_id", anexoID), slog.String("erro", err.Error()))
}
