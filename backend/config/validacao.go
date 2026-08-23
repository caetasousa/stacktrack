// Validação da configuração de produção: o que precisa estar definido, e
// definido de forma segura, para o processo poder abrir a porta HTTP.

package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Validar confere a configuração e devolve todos os problemas encontrados.
//
// É chamada ANTES de o servidor escutar, e o erro é fatal. A razão de ser
// fatal, em vez de um aviso: configuração insegura em produção não se manifesta
// como falha — se manifesta como funcionamento normal com uma proteção
// desligada. Um `.env` de desenvolvimento copiado para o servidor sobe, atende,
// responde 200, e só o cookie não é Secure, ou o teto de login está em zero, ou
// o CORS aceita http://localhost. Nada nisso aparece num healthcheck.
//
// Fora de produção a função é permissiva de propósito: exigir origem HTTPS e
// lista de proxies para rodar `go test` ou `docker compose up` só ensinaria a
// preencher as variáveis com qualquer coisa.
//
// Devolve TODOS os problemas de uma vez, e não o primeiro: quem está subindo o
// ambiente conserta a lista inteira num ciclo, em vez de descobrir um problema
// por reinício.
func Validar() error {
	ambiente := strings.TrimSpace(os.Getenv("APP_ENV"))
	switch ambiente {
	case "development", "test":
		return nil
	case "production":
		// Continua abaixo.
	case "":
		return errors.New("APP_ENV nao definida: use development, test ou production")
	default:
		return fmt.Errorf("APP_ENV=%q invalida: use development, test ou production", ambiente)
	}

	var problemas []string
	anotar := func(formato string, args ...any) {
		problemas = append(problemas, fmt.Sprintf(formato, args...))
	}

	validarOrigem(anotar)
	validarBanco(anotar)
	validarAnexos(anotar)
	validarCookie(anotar)
	validarProxies(anotar)
	validarInteiros(anotar)
	validarTetos(anotar)

	if len(problemas) == 0 {
		return nil
	}
	sort.Strings(problemas)
	return fmt.Errorf("configuração de produção insegura ou incompleta:\n  - %s",
		strings.Join(problemas, "\n  - "))
}

// validarOrigem exige uma origem pública real. É a mesma string que alimenta o
// CORS e o OriginPatterns do WebSocket: com o padrão de desenvolvimento
// (http://localhost:5173) valendo em produção, o CORS libera uma origem que o
// atacante controla trivialmente em qualquer máquina.
func validarOrigem(anotar func(string, ...any)) {
	bruta := os.Getenv("FRONTEND_ORIGIN")
	if strings.TrimSpace(bruta) == "" {
		anotar("FRONTEND_ORIGIN não definida (o padrão de desenvolvimento libera http://localhost no CORS e no WebSocket)")
		return
	}
	u, err := url.Parse(bruta)
	if err != nil || u.Host == "" {
		anotar("FRONTEND_ORIGIN=%q não é uma origem válida (esperado https://dominio)", bruta)
		return
	}
	if u.Scheme != "https" {
		anotar("FRONTEND_ORIGIN precisa usar https em produção (veio %q)", u.Scheme)
	}
	if ehLocal(u.Hostname()) {
		anotar("FRONTEND_ORIGIN aponta para um host local (%q)", u.Hostname())
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		anotar("FRONTEND_ORIGIN deve conter exatamente esquema e host, sem credencial, caminho, barra final, query ou fragmento")
	}
}

func ehLocal(host string) bool {
	host = strings.ToLower(host)
	return host == "localhost" || host == "127.0.0.1" || host == "::1" ||
		strings.HasSuffix(host, ".localhost")
}

// validarBanco exige as variáveis sem as quais a DSN é montada com campos
// vazios — o que produz um erro de conexão tardio, no primeiro acesso, em vez
// de no boot.
func validarBanco(anotar func(string, ...any)) {
	for _, nome := range []string{"DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD"} {
		if strings.TrimSpace(os.Getenv(nome)) == "" {
			anotar("%s não definida", nome)
		}
	}
}

