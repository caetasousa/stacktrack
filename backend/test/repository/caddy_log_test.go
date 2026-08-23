//go:build integracao

// O ENSAIO do access log do Caddy: prova que o proxy EM EXECUÇÃO não grava
// token nenhum.
//
// Por que isto é um teste, e não uma conferência manual: `caddy validate` prova
// que a configuração é ACEITA, e não o que o proxy efetivamente escreve. A
// diferença importa porque a redação é uma expressão regular — uma alteração
// inocente nela continua validando e para de redigir, e o sintoma só aparece
// quando alguém for ler o log procurando outra coisa.
//
// O teste sobe o Caddy de verdade com o filtro do bloco de site do repositório,
// faz requisições com um token real e lê o access log que ele produziu.
package repository_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// tokenDeVerdade gera um token no formato do projeto: base64url de 256 bits.
//
// Gerado, e não fixo, para o teste não passar por acidente caso alguém
// acrescente um literal ao filtro em vez de casar pela FORMA do segredo.
func tokenDeVerdade(t *testing.T) string {
	t.Helper()
	bruto := make([]byte, 32)
	if _, err := rand.Read(bruto); err != nil {
		t.Fatalf("sortear token: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(bruto)
}

// extrairFiltroDeLog recorta a diretiva `log { ... }` do arquivo de PRODUÇÃO.
//
// Lido do repositório, e não copiado para cá: copiar faria este teste continuar
// verde depois de alguém mudar o filtro de verdade.
func extrairFiltroDeLog(t *testing.T) string {
	t.Helper()
	bloco, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "caddy", "stacktrack.caddy"))
	if err != nil {
		t.Fatalf("ler o bloco de site: %v", err)
	}
	texto := string(bloco)
	inicio := strings.Index(texto, "\tlog {")
	if inicio < 0 {
		t.Fatal("o bloco de site não tem diretiva `log`: o access log não está sendo filtrado")
	}
	resto := texto[inicio:]
	fim := strings.Index(resto, "\n\t}\n")
	if fim < 0 {
		t.Fatal("não foi possível delimitar a diretiva `log`")
	}
	return resto[:fim+len("\n\t}")]
}

func TestOAccessLogDoCaddyNaoGravaToken(t *testing.T) {
	ctx := context.Background()
	const porta = 9080
	token := tokenDeVerdade(t)

	dir := t.TempDir()
	caminho := filepath.Join(dir, "Caddyfile")
	conteudo := fmt.Sprintf("{\n\tauto_https off\n\tadmin off\n}\n:%d {\n%s\n\trespond \"ok\" 200\n}\n",
		porta, extrairFiltroDeLog(t))
	if err := os.WriteFile(caminho, []byte(conteudo), 0o644); err != nil {
		t.Fatalf("escrever Caddyfile: %v", err)
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "caddy:2",
			ExposedPorts: []string{fmt.Sprintf("%d/tcp", porta)},
			Files: []testcontainers.ContainerFile{{
				HostFilePath:      caminho,
				ContainerFilePath: "/etc/caddy/Caddyfile",
				FileMode:          0o644,
			}},
			WaitingFor: wait.ForLog("serving initial configuration").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("subir o Caddy de teste: %v", err)
	}
	defer container.Terminate(ctx) //nolint:errcheck

	endereco, err := container.PortEndpoint(ctx, fmt.Sprintf("%d/tcp", porta), "http")
	if err != nil {
		t.Fatalf("endereço do container: %v", err)
	}

	// Caminhos VÁLIDOS e INVÁLIDOS com o mesmo token.
	//
	// Os inválidos são o ponto. Um filtro que liste as rotas conhecidas redige
	// `/convites/<token>` e deixa passar `/convitess/<token>` — a rota certa
	// errada por uma letra, que dá 404 e grava o token inteiro. E um segredo
	// tentado na raiz não casa com rota nenhuma.
	caminhos := []string{
		"/convite/" + token,
		"/api/convites/" + token,
		"/publico/" + token,
		"/api/publico/" + token,
		"/convitess/" + token,
		"/" + token,
		"/api/boards/" + token + "/membros",
		"/qualquer/coisa/" + token,
	}
	cliente := &http.Client{Timeout: 5 * time.Second}
	for _, c := range caminhos {
		resp, err := cliente.Get(endereco + c)
		if err != nil {
			t.Fatalf("requisitar %q: %v", c, err)
		}
		resp.Body.Close()
	}

	// O Caddy escreve o access log em stdout; dá tempo de ele sair.
	time.Sleep(500 * time.Millisecond)
	saida, err := container.Logs(ctx)
	if err != nil {
		t.Fatalf("ler o log do container: %v", err)
	}
	defer saida.Close()

	var registrado strings.Builder
	if _, err := io.Copy(&registrado, saida); err != nil {
		t.Fatalf("copiar o log: %v", err)
	}
	log := registrado.String()

	if strings.Contains(log, token) {
		t.Errorf("o token apareceu no access log do Caddy em execução")
	}
	// O log EXISTE: um teste que passasse por não haver log nenhum não provaria
	// nada — seria verde por ausência.
	if !strings.Contains(log, "handled request") {
		t.Error("o Caddy não registrou requisição nenhuma; o ensaio não provou nada")
	}
	// E a redação de fato aconteceu, em vez de o token nunca ter chegado lá.
	if !strings.Contains(log, "REDACTED") {
		t.Error("nada foi redigido: o filtro não está sendo aplicado")
	}
}
