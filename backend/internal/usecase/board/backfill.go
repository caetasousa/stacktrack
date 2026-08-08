package board

import (
	"context"

	"stacktrack/internal/domain/ordem"
)

// LoteDoBackfill é quantas linhas o comando processa por passada.
//
// Existe para o backfill não montar a tabela inteira em memória num quadro
// grande, e para poder ser interrompido e retomado — ele é idempotente, então
// rodar de novo só alcança quem ainda falta.
const LoteDoBackfill = 500

// ResultadoDoBackfill diz quanto o comando preencheu.
type ResultadoDoBackfill struct {
	Colunas int
	Cards   int
}

// Completo informa se não sobrou nada para preencher — é como quem chama sabe
// que pode parar de repetir.
func (r ResultadoDoBackfill) Completo() bool {
	return r.Colunas == 0 && r.Cards == 0
}

// BackfillUseCase preenche a chave de ordenação das linhas criadas ANTES do
// expand da fase 9.
//
// ⚠️ Isto é um comando do DOMÍNIO, e não SQL numa migration, e a razão está no
// CLAUDE.md: decidir com que valor as linhas antigas ficam é decisão de
// negócio. Em SQL ela viraria uma segunda fonte da verdade — sem teste, sem
// conserto (migration aplicada não se corrige) e sem acesso à regra que gera a
// chave, que vive em `ordem`.
//
// A ordem preservada é a que a POSIÇÃO ditava: as linhas antigas continuam
// aparecendo onde apareciam, só que agora com chave. Um backfill que mudasse a
// ordem seria pior que nenhum — a pessoa abriria o quadro e encontraria os
// cards embaralhados.
type BackfillUseCase struct {
	colunas RepositorioColuna
	cards   RepositorioCard
}

// NovoBackfillUseCase cria uma instância de BackfillUseCase com as dependências injetadas.
func NovoBackfillUseCase(colunas RepositorioColuna, cards RepositorioCard) *BackfillUseCase {
	return &BackfillUseCase{colunas: colunas, cards: cards}
}

// Executar preenche um lote e devolve quanto preencheu.
//
// É idempotente e retomável: só alcança quem está sem chave, então rodar duas
// vezes não estraga nada e uma interrupção no meio não deixa estado inválido.
func (uc *BackfillUseCase) Executar(ctx context.Context) (ResultadoDoBackfill, error) {
	var r ResultadoDoBackfill

	colunas, err := uc.colunas.SemChave(ctx, LoteDoBackfill)
	if err != nil {
		return r, err
	}
	// A chave de cada grupo continua de onde a última parou: o SemChave devolve
	// agrupado e em ordem de posição, então basta encadear.
	ultima := map[string]string{}
	for _, c := range colunas {
		anterior, visto := ultima[c.BoardID]
		if !visto {
			// A primeira do quadro pode ter vizinhas JÁ preenchidas por uma
			// passada anterior — encadear a partir delas é o que mantém a ordem
			// entre lotes.
			anterior, err = uc.colunas.UltimaChave(ctx, c.BoardID)
			if err != nil {
				return r, err
			}
		}
		chave, err := ordem.ChaveEntre(anterior, "")
		if err != nil {
			return r, err
		}
		if err := uc.colunas.GravarChave(ctx, c.ID, chave); err != nil {
			return r, err
		}
		ultima[c.BoardID] = chave
		r.Colunas++
	}

	cards, err := uc.cards.SemChave(ctx, LoteDoBackfill)
	if err != nil {
		return r, err
	}
	ultimaDoCard := map[string]string{}
	for _, c := range cards {
		anterior, visto := ultimaDoCard[c.ColunaID]
		if !visto {
			anterior, err = uc.cards.UltimaChave(ctx, c.ColunaID)
			if err != nil {
				return r, err
			}
		}
		chave, err := ordem.ChaveEntre(anterior, "")
		if err != nil {
			return r, err
		}
		if err := uc.cards.GravarChave(ctx, c.ID, chave); err != nil {
			return r, err
		}
		ultimaDoCard[c.ColunaID] = chave
		r.Cards++
	}

	return r, nil
}

// ExecutarTudo repete o comando até não sobrar nada.
//
// O teto de passadas evita laço infinito se algo impedir uma linha de ser
// preenchida — sem ele, um erro silencioso viraria um processo girando para
// sempre no start da aplicação.
func (uc *BackfillUseCase) ExecutarTudo(ctx context.Context, maxPassadas int) (ResultadoDoBackfill, error) {
	var total ResultadoDoBackfill
	for i := 0; i < maxPassadas; i++ {
		r, err := uc.Executar(ctx)
		if err != nil {
			return total, err
		}
		total.Colunas += r.Colunas
		total.Cards += r.Cards
		if r.Completo() {
			return total, nil
		}
	}
	return total, nil
}