// validarAnexos exige um diretório existente e ESCRIVÍVEL.
//
// Escrivível é o ponto: o diretório costuma vir de um volume montado, e um
// volume montado somente-leitura (ou com dono errado) deixa a API subir
// perfeitamente e falhar no primeiro upload — com o arquivo já enviado pela
// pessoa, e o erro aparecendo como 500 no meio de um formulário.
func validarAnexos(anotar func(string, ...any)) {
	dir := DiretorioDeAnexos()
	info, err := os.Stat(dir)
	switch {
	case errors.Is(err, os.ErrNotExist):
		anotar("o diretório de anexos %q não existe", dir)
		return
	case err != nil:
		anotar("o diretório de anexos %q não pôde ser lido: %v", dir, err)
		return
	case !info.IsDir():
		anotar("o caminho de anexos %q não é um diretório", dir)
		return
	}

	f, err := os.CreateTemp(dir, ".escrita-permitida-*")
	if err != nil {
		anotar("o diretório de anexos %q não é escrivível: %v", dir, err)
		return
	}
	teste := f.Name()
	f.Close()
	os.Remove(teste)
}

// validarCookie confere a coerência entre APP_ENV e o atributo Secure. Hoje um
// deriva do outro, então o desacordo é impossível — a checagem existe para o
// dia em que CookieSeguro passar a ter variável própria, e para que esse dia
// não chegue sem ninguém perceber.
func validarCookie(anotar func(string, ...any)) {
	if !CookieSeguro() {
		anotar("o cookie de sessão precisa ser Secure em produção")
	}
}

// validarProxies exige a lista de proxies confiáveis. Sem ela, IPReal não
// confia em cabeçalho nenhum e TODO cliente aparece com o IP do container do
// Caddy: os tetos por IP passam a contar o mundo inteiro num balde só, e a
// primeira rajada tranca todo mundo para fora.
func validarProxies(anotar func(string, ...any)) {
	faixas, err := ProxiesConfiaveis()
	if err != nil {
		anotar("PROXIES_CONFIAVEIS: %v", err)
		return
	}
	if strings.TrimSpace(os.Getenv("PROXIES_CONFIAVEIS")) == "" || len(faixas) == 0 {
		anotar("PROXIES_CONFIAVEIS não definida (sem ela todos os clientes contam no mesmo balde de rate limit, com o IP do proxy)")
	}
	for _, faixa := range faixas {
		minimo := 64
		if faixa.Addr().Is4() {
			// Bridges Docker normalmente usam /16. O que recusamos aqui e o
			// fallback generico /12, que inclui muitas redes que nao sao a borda.
			minimo = 16
		}
		if faixa.Bits() < minimo {
			anotar("PROXIES_CONFIAVEIS contem faixa ampla demais (%s); informe o IP do proxy ou a sub-rede exata", faixa)
		}
	}
}

type regraInteiro struct {
	nome   string
	minimo int64
	maximo int64
}

