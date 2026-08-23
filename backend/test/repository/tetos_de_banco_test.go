//go:build integracao

// Os tetos que o BANCO aplica sozinho, definidos no startup da conexão.
//
// A unidade de trabalho e o instantâneo já definiam `statement_timeout` por
// transação (SET LOCAL). O que faltava era o resto: toda leitura solta — listar
// quadros, auditoria, detalhe do card — roda fora das duas e não tinha teto
// nenhum do lado do banco. Uma consulta travada ali segura a conexão do pool
// até o contexto da requisição expirar.
package repository_test

import (
	"context"
	"net/url"
	"testing"
	"time"

	"stacktrack/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

// poolDeProducao constrói o pool pelo MESMO caminho da API, apontado para o
// container do teste.
//
// É a única forma de provar a fiação: reaproveitar o pool do pacote testaria o
// pgx, não a configuração deste projeto.
func poolDeProducao(t *testing.T) *pgxpool.Pool {
	t.Helper()

	u, err := url.Parse(urlDoBanco)
	if err != nil {
		t.Fatalf("dsn do container: %v", err)
	}
	senha, _ := u.User.Password()
	t.Setenv("DB_HOST", u.Hostname())
	t.Setenv("DB_PORT", u.Port())
	t.Setenv("DB_NAME", u.Path[1:])
	t.Setenv("DB_USER", u.User.Username())
	t.Setenv("DB_PASSWORD", senha)

	p, err := config.NovoPool(context.Background())
	if err != nil {
		t.Fatalf("criar pool de produção: %v", err)
	}
	t.Cleanup(p.Close)
	return p
}

// Consulta solta — fora de transação — é cortada pelo statement_timeout da
// conexão, e não fica esperando o contexto de quem chamou.
func TestConsultaForaDeTransacaoTemTetoDeTempo(t *testing.T) {
	t.Setenv("TEMPO_MAXIMO_COMANDO_MS", "300")
	p := poolDeProducao(t)

	// Contexto generoso: quem precisa cortar é o BANCO.
	ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelar()

	inicio := time.Now()
	_, err := p.Exec(ctx, `SELECT pg_sleep(10)`)
	decorrido := time.Since(inicio)

	if err == nil {
		t.Fatal("a consulta de 10s devia ter sido cortada pelo statement_timeout")
	}
	if decorrido > 5*time.Second {
		t.Errorf("levou %v: o statement_timeout da conexão não foi aplicado", decorrido)
	}
}

// Transação aberta e OCIOSA — sem comando em curso — é cortada pelo
// idle_in_transaction_session_timeout.
//
// É o caso que o statement_timeout não vê: não há statement rodando, então nada
// dispara, e o lock do quadro fica preso enquanto o processo espera algo que
// não é o banco.
func TestTransacaoOciosaTemTetoDeTempo(t *testing.T) {
	t.Setenv("TEMPO_MAXIMO_OCIOSO_TRANSACAO_MS", "500")
	p := poolDeProducao(t)

	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `SELECT 1`); err != nil {
		t.Fatalf("primeiro comando: %v", err)
	}

	// O processo "trava" sem mandar nada ao banco.
	time.Sleep(1500 * time.Millisecond)

	if _, err := tx.Exec(ctx, `SELECT 1`); err == nil {
		t.Error("a transação ociosa devia ter sido encerrada pelo banco")
	}
}
