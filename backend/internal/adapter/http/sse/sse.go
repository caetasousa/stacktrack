// Package sse é o adaptador de Server-Sent Events das notificações: mantém
// aberta a resposta de um GET e escreve nela, a cada toque da campainha,
// quantos avisos a pessoa tem sem ler.
//
// SSE, e não o WebSocket do quadro, porque aqui o caminho é de MÃO ÚNICA: o
// servidor avisa e a tela relê — marcar como lido é um POST comum. Sem caminho
// de volta, um GET que não termina resolve com HTTP puro: o cookie de sessão
// viaja como em qualquer requisição, a borda o repassa como qualquer resposta,
// e o navegador reconecta sozinho (EventSource) quando a conexão cai.
//
// O stream não carrega notificação nenhuma, só a contagem. A lista mora no
// banco, e a tela a relê quando quer mostrá-la: por isso não há `id:` nem
// reposição do que se perdeu numa queda — não há o que repor, a próxima leitura
// já é a verdade inteira.
//
// O formato no fio, para quem lê os bytes:
//
//	retry: 5000
//
//	event: notificacoes
//	data: {"versao":1,"naoLidas":3}
//
//	: batimento
//
//	event: encerrada
//	data: {"versao":1,"motivo":"sessao"}
package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	httpmiddleware "stacktrack/internal/adapter/http/middleware"
	"stacktrack/internal/adapter/realtime/campainha"
	"stacktrack/internal/adapter/realtime/hub"
	"stacktrack/internal/pkg/limite"

	"github.com/go-chi/httprate"
)

// Contador devolve quantos avisos a pessoa tem sem ler.
type Contador func(ctx context.Context, usuarioID string) (int, error)

// Identificador extrai a identidade do contexto, já validada pelo middleware
// de sessão.
type Identificador func(ctx context.Context) (string, bool)

// SessaoValida informa se o token de sessão continua valendo. O middleware de
// autenticação roda uma vez, na abertura, e o stream dura horas: sem
// reconferir, o logout apagaria a sessão no banco e a aba continuaria
// recebendo.
type SessaoValida func(ctx context.Context, token string) bool

// versaoDoEnvelope identifica o formato dos dados, como no WebSocket: o cliente
// que receber uma versão maior do que conhece relê pela API em vez de
// interpretar pela metade.
const versaoDoEnvelope = 1

// esperaDeReconexao é o `retry:` mandado no começo do stream: quanto o
// navegador espera para reabrir quando a conexão cai. Cinco segundos, e não o
// padrão de cerca de três: o aviso é de "olhe de novo", e martelar um servidor
// que acabou de reiniciar não traz nada mais rápido para quem lê.
const esperaDeReconexao = 5 * time.Second

// Motivos do evento `encerrada`. O cliente decide por eles se reabre: sessão
// encerrada não adianta reabrir; servidor desligando, sim.
const (
	motivoSessao   = "sessao"
	motivoServidor = "servidor"
)

// Handler serve o stream de notificações.
type Handler struct {
	campainha    *campainha.Campainha
	contar       Contador
	identidade   Identificador
	sessaoValida SessaoValida
	nomeCookie   string
	limites      *hub.Limites
	aberturas    *limite.PorChave
	log          *slog.Logger

	// batimento é o intervalo do comentário que mantém a conexão viva. A
	// Cloudflare derruba resposta parada há 100 s, e o nginx da borda a 1 h —
	// 25 s fica com folga abaixo dos dois.
	batimento time.Duration
	// revalidarACada é de quanto em quanto a sessão é reconferida e a contagem
	// relida. Reler aqui cobre o que muda só com o relógio: o convite que
	// venceu deixa de contar sem ninguém tocar campainha nenhuma.
	revalidarACada time.Duration
	// prazoDeEscrita limita cada escrita: um cliente que parou de ler não pode
	// prender a goroutine para sempre.
	prazoDeEscrita time.Duration
	// prazoDeConsulta limita cada ida ao banco feita durante o stream, que não
	// tem prazo próprio.
	prazoDeConsulta time.Duration
}

