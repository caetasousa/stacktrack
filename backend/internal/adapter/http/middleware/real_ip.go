package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// cabecalhosDeIPDeCliente são os cabeçalhos que só um proxy confiável pode
// escrever. Vindos de qualquer outro lugar, são entrada de atacante.
var cabecalhosDeIPDeCliente = []string{"X-Real-IP", "X-Forwarded-For"}

// IPReal reescreve r.RemoteAddr com o IP do cliente informado pelo proxy
// reverso — e SOMENTE quando quem entregou a requisição é um proxy da lista de
// confiança.
//
// A versão anterior confiava em X-Real-IP incondicionalmente. O raciocínio era
// "em produção só o nginx da borda fala com a API, e ele sobrescreve o
// cabeçalho", e ele está certo enquanto essa topologia valer — mas é uma
// premissa que não estava verificada em lugar nenhum: bastava a porta da API
// ficar alcançável (um `ports:` a mais no compose, uma regra de firewall
// errada, o binário rodando direto em desenvolvimento) para qualquer cliente
// escolher o próprio IP. E escolher o próprio IP é escolher o próprio balde de
// rate limit: um cabeçalho novo a cada requisição zera o contador para sempre,
// que é justamente o ataque que os tetos existem para impedir.
//
// Agora a confiança é explícita e verificada a cada requisição, contra o peer
// direto da conexão TCP — o único endereço que o cliente não consegue forjar.
// Vindo de fora da lista, os cabeçalhos são APAGADOS antes de seguir: deixá-los
// passar entregaria a decisão a qualquer handler que os lesse por conta própria
// mais adiante.
//
// Com a lista vazia (o padrão em desenvolvimento, sem proxy), nada é confiado e
// r.RemoteAddr fica com o peer direto. Em produção a lista é obrigatória —
// config.Validar recusa subir sem ela.
func IPReal(confiaveis []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !peerConfiavel(r.RemoteAddr, confiaveis) {
				for _, h := range cabecalhosDeIPDeCliente {
					r.Header.Del(h)
				}
				next.ServeHTTP(w, r)
				return
			}

			// O valor do cabeçalho é PARSEADO, e não copiado. Um proxy confiável
			// não manda lixo, mas o custo de conferir é uma comparação e o custo
			// de não conferir é uma chave de rate limit por string arbitrária —
			// memória sem teto no contador, alimentada de fora.
			if ip, ok := primeiroIPValido(r.Header.Get("X-Real-IP")); ok {
				r.RemoteAddr = ip
			}
			next.ServeHTTP(w, r)
		})
	}
}

// peerConfiavel informa se o endereço do peer direto está em alguma das faixas
// confiáveis.
func peerConfiavel(remoteAddr string, confiaveis []netip.Prefix) bool {
	if len(confiaveis) == 0 {
		return false
	}
	ip, ok := enderecoDe(remoteAddr)
	if !ok {
		return false
	}
	for _, faixa := range confiaveis {
		// Unmap antes de comparar: um peer IPv4 que chega por socket dual-stack
		// aparece como ::ffff:10.0.0.5, que não casa com a faixa 10.0.0.0/8 sem
		// isto — e a confiança falharia justamente na topologia mais comum.
		if faixa.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}

// enderecoDe extrai o IP de um "host:porta" ou de um IP puro.
func enderecoDe(remoteAddr string) (netip.Addr, bool) {
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(host))
	if err != nil {
		return netip.Addr{}, false
	}
	return ip, true
}

// primeiroIPValido devolve o primeiro endereço de uma lista separada por
// vírgulas, no formato de X-Forwarded-For. Vazio ou inválido devolve false, e
// o chamador mantém o peer direto.
func primeiroIPValido(valor string) (string, bool) {
	if valor == "" {
		return "", false
	}
	primeiro, _, _ := strings.Cut(valor, ",")
	ip, err := netip.ParseAddr(strings.TrimSpace(primeiro))
	if err != nil {
		return "", false
	}
	return ip.Unmap().String(), true
}
