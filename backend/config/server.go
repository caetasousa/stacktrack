package config

import (
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	appmiddleware "stacktrack/internal/adapter/http/middleware"
	"stacktrack/internal/pkg/logging"

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
// real. Esta mesma origem alimenta o OriginPatterns do handshake do WebSocket
// — que NÃO obedece CORS e precisa checar o Origin por conta própria.
func OrigemFrontend() string {
	if o := os.Getenv("FRONTEND_ORIGIN"); o != "" {
		return o
	}
	return "http://localhost:5173"
}

// EhProducao informa se o processo roda em produção (env APP_ENV=production).
// Decide o formato do log (JSON vs texto) e o atributo Secure do cookie de
// sessão — que por sua vez decide o nome dele, já que o prefixo __Host- exige
// Secure.
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

// RateLimitSessaoDesconhecida é quantos cookies de sessão DESCONHECIDOS um
// mesmo IP pode apresentar dentro de JanelaLimitePorConta antes de levar 429
// (env RATE_LIMIT_SESSAO_DESCONHECIDA; 0 desliga).
//
// O teto por sessão do roteador não cobre este caso: ele chaveia pelo valor do
// cookie, e quem manda um cookie diferente a cada requisição ganha um balde
// novo a cada requisição. Aqui a chave é o IP, e a contagem acontece antes da
// consulta ao banco — ver middleware.Auth.Autenticar.
//
// O padrão é generoso em relação ao uso legítimo: um cookie inválido acontece
// quando a sessão expira ou é revogada, e nessa hora a aba dispara algumas
// requisições em paralelo antes de a tela mandar para o login. Trinta cobre
// isso com folga e ainda corta um laço de varredura na primeira dezena.
func RateLimitSessaoDesconhecida() int {
	return intDoAmbiente("RATE_LIMIT_SESSAO_DESCONHECIDA", 30)
}

// ProxiesConfiaveis devolve as faixas de onde um cabeçalho X-Real-IP é aceito
// (env PROXIES_CONFIAVEIS, faixas CIDR ou IPs separados por vírgula).
//
// Em producao prefira o IP exato do Caddy ou a sub-rede exata da bridge.
// Confiar em toda 172.16.0.0/12 daria o mesmo poder a containers vizinhos.
//
// Vazia (o padrão) significa NÃO CONFIAR EM NINGUÉM: o IP do cliente passa a
// ser sempre o peer direto da conexão. É o comportamento certo em
// desenvolvimento, onde não há proxy, e é o que Validar recusa em produção,
// onde há.
//
// IP solto é aceito e vira faixa /32 ou /128 — escrever o endereço exato do
// proxy é o caso mais comum e não deveria exigir saber notação CIDR.
func ProxiesConfiaveis() ([]netip.Prefix, error) {
	bruta := strings.TrimSpace(os.Getenv("PROXIES_CONFIAVEIS"))
	if bruta == "" {
		return nil, nil
	}

	var faixas []netip.Prefix
	for _, item := range strings.Split(bruta, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.Contains(item, "/") {
			faixa, err := netip.ParsePrefix(item)
			if err != nil {
				return nil, fmt.Errorf("faixa %q inválida: %w", item, err)
			}
			faixas = append(faixas, faixa.Masked())
			continue
		}
		ip, err := netip.ParseAddr(item)
		if err != nil {
			return nil, fmt.Errorf("endereço %q inválido: %w", item, err)
		}
		faixas = append(faixas, netip.PrefixFrom(ip.Unmap(), ip.Unmap().BitLen()))
	}
	return faixas, nil
}

// PrazoDaRequisicao é o orçamento total de uma requisição HTTP comum
// (env PRAZO_REQUISICAO_MS, em milissegundos).
//
// Dez segundos: é o teto entre o ReadHeaderTimeout, que corta quem não manda
// cabeçalho, e o statement_timeout, que corta uma query lenta. Sem ele, uma
// requisição pode ficar presa sem nenhum dos dois disparar — esperando conexão
// do pool, ou somando operações rápidas que juntas não terminam.
func PrazoDaRequisicao() time.Duration {
	return time.Duration(intDoAmbiente("PRAZO_REQUISICAO_MS", 10_000)) * time.Millisecond
}

// PrazoDoUpload é o orçamento de um envio de anexo (env PRAZO_UPLOAD_MS).
//
// Dois minutos, e não os dez segundos das demais: dez megabytes numa conexão
// móvel ruim levam mais que dez segundos sem nada de errado acontecendo, e
// herdar o teto comum recusaria envios legítimos.
func PrazoDoUpload() time.Duration {
	return time.Duration(intDoAmbiente("PRAZO_UPLOAD_MS", 120_000)) * time.Millisecond
}

// TempoParaConectarAoBanco e quanto uma NOVA conexao TCP/TLS pode levar para
// ser estabelecida (env TEMPO_CONEXAO_BANCO_MS).
//
// Nao e o tempo de espera por uma conexao LIVRE do pool: Acquire respeita o
// contexto da requisicao e portanto usa PRAZO_REQUISICAO_MS/PRAZO_UPLOAD_MS.
func TempoParaConectarAoBanco() time.Duration {
	return time.Duration(intDoAmbiente("TEMPO_CONEXAO_BANCO_MS", 2000)) * time.Millisecond
}

// EsperaPorLockDeQuadro é o teto de espera pelo lock da linha do quadro dentro
// de uma unidade de trabalho (env ESPERA_LOCK_QUADRO_MS, em milissegundos).
//
// Dois segundos: acima disso a pessoa já desistiu de esperar a tela responder,
// e segurar a conexão do pool para chegar a um sucesso que ninguém vai ver é
// pior do que devolver "tente de novo". Sem teto, uma transação travada
// segurando o quadro faria cada mutação daquele quadro esperar para sempre, uma
// conexão do pool por vez, até o pool acabar — e aí um quadro travado derruba a
// API inteira.
func EsperaPorLockDeQuadro() time.Duration {
	return time.Duration(intDoAmbiente("ESPERA_LOCK_QUADRO_MS", 2000)) * time.Millisecond
}

// TempoMaximoDeComando é o teto de cada comando SQL dentro de uma unidade de
// trabalho (env TEMPO_MAXIMO_COMANDO_MS, em milissegundos).
//
// Vale SÓ dentro da transação de escrita (SET LOCAL): as leituras longas de
// auditoria não herdam este limite.
func TempoMaximoDeComando() time.Duration {
	return time.Duration(intDoAmbiente("TEMPO_MAXIMO_COMANDO_MS", 5000)) * time.Millisecond
}

// ConexoesPorConta é quantas conexões de tempo real uma conta pode ter abertas
// ao mesmo tempo (env WS_CONEXOES_POR_CONTA; 0 desliga).
//
// Cinco: cobre computador, celular e algumas abas com folga. O teto existe
// porque, sem ele, uma conta sozinha consome a capacidade de tempo real de todo
// mundo — e o sintoma, do lado de fora, é "o quadro parou de atualizar", sem
// nada parecendo errado no servidor.
func ConexoesPorConta() int {
	return intDoAmbiente("WS_CONEXOES_POR_CONTA", 5)
}

// ConexoesSimultaneas é o teto global de conexões de tempo real do processo
// (env WS_CONEXOES_SIMULTANEAS; 0 desliga).
//
// Cem, calibrado pela memória: cada conexão carrega uma fila de 32 eventos mais
// o buffer do socket, e o container da API tem 384 MiB. É o número a revisar
// quando A10 medir de verdade — o perfil desta rodada prevê 25 conexões
// simultâneas, então cem é quatro vezes a carga alvo.
func ConexoesSimultaneas() int {
	return intDoAmbiente("WS_CONEXOES_SIMULTANEAS", 100)
}

// HandshakesPorMinuto é o teto de tentativas de conexão de tempo real por IP
// (env WS_HANDSHAKES_POR_MINUTO; 0 desliga).
//
// Cobre o que os tetos de ocupação não veem: quem abre e fecha em laço nunca
// ocupa vaga, e mesmo assim faz o servidor pagar autorização, consulta de nome
// e replay a cada tentativa.
func HandshakesPorMinuto() int {
	return intDoAmbiente("WS_HANDSHAKES_POR_MINUTO", 10)
}

// IntervaloDaFaxina é de quanto em quanto tempo a limpeza periódica roda
// (env FAXINA_INTERVALO_MIN, em minutos).
//
// Uma hora: sessão vencida e convite terminal não fazem mal nenhum enquanto
// estão lá — só ocupam espaço. Limpar mais rápido gastaria I/O sem resolver
// problema; limpar muito mais devagar deixaria a tabela crescer entre passadas.
func IntervaloDaFaxina() time.Duration {
	return time.Duration(intDoAmbiente("FAXINA_INTERVALO_MIN", 60)) * time.Minute
}

// PrazoDaFaxina é o teto de duração de UMA passada
// (env FAXINA_PRAZO_SEG, em segundos).
//
// Sem teto, uma tabela muito atrasada faria a primeira passada rodar por horas,
// segurando conexão do pool e competindo com o tráfego real. Estourado o prazo,
// a passada para e o resto fica para a próxima — o trabalho é retomável por
// construção, já que ele apaga o que sobrou.
func PrazoDaFaxina() time.Duration {
	return time.Duration(intDoAmbiente("FAXINA_PRAZO_SEG", 60)) * time.Second
}

// MinimoDeDiscoLivreBytes é o piso absoluto de espaço livre no volume dos
// anexos (env DISCO_MINIMO_BYTES). Abaixo dele a escrita é suspensa.
//
// 2 GiB: espaço para o Postgres crescer o WAL durante um checkpoint, para o log
// de acesso e para o maior upload aceito, com folga para alguém conseguir
// entrar e apagar alguma coisa.
func MinimoDeDiscoLivreBytes() uint64 {
	return uint64(intDoAmbiente("DISCO_MINIMO_BYTES", 2<<30))
}

// MinimoDeDiscoLivrePorCem é o piso PERCENTUAL de espaço livre
// (env DISCO_MINIMO_POR_CEM).
//
// Vale junto com o absoluto, e prevalece o mais conservador dos dois. Só o
// percentual falharia num volume grande (1% de 2 TB ainda são 20 GB); só o
// absoluto falharia num volume pequeno, onde 2 GiB podem ser metade do disco.
func MinimoDeDiscoLivrePorCem() float64 {
	return float64(intDoAmbiente("DISCO_MINIMO_POR_CEM", 10))
}

// ValidadeDaMedicaoDeDisco é por quanto tempo a última medição vale
// (env DISCO_MEDICAO_VALIDADE_SEG).
//
// Medir a cada requisição seria um statfs por requisição — barato, mas não de
// graça, e o espaço livre não muda de forma relevante entre duas requisições
// consecutivas.
func ValidadeDaMedicaoDeDisco() time.Duration {
	return time.Duration(intDoAmbiente("DISCO_MEDICAO_VALIDADE_SEG", 10)) * time.Second
}

// IntervaloDoDespachante é o polling que corrige wake-up perdido na entrega dos
// eventos (env DESPACHANTE_INTERVALO_MS).
//
// Um segundo. O caminho normal é o aviso imediato de quem comita; este é a rede
// que pega o que escapou — fila de avisos cheia, entrega que falhou, processo
// reiniciado. Curto porque o pior caso precisa ser "chegou um segundo depois",
// e não "chegou quando alguém recarregou".
func IntervaloDoDespachante() time.Duration {
	return time.Duration(intDoAmbiente("DESPACHANTE_INTERVALO_MS", 1000)) * time.Millisecond
}

// EsperaPorConexaoLivre é o teto de espera por uma conexão livre do pool
// (env ESPERA_CONEXAO_LIVRE_MS).
//
// Dois segundos. Sem teto, a espera herda o orçamento da requisição inteira, e
// sob saturação a fila do pool cresce sem limite: as requisições ficam vivas
// segurando goroutine e memória para serem atendidas muito depois de quem pediu
// ter desistido. É melhor recusar rápido — o cliente repete — do que aceitar
// tudo e atender ninguém.
//
// Diferente de TempoParaConectarAoBanco, que cobre ABRIR uma conexão de rede
// nova: este cobre ESPERAR por uma que já existe e está ocupada.
func EsperaPorConexaoLivre() time.Duration {
	return time.Duration(intDoAmbiente("ESPERA_CONEXAO_LIVRE_MS", 2000)) * time.Millisecond
}

// TempoMaximoOciosoEmTransacao é quanto uma transação pode ficar aberta SEM
// comando em curso (env TEMPO_MAXIMO_OCIOSO_TRANSACAO_MS).
//
// É o par que falta ao statement_timeout, e cobre um caso que ele não vê: uma
// transação que abre, executa, e fica esperando algo que não é o banco — I/O de
// arquivo, uma chamada de rede, um bug. Não há statement em curso, então o
// statement_timeout não dispara, e o lock do quadro fica preso indefinidamente.
//
// Dez segundos: folgado o bastante para qualquer transação legítima deste
// domínio (todas terminam em milissegundos) e curto o bastante para que um
// caminho travado não pare o quadro por minutos.
func TempoMaximoOciosoEmTransacao() time.Duration {
	return time.Duration(intDoAmbiente("TEMPO_MAXIMO_OCIOSO_TRANSACAO_MS", 10_000)) * time.Millisecond
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
// ⚠️ WriteTimeout e ReadTimeout são ZERO de propósito, desde a fase 5.
//
// Eles valem para a conexão inteira, não por requisição — e uma conexão de
// WebSocket fica aberta por horas. Com os 15s que havia aqui, o quadro em tempo
// real caía sozinho sempre no mesmo tempo, sem erro do lado do cliente e sem
// nada no log. Era o teto que estava certo para uma API que só troca JSON
// pequeno e errado para esta.
//
// O que substitui a proteção que eles davam:
//   - ReadHeaderTimeout continua cortando quem abre conexão e não manda
//     cabeçalho (o Slowloris), que é o ataque que o ReadTimeout barrava aqui;
//   - IdleTimeout fecha conexão ociosa entre requisições HTTP comuns;
//   - middleware.Prazo limita o contexto e os deadlines de leitura/escrita de
//     cada requisição, e LimitarCorpo limita os bytes recebidos;
//   - no WebSocket, cada envio tem prazo próprio e o ping/pong derruba quem
//     morreu (ver adapter/http/ws).
func NovoServidor(r *chi.Mux) *Servidor {
	return &Servidor{
		Server: http.Server{
			Addr:              Porta(),
			Handler:           r,
			ReadHeaderTimeout: 15 * time.Second,
			IdleTimeout:       120 * time.Second,
			MaxHeaderBytes:    1 << 20,
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
// (atrás do proxy), política de referrer, log estruturado de acesso,
// recuperação de panics, limite de corpo e CORS.
//
// A ordem importa: RequestID primeiro (para o log e os handlers terem o id);
// IPReal antes do log (para ver o cliente, não o proxy, e para apagar cabeçalho
// forjado antes que qualquer outro middleware o leia); o log de acesso por fora
// do Recoverer (para registrar também os 500 que o Recoverer produz).
//
// SemReferrer é global; SemCache não é, e fica nos grupos que servem dado
// privado — marcar o mundo inteiro como no-store impediria o navegador de
// cachear até o que é público e imutável.
func NovoRouter(proxiesConfiaveis []netip.Prefix) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(appmiddleware.IPReal(proxiesConfiaveis))
	r.Use(appmiddleware.SemReferrer)
	r.Use(logging.Middleware)
	r.Use(middleware.Recoverer)
	// O prazo vem DEPOIS do Recoverer e do log: assim o 503 de tempo esgotado
	// aparece no log de acesso como qualquer outra resposta, e um panic dentro
	// do handler continua virando 500 em vez de escapar por cima do deadline.
	r.Use(appmiddleware.Prazo(PrazoDaRequisicao(), PrazoDoUpload()))
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
	return "/var/lib/stacktrack/anexos"
}