// NovoHandler cria o handler. Sem ComLimites, nada é contado — o que os testes
// querem; produção os liga, e config.Validar recusa subir sem eles.
func NovoHandler(
	c *campainha.Campainha,
	contar Contador,
	identidade Identificador,
	sessaoValida SessaoValida,
	nomeCookie string,
	log *slog.Logger,
) *Handler {
	return &Handler{
		campainha: c, contar: contar, identidade: identidade,
		sessaoValida: sessaoValida, nomeCookie: nomeCookie, log: log,
		limites:         hub.NovosLimites(0, 0),
		batimento:       25 * time.Second,
		revalidarACada:  30 * time.Second,
		prazoDeEscrita:  10 * time.Second,
		prazoDeConsulta: 5 * time.Second,
	}
}

// ComIntervalos troca o batimento e a revalidação — os testes não esperam
// trinta segundos. Valor não positivo mantém o atual.
func (h *Handler) ComIntervalos(batimento, revalidacao time.Duration) *Handler {
	if batimento > 0 {
		h.batimento = batimento
	}
	if revalidacao > 0 {
		h.revalidarACada = revalidacao
	}
	return h
}

// ComLimites liga o teto de streams simultâneos (por conta e no processo) e o
// de aberturas por minuto por IP, com a mesma lógica dos tetos do WebSocket:
// por conta impede uma pessoa de consumir a capacidade de todo mundo, o global
// protege a memória do processo, e o por IP cobre quem abre e fecha em laço
// sem nunca ocupar vaga.
func (h *Handler) ComLimites(porConta, global, aberturasPorMinuto int, janela time.Duration) *Handler {
	h.limites = hub.NovosLimites(porConta, global)
	h.aberturas = limite.NovoPorChave(aberturasPorMinuto, janela)
	return h
}

// Abertas informa quantos streams estão abertos agora.
func (h *Handler) Abertas() int {
	return h.limites.Abertas()
}

