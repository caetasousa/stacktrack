package board

import (
	"context"
	"log/slog"
	"time"

	"stacktrack/internal/domain/notificacao"

	"github.com/google/uuid"
)

// PrazoParaAvisar é uma pessoa que responde por um card cujo prazo chega nas
// próximas horas, e que ainda não foi avisada daquela data.
type PrazoParaAvisar struct {
	CardID    string
	BoardID   string
	UsuarioID string
	Prazo     time.Time
}

// FonteDePrazos responde quem precisa ser avisado de um prazo chegando.
type FonteDePrazos interface {
	// PrazosParaAvisar devolve até `limite` pessoas a avisar, para os prazos
	// entre agora (exclusive) e `ate` (inclusive). Prazo que já passou não
	// avisa: o card já aparece vencido, e "vence amanhã" seria mentira.
	PrazosParaAvisar(ctx context.Context, agora, ate time.Time, limite int) ([]PrazoParaAvisar, error)
}

// TransacaoSemEvento grava, numa transação, o que não é estado de quadro
// nenhum: o aviso que nenhuma mutação causou, só a passagem do tempo.
//
// Sem lock de quadro, sem revisão e sem evento. Não precisa de nenhum dos três:
// se o prazo mudar, ou a pessoa deixar de responder pelo card, enquanto o aviso
// é gravado, ele simplesmente deixa de valer — o que vale é calculado na
// leitura, e não no momento da gravação.
type TransacaoSemEvento interface {
	EscreverSemEvento(ctx context.Context, mudanca func(Escrita) error) error
}

const (
	// prazosPorLote é quantos avisos cada leitura da fonte traz.
	prazosPorLote = 200
	// lotesPorPassada limita uma passada: o que sobrar fica para a seguinte,
	// em vez de uma volta só segurar conexão por minutos.
	lotesPorPassada = 10
)

// VigiaDePrazos avisa quem responde por um card de que o prazo dele chega
// amanhã.
//
// É o único aviso que nenhuma ação de pessoa causa: ninguém faz nada quando o
// prazo entra na véspera, e por isso alguém precisa olhar o relógio. Olha em
// intervalo fixo e também quando acordado — depois de um prazo marcado ou de
// uma atribuição, para quem passou a responder por um card que vence amanhã
// saber agora, e não na próxima volta.
type VigiaDePrazos struct {
	notificacoes
	fonte     FonteDePrazos
	transacao TransacaoSemEvento
	log       *slog.Logger
	relogio   func() time.Time
	intervalo time.Duration
	acordar   chan struct{}
}

// NovoVigiaDePrazos cria o vigia. Sem ComIntervalo, ele olha a cada dez
// minutos: o aviso é da véspera, e dez minutos de atraso nele não mudam nada.
func NovoVigiaDePrazos(fonte FonteDePrazos, transacao TransacaoSemEvento, notificador *Notificador, log *slog.Logger) *VigiaDePrazos {
	v := &VigiaDePrazos{
		fonte: fonte, transacao: transacao, log: log, relogio: time.Now,
		intervalo: 10 * time.Minute,
		acordar:   make(chan struct{}, 1),
	}
	v.ComNotificador(notificador)
	return v
}

// ComIntervalo troca de quanto em quanto tempo o vigia olha os prazos.
func (v *VigiaDePrazos) ComIntervalo(intervalo time.Duration) *VigiaDePrazos {
	if intervalo > 0 {
		v.intervalo = intervalo
	}
	return v
}

// ComRelogio troca o relógio — o dublê de fronteira dos testes, que avançam o
// tempo até a véspera em vez de esperar um dia.
func (v *VigiaDePrazos) ComRelogio(relogio func() time.Time) *VigiaDePrazos {
	v.relogio = relogio
	return v
}

// Acordar pede uma passada agora. Não bloqueia: um pedido pendente cobre os
// que chegarem antes de ele ser atendido.
func (v *VigiaDePrazos) Acordar() {
	select {
	case v.acordar <- struct{}{}:
	default:
	}
}

// Rodar é o laço do vigia, até o contexto acabar.
func (v *VigiaDePrazos) Rodar(ctx context.Context) {
	relogio := time.NewTicker(v.intervalo)
	defer relogio.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-relogio.C:
		case <-v.acordar:
		}
		if _, err := v.Passada(ctx); err != nil && ctx.Err() == nil {
			v.log.Warn("vigia de prazos: passada falhou", slog.String("erro", err.Error()))
		}
	}
}

// Passada avisa quem precisa ser avisado agora. Devolve quantos avisos
// nasceram.
//
// Cada aviso tem a própria transação: um card com problema não impede os
// outros de serem avisados, e a chave do aviso torna a passada repetível — o
// que já foi avisado bate na chave e não vira um segundo aviso.
func (v *VigiaDePrazos) Passada(ctx context.Context) (int, error) {
	agora := v.relogio()
	var avisados []string
	for lote := 0; lote < lotesPorPassada; lote++ {
		candidatos, err := v.fonte.PrazosParaAvisar(ctx, agora, agora.Add(notificacao.AntecedenciaDoPrazo), prazosPorLote)
		if err != nil {
			v.depois(avisados...)
			return len(avisados), err
		}
		criadosNoLote := 0
		for _, c := range candidatos {
			if ctx.Err() != nil {
				v.depois(avisados...)
				return len(avisados), nil
			}
			var criada bool
			err := v.transacao.EscreverSemEvento(ctx, func(e Escrita) error {
				var err error
				criada, err = v.avisar(ctx, e,
					notificacao.NovaDePrazo(uuid.NewString(), c.UsuarioID, c.BoardID, c.CardID, c.Prazo, agora))
				return err
			})
			if err != nil {
				v.log.Warn("vigia de prazos: aviso não gravado",
					slog.String("card", c.CardID), slog.String("erro", err.Error()))
				continue
			}
			if criada {
				criadosNoLote++
				avisados = append(avisados, c.UsuarioID)
			}
		}
		// Lote incompleto: não há mais o que avisar. Lote cheio sem nenhum
		// aviso novo também encerra — seria o mesmo lote de novo, para sempre.
		if len(candidatos) < prazosPorLote || criadosNoLote == 0 {
			break
		}
	}
	v.depois(avisados...)
	return len(avisados), nil
}
