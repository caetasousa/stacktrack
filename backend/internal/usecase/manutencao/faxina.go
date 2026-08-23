// Package manutencao roda a limpeza periódica do que expira sozinho: sessões
// vencidas e convites que já são terminais.
//
// Fica FORA de qualquer requisição, e essa é a razão de existir. A limpeza
// morava no login — um DELETE varrendo `sessions` a cada sessão aberta —, o que
// fazia cada pessoa que entra pagar a limpeza de todo mundo, e fazia a conta
// crescer justamente quando há mais gente usando. O custo aparecia na rajada de
// logins da manhã de segunda-feira, que é exatamente quando ninguém pode
// esperar.
package manutencao

import (
	"context"
	"log/slog"
	"time"
)

// TamanhoDoLote é quantas linhas cada passada apaga.
//
// Mil, e não "tudo de uma vez": um DELETE que apaga cem mil linhas segura lock
// e infla o WAL por minutos, e o autovacuum fica para trás. Em lotes, cada
// transação é curta, o trabalho é retomável (a passada seguinte pega o que
// sobrou) e uma falha no meio não desfaz o que já foi limpo.
const TamanhoDoLote = 1000

// MaxLotesPorPassada limita quanto tempo uma passada pode durar.
//
// Sem teto, uma tabela muito atrasada faria a primeira passada rodar por horas,
// segurando uma conexão do pool e competindo com o tráfego real. Com teto, ela
// limpa um milhão de linhas em algumas passadas — e o backlog aparece na
// métrica em vez de virar uma pausa longa e invisível.
const MaxLotesPorPassada = 50

// Faxineiro é quem sabe apagar um lote. Devolve quantas linhas apagou.
//
// Devolver a CONTAGEM é o que permite ao laço saber se ainda há trabalho: menos
// que o lote significa que acabou, e é assim que a passada termina sem precisar
// de um SELECT COUNT antes.
type Faxineiro interface {
	// Nome identifica a limpeza no log e na métrica.
	Nome() string
	// LimparLote apaga até `limite` linhas e devolve quantas apagou.
	LimparLote(ctx context.Context, limite int) (int64, error)
}

// Faxina roda os faxineiros em intervalo fixo.
type Faxina struct {
	faxineiros []Faxineiro
	intervalo  time.Duration
	prazo      time.Duration
	log        *slog.Logger
}

// Nova cria a faxina. prazo é o teto de uma passada inteira.
func Nova(intervalo, prazo time.Duration, log *slog.Logger, faxineiros ...Faxineiro) *Faxina {
	return &Faxina{faxineiros: faxineiros, intervalo: intervalo, prazo: prazo, log: log}
}

// Rodar executa a faxina até o contexto ser cancelado.
//
// A primeira passada acontece depois do primeiro intervalo, e não no boot: subir
// a aplicação já disparando um DELETE grande é a pior hora possível para isso —
// é quando o pool está frio e o tráfego está voltando depois de um deploy.
//
// Chamar numa goroutine própria. Bloqueia até o contexto encerrar.
func (f *Faxina) Rodar(ctx context.Context) {
	relogio := time.NewTicker(f.intervalo)
	defer relogio.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-relogio.C:
			f.umaPassada(ctx)
		}
	}
}

// umaPassada roda todos os faxineiros uma vez.
//
// O erro de um NÃO impede os outros: são limpezas independentes, e deixar a de
// convites parada porque a de sessões falhou seria transformar um problema em
// dois.
func (f *Faxina) umaPassada(ctx context.Context) {
	if len(f.faxineiros) == 0 {
		return
	}
	fim := time.Now().Add(f.prazo)

	for indice, faxineiro := range f.faxineiros {
		// Divide o tempo restante por quem ainda nao rodou. Um backlog enorme de
		// sessoes nao pode consumir sozinho o prazo inteiro e impedir para sempre
		// a limpeza de convites que vem depois.
		restante := time.Until(fim)
		faltam := len(f.faxineiros) - indice
		if restante <= 0 {
			f.log.Warn("faxina sem orcamento para limpeza",
				slog.String("limpeza", faxineiro.Nome()))
			continue
		}
		fatia := restante / time.Duration(faltam)
		prazoDaLimpeza, cancelar := context.WithTimeout(ctx, fatia)
		inicio := time.Now()
		apagadas, restou, err := limparTudo(prazoDaLimpeza, faxineiro)
		cancelar()

		switch {
		case err != nil:
			// WARN, e não ERROR: a faxina atrasada não quebra nada agora — ela
			// acumula linha. Quem transforma isso em alerta é A8, quando o
			// backlog passar de um teto.
			f.log.Warn("faxina falhou",
				slog.String("limpeza", faxineiro.Nome()),
				slog.Int64("apagadas", apagadas),
				slog.String("erro", err.Error()))
		case apagadas > 0:
			f.log.Info("faxina concluída",
				slog.String("limpeza", faxineiro.Nome()),
				slog.Int64("apagadas", apagadas),
				slog.Bool("restou_trabalho", restou),
				slog.Duration("duracao", time.Since(inicio)))
		}
	}
}

// limparTudo apaga em lotes até acabar, estourar o teto de lotes ou o contexto
// encerrar. Devolve quantas linhas apagou e se ainda restou trabalho.
func limparTudo(ctx context.Context, faxineiro Faxineiro) (int64, bool, error) {
	var total int64
	for lote := 0; lote < MaxLotesPorPassada; lote++ {
		if err := ctx.Err(); err != nil {
			return total, true, err
		}
		apagadas, err := faxineiro.LimparLote(ctx, TamanhoDoLote)
		total += apagadas
		if err != nil {
			return total, true, err
		}
		// Lote incompleto significa que a tabela acabou. É o que evita um
		// SELECT COUNT antes de cada passada.
		if apagadas < int64(TamanhoDoLote) {
			return total, false, nil
		}
	}
	return total, true, nil
}
