package middleware

import "net/http"

// SemCache marca a resposta como não armazenável por navegador, proxy ou CDN.
//
// Vale para tudo que depende de sessão, para o convite e para o quadro
// publicado: são respostas com conteúdo de uma pessoa só, e "de uma pessoa só"
// é exatamente o que um cache compartilhado não sabe distinguir. Sem isto,
// basta um proxy corporativo entre o cliente e o nginx da borda para o quadro de alguém
// ser servido a outra pessoa da mesma rede — e o bug aparece como "vi o quadro
// errado", muito depois de a causa ter sumido.
//
// `no-store` (e não `no-cache`) porque a resposta não deve nem ser GRAVADA: o
// `no-cache` permite guardar e só obriga a revalidar, o que já deixa o conteúdo
// em disco no caminho.
//
// O cabeçalho é definido ANTES de chamar o próximo handler. Depois seria tarde:
// quem escreve o corpo já enviou os cabeçalhos, e um Set posterior não sai no
// fio.
func SemCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// SemReferrer impede o navegador de contar a terceiros de que URL a pessoa
// veio.
//
// Importa aqui porque a URL do convite CARREGA o segredo:
// `/convite/<token>`. Sem esta política, qualquer imagem, fonte ou link externo
// carregado naquela página envia o token inteiro no cabeçalho Referer para um
// domínio de fora — e o convite vaza pelo log de acesso de outra pessoa, sem
// nada de errado acontecendo no nosso lado.
//
// Global, e não só na rota do convite: o custo é um cabeçalho por resposta, e a
// alternativa é lembrar de aplicá-lo a cada rota nova que passe a carregar
// segredo no caminho.
func SemReferrer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
