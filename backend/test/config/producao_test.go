// A validação de produção. Cada caso aqui é um jeito real de o servidor subir
// atendendo requisições com uma proteção desligada — e nenhum deles apareceria
// num healthcheck.
package config_test

import (
	"os"
	"strings"
	"testing"

	"stacktrack/config"
)

// ambienteDeProducao define um ambiente de produção COMPLETO e válido, e
// devolve uma função para sobrescrever variáveis caso a caso.
//
// Partir do válido, e não do vazio, é o que faz cada teste isolar UM problema:
// com o ambiente vazio, todo caso reprovaria por todos os motivos e nenhum
// deles provaria nada sobre a checagem que está sendo exercitada.
func ambienteDeProducao(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	for nome, valor := range map[string]string{
		"APP_ENV":                           "production",
		"FRONTEND_ORIGIN":                   "https://stacktrack.exemplo",
		"DB_HOST":                           "db",
		"DB_PORT":                           "5432",
		"DB_NAME":                           "stacktrack",
		"DB_USER":                           "app",
		"DB_PASSWORD":                       "uma-senha",
		"ANEXOS_DIR":                        dir,
		"PROXIES_CONFIAVEIS":                "172.20.0.2",
		"RATE_LIMIT_LOGIN_POR_MINUTO":       "10",
		"RATE_LIMIT_CADASTRO_POR_MINUTO":    "5",
		"RATE_LIMIT_LOGIN_POR_CONTA":        "5",
		"RATE_LIMIT_AUTENTICADO_POR_MINUTO": "120",
		"RATE_LIMIT_PUBLICO_POR_MINUTO":     "30",
		"RATE_LIMIT_SESSAO_DESCONHECIDA":    "30",
	} {
		t.Setenv(nome, valor)
	}
}

func TestProducaoCompletaEValidaPassa(t *testing.T) {
	ambienteDeProducao(t)
	if err := config.Validar(); err != nil {
		t.Fatalf("um ambiente de produção completo devia passar: %v", err)
	}
}

// Fora de produção a validação é permissiva de propósito: exigir origem HTTPS
// para rodar `go test` só ensinaria a preencher as variáveis com qualquer coisa.
func TestForaDeProducaoNadaEhExigido(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("FRONTEND_ORIGIN", "")
	t.Setenv("PROXIES_CONFIAVEIS", "")
	if err := config.Validar(); err != nil {
		t.Errorf("fora de produção não se exige nada: %v", err)
	}
}

func TestAppEnvAusenteOuComErroNaoDesligaValidacao(t *testing.T) {
	for _, valor := range []string{"", "prodution", "prod"} {
		t.Run(valor, func(t *testing.T) {
			t.Setenv("APP_ENV", valor)
			if err := config.Validar(); err == nil || !strings.Contains(err.Error(), "APP_ENV") {
				t.Errorf("APP_ENV=%q: erro = %v, esperado fail-closed", valor, err)
			}
		})
	}
}