// validarInteiros impede que erro de digitacao caia silenciosamente no padrao
// e que um zero ou valor absurdo desligue uma protecao em producao.
func validarInteiros(anotar func(string, ...any)) {
	regras := []regraInteiro{
		{"PORT", 1, 65535},
		{"DB_PORT", 1, 65535},
		{"DB_MAX_CONNS", 1, 1000},
		{"DB_MIN_CONNS", 0, 1000},
		{"DB_MAX_CONN_LIFETIME_MIN", 1, 10080},
		{"DB_MAX_CONN_IDLE_MIN", 1, 10080},
		{"DB_HEALTHCHECK_SEC", 1, 3600},
		{"RATE_LIMIT_LOGIN_POR_MINUTO", 1, 1_000_000},
		{"RATE_LIMIT_CADASTRO_POR_MINUTO", 1, 1_000_000},
		{"RATE_LIMIT_LOGIN_POR_CONTA", 1, 1_000_000},
		{"RATE_LIMIT_AUTENTICADO_POR_MINUTO", 1, 1_000_000},
		{"RATE_LIMIT_PUBLICO_POR_MINUTO", 1, 1_000_000},
		{"RATE_LIMIT_SESSAO_DESCONHECIDA", 1, 1_000_000},
		{"PRAZO_REQUISICAO_MS", 100, 3_600_000},
		{"PRAZO_UPLOAD_MS", 100, 3_600_000},
		{"TEMPO_CONEXAO_BANCO_MS", 1, 60_000},
		{"ESPERA_LOCK_QUADRO_MS", 1, 60_000},
		{"TEMPO_MAXIMO_COMANDO_MS", 1, 600_000},
		{"WS_CONEXOES_POR_CONTA", 1, 100_000},
		{"WS_CONEXOES_SIMULTANEAS", 1, 100_000},
		{"WS_HANDSHAKES_POR_MINUTO", 1, 1_000_000},
		{"FAXINA_INTERVALO_MIN", 1, 10_080},
		{"FAXINA_PRAZO_SEG", 1, 3600},
		{"DISCO_MINIMO_BYTES", 0, 1 << 60},
		{"DISCO_MINIMO_POR_CEM", 0, 100},
		{"DISCO_MEDICAO_VALIDADE_SEG", 1, 3600},
	}
	for _, regra := range regras {
		bruto := strings.TrimSpace(os.Getenv(regra.nome))
		if bruto == "" {
			continue
		}
		valor, err := strconv.ParseInt(bruto, 10, 64)
		if err != nil {
			anotar("%s=%q precisa ser um inteiro", regra.nome, bruto)
			continue
		}
		if valor < regra.minimo || valor > regra.maximo {
			anotar("%s=%d fora da faixa segura [%d, %d]", regra.nome, valor, regra.minimo, regra.maximo)
		}
	}

	if MinConexoesBanco() > MaxConexoesBanco() {
		anotar("DB_MIN_CONNS nao pode ser maior que DB_MAX_CONNS")
	}
	if PrazoDoUpload() < PrazoDaRequisicao() {
		anotar("PRAZO_UPLOAD_MS nao pode ser menor que PRAZO_REQUISICAO_MS")
	}
	if ConexoesPorConta() > ConexoesSimultaneas() {
		anotar("WS_CONEXOES_POR_CONTA nao pode ser maior que WS_CONEXOES_SIMULTANEAS")
	}
	if MinimoDeDiscoLivreBytes() == 0 && MinimoDeDiscoLivrePorCem() == 0 {
		anotar("DISCO_MINIMO_BYTES e DISCO_MINIMO_POR_CEM nao podem estar ambos em zero")
	}
}

// validarTetos recusa rate limiting desligado em produção.
//
// Fora de produção o zero é legítimo — os testes de integração disparam
// centenas de requisições e não podem levar 429. Em produção, o mesmo zero é o
// login sem proteção contra brute-force, e ele chega copiando o .env de
// desenvolvimento. Era um WARN; virou erro porque um WARN no meio do log de
// boot é exatamente o que ninguém lê.
func validarTetos(anotar func(string, ...any)) {
	tetos := map[string]int{
		"RATE_LIMIT_LOGIN_POR_MINUTO":       RateLimitLoginPorMinuto(),
		"RATE_LIMIT_CADASTRO_POR_MINUTO":    RateLimitCadastroPorMinuto(),
		"RATE_LIMIT_LOGIN_POR_CONTA":        RateLimitLoginPorConta(),
		"RATE_LIMIT_AUTENTICADO_POR_MINUTO": RateLimitAutenticadoPorMinuto(),
		"RATE_LIMIT_PUBLICO_POR_MINUTO":     RateLimitPublicoPorMinuto(),
		"RATE_LIMIT_SESSAO_DESCONHECIDA":    RateLimitSessaoDesconhecida(),
		"WS_CONEXOES_POR_CONTA":             ConexoesPorConta(),
		"WS_CONEXOES_SIMULTANEAS":           ConexoesSimultaneas(),
		"WS_HANDSHAKES_POR_MINUTO":          HandshakesPorMinuto(),
	}
	for nome, teto := range tetos {
		if teto <= 0 {
			anotar("%s está em zero: o teto correspondente fica desligado", nome)
		}
	}
}
