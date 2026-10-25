package comentario

import (
	"strings"
	"unicode"
)

// Mencionavel é quem pode ser chamado num comentário: quem participa do
// quadro. O comentário não conhece membro nem usuário — recebe a lista pronta.
type Mencionavel struct {
	ID   string
	Nome string
}

// Mencoes devolve quem o texto chama pelo nome, na ordem em que aparece e sem
// repetição.
//
// Uma menção é "@" seguido do NOME COMPLETO de quem participa (sem diferença de
// maiúsculas), ou só do PRIMEIRO NOME, quando uma única pessoa do quadro o tem.
// O nome precisa terminar ali: "@Brunox" não chama o Bruno. Quando um nome é o
// começo de outro, vale o mais longo que casar inteiro.
//
// Texto livre, e não um código escondido como "@[Ana](id)": quem lê o
// comentário vê o que foi escrito, e quem escreve não precisa saber de id
// nenhum — o autocompletar da tela só ajuda a digitar o nome certo.
//
// Duas pessoas com o MESMO nome completo são chamadas as duas: não há o que
// digitar a mais para distingui-las, e as duas leem o comentário de qualquer
// jeito. Já o primeiro nome repetido não chama ninguém — ali dá para ser mais
// específico, e chamar a pessoa errada seria pior que não chamar.
func Mencoes(texto string, candidatos []Mencionavel) []string {
	nomes := make([][]rune, len(candidatos))
	primeiros := make(map[string][]string)
	for i, c := range candidatos {
		partes := strings.Fields(c.Nome)
		nomes[i] = []rune(strings.Join(partes, " "))
		if len(partes) > 0 {
			chave := strings.ToLower(partes[0])
			primeiros[chave] = append(primeiros[chave], c.ID)
		}
	}

	runas := []rune(texto)
	vistos := make(map[string]bool)
	var mencionados []string
	anotar := func(id string) {
		if !vistos[id] {
			vistos[id] = true
			mencionados = append(mencionados, id)
		}
	}

	for i, r := range runas {
		// Antes do "@" não pode haver letra, dígito nem caractere de endereço:
		// é o que impede "bruno@exemplo.com" de chamar alguém.
		if r != '@' || (i > 0 && parteDeEndereco(runas[i-1])) {
			continue
		}
		resto := runas[i+1:]

		maior := 0
		for _, nome := range nomes {
			if n := casaNoInicio(resto, nome); n > maior {
				maior = n
			}
		}
		if maior > 0 {
			for j, nome := range nomes {
				if casaNoInicio(resto, nome) == maior {
					anotar(candidatos[j].ID)
				}
			}
			continue
		}

		if ids := primeiros[strings.ToLower(palavraNoInicio(resto))]; len(ids) == 1 {
			anotar(ids[0])
		}
	}
	return mencionados
}

// casaNoInicio informa quantas runas do texto o nome ocupa a partir do começo,
// ou zero quando não casa. O espaço do nome casa com qualquer sequência de
// espaços do texto, e o nome precisa terminar numa fronteira de palavra.
func casaNoInicio(texto, nome []rune) int {
	if len(nome) == 0 {
		return 0
	}
	t := 0
	for _, r := range nome {
		if r == ' ' {
			if t >= len(texto) || !unicode.IsSpace(texto[t]) {
				return 0
			}
			for t < len(texto) && unicode.IsSpace(texto[t]) {
				t++
			}
			continue
		}
		if t >= len(texto) || !mesmaLetra(texto[t], r) {
			return 0
		}
		t++
	}
	if t < len(texto) && (unicode.IsLetter(texto[t]) || unicode.IsDigit(texto[t])) {
		return 0
	}
	return t
}

// palavraNoInicio devolve a palavra que começa o texto: letras e dígitos.
func palavraNoInicio(texto []rune) string {
	fim := 0
	for fim < len(texto) && (unicode.IsLetter(texto[fim]) || unicode.IsDigit(texto[fim])) {
		fim++
	}
	return string(texto[:fim])
}

func mesmaLetra(a, b rune) bool {
	return a == b || unicode.ToLower(a) == unicode.ToLower(b)
}

func parteDeEndereco(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("._%+-@", r)
}
