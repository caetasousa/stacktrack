package middleware

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"
)

// Prazo põe um deadline no contexto de cada requisição HTTP comum.
//
// É o teto que faltava entre o ReadHeaderTimeout (que corta quem não manda
// cabeçalho) e o statement_timeout (que corta uma query lenta): uma requisição
// pode ficar presa sem nenhum dos dois disparar — esperando conexão do pool,
// esperando um lock que não é o do quadro, ou somando várias operações rápidas
// que juntas não terminam nunca.
//
// O deadline vai no CONTEXTO, e não num timer que responde por fora. A
// diferença importa: o contexto cancelado propaga até o pgx, que aborta a query
// e devolve a conexão ao pool. Um timeout que só escrevesse a resposta deixaria
// o trabalho rodando atrás dela, segurando exatamente os recursos que o teto
// existe para proteger.
//
// Cancelamento do CLIENTE — a aba fechada no meio de um carregamento — já chega
// pelo mesmo caminho: o contexto da requisição é cancelado pelo net/http, e o
// caso de uso e a query morrem junto.
//
// O handshake do WebSocket passa por aqui como qualquer GET. Depois do 101 o
// handler destaca o contexto da conexão longa; assim autenticação, autorização
// e upgrade têm teto, mas o socket não morre dez segundos depois.
func Prazo(comum, upload time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limite := comum
			if ehUpload(r) {
				// O upload tem orçamento próprio, e maior: dez megabytes numa
				// conexão móvel ruim passam de dez segundos sem nada de errado
				// acontecendo. Herdar o teto comum recusaria envios legítimos.
				limite = upload
			}

			ctx, cancelar := context.WithTimeout(r.Context(), limite)
			defer cancelar()
			// O contexto de fora fica guardado para a única saída prevista:
			// a resposta longa do SSE, que pede para sair do prazo por
			// LiberarDoPrazo. Ver lá por que não basta ignorar o deadline.
			ctx = context.WithValue(ctx, chaveDoContextoSemPrazo{}, r.Context())

			// O vigia e a checagem final ficam AQUI, e não num middleware
			// separado por fora. Tentei separá-los e o teste mostrou por que
			// não dá: o middleware de fora enxerga o contexto de FORA, que não
			// tem deadline nenhum — ele nunca veria o prazo estourar. Quem cria
			// o contexto é quem pode conferir se ele expirou.
			vigia := &respostaVigiada{ResponseWriter: w}
			// Cancelar o contexto interrompe pgx, mas não uma leitura bloqueada no
			// socket. O deadline de leitura fecha esse segundo caminho. No upgrade,
			// Hijack limpa o prazo antes de entregar a conexao ao WebSocket.
			controle := http.NewResponseController(vigia)
			agora := time.Now()
			_ = controle.SetReadDeadline(agora.Add(limite))
			// Um segundo de folga permite escrever o 503 depois que o contexto
			// expira; sem ela o proprio deadline impediria a resposta de sair.
			_ = controle.SetWriteDeadline(agora.Add(limite + time.Second))
			defer func() {
				_ = controle.SetReadDeadline(time.Time{})
				_ = controle.SetWriteDeadline(time.Time{})
			}()
			next.ServeHTTP(vigia, r.WithContext(ctx))

			// Só responde se NADA foi escrito. Um handler que já começou a
			// mandar corpo não pode receber outro status, e sobrescrever ali
			// produziria uma resposta corrompida no lugar de uma truncada.
			if vigia.escreveu || !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return
			}
			// 503 com Retry-After, e não 500: o servidor não falhou, ele
			// desistiu de esperar — e repetir é a ação certa.
			w.Header().Set("Retry-After", "1")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{
				"erro": "a requisição demorou demais; tente de novo",
			})
		})
	}
}

type chaveDoContextoSemPrazo struct{}

// LiberarDoPrazo tira uma resposta LONGA do teto de Prazo e devolve o contexto
// com que ela deve viver: com os valores da requisição (identidade, request
// id), sem deadline, e cancelado quando o cliente desconecta — ou quando quem
// chamou usa o cancel devolvido.
//
// Existe para o SSE, que é um GET comum cuja resposta não termina: o handshake
// do WebSocket tem a saída dele (o Hijack), e uma resposta que continua sendo
// escrita não tem. A saída é EXPLÍCITA, chamada pelo handler que foi feito
// para viver horas — e nunca decidida por cabeçalho do cliente, que daria a
// qualquer requisição um jeito de escapar do teto.
//
// Limpa também os deadlines de leitura e de escrita do socket. O de leitura não
// é detalhe: no HTTP/1.1 o servidor fica lendo o socket em segundo plano para
// perceber o cliente que fechou, e essa leitura esbarrando no deadline seria
// tomada por desconexão — a resposta cairia nos mesmos dez segundos. Quem chama
// passa a dar prazo a cada escrita.
func LiberarDoPrazo(w http.ResponseWriter, r *http.Request) (context.Context, context.CancelFunc) {
	controle := http.NewResponseController(w)
	_ = controle.SetReadDeadline(time.Time{})
	_ = controle.SetWriteDeadline(time.Time{})

	pai, ok := r.Context().Value(chaveDoContextoSemPrazo{}).(context.Context)
	if !ok {
		// Fora de Prazo (nos testes do handler, por exemplo), o contexto da
		// requisição já é o que se quer.
		pai = r.Context()
	}
	ctx, cancelar := context.WithCancel(context.WithoutCancel(r.Context()))
	parar := context.AfterFunc(pai, cancelar)
	return ctx, func() {
		parar()
		cancelar()
	}
}

// respostaVigiada registra se o handler já escreveu alguma coisa.
type respostaVigiada struct {
	http.ResponseWriter
	escreveu bool
}

// Unwrap permite que ResponseController alcance os recursos opcionais do
// writer real.
func (v *respostaVigiada) Unwrap() http.ResponseWriter { return v.ResponseWriter }

// Hijack preserva o upgrade do WebSocket e, antes de entregar o net.Conn,
// remove o deadline usado para limitar a leitura do handshake HTTP. Sem limpar,
// o socket longo cairia quando o prazo da requisição vencesse.
func (v *respostaVigiada) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conexao, buffer, err := http.NewResponseController(v.ResponseWriter).Hijack()
	if err != nil {
		return nil, nil, err
	}
	v.escreveu = true
	_ = conexao.SetReadDeadline(time.Time{})
	_ = conexao.SetWriteDeadline(time.Time{})
	return conexao, buffer, nil
}

func (v *respostaVigiada) WriteHeader(status int) {
	v.escreveu = true
	v.ResponseWriter.WriteHeader(status)
}

func (v *respostaVigiada) Write(b []byte) (int, error) {
	v.escreveu = true
	return v.ResponseWriter.Write(b)
}
