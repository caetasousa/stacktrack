// Package correio entrega o que o outbox de email guarda: o worker que pega
// as mensagens na fila, as manda pelo provedor e registra o desfecho.
//
// Fica separado de usecase/board de propósito. O quadro PEDE envios, dentro da
// transação do convite; este pacote os ENTREGA, depois do commit e fora de
// qualquer requisição. Um não espera pelo outro — é isso que impede o SMTP de
// virar parte da transação ou um ponto único de falha do convite.
package correio

import (
	"context"
	"errors"
	"log/slog"
	"time"

	demail "stacktrack/internal/domain/email"
)

// Reservado é um envio que o worker tirou da fila para tentar agora, com a
// mensagem já decifrada — ou o motivo de ela não poder ser lida.
type Reservado struct {
	Envio    demail.Envio
	Mensagem demail.Mensagem
	// ErroDeLeitura é preenchido quando a mensagem não decifra (chave trocada,
	// dado adulterado). É definitivo: o envio falha sem tentar.
	ErroDeLeitura error
}

// Fila é o outbox do ponto de vista de quem entrega.
type Fila interface {
	Reservar(ctx context.Context, agora time.Time, limite int, reserva time.Duration) ([]Reservado, error)
	MarcarEnviado(ctx context.Context, id string, tentativas int, em time.Time) error
	Adiar(ctx context.Context, id string, tentativas int, proxima time.Time, erro string) error
	MarcarFalhou(ctx context.Context, id string, tentativas int, erro string) error
}

// Enviador entrega uma mensagem ao provedor. A recusa definitiva vem
// embrulhada em demail.ErrRecusaPermanente.
type Enviador interface {
	Enviar(ctx context.Context, m demail.Mensagem) error
}

// Carteiro é o worker do outbox.
type Carteiro struct {
	fila     Fila
	enviador Enviador
	log      *slog.Logger
	relogio  func() time.Time

	intervalo time.Duration
	// porPassada é quantos envios cada volta pega. O volume é de convites, e
	// uma passada pequena mantém curta a reserva que outra instância respeita.
	porPassada int
	// reserva é por quanto tempo um envio pego fica fora da fila. Maior que o
	// prazo da conversa SMTP: um envio só pode voltar à fila depois de a
	// tentativa em curso já ter certamente acabado.
	reserva time.Duration
	// prazoPorEnvio é o teto de uma entrega.
	prazoPorEnvio time.Duration

	acordar chan struct{}
}

// NovoCarteiro cria o worker. Sem ComIntervalo, ele olha a fila a cada cinco
// segundos — e é acordado na hora por quem enfileira (Acordar).
func NovoCarteiro(fila Fila, enviador Enviador, log *slog.Logger) *Carteiro {
	return &Carteiro{
		fila: fila, enviador: enviador, log: log, relogio: time.Now,
		intervalo:     5 * time.Second,
		porPassada:    20,
		reserva:       2 * time.Minute,
		prazoPorEnvio: 45 * time.Second,
		acordar:       make(chan struct{}, 1),
	}
}

// ComRelogio troca o relógio — o dublê de fronteira dos testes, que avançam o
// tempo em vez de dormir.
func (c *Carteiro) ComRelogio(relogio func() time.Time) *Carteiro {
	c.relogio = relogio
	return c
}

// ComIntervalo troca de quanto em quanto tempo a fila é olhada.
func (c *Carteiro) ComIntervalo(intervalo time.Duration) *Carteiro {
	if intervalo > 0 {
		c.intervalo = intervalo
	}
	return c
}

// Acordar pede uma passada agora. Não bloqueia: se já há um pedido pendente,
// este se junta a ele.
func (c *Carteiro) Acordar() {
	select {
	case c.acordar <- struct{}{}:
	default:
	}
}

// Rodar é o laço do worker, até o contexto acabar.
func (c *Carteiro) Rodar(ctx context.Context) {
	relogio := time.NewTicker(c.intervalo)
	defer relogio.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-relogio.C:
		case <-c.acordar:
		}
		if _, err := c.Passada(ctx); err != nil && ctx.Err() == nil {
			c.log.Warn("outbox de email: passada falhou", slog.String("erro", err.Error()))
		}
	}
}

// Passada pega o que está na hora e tenta entregar. Devolve quantos saíram.
func (c *Carteiro) Passada(ctx context.Context) (int, error) {
	agora := c.relogio()
	reservados, err := c.fila.Reservar(ctx, agora, c.porPassada, c.reserva)
	if err != nil {
		return 0, err
	}
	enviados := 0
	for _, item := range reservados {
		if ctx.Err() != nil {
			// A reserva vence sozinha e o envio volta à fila: interromper aqui
			// não perde nada.
			return enviados, nil
		}
		if c.entregar(ctx, item) {
			enviados++
		}
	}
	return enviados, nil
}

// entregar tenta um envio e registra o desfecho. Devolve true se saiu.
func (c *Carteiro) entregar(ctx context.Context, item Reservado) bool {
	e := item.Envio
	tentativas := e.Tentativas + 1

	err := item.ErroDeLeitura
	if err == nil {
		prazo, cancelar := context.WithTimeout(ctx, c.prazoPorEnvio)
		err = c.enviador.Enviar(prazo, item.Mensagem)
		cancelar()
	} else {
		err = errors.Join(demail.ErrRecusaPermanente, err)
	}

	if err == nil {
		if errMarcar := c.fila.MarcarEnviado(ctx, e.ID, tentativas, c.relogio()); errMarcar != nil {
			// O servidor aceitou e a marca não entrou: quando a reserva vencer,
			// o email sai de novo. Uma duplicata é o desfecho tolerável de um
			// estado torto — perder o convite não seria.
			c.log.Warn("email entregue mas não marcado",
				slog.String("envio", e.ID), slog.String("erro", errMarcar.Error()))
		}
		c.log.Info("email entregue", slog.String("envio", e.ID), slog.String("modelo", e.Modelo),
			slog.Int("tentativas", tentativas))
		return true
	}

	// O texto do erro vem do servidor SMTP, que costuma repetir o destinatário:
	// nem o banco nem o log recebem o endereço inteiro.
	motivo := demail.MascararEnderecos(err.Error())
	desfecho := demail.AposFalha(tentativas, err, c.relogio())
	if desfecho.Desistir {
		if errMarcar := c.fila.MarcarFalhou(ctx, e.ID, tentativas, motivo); errMarcar != nil {
			c.log.Warn("não foi possível registrar a desistência do email",
				slog.String("envio", e.ID), slog.String("erro", errMarcar.Error()))
		}
		c.log.Error("email não entregue: desistiu", slog.String("envio", e.ID),
			slog.String("modelo", e.Modelo), slog.Int("tentativas", tentativas), slog.String("erro", motivo))
		return false
	}
	if errAdiar := c.fila.Adiar(ctx, e.ID, tentativas, desfecho.ProximaEm, motivo); errAdiar != nil {
		c.log.Warn("não foi possível adiar o email",
			slog.String("envio", e.ID), slog.String("erro", errAdiar.Error()))
	}
	c.log.Warn("email não entregue: vai tentar de novo", slog.String("envio", e.ID),
		slog.Int("tentativas", tentativas), slog.Time("proxima", desfecho.ProximaEm), slog.String("erro", motivo))
	return false
}
