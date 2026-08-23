// O coletor de arquivos excluídos: remove do disco o que o backup já cobriu.
//
// É a segunda metade da exclusão recuperável. A primeira metade — registrar a
// chave física no outbox, na mesma transação do CASCADE — está em
// usecase/board/limpeza.go. Aqui os bytes efetivamente saem, e SÓ depois de
// haver prova de que a exclusão está num snapshot externo bem-sucedido.
//
// ⚠️ Enquanto a porta de cobertura for board.CoberturaNegada — isto é, até A6
// existir —, este worker NÃO REMOVE NADA. Ele acumula, relata e fica quieto. É
// o fail-closed do plano, e é a decisão certa: disco é barato, e um anexo
// apagado cedo demais não volta.

package manutencao

import (
	"context"
	"log/slog"
	"time"

	ucboard "stacktrack/internal/usecase/board"
)

// RemovedorDeArquivo apaga os bytes do volume.
type RemovedorDeArquivo interface {
	Remover(caminho string) error
}

// Coletor remove do disco as exclusões já cobertas por backup.
type Coletor struct {
	exclusoes ucboard.RepositorioExclusaoDeArquivo
	cobertura ucboard.PortaDeCobertura
	armazem   RemovedorDeArquivo
	log       *slog.Logger

	// porPassada é quantas exclusões o coletor tenta de cada vez.
	porPassada int
	// esperaAposFalha é a base do adiamento exponencial.
	esperaAposFalha time.Duration
	// tentativasMaximas é quando o coletor desiste de tentar sozinho.
	tentativasMaximas int
}

// NovoColetor cria o worker.
func NovoColetor(
	exclusoes ucboard.RepositorioExclusaoDeArquivo,
	cobertura ucboard.PortaDeCobertura,
	armazem RemovedorDeArquivo,
	log *slog.Logger,
) *Coletor {
	return &Coletor{
		exclusoes: exclusoes, cobertura: cobertura, armazem: armazem, log: log,
		porPassada:        200,
		esperaAposFalha:   5 * time.Minute,
		tentativasMaximas: 10,
	}
}

// Nome identifica esta limpeza no log e na métrica.
func (c *Coletor) Nome() string { return "coletor_de_arquivos" }

// LimparLote roda uma passada: pergunta pela cobertura, marca o que está
// coberto e remove os bytes do que já pode sair.
//
// Devolve quantos ARQUIVOS removeu, que é o que a Faxina usa para decidir se
// ainda há trabalho.
//
// A ordem é deliberada. Primeiro perguntar pela cobertura e só depois remover:
// perguntar depois significaria remover primeiro, que é exatamente o que não
// pode acontecer.
func (c *Coletor) LimparLote(ctx context.Context, limite int) (int64, error) {
	if limite <= 0 || limite > c.porPassada {
		limite = c.porPassada
	}

	if err := c.atualizarCobertura(ctx, limite); err != nil {
		return 0, err
	}

	pendentes, err := c.exclusoes.Pendentes(ctx, time.Now(), limite)
	if err != nil {
		return 0, err
	}

	var removidos int64
	for _, exclusao := range pendentes {
		if err := ctx.Err(); err != nil {
			return removidos, nil
		}
		if c.removerUma(ctx, exclusao) {
			removidos++
		}
	}
	return removidos, nil
}

// atualizarCobertura pergunta à porta quais das exclusões SEM PROVA passaram a
// estar cobertas, e marca exatamente essas.
//
// A pergunta é pelos IDs EXATOS. Timestamp, relógio do host ou `max(id)` não
// servem: o dump do banco e a cópia dos arquivos acontecem em instantes
// diferentes, o relógio do VPS pode andar para trás, e uma exclusão gravada
// depois do dump pode ter id menor que outra gravada antes. Só a lista literal
// do que o snapshot levou responde sem margem.
func (c *Coletor) atualizarCobertura(ctx context.Context, limite int) error {
	semProva, err := c.exclusoes.SemCobertura(ctx, limite)
	if err != nil {
		return err
	}
	if len(semProva) == 0 {
		return nil
	}

	ids := make([]int64, 0, len(semProva))
	for _, e := range semProva {
		ids = append(ids, e.ID)
	}

	cobertos, err := c.cobertura.Cobertos(ctx, ids)
	if err != nil {
		// Falha ao CONSULTAR a cobertura não pode virar remoção: sem resposta,
		// a resposta é não.
		return err
	}
	if len(cobertos) == 0 {
		// O caso normal enquanto A6 não existe. Um INFO por passada seria ruído;
		// o tamanho da fila é o que interessa, e A8 o publica como métrica.
		c.log.Debug("nenhuma exclusão coberta por backup ainda",
			slog.Int("aguardando", len(semProva)))
		return nil
	}
	return c.exclusoes.MarcarCobertos(ctx, cobertos, time.Now())
}

// removerUma apaga os bytes de uma exclusão e registra o desfecho.
func (c *Coletor) removerUma(ctx context.Context, exclusao ucboard.ExclusaoDeArquivo) bool {
	err := c.armazem.Remover(exclusao.Caminho)
	if err == nil {
		if err := c.exclusoes.MarcarRemovido(ctx, exclusao.ID, time.Now()); err != nil {
			// Os bytes já saíram e a marca não entrou: a próxima passada tenta
			// remover de novo um arquivo que já não existe, e o armazém
			// responde sucesso. É o desfecho certo de um estado torto.
			c.log.Warn("arquivo removido mas não marcado",
				slog.Int64("exclusao", exclusao.ID), slog.String("erro", err.Error()))
		}
		return true
	}

	// A falha é adiada com espera CRESCENTE: um volume fora do ar não melhora
	// sendo martelado, e um erro permanente (permissão errada) não deve
	// consumir a passada inteira.
	espera := c.esperaAposFalha * time.Duration(1<<min(exclusao.Tentativas, 6))
	if exclusao.Tentativas+1 >= c.tentativasMaximas {
		// Desistir de tentar SOZINHO não é desistir do arquivo: a linha
		// continua no outbox, com o erro registrado, esperando alguém olhar. É
		// o que A8 transforma em alerta.
		c.log.Error("exclusão de arquivo falhou repetidamente; parou de tentar sozinha",
			slog.Int64("exclusao", exclusao.ID),
			slog.Int("tentativas", exclusao.Tentativas+1),
			slog.String("erro", err.Error()))
		espera = 24 * time.Hour
	}
	if err := c.exclusoes.AdiarComErro(ctx, exclusao.ID, err.Error(), time.Now().Add(espera)); err != nil {
		c.log.Warn("não foi possível adiar a exclusão",
			slog.Int64("exclusao", exclusao.ID), slog.String("erro", err.Error()))
	}
	return false
}
