//go:build integracao

// O teto de espera por conexão livre do pool.
//
// `pgxpool` não expõe prazo de aquisição separado do contexto de quem chama:
// sem embrulho, a espera herda o orçamento inteiro da requisição. Sob
// saturação, cada requisição nova fica viva esses segundos todos segurando
// goroutine e memória para ser atendida depois de quem pediu já ter desistido.
package repository_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"stacktrack/internal/adapter/repository"

	"github.com/jackc/pgx/v5/pgxpool"
)

// poolDeUmaConexao sobe um pool com UMA conexão só, para a saturação ser
// trivial de produzir: basta segurar essa conexão.
func poolDeUmaConexao(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg := pool.Config().Copy()
	cfg.MaxConns = 1
	cfg.MinConns = 0
	pequeno, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("pool de uma conexão: %v", err)
	}
	t.Cleanup(pequeno.Close)
	return pequeno
}

// Com o pool saturado, a aquisição TERMINA no prazo em vez de esperar o
// orçamento inteiro da requisição.
func TestEsperaPorConexaoLivreTerminaNoPrazo(t *testing.T) {
	ctx := context.Background()
	pequeno := poolDeUmaConexao(t)

	// Segura a única conexão.
	presa, err := pequeno.Acquire(ctx)
	if err != nil {
		t.Fatalf("adquirir: %v", err)
	}
	defer presa.Release()

	banco := repository.NovoPoolComEspera(pequeno, 200*time.Millisecond)

	// O contexto de quem chama é generoso — como o de uma requisição HTTP. É o
	// TETO DO POOL que precisa cortar, não ele.
	amplo, cancelar := context.WithTimeout(ctx, 10*time.Second)
	defer cancelar()

	inicio := time.Now()
	_, err = banco.Exec(amplo, `SELECT 1`)
	decorrido := time.Since(inicio)

	if err == nil {
		t.Fatal("a aquisição devia ter falhado: a única conexão está presa")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("erro = %v, esperado DeadlineExceeded da aquisição", err)
	}
	if decorrido > 3*time.Second {
		t.Errorf("esperou %v: o teto do pool não foi aplicado", decorrido)
	}
}

// E o caminho normal continua funcionando: com conexão livre, a consulta roda.
func TestComConexaoLivreAConsultaRoda(t *testing.T) {
	banco := repository.NovoPoolComEspera(pool, 2*time.Second)

	var um int
	if err := banco.QueryRow(context.Background(), `SELECT 1`).Scan(&um); err != nil {
		t.Fatalf("consultar: %v", err)
	}
	if um != 1 {
		t.Errorf("SELECT 1 devolveu %d", um)
	}
}

// A CONEXÃO VOLTA ao pool depois de cada operação.
//
// É o defeito que um embrulho mal feito produz: as linhas seguram a conexão até
// serem fechadas, e liberar cedo demais devolveria ao pool uma conexão que
// ainda está sendo lida por outra requisição. Liberar tarde demais — ou nunca —
// esgota o pool depois de algumas consultas.
func TestAConexaoVoltaAoPoolDepoisDeCadaOperacao(t *testing.T) {
	ctx := context.Background()
	pequeno := poolDeUmaConexao(t)
	banco := repository.NovoPoolComEspera(pequeno, 2*time.Second)

	// Muito mais operações do que conexões: só passa se cada uma devolver.
	for i := 0; i < 20; i++ {
		linhas, err := banco.Query(ctx, `SELECT generate_series(1, 3)`)
		if err != nil {
			t.Fatalf("query %d: %v", i, err)
		}
		for linhas.Next() {
		}
		linhas.Close()

		var um int
		if err := banco.QueryRow(ctx, `SELECT 1`).Scan(&um); err != nil {
			t.Fatalf("queryrow %d: %v", i, err)
		}
		if _, err := banco.Exec(ctx, `SELECT 1`); err != nil {
			t.Fatalf("exec %d: %v", i, err)
		}

		tx, err := banco.Begin(ctx)
		if err != nil {
			t.Fatalf("begin %d: %v", i, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit %d: %v", i, err)
		}
	}
}

// Falha de AQUISIÇÃO no QueryRow chega pelo Scan: `pgx.Row` não tem outro lugar
// para reportar erro, e engoli-la devolveria uma linha vazia como se fosse dado.
func TestFalhaDeAquisicaoNoQueryRowApareceNoScan(t *testing.T) {
	ctx := context.Background()
	pequeno := poolDeUmaConexao(t)

	presa, err := pequeno.Acquire(ctx)
	if err != nil {
		t.Fatalf("adquirir: %v", err)
	}
	defer presa.Release()

	banco := repository.NovoPoolComEspera(pequeno, 100*time.Millisecond)

	var um int
	if err := banco.QueryRow(ctx, `SELECT 1`).Scan(&um); err == nil {
		t.Error("o Scan devia devolver o erro da aquisição")
	}
}

// A liberação é feita UMA vez por operação, mesmo com Close repetido.
//
// Liberar duas vezes devolveria ao pool uma conexão que outra requisição já
// pegou — e as duas passariam a usar a mesma conexão ao mesmo tempo.
func TestFecharAsLinhasDuasVezesNaoLiberaDuasVezes(t *testing.T) {
	ctx := context.Background()
	pequeno := poolDeUmaConexao(t)
	banco := repository.NovoPoolComEspera(pequeno, 2*time.Second)

	linhas, err := banco.Query(ctx, `SELECT 1`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	for linhas.Next() {
	}
	linhas.Close()
	linhas.Close()

	// Se a segunda liberação tivesse contado, o pool acharia que há duas
	// conexões livres onde só existe uma — e esta consulta pegaria uma conexão
	// que não existe.
	var um int
	if err := banco.QueryRow(ctx, `SELECT 1`).Scan(&um); err != nil {
		t.Fatalf("consulta seguinte: %v", err)
	}
}

// Sob concorrência real, com -race: nenhuma corrida no controle de liberação.
func TestPoolComEsperaSobConcorrencia(t *testing.T) {
	ctx := context.Background()
	banco := repository.NovoPoolComEspera(pool, 5*time.Second)

	var espera sync.WaitGroup
	erros := make([]error, 30)
	for i := 0; i < 30; i++ {
		espera.Add(1)
		go func(i int) {
			defer espera.Done()
			var um int
			erros[i] = banco.QueryRow(ctx, `SELECT 1`).Scan(&um)
		}(i)
	}
	espera.Wait()

	for i, err := range erros {
		if err != nil {
			t.Errorf("consulta %d: %v", i, err)
		}
	}
}