// Acompanhar abre o stream da pessoa autenticada e o mantém até ela sair, a
// sessão acabar ou o servidor desligar.
func (h *Handler) Acompanhar(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.identidade(r.Context())
	if !ok {
		http.Error(w, "não autenticado", http.StatusUnauthorized)
		return
	}

	if h.aberturas != nil {
		chave, err := httprate.KeyByIP(r)
		if err != nil {
			chave = r.RemoteAddr
		}
		reserva, permitido := h.aberturas.Reservar(chave)
		if !permitido {
			w.Header().Set("Retry-After", strconv.Itoa(h.aberturas.SegundosDeEspera()))
			http.Error(w, "muitas aberturas do stream de notificações", http.StatusTooManyRequests)
			return
		}
		reserva.Confirmar(w)
	}

	liberar, motivo := h.limites.Reservar(usuarioID)
	if motivo != "" {
		h.log.Info("stream de notificações recusado", "motivo", string(motivo), "usuario", usuarioID)
		w.Header().Set("Retry-After", "30")
		http.Error(w, string(motivo), http.StatusServiceUnavailable)
		return
	}
	defer liberar()

	// A campainha é assinada ANTES da primeira contagem, pela mesma razão de o
	// WebSocket assinar a sala antes de repor o histórico: um aviso gravado
	// entre a contagem e a assinatura não tocaria ninguém, e a tela abriria com
	// um número velho até o próximo toque.
	ouvinte := h.campainha.Assinar(usuarioID)
	if ouvinte == nil {
		w.Header().Set("Retry-After", "5")
		http.Error(w, "servidor encerrando", http.StatusServiceUnavailable)
		return
	}
	defer h.campainha.Cancelar(ouvinte)

	ctx, encerrar := httpmiddleware.LiberarDoPrazo(w, r)
	defer encerrar()

	// O token da sessão, guardado para ser reconferido enquanto o stream vive.
	var token string
	if c, err := r.Cookie(h.nomeCookie); err == nil {
		token = c.Value
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	// O nginx guarda a resposta em buffer por padrão, e o navegador receberia
	// os avisos todos juntos, atrasados. Este cabeçalho desliga o buffer só
	// para esta resposta — a config da borda também o desliga na rota, e os
	// dois juntos não dependem de alguém lembrar do outro.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	controle := http.NewResponseController(w)
	escrever := func(bloco string) error {
		_ = controle.SetWriteDeadline(time.Now().Add(h.prazoDeEscrita))
		if _, err := io.WriteString(w, bloco); err != nil {
			return err
		}
		return controle.Flush()
	}

	if err := escrever(fmt.Sprintf("retry: %d\n\n", esperaDeReconexao.Milliseconds())); err != nil {
		return
	}
	ultima := -1
	enviarContagem := func(sempre bool) error {
		consulta, cancelar := context.WithTimeout(ctx, h.prazoDeConsulta)
		n, err := h.contar(consulta, usuarioID)
		cancelar()
		if err != nil {
			// Sem a contagem, não manda nada: um número inventado seria pior
			// que o atraso até o próximo toque ou revalidação.
			h.log.Warn("stream de notificações: contagem falhou", "usuario", usuarioID, "erro", err)
			return nil
		}
		if !sempre && n == ultima {
			return nil
		}
		ultima = n
		return escrever(evento("notificacoes", map[string]any{"versao": versaoDoEnvelope, "naoLidas": n}))
	}
	if err := enviarContagem(true); err != nil {
		return
	}

	batimento := time.NewTicker(h.batimento)
	defer batimento.Stop()
	revalidar := time.NewTicker(h.revalidarACada)
	defer revalidar.Stop()

	for {
		select {
		case <-ctx.Done():
			// O cliente fechou a aba ou a conexão caiu.
			return

		case _, aberto := <-ouvinte.Toques:
			if !aberto {
				_ = escrever(evento("encerrada", map[string]any{"versao": versaoDoEnvelope, "motivo": motivoServidor}))
				return
			}
			// O toque sempre vira evento, mesmo com a contagem igual: um aviso
			// lido e outro novo deixam o número onde estava, e a lista aberta
			// na tela precisa ser relida do mesmo jeito.
			if err := enviarContagem(true); err != nil {
				return
			}

		case <-revalidar.C:
			if !h.aindaValida(ctx, token) {
				h.log.Info("stream de notificações encerrado: a sessão deixou de valer", "usuario", usuarioID)
				_ = escrever(evento("encerrada", map[string]any{"versao": versaoDoEnvelope, "motivo": motivoSessao}))
				return
			}
			if err := enviarContagem(false); err != nil {
				return
			}

		case <-batimento.C:
			if err := escrever(": batimento\n\n"); err != nil {
				return
			}
		}
	}
}

// aindaValida reconfere a sessão, com prazo próprio: o contexto do stream não
// tem deadline, e um banco travado prenderia a goroutine e a vaga.
func (h *Handler) aindaValida(ctx context.Context, token string) bool {
	if h.sessaoValida == nil {
		return true
	}
	prazo, cancelar := context.WithTimeout(ctx, h.prazoDeConsulta)
	defer cancelar()
	return h.sessaoValida(prazo, token)
}

// evento monta um bloco do stream. Os dados vão numa linha só — o JSON não tem
// quebra de linha —, porque cada `data:` a mais seria juntado pelo navegador
// com um \n no meio.
func evento(nome string, dados any) string {
	corpo, err := json.Marshal(dados)
	if err != nil {
		corpo = []byte("{}")
	}
	return "event: " + nome + "\ndata: " + string(corpo) + "\n\n"
}
