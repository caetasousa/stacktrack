// Package ordem resolve o problema de manter itens ordenados quando várias
// pessoas inserem no meio ao mesmo tempo.
//
// São DUAS ordenações aqui, e a diferença entre elas é o assunto da fase 9:
//
//   - CHAVE TEXTUAL (chave.go), usada por cards e colunas. Entre duas chaves
//     sempre cabe outra, então arrastar para o mesmo ponto nunca esgota;
//   - POSIÇÃO FRACIONÁRIA (aqui), que sobrou para etiqueta e checklist.
//
// A posição continua onde está porque etiqueta e checklist só ACRESCENTAM no
// fim — nunca inserem entre dois vizinhos. É a inserção repetida no meio que
// esgota a mantissa do double precision, e sem ela o limite não existe na
// prática. Cards e colunas faziam exatamente isso, e por isso migraram.
//
// Se um dia a etiqueta ganhar arrastar-e-soltar, ela migra também — e o que
// falta para isso já está pronto em chave.go.
package ordem

// Passo é o intervalo deixado entre posições ao acrescentar no fim.
//
// Não é 1: o espaço entre duas posições vizinhas é o que permite inserir no
// meio sem renumerar ninguém, e um passo largo adia por muito tempo o dia em
// que as divisões pela metade esgotam a precisão.
const Passo = 1024.0

// NoFim devolve a posição de um item acrescentado depois do último. Recebe a
// maior posição em uso, ou 0 quando não há item nenhum.
func NoFim(ultimaPosicao float64) float64 {
	return ultimaPosicao + Passo
}
