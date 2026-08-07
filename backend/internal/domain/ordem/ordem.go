// Package ordem resolve o problema de manter itens ordenados quando várias
// pessoas inserem no meio ao mesmo tempo.
//
// A ideia é a "indexação fracionária". Se a posição fosse um inteiro
// sequencial — 1, 2, 3, 4 —, arrastar um item para o meio obrigaria a
// reescrever a numeração de todos os itens abaixo dele: muitas linhas alteradas
// e corrida garantida com duas pessoas mexendo no mesmo quadro.
//
// Com posições fracionárias, inserir entre 2.0 e 3.0 é gravar 2.5:
//
//	antes:   A(1.0)   B(2.0)          C(3.0)
//	                      ▲ solta aqui
//	depois:  A(1.0)   B(2.0)  X(2.5)  C(3.0)     ← só X foi escrito
//
// Uma linha alterada, nenhuma renumeração, nenhuma corrida.
//
// O preço é que dividir o mesmo intervalo indefinidamente esgota a precisão do
// float. Ver PosicaoEntre.
package ordem

import "errors"

// Passo é o intervalo deixado entre posições ao acrescentar no fim.
//
// Não é 1: o espaço entre duas posições vizinhas é o que permite inserir no
// meio sem renumerar ninguém, e um passo largo adia por muito tempo o dia em
// que as divisões pela metade esgotam a precisão.
const Passo = 1024.0

// ErrSemEspaco é retornado quando não cabe mais nenhuma posição entre os dois
// vizinhos informados.
//
// É o limite do double precision aparecendo: a mantissa tem 53 bits, então
// depois de ~50 inserções sucessivas no MESMO ponto os dois vizinhos ficam
// adjacentes e a média entre eles é igual a um deles. Duas linhas com a mesma
// posição não quebram a leitura (o desempate por id mantém a ordem estável),
// mas o item não iria para onde a pessoa soltou — e falhar dizendo isso é
// melhor do que gravar em silêncio uma ordem errada.
//
// A saída definitiva é trocar a posição por chave textual (LexoRank), onde
// entre "a" e "b" sempre cabe "an". É a fase 9 do plano.
var ErrSemEspaco = errors.New("não há espaço entre essas posições; reordene o quadro")

// NoFim devolve a posição de um item acrescentado depois do último. Recebe a
// maior posição em uso, ou 0 quando não há item nenhum.
func NoFim(ultimaPosicao float64) float64 {
	return ultimaPosicao + Passo
}

// NoComeco devolve a posição de um item colocado antes do primeiro. Recebe a
// menor posição em uso.
//
// Divide por dois em vez de subtrair o passo: assim a sequência nunca fica
// negativa, e o espaço até o zero — que também é finito — é consumido pela
// metade a cada vez, do mesmo jeito que no meio.
func NoComeco(primeiraPosicao float64) float64 {
	if primeiraPosicao <= 0 {
		return Passo
	}
	return primeiraPosicao / 2
}

// Entre devolve a posição de um item solto entre dois vizinhos.
//
// anterior é a posição de quem fica acima (0 quando o item vai para o topo) e
// proximo é a de quem fica abaixo (0 quando vai para o fim). Retorna
// ErrSemEspaco quando o intervalo já não comporta outro valor.
func Entre(anterior, proximo float64) (float64, error) {
	switch {
	case anterior <= 0 && proximo <= 0:
		// lista vazia
		return Passo, nil
	case anterior <= 0:
		return NoComeco(proximo), nil
	case proximo <= 0:
		return NoFim(anterior), nil
	}

	if proximo <= anterior {
		return 0, ErrSemEspaco
	}

	meio := anterior + (proximo-anterior)/2
	// A comparação é com os próprios vizinhos, e não com um epsilon escolhido a
	// dedo: o que importa é se o valor calculado ainda é DISTINGUÍVEL deles na
	// precisão disponível, e isso depende da magnitude dos números.
	if meio <= anterior || meio >= proximo {
		return 0, ErrSemEspaco
	}
	return meio, nil
}
