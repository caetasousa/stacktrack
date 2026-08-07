// Testes das decisões que vivem em config/: os padrões que valem quando a env
// var não existe, e o escape do DSN do banco.
package config_test

import (
	"net/url"
	"testing"

	"stacktrack/config"
)

func TestPortaCaiNoPadraoQuandoEnvAusente(t *testing.T) {
	t.Setenv("PORT", "")

	if got := config.Porta(); got != ":8080" {
		t.Errorf("Porta() = %q, esperado %q", got, ":8080")
	}
}

func TestPortaUsaEnvQuandoDefinida(t *testing.T) {
	t.Setenv("PORT", "9090")

	if got := config.Porta(); got != ":9090" {
		t.Errorf("Porta() = %q, esperado %q", got, ":9090")
	}
}

func TestOrigemFrontendCaiNoFrontendDeDesenvolvimento(t *testing.T) {
	t.Setenv("FRONTEND_ORIGIN", "")

	if got := config.OrigemFrontend(); got != "http://localhost:5173" {
		t.Errorf("OrigemFrontend() = %q, esperado o frontend de dev", got)
	}
}

func TestEhProducaoSoComAppEnvProduction(t *testing.T) {
	casos := map[string]bool{
		"production":  true,
		"development": false,
		"":            false,
		"prod":        false,
	}

	for valor, esperado := range casos {
		t.Setenv("APP_ENV", valor)
		if got := config.EhProducao(); got != esperado {
			t.Errorf("APP_ENV=%q: EhProducao() = %v, esperado %v", valor, got, esperado)
		}
	}
}

// O pool não pode herdar do ambiente um valor sem sentido: env var inválida ou
// negativa cai no padrão do projeto, em vez de virar um teto de conexões zero
// que só se manifesta sob carga.
func TestMaxConexoesBancoIgnoraValorInvalido(t *testing.T) {
	casos := map[string]int{
		"":     10,
		"25":   25,
		"zero": 10,
		"-5":   10,
	}

	for valor, esperado := range casos {
		t.Setenv("DB_MAX_CONNS", valor)
		if got := config.MaxConexoesBanco(); got != esperado {
			t.Errorf("DB_MAX_CONNS=%q: MaxConexoesBanco() = %d, esperado %d", valor, got, esperado)
		}
	}
}

// Senha gerada por `openssl rand -base64 24` contém `/`, `+` e `=` com
// frequência. Concatenar isso numa URL quebra a conexão no boot com um erro de
// parse que não parece ter nada a ver com a senha — daí o net/url em DSNBanco.
func TestDSNBancoEscapaSenhaComCaracteresReservados(t *testing.T) {
	const senha = "s3nh@/forte+com=reservados"

	t.Setenv("DB_USER", "stacktrack")
	t.Setenv("DB_PASSWORD", senha)
	t.Setenv("DB_HOST", "postgres")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "stacktrack")

	u, err := url.Parse(config.DSNBanco())
	if err != nil {
		t.Fatalf("DSN gerado não é uma URL válida: %v", err)
	}

	if got, _ := u.User.Password(); got != senha {
		t.Errorf("senha do DSN = %q, esperado %q", got, senha)
	}
	if u.User.Username() != "stacktrack" {
		t.Errorf("usuário do DSN = %q, esperado %q", u.User.Username(), "stacktrack")
	}
	if u.Host != "postgres:5432" {
		t.Errorf("host do DSN = %q, esperado %q", u.Host, "postgres:5432")
	}
	if u.Path != "/stacktrack" {
		t.Errorf("banco do DSN = %q, esperado %q", u.Path, "/stacktrack")
	}
	if u.Query().Get("sslmode") != "disable" {
		t.Errorf("sslmode do DSN = %q, esperado %q", u.Query().Get("sslmode"), "disable")
	}
}
