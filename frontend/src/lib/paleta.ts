// Regras de cor do quadro: qual cor uma coluna nova ganha e qual cor um card
// veste. Vive num módulo próprio porque a tela do quadro e a página pública
// desenham os mesmos cards e precisam chegar à mesma cor.

import type { Cor } from '$lib/api/boards';

// A ordem do rodízio não é a da paleta. `cinza` fica por último porque é a cor
// que mais se parece com "sem cor" — abrir um quadro e ver a primeira coluna
// cinza pareceria que a cor não pegou. As demais vêm alternadas em matiz para
// que duas colunas vizinhas nunca nasçam em tons parecidos.
export const CORES_AUTOMATICAS: Cor[] = [
	'azul',
	'verde',
	'roxo',
	'laranja',
	'rosa',
	'amarelo',
	'vermelho',
	'cinza'
];

/**
 * proximaCor escolhe a cor de uma coluna nova: a primeira do rodízio que
 * ainda não está em uso no quadro.
 *
 * Esgotadas as oito, ela recomeça pelo início — repetir é melhor do que voltar
 * a criar coluna sem cor depois que o quadro inteiro já é colorido. Entradas
 * sem cor são ignoradas, e não contam como cor usada.
 */
export function proximaCor(usadas: readonly (Cor | '')[]): Cor {
	const emUso = new Set(usadas.filter((c): c is Cor => c !== ''));
	const livre = CORES_AUTOMATICAS.find((c) => !emUso.has(c));
	if (livre) return livre;
	return CORES_AUTOMATICAS[emUso.size % CORES_AUTOMATICAS.length];
}

/**
 * corDoCard devolve a cor com que o card é pintado: a dele, quando escolheram
 * uma, senão a da coluna onde ele está.
 *
 * A herança é a regra e não a exceção: um quadro em que cada coluna tem a sua
 * cor só se lê de relance se os cards a acompanharem. Por ser herança — e não
 * cópia no momento da criação — o card muda de cor ao ser arrastado para outra
 * coluna, que é o que a pessoa acabou de dizer sobre ele.
 */
export function corDoCard(cardCor: Cor | '', colunaCor: Cor | ''): Cor | '' {
	return cardCor || colunaCor;
}
