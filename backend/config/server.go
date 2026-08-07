package config

import (
	"net/http"
	"os"
	"strconv"
	"time"

	appmiddleware "kanbango/internal/adapter/http/middleware"
	"kanbango/internal/pkg/logging"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// Porta devolve o endereço em que o servidor HTTP escuta
// (env PORT, padrão 8080).
func Porta() string {
	if p := os.Getenv("PORT"); p != "" {
		return ":" + p
	}
	return ":8080"
}

// OrigemFrontend devolve a origem permitida no CORS (env FRONTEND_ORIGIN).
// O padrão é o frontend de desenvolvimento; em produção aponte para o domínio
// real. Na fase 5 esta mesma origem passa a valer para o handshake do
// WebSocket — que NÃO obedece CORS e precisa checar o Origin por conta própria.
func OrigemFrontend() string {
	if o := os.Getenv("FRONTEND_ORIGIN"); o != "" {
		return o
	}
	return "http://localhost:5173"
}

// EhProducao informa se o processo roda em produção (env APP_ENV=production).
// Decide o formato do log (JSON vs texto) e, a partir da fase 1, o atributo
// Secure do cookie de sessão.
func EhProducao() bool {
	return os.Getenv("APP_ENV") == "production"
}

// CookieSeguro informa se o cookie de sessão deve ter o atributo Secure.
// Em desenvolvimento (http://localhost) o navegador não entrega cookies Secure
// de forma confiável, por isso o atributo só é ativado em produção — é também
// o que decide o nome do cookie (o prefixo __Host- exige Secure).
func CookieSeguro() bool {
	return EhProducao()
}

// RateLimitLoginPorMinuto é o teto de tentativas de login por IP por minuto
// (env RATE_LIMIT_LOGIN_POR_MINUTO; 0 desliga). Mitiga brute-force e o custo
// de CPU do Argon2id em rajadas.
func RateLimitLoginPorMinuto() int {
	return intDoAmbiente("RATE_LIMIT_LOGIN_POR_MINUTO", 10)
}

// RateLimitCadastroPorMinuto é o teto de cadastros por IP por minuto
// (env RATE_LIMIT_CADASTRO_POR_MINUTO; 0 desliga). A rota é pública e cria
// linha no banco: sem teto, uma rajada enche a tabela de contas de lixo.
func RateLimitCadastroPorMinuto() int {
	return intDoAmbiente("RATE_LIMIT_CADASTRO_POR_MINUTO", 5)
}

// RateLimitLoginPorConta é o teto de tentativas fracassadas de login por
// CONTA, dentro de JanelaLimitePorConta (env RATE_LIMIT_LOGIN_POR_CONTA;
// 0 desliga). Complementa o teto por IP, que não vê nada quando o atacante
// troca de endereço a cada tentativa.
func RateLimitLoginPorConta() int {
	return intDoAmbiente("RATE_LIMIT_LOGIN_POR_CONTA", 5)
}

// RateLimitPublicoPorMinuto é o teto por IP por minuto das rotas que respondem
// a quem não se identificou (env RATE_LIMIT_PUBLICO_POR_MINUTO; 0 desliga).
// Hoje é só o detalhe do convite — e é justamente onde um teto importa, porque
// sem ele daria para varrer tokens à vontade.
func RateLimitPublicoPorMinuto() int {
	return intDoAmbiente("RATE_LIMIT_PUBLICO_POR_MINUTO", 30)
}

// RateLimitAutenticadoPorMinuto é o teto de requisições por SESSÃO por minuto
// (env RATE_LIMIT_AUTENTICADO_POR_MINUTO; 0 desliga). Depois do login o IP
// deixa de identificar quem abusa — várias pessoas atrás do mesmo NAT
// compartilham endereço, e a mesma conta pode trocar de rede. Sem este teto,
// uma sessão válida dispararia criação de quadros e cards em laço.
func RateLimitAutenticadoPorMinuto() int {
	return intDoAmbiente("RATE_LIMIT_AUTENTICADO_POR_MINUTO", 120)
}

// JanelaLimitePorConta é a janela do teto por conta. Curta de propósito: o
// preço de errar a senha cinco vezes é esperar alguns minutos, não perder o
// acesso — e é tempo suficiente para inviabilizar brute-force.
const JanelaLimitePorConta = 5 * time.Minute

// intDoAmbiente lê um inteiro não negativo da env var, caindo no padrão quando
// ausente ou inválida.
func intDoAmbiente(nome string, padrao int) int {
	v := os.Getenv(nome)
	if v == "" {
		return padrao
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return padrao
	}
	return n
}

// Servidor encapsula o http.Server com as configurações do projeto.
type Servidor struct {
	http.Server
}

// NovoServidor cria um Servidor HTTP com timeouts configurados.
//
// ⚠️ `Handler` precisa continuar sendo informado explicitamente. Deixá-lo nil
// faz o http.Server cair no http.DefaultServeMux — que já vem com
// `/debug/pprof/*` registrado, porque o `init()` de net/http/pprof faz isso, e
// ele entra no binário como dependência transitiva de chi/v5/middleware. Ou
// seja: os endpoints de debug existem no processo, com dump de heap e de
// goroutines; o que os torna inalcançáveis é esta linha, e nada além dela.
//
// ⚠️ WriteTimeout e a fase 5: estes tetos são a decisão certa para uma API que
// só troca JSONs pequenos, e a errada para uma conexão que fica aberta por
// horas. Quando o WebSocket entrar, este é o primeiro lugar a revisitar — o
// diagnóstico típico é "a conexão cai sozinha sempre no mesmo tempo".
func NovoServidor(r *chi.Mux) *Servidor {
	return &Servidor{
		Server: http.Server{
			Addr:           Porta(),
			Handler:        r,
			ReadTimeout:    15 * time.Second,
			WriteTimeout:   15 * time.Second,
			IdleTimeout:    60 * time.Second,
			MaxHeaderBytes: 1 << 20,
		},
	}
}

const (
	// maxBytesJSON limita o corpo das requisições comuns — a API só troca
	// JSONs pequenos.
	maxBytesJSON = 1 << 20 // 1 MiB
	// maxBytesUpload é o teto de transporte do envio de anexo: o limite do
	// domínio (10 MiB) mais folga para o cabeçalho do multipart, que viaja
	// junto. Sem essa folga, um arquivo no limite exato seria cortado pelo
	// transporte antes de o domínio poder responder direito.
	maxBytesUpload = (10 << 20) + (1 << 20)
)

// NovoRouter cria um roteador chi com middlewares de request ID, IP real
// (atrás do proxy), log estruturado de acesso, recuperação de panics, limite
// de corpo e CORS.
// A ordem importa: RequestID primeiro (para o log e os handlers terem o id);
// IPReal antes do log (para ver o cliente, não o proxy); o log de acesso por
// fora do Recoverer (para registrar também os 500 que o Recoverer produz).
func NovoRouter() *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(appmiddleware.IPReal)
	r.Use(logging.Middleware)
	r.Use(middleware.Recoverer)
	r.Use(appmiddleware.LimitarCorpo(maxBytesJSON, maxBytesUpload))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{OrigemFrontend()},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
	}))
	return r
}

// DiretorioDeAnexos é onde os arquivos enviados são gravados
// (env ANEXOS_DIR). O padrão aponta para fora da árvore de código: o
// diretório do projeto é montado do host em desenvolvimento, e arquivo de
// usuário não pode aparecer no `git status`.
func DiretorioDeAnexos() string {
	if d := os.Getenv("ANEXOS_DIR"); d != "" {
		return d
	}
	return "/var/lib/kanbango/anexos"
}
