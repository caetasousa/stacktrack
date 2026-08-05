package handler

import (
	"net/http"
	"time"
)

const (
	// nomeCookieSessaoInseguro é o nome usado fora de produção, onde o cookie
	// não pode ser Secure (o navegador não entrega cookies Secure de forma
	// confiável em http://localhost).
	nomeCookieSessaoInseguro = "kanbango_session"
	// nomeCookieSessaoSeguro leva o prefixo __Host-, que o navegador só aceita
	// com Secure, Path=/ e SEM atributo Domain. Na prática isso amarra o cookie
	// a esta origem exata: um subdomínio comprometido (ou um ataque que consiga
	// escrever cookies para o domínio pai) não consegue sobrescrever a sessão.
	nomeCookieSessaoSeguro = "__Host-kanbango_session"
)

// NomeCookieSessao devolve o nome do cookie que carrega o token opaco de
// sessão. Depende do ambiente porque o prefixo __Host- exige Secure, que só
// existe em produção (HTTPS).
func NomeCookieSessao(seguro bool) string {
	if seguro {
		return nomeCookieSessaoSeguro
	}
	return nomeCookieSessaoInseguro
}

// novoCookieSessao monta o cookie de sessão: HttpOnly, SameSite=Lax, Path=/,
// Secure conforme o ambiente, e Max-Age derivado de expiraEm.
//
// HttpOnly é o que impede o JavaScript de ler o token — sem ele, um único XSS
// vira sessão roubada. SameSite=Lax é a defesa de CSRF: o navegador não manda
// o cookie em requisição disparada por outro site.
//
// Na fase 5 este mesmo cookie autentica o handshake do WebSocket (que é HTTP e
// leva os cookies junto), e aí o SameSite deixa de bastar: WebSocket não
// obedece CORS, então a origem terá de ser checada à mão.
func novoCookieSessao(tokenPuro string, expiraEm time.Time, seguro bool) *http.Cookie {
	return &http.Cookie{
		Name:     NomeCookieSessao(seguro),
		Value:    tokenPuro,
		Path:     "/",
		Expires:  expiraEm,
		MaxAge:   int(time.Until(expiraEm).Seconds()),
		HttpOnly: true,
		Secure:   seguro,
		SameSite: http.SameSiteLaxMode,
	}
}

// cookieSessaoExpirado devolve um cookie com Max-Age negativo, usado para
// apagar o cookie de sessão do navegador no logout.
func cookieSessaoExpirado(seguro bool) *http.Cookie {
	return &http.Cookie{
		Name:     NomeCookieSessao(seguro),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   seguro,
		SameSite: http.SameSiteLaxMode,
	}
}
