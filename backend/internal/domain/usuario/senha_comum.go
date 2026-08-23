package usuario

import (
	_ "embed"
	"strings"
	"sync"
	"unicode/utf8"
)

// A lista é EMBUTIDA no binário, e não lida de arquivo em disco, porque ela é
// regra de negócio versionada junto do código: um arquivo externo poderia
// sumir no deploy e a validação passaria a aceitar tudo em silêncio — falha
// aberta, no lugar exatamente errado.
//
//go:embed senhas_comuns.txt
var listaDeSenhasComuns string

var (
	senhasComunsUmaVez sync.Once
	senhasComuns       map[string]struct{}
)

// carregarSenhasComuns monta o conjunto na primeira consulta. Uma vez só: a
// lista não muda em tempo de execução, e reprocessá-la a cada cadastro seria
// pagar o parse por requisição.
func carregarSenhasComuns() {
	senhasComuns = make(map[string]struct{})
	for _, linha := range strings.Split(listaDeSenhasComuns, "\n") {
		linha = strings.TrimSpace(linha)
		if linha == "" || strings.HasPrefix(linha, "#") {
			continue
		}
		senhasComuns[strings.ToLower(linha)] = struct{}{}
	}
}

// SenhaComum informa se a senha consta da lista local de senhas comuns ou
// vazadas, ou se é a repetição de um único caractere.
//
// A comparação ignora caixa e espaços nas pontas: "Senha12345678901" e
// "senha12345678901 " são o mesmo palpite para quem ataca, e tratá-las como
// senhas diferentes tornaria a lista contornável por uma tecla de Shift.
//
// O caractere repetido é checado por regra, e não por entrada na lista, porque
// não dá para enumerá-lo: "aaaaaaaaaaaaaaa" tem quinze letras, "aaaaaaaaaaaaaaaa"
// tem dezesseis, e assim por diante sem fim. É a única regra estrutural aqui —
// as demais ficam na lista, que se lê e se audita.
func SenhaComum(senha string) bool {
	senhasComunsUmaVez.Do(carregarSenhasComuns)

	normalizada := strings.ToLower(strings.TrimSpace(senha))
	if normalizada == "" {
		return false
	}
	if _, achou := senhasComuns[normalizada]; achou {
		return true
	}
	return umCaractereRepetido(normalizada)
}

// umCaractereRepetido informa se a senha é o mesmo caractere do início ao fim.
func umCaractereRepetido(s string) bool {
	primeiro, tamanho := utf8.DecodeRuneInString(s)
	if tamanho == 0 {
		return false
	}
	for _, r := range s {
		if r != primeiro {
			return false
		}
	}
	return true
}
