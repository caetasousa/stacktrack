package ordem

import (
	"errors"
	"math/rand/v2"
	"strings"
)

// A chave TEXTUAL de ordenação — o conserto do limite que `Entre` tem.
//
// A posição fracionária em float funciona até a mantissa acabar: dividir o
// mesmo intervalo pela metade esgota os 53 bits do double precision em algumas
// dezenas de inserções seguidas no mesmo ponto, e aí não cabe mais nada (ver
// ErrSemEspaco). Com chave textual isso deixa de existir: entre "b" e "c" cabe
// "bn", entre "b" e "bn" cabe "bg", e assim por diante — **infinitamente**,
// porque a chave simplesmente cresce um caractere.
//
// (O plano ilustrava com "entre a e b cabe an". Na implementação "a" não é
// chave válida: ela termina no menor caractere, e é justamente o beco sem saída
// que a invariante abaixo existe para proibir.)
//
// O preço é que a chave cresce quando se insere sempre no mesmo ponto. É um
// preço bom: um caractere a mais por colisão, contra um erro que a pessoa não
// tem como resolver pela interface.
//
// "Infinitamente" é verdade no papel, e o papel não é onde isto roda: a coluna
// é VARCHAR(200). Medido contra a API de verdade, a chave cresce ~0,27
// caractere por movimento no MESMO ponto — 400 movimentos produziram 109
// caracteres —, então o teto real fica perto de 730 movimentos consecutivos no
// mesmo lugar. É 14 vezes o que o float aguentava, e continua sendo um teto.
// TamanhoMaximo existe para que ele apareça como 409 do domínio e não como
// erro de VARCHAR vindo do driver, que viraria 500. Se um dia isso for
// alcançado de verdade, a resposta não é uma coluna mais larga (adia o mesmo
// problema): é redistribuir as chaves da coluna, aceitando de propósito a
// reescrita em massa que este esquema evita no caso comum.

// alfabeto é a ordem em que os caracteres valem. Só minúsculas de propósito:
//
//  1. a ordenação de texto no Postgres depende da COLLATION, e uma que ignore
//     caixa (ou trate acentos) ordenaria diferente do que este domínio assume.
//     Com um alfabeto de a-z, toda collation usual concorda — e a consulta
//     ainda assim ordena com COLLATE "C" para não depender disso;
//  2. chave que se lê e se digita sem ambiguidade entre O e 0, l e 1.
const alfabeto = "abcdefghijklmnopqrstuvwxyz"

const (
	menor byte = 'a'
	maior byte = 'z'
	meio  byte = 'n' // o do meio do alfabeto
)

// ChaveInicial é a chave do primeiro item de uma lista vazia.
//
// É o MEIO do alfabeto, e não o começo: começar em "a" gastaria de saída toda a
// margem de quem for inserido antes dele.
const ChaveInicial = string(meio)

// ErrChaveInvalida é retornado quando uma chave não respeita a invariante do
// esquema.
var ErrChaveInvalida = errors.New("chave de ordenação inválida")

// ErrForaDeOrdem é retornado quando os vizinhos informados não estão em ordem —
// o anterior deveria vir antes do próximo.
var ErrForaDeOrdem = errors.New("os vizinhos informados estão fora de ordem")

// TamanhoMaximo é o comprimento máximo de uma chave, e ele NÃO é uma escolha
// do domínio: é o tamanho da coluna (VARCHAR(200) em V18). Estão amarrados de
// propósito — um domínio que gerasse chave maior que a coluna transformaria uma
// regra de negócio em erro de driver, e o teste
// test/repository/chave_cabe_na_coluna_test.go trava os dois juntos.
const TamanhoMaximo = 200

// ErrChaveLonga é retornado quando a chave calculada não caberia na coluna.
// Acontece só depois de centenas de movimentos consecutivos exatamente no mesmo
// ponto de uma lista; ver o comentário do topo para o que fazer nesse dia.
var ErrChaveLonga = errors.New("a lista foi reordenada vezes demais neste ponto")

// sorteioEntre devolve um caractere aleatório no intervalo FECHADO [de, ate].
//
// Sortear, em vez de pegar sempre o do meio, é o que evita COLISÃO entre duas
// inserções simultâneas no mesmo ponto. Sem isso, duas pessoas arrastando um
// card para o mesmo lugar ao mesmo tempo calculam a MESMA chave — e o estrago
// não é o empate em si (a ordem entre eles é arbitrária de qualquer forma, já
// que ninguém pediu uma), e sim o que vem depois: arrastar algo ENTRE os dois
// passa a ser impossível, porque não existe chave entre duas iguais.
//
// E é de graça: o sorteio acontece DENTRO do espaço que já existia, então a
// chave não fica um caractere mais longa por causa dele.
func sorteioEntre(de, ate byte) byte {
	if ate <= de {
		return de
	}
	return de + byte(rand.IntN(int(ate-de)+1))
}

