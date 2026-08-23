package ordem

// Redistribuir devolve `quantidade` chaves válidas, em ordem crescente e
// igualmente espaçadas.
//
// É a saída de emergência do esquema de chave textual, e existe para dois
// becos:
//
//  1. CHAVES REPETIDAS. Duas inserções simultâneas exatamente no mesmo ponto
//     podem calcular a mesma chave (o sorteio reduz a chance, não a elimina).
//     O empate em si é inofensivo — a ordem entre dois itens que ninguém
//     ordenou é arbitrária de qualquer forma —, mas ele fecha o espaço: não
//     existe chave ENTRE duas iguais, então arrastar algo para o meio delas
//     passa a ser impossível pela interface.
//
//  2. CHAVE SEM ESPAÇO. Inserir centenas de vezes no mesmo ponto faz a chave
//     crescer até não caber na coluna (ErrChaveLonga). O comentário no topo de
//     chave.go já antecipava a resposta para esse dia: redistribuir as chaves
//     do contêiner, aceitando de propósito a reescrita em massa que o esquema
//     evita no caso comum.
//
// A reescrita é cara e por isso não é preventiva: ela roda quando o caminho
// normal falha, sob o lock do quadro, e não a cada movimento.
//
// As chaves saem espaçadas para que o próximo movimento volte a caber entre
// duas vizinhas sem precisar crescer — redistribuir e já deixar tudo colado
// seria trocar um rebalanceamento por outro logo em seguida.
func Redistribuir(quantidade int) ([]string, error) {
	if quantidade <= 0 {
		return nil, nil
	}

	// O comprimento cresce até haver folga de sobra: `2*(quantidade+1)` garante
	// passo >= 2, e é o passo >= 2 que deixa espaço livre entre duas chaves
	// consecutivas — sem ele, a lista sairia redistribuída e imediatamente sem
	// lugar para inserir no meio.
	comprimento, espaco := 1, len(alfabeto)
	for espaco < 2*(quantidade+1) {
		comprimento++
		if comprimento > TamanhoMaximo {
			return nil, ErrChaveLonga
		}
		espaco *= len(alfabeto)
	}

	passo := espaco / (quantidade + 1)
	chaves := make([]string, 0, quantidade)
	for i := 1; i <= quantidade; i++ {
		chaves = append(chaves, chaveDoValor(passo*i, comprimento))
	}
	return chaves, nil
}

// chaveDoValor escreve o valor em base 26 (a=0 … z=25) com o comprimento dado,
// respeitando a invariante de que nenhuma chave termina no menor caractere.
//
// O ajuste do último caractere é seguro porque Redistribuir garante passo >= 2:
// somar 1 a um valor nunca o leva ao valor seguinte da sequência, então as
// chaves continuam estritamente crescentes.
func chaveDoValor(valor, comprimento int) string {
	if valor%len(alfabeto) == 0 {
		valor++
	}
	digitos := make([]byte, comprimento)
	for i := comprimento - 1; i >= 0; i-- {
		digitos[i] = menor + byte(valor%len(alfabeto))
		valor /= len(alfabeto)
	}
	return string(digitos)
}
