// Package dominio é a regra da entrada pelo domínio do email: o dono abre o
// quadro a "quem tiver email @acme.com.br confirmado", e a colega entra sem
// convite.
//
// Não existe conta de empresa. O que prova que o domínio é da empresa é o
// email CONFIRMADO de quem o abre: só se abre o próprio domínio. É por isso
// que nenhuma rota recebe o domínio como texto — ele é sempre derivado da
// conta.
package dominio

import (
	"errors"
	"strings"

	"stacktrack/internal/domain/membro"
)

var (
	// ErrDominioPessoal é retornado quando o domínio é de um provedor de email
	// pessoal: abri-lo seria abrir o quadro a qualquer pessoa com conta lá.
	ErrDominioPessoal = errors.New("o domínio de um email pessoal não pode ser aberto: use o email da empresa")
	// ErrPapelDoDominio é retornado quando o papel de quem entra pelo domínio
	// não é editor nem leitor. Dono só por promoção, uma pessoa por vez.
	ErrPapelDoDominio = errors.New("quem entra pelo domínio é editor ou leitor")
	// ErrEmailNaoConfirmado é retornado quando a conta ainda não confirmou o
	// email: sem a confirmação, nada prova que o domínio é dela.
	ErrEmailNaoConfirmado = errors.New("confirme o seu email antes de abrir o quadro à empresa")
)

// pessoais são os provedores de email pessoal mais usados no Brasil e fora.
// A lista não precisa ser completa para cumprir o papel: ela barra o engano
// comum (o dono que só tem Gmail), e o domínio raro que escapar continua
// exigindo email confirmado de quem entra.
var pessoais = map[string]bool{
	"gmail.com": true, "googlemail.com": true,
	"outlook.com": true, "outlook.com.br": true, "hotmail.com": true, "hotmail.com.br": true,
	"live.com": true, "msn.com": true,
	"yahoo.com": true, "yahoo.com.br": true, "ymail.com": true, "rocketmail.com": true,
	"icloud.com": true, "me.com": true, "mac.com": true,
	"aol.com": true, "gmx.com": true, "gmx.net": true, "mail.com": true,
	"proton.me": true, "protonmail.com": true, "pm.me": true,
	"zoho.com": true, "yandex.com": true, "tutanota.com": true,
	"uol.com.br": true, "bol.com.br": true, "terra.com.br": true, "ig.com.br": true,
	"r7.com": true, "globo.com": true, "globomail.com": true, "oi.com.br": true,
}

// DoEmail devolve o domínio do email, normalizado (minúsculo, sem espaços).
// Vazio quando o email não tem domínio.
func DoEmail(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	arroba := strings.LastIndex(email, "@")
	if arroba < 0 || arroba == len(email)-1 {
		return ""
	}
	return email[arroba+1:]
}

// Pessoal informa se o domínio é de um provedor de email pessoal.
func Pessoal(dominio string) bool {
	return pessoais[dominio]
}

// Pertence informa se o email é EXATAMENTE do domínio. Subdomínio não conta,
// nos dois sentidos: "vendas.acme.com.br" é outra área, e abrir o pai não deve
// abrir o filho, nem o contrário.
func Pertence(email, dominio string) bool {
	return dominio != "" && DoEmail(email) == dominio
}

// ParaAbrir valida a abertura de um quadro ao domínio de quem a pede e devolve
// o domínio a gravar. Retorna ErrEmailNaoConfirmado, ErrDominioPessoal ou
// ErrPapelDoDominio, nessa ordem.
func ParaAbrir(emailDoDono string, confirmado bool, papel membro.Papel) (string, error) {
	if !confirmado {
		return "", ErrEmailNaoConfirmado
	}
	d := DoEmail(emailDoDono)
	if d == "" || Pessoal(d) {
		return "", ErrDominioPessoal
	}
	if papel != membro.PapelEditor && papel != membro.PapelLeitor {
		return "", ErrPapelDoDominio
	}
	return d, nil
}
