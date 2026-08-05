package config

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DSNBanco monta a string de conexão do PostgreSQL a partir das variáveis
// de ambiente DB_*. sslmode=disable porque o banco local não usa TLS.
//
// A URL é montada com net/url, não por concatenação: senha forte contém
// caracteres reservados (`/`, `+`, `@`, `:`) — `openssl rand -base64 24` gera
// isso com frequência. Sem escapar, o `/` encerra a autoridade da URL e a
// conexão falha no boot com um erro de parse que não parece ter nada a ver com
// a senha.
func DSNBanco() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD")),
		Host:     net.JoinHostPort(os.Getenv("DB_HOST"), os.Getenv("DB_PORT")),
		Path:     "/" + os.Getenv("DB_NAME"),
		RawQuery: "sslmode=disable",
	}
	return u.String()
}

// MaxConexoesBanco é o teto de conexões simultâneas do pool
// (env DB_MAX_CONNS). O padrão do pgx é max(4, núcleos de CPU) — numa VPS de
// 1 vCPU isso dá 4, o que torna o teto de concorrência da API um efeito
// colateral do hardware em vez de uma decisão. Ao subir, lembrar que o teto do
// Postgres (max_connections, 100 por padrão) é compartilhado com o job de
// migration e com qualquer psql aberto.
func MaxConexoesBanco() int {
	return intDoAmbiente("DB_MAX_CONNS", 10)
}

// MinConexoesBanco é o número de conexões que o pool mantém abertas mesmo
// ocioso (env DB_MIN_CONNS), para que a primeira requisição depois de um
// período parado não pague o handshake.
func MinConexoesBanco() int {
	return intDoAmbiente("DB_MIN_CONNS", 2)
}

// VidaMaximaConexaoBanco é por quanto tempo uma conexão é reaproveitada antes
// de ser reciclada (env DB_MAX_CONN_LIFETIME_MIN, em minutos). Reciclar evita
// que conexões longas segurem memória no servidor e força a repactuação depois
// de uma troca de credencial ou de um failover.
func VidaMaximaConexaoBanco() time.Duration {
	return time.Duration(intDoAmbiente("DB_MAX_CONN_LIFETIME_MIN", 60)) * time.Minute
}

// OciosidadeMaximaConexaoBanco é quanto tempo uma conexão ociosa sobrevive
// antes de ser fechada (env DB_MAX_CONN_IDLE_MIN, em minutos).
func OciosidadeMaximaConexaoBanco() time.Duration {
	return time.Duration(intDoAmbiente("DB_MAX_CONN_IDLE_MIN", 30)) * time.Minute
}

// IntervaloChecagemPoolBanco é a frequência com que o pool verifica a saúde das
// conexões ociosas (env DB_HEALTHCHECK_SEC, em segundos). É o que faz o pool
// descartar conexão morta por reinício do banco antes de a requisição encontrá-la.
func IntervaloChecagemPoolBanco() time.Duration {
	return time.Duration(intDoAmbiente("DB_HEALTHCHECK_SEC", 60)) * time.Second
}

// NovoPool cria um pool de conexões com o PostgreSQL e valida com um ping.
// Os limites do pool são definidos explicitamente a partir das env vars DB_*
// (ver MaxConexoesBanco) em vez de herdar o padrão do pgx.
func NovoPool(ctx context.Context) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(DSNBanco())
	if err != nil {
		return nil, fmt.Errorf("configurar pool: %w", err)
	}
	cfg.MaxConns = int32(MaxConexoesBanco())
	cfg.MinConns = int32(MinConexoesBanco())
	cfg.MaxConnLifetime = VidaMaximaConexaoBanco()
	cfg.MaxConnIdleTime = OciosidadeMaximaConexaoBanco()
	cfg.HealthCheckPeriod = IntervaloChecagemPoolBanco()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("criar pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping no banco: %w", err)
	}
	return pool, nil
}
