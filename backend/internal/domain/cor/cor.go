// Package cor guarda a paleta compartilhada por etiqueta, coluna e card.
//
// O banco guarda o NOME da cor ('vermelho'), e não o hex. A paleta é decisão de
// design e muda com o tema claro/escuro; gravar #F04438 congelaria no dado uma
// escolha que pertence ao CSS. Também é o que permite a mesma classe
// `cor-vermelho` pintar uma etiqueta, o cabeçalho de uma coluna e a tarja de um
// card sem três tabelas de cores.
package cor

import "errors"

// Cor é o nome de uma cor da paleta.
type Cor string

const (
	// Nenhuma é a ausência de cor — o item usa o visual padrão.
	Nenhuma  Cor = ""
	Cinza    Cor = "cinza"
	Vermelho Cor = "vermelho"
	Laranja  Cor = "laranja"
	Amarelo  Cor = "amarelo"
	Verde    Cor = "verde"
	Azul     Cor = "azul"
	Roxo     Cor = "roxo"
	Rosa     Cor = "rosa"
)

// Paleta é a lista inteira, na ordem em que a tela deve oferecê-la.
var Paleta = []Cor{Cinza, Vermelho, Laranja, Amarelo, Verde, Azul, Roxo, Rosa}

// ErrInvalida é retornado quando a cor não pertence à paleta.
var ErrInvalida = errors.New("cor inválida")

// Valida informa se a cor pertence à paleta. Não aceita a ausência de cor —
// para isso existe ValidaOuVazia.
func Valida(c Cor) bool {
	for _, conhecida := range Paleta {
		if c == conhecida {
			return true
		}
	}
	return false
}

// ValidaOuVazia aceita também a ausência de cor. É o que coluna e card usam:
// para eles a cor é opcional, ao contrário da etiqueta, que sem cor não teria
// como ser reconhecida de relance.
func ValidaOuVazia(c Cor) bool {
	return c == Nenhuma || Valida(c)
}
