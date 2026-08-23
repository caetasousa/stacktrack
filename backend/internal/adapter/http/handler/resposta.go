// Helpers de resposta HTTP compartilhados pelos handlers: serialização,
// decodificação com validação e o tratamento de erro inesperado.

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"

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
// correlação) e responde 500 genérico, salvo quando o prazo da requisição
// venceu: nesse caso devolve 503 com Retry-After. O erro nunca vai para o corpo
// da resposta — mensagem de erro de banco expõe nome de tabela, de coluna e às
// vezes o próprio dado; quem precisa dela é quem lê o log, não quem chamou.
func responderErroInterno(w http.ResponseWriter, r *http.Request, msg string, err error) {
	logging.RequisicaoLogger(r).Error(msg, slog.String("erro", err.Error()))
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(r.Context().Err(), context.DeadlineExceeded) {
		w.Header().Set("Retry-After", "1")
		responderErro(w, http.StatusServiceUnavailable, "a requisição demorou demais; tente de novo")
		return
	}
	responderErro(w, http.StatusInternalServerError, "erro interno")
}

// validavel é satisfeito pelos DTOs de entrada.
type validavel interface {
	Validar() error
}

// decodificarJSON lê o corpo da requisição em T e valida os campos. Responde
// 400 e devolve false quando o corpo não presta — o handler só precisa retornar.
//
// A leitura é ESTRITA, e cada regra abaixo fecha um caminho real:
//
//   - Content-Type precisa ser JSON. Sem a checagem, um formulário HTML de
//     outro site consegue POSTar para a API sem preflight de CORS (o navegador
//     considera `text/plain` e `multipart/form-data` requisições "simples"), e
//     o SameSite=Lax do cookie vira a única defesa de CSRF em vez da segunda.
//
//   - Campo desconhecido é RECUSADO. Aceitá-lo em silêncio faz um erro de
//     digitação do cliente (`titulo` como `titlo`) virar "o servidor ignorou o
//     que mandei", que é diagnosticado por eliminação depois de muito tempo. E
//     fecha a porta de um campo antigo continuar sendo enviado depois de a API
//     ter deixado de aceitá-lo.
//
//   - Lixo DEPOIS do objeto é recusado. `{"a":1}{"b":2}` era lido como o
//     primeiro documento e o resto era descartado: dois pedidos entravam como
//     um, e o segundo desaparecia sem erro.
//
// O corpo já vem limitado por middleware.LimitarCorpo, então não há teto de
// tamanho a aplicar aqui.
func decodificarJSON[T validavel](w http.ResponseWriter, r *http.Request) (T, bool) {
	var req T

	if !ehJSON(r) {
		responderErro(w, http.StatusUnsupportedMediaType, "envie o corpo como application/json")
		return req, false
	}

	decodificador := json.NewDecoder(r.Body)
	decodificador.DisallowUnknownFields()
	if err := decodificador.Decode(&req); err != nil {
		responderErro(w, http.StatusBadRequest, mensagemDeCorpoInvalido(err))
		return req, false
	}
	// Um segundo Decode que NÃO devolva io.EOF significa que sobrou conteúdo.
	if err := decodificador.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		responderErro(w, http.StatusBadRequest, "envie um único objeto JSON no corpo")
		return req, false
	}

	if err := req.Validar(); err != nil {
		responderErro(w, http.StatusBadRequest, mensagemDeValidacao(err))
		return req, false
	}
	return req, true
}

// ehJSON informa se o Content-Type declara JSON. Corpo vazio sem Content-Type
// nenhum passa: é o caso das rotas que aceitam corpo opcional.
func ehJSON(r *http.Request) bool {
	bruto := r.Header.Get("Content-Type")
	if bruto == "" {
		return true
	}
	tipo, _, err := mime.ParseMediaType(bruto)
	if err != nil {
		return false
	}
	return tipo == "application/json"
}

// mensagemDeCorpoInvalido traduz o erro do decodificador numa frase útil.
//
// O campo desconhecido merece nome próprio: "corpo inválido" mandaria quem
// integra procurar erro de sintaxe num JSON que está perfeitamente bem
// formado.
func mensagemDeCorpoInvalido(err error) string {
	var maior *http.MaxBytesError
	if errors.As(err, &maior) {
		return "corpo da requisição grande demais"
	}
	if msg := err.Error(); strings.HasPrefix(msg, "json: unknown field ") {
		return "campo desconhecido no corpo: " + strings.TrimPrefix(msg, "json: unknown field ")
	}
	return "corpo da requisição inválido"
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