func TestProducaoInseguraFalha(t *testing.T) {
	casos := []struct {
		nome     string
		variavel string
		valor    string
		trecho   string
	}{
		{"origem ausente cai no localhost do CORS", "FRONTEND_ORIGIN", "", "FRONTEND_ORIGIN"},
		{"origem sem TLS", "FRONTEND_ORIGIN", "http://stacktrack.exemplo", "https"},
		{"origem apontando para a própria máquina", "FRONTEND_ORIGIN", "https://localhost:5173", "host local"},
		{"origem com caminho", "FRONTEND_ORIGIN", "https://stacktrack.exemplo/app", "caminho"},
		{"origem com barra final", "FRONTEND_ORIGIN", "https://stacktrack.exemplo/", "barra final"},
		{"porta do banco invalida", "DB_PORT", "postgres", "inteiro"},
		{"banco sem host", "DB_HOST", "", "DB_HOST"},
		{"banco sem senha", "DB_PASSWORD", "", "DB_PASSWORD"},
		{"diretório de anexos inexistente", "ANEXOS_DIR", "/nao/existe/em/lugar/nenhum", "não existe"},
		{"sem lista de proxies o rate limit vira um balde só", "PROXIES_CONFIAVEIS", "", "PROXIES_CONFIAVEIS"},
		{"lista de proxies so com separadores fica vazia", "PROXIES_CONFIAVEIS", " , , ", "PROXIES_CONFIAVEIS"},
		{"lista de proxies inválida", "PROXIES_CONFIAVEIS", "isto-nao-e-cidr", "PROXIES_CONFIAVEIS"},
		{"faixa de proxies ampla", "PROXIES_CONFIAVEIS", "172.16.0.0/12", "ampla demais"},
		{"inteiro malformado nao cai no padrao", "PRAZO_REQUISICAO_MS", "dez-segundos", "inteiro"},
		{"percentual fora da faixa", "DISCO_MINIMO_POR_CEM", "101", "faixa segura"},
		{"login sem teto por IP", "RATE_LIMIT_LOGIN_POR_MINUTO", "0", "RATE_LIMIT_LOGIN_POR_MINUTO"},
		{"login sem teto por conta", "RATE_LIMIT_LOGIN_POR_CONTA", "0", "RATE_LIMIT_LOGIN_POR_CONTA"},
		{"sessão desconhecida sem teto", "RATE_LIMIT_SESSAO_DESCONHECIDA", "0", "RATE_LIMIT_SESSAO_DESCONHECIDA"},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			ambienteDeProducao(t)
			t.Setenv(caso.variavel, caso.valor)

			err := config.Validar()
			if err == nil {
				t.Fatalf("%s=%q devia reprovar a configuração", caso.variavel, caso.valor)
			}
			if !strings.Contains(err.Error(), caso.trecho) {
				t.Errorf("a mensagem devia citar %q, veio: %v", caso.trecho, err)
			}
		})
	}
}

// O diretório existe mas não é escrivível: a API sobe, atende, e falha no
// primeiro upload — com o arquivo já enviado pela pessoa.
func TestProducaoRecusaDiretorioDeAnexosSomenteLeitura(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("como root toda permissão de escrita passa")
	}
	ambienteDeProducao(t)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	t.Setenv("ANEXOS_DIR", dir)

	err := config.Validar()
	if err == nil || !strings.Contains(err.Error(), "escrivível") {
		t.Errorf("erro = %v, esperado recusa por diretório não escrivível", err)
	}
}

// Todos os problemas de uma vez: quem sobe o ambiente conserta a lista inteira
// num ciclo, em vez de descobrir um problema por reinício.
func TestValidarRelataTodosOsProblemasDeUmaVez(t *testing.T) {
	ambienteDeProducao(t)
	t.Setenv("FRONTEND_ORIGIN", "http://localhost:5173")
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("RATE_LIMIT_LOGIN_POR_MINUTO", "0")

	err := config.Validar()
	if err == nil {
		t.Fatal("esperava reprovação")
	}
	for _, trecho := range []string{"https", "DB_PASSWORD", "RATE_LIMIT_LOGIN_POR_MINUTO"} {
		if !strings.Contains(err.Error(), trecho) {
			t.Errorf("a mensagem devia citar %q, veio: %v", trecho, err)
		}
	}
}

func TestProxiesConfiaveisAceitaFaixaEEnderecoSolto(t *testing.T) {
	t.Setenv("PROXIES_CONFIAVEIS", " 172.16.0.0/12 , 10.0.0.7 ")
	faixas, err := config.ProxiesConfiaveis()
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(faixas) != 2 {
		t.Fatalf("faixas = %d, esperado 2", len(faixas))
	}
	// O endereço solto vira /32: escrever o IP exato do proxy não deve exigir
	// saber notação CIDR.
	if faixas[1].String() != "10.0.0.7/32" {
		t.Errorf("faixa = %q, esperado 10.0.0.7/32", faixas[1])
	}
}