// A INVARIANTE que sustenta tudo: nenhuma chave termina com o menor caractere.
//
// É ela que garante que SEMPRE cabe alguém antes de qualquer item. Sem ela,
// uma chave "a" seria um beco sem saída: não existe string de a-z entre "" e
// "a", e inserir no topo passaria a ser impossível — exatamente o problema que
// esta fase existe para eliminar.
func chaveValida(k string) bool {
	if k == "" {
		return false
	}
	for i := 0; i < len(k); i++ {
		if k[i] < menor || k[i] > maior {
			return false
		}
	}
	return k[len(k)-1] != menor
}

// ChaveEntre devolve uma chave que fica ENTRE as duas informadas.
//
// Vazio significa ponta: anterior vazio é o topo da lista, próximo vazio é o
// fim.
//
// Erra por entrada inválida, por vizinhos fora de ordem, ou — só no extremo
// descrito no topo do arquivo — por a chave não caber mais na coluna.
func ChaveEntre(anterior, proximo string) (string, error) {
	if anterior != "" && !chaveValida(anterior) {
		return "", ErrChaveInvalida
	}
	if proximo != "" && !chaveValida(proximo) {
		return "", ErrChaveInvalida
	}

	var nova string
	switch {
	case anterior == "" && proximo == "":
		// Lista vazia: a chave nasce no meio do alfabeto, com uma folga
		// sorteada, para preservar margem dos dois lados.
		nova = string(sorteioEntre(meio-2, meio+2))
	case anterior == "":
		nova = antesDe(proximo)
	case proximo == "":
		nova = depoisDe(anterior)
	case anterior >= proximo:
		return "", ErrForaDeOrdem
	default:
		nova = entreDuas(anterior, proximo)
	}

	if len(nova) > TamanhoMaximo {
		return "", ErrChaveLonga
	}
	return nova, nil
}

// antesDe devolve uma chave menor que a informada.
//
// Sempre existe, e é a invariante que garante isso: como nenhuma chave termina
// no menor caractere, dá para decrementar o último — e quando o decremento
// chegaria ao menor, estende-se a chave em vez disso.
func antesDe(k string) string {
	ultimo := k[len(k)-1]
	if ultimo > menor+1 {
		return k[:len(k)-1] + string(sorteioEntre(menor+1, ultimo-1))
	}
	// Decrementar cairia no menor caractere, que a invariante proíbe no fim.
	// Estender é sempre seguro: prefixo + "an" é menor que prefixo + "b".
	return k[:len(k)-1] + string(menor) + string(sorteioEntre(menor+1, maior))
}

// depoisDe devolve uma chave maior que a informada.
func depoisDe(k string) string {
	ultimo := k[len(k)-1]
	if ultimo < maior {
		return k[:len(k)-1] + string(sorteioEntre(ultimo+1, maior))
	}
	// Já está no maior caractere: estender é o que sempre cabe.
	return k + string(sorteioEntre(menor+1, maior))
}

// entreDuas devolve uma chave estritamente entre duas chaves ordenadas.
func entreDuas(a, b string) string {
	// O prefixo comum não participa da decisão: o que separa as duas chaves é o
	// primeiro caractere em que elas divergem.
	comum := 0
	for comum < len(a) && comum < len(b) && a[comum] == b[comum] {
		comum++
	}

	// `a` é prefixo de `b` (ex.: "an" e "anx"): a folga está no resto de `b`.
	if comum == len(a) {
		return a + antesDe(b[comum:])
	}

	if int(b[comum])-int(a[comum]) > 1 {
		// Cabe um caractere inteiro entre os dois — é o caso barato, e o
		// sorteio acontece dentro dessa folga.
		return a[:comum] + string(sorteioEntre(a[comum]+1, b[comum]-1))
	}

	// Caracteres vizinhos no alfabeto ("bq" e "c"): não cabe letra entre eles na
	// posição em que divergem, então a chave nova tem de manter o prefixo de `a`
	// até ali — e QUALQUER coisa com esse prefixo já é menor que `b`, porque `b`
	// diverge para cima justamente nessa posição.
	//
	// Isso deixa duas saídas, e escolher a errada custa caro. Estender `a` sempre
	// funciona, mas gasta um caractere POR INSERÇÃO: entre "bq" e "c" produziria
	// "bq?", depois "bq??", e a chave crescia uma letra de cada vez — 199
	// inserções e ela não cabia mais na coluna. Se ainda existe sufixo depois do
	// ponto de divergência, dá para crescer DENTRO do comprimento atual: entre
	// "bq" e "c" cabem "br".."bz", e só quando esse espaço acaba é que a chave
	// precisa de mais um caractere.
	if comum < len(a)-1 {
		return depoisDe(a)
	}
	return a + string(sorteioEntre(menor+1, maior))
}

// NormalizarChave devolve a chave em forma canônica, ou erro se ela não
// respeitar a invariante. Serve à fronteira: chave vinda do banco antigo, ou de
// um cliente, não é confiável.
func NormalizarChave(k string) (string, error) {
	k = strings.TrimSpace(k)
	if !chaveValida(k) {
		return "", ErrChaveInvalida
	}
	if len(k) > TamanhoMaximo {
		return "", ErrChaveInvalida
	}
	return k, nil
}
