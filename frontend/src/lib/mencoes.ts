// O autocompletar de menções: reconhecer que a pessoa está digitando "@nome",
// sugerir quem participa do quadro e trocar o que ela digitou pelo nome
// completo.
//
// O que vale como menção é decidido pelo SERVIDOR (domain/comentario/mencao.go):
// "@" seguido do nome completo, ou do primeiro nome quando só uma pessoa o tem.
// Esta ajuda só digita o nome completo por quem escolhe na lista — que é a forma
// que o servidor sempre reconhece.

/** Onde começa a menção que está sendo digitada, e o que já foi digitado. */
export interface ConsultaDeMencao {
	/** A posição do "@" no texto. */
	inicio: number;
	termo: string;
}

/** Quem pode ser mencionado: o mínimo que a lista precisa mostrar. */
export interface Mencionavel {
	usuarioId: string;
	nome: string;
}

// Até onde vale procurar o "@" para trás: um nome comprido e um pouco de folga.
const TAMANHO_MAXIMO_DO_TERMO = 40;

/**
 * consultaNoCursor responde se o cursor está logo depois de um "@algo" que pode
 * ser uma menção — e devolve onde ele começa.
 *
 * O "@" precisa estar no começo ou depois de algo que não seja letra, dígito ou
 * caractere de endereço: é o que impede "bruno@exemplo" de abrir a lista. O
 * termo aceita o espaço do sobrenome, mas não quebra de linha nem espaço duplo.
 */
export function consultaNoCursor(texto: string, cursor: number): ConsultaDeMencao | null {
	const antes = texto.slice(0, cursor);
	const arroba = antes.lastIndexOf('@');
	if (arroba < 0) return null;
	const termo = antes.slice(arroba + 1);
	if (termo.length > TAMANHO_MAXIMO_DO_TERMO || /[\n\r@]|\s{2}|^\s/.test(termo)) return null;
	if (arroba > 0 && /[\p{L}\p{N}._%+-]/u.test(antes[arroba - 1])) return null;
	return { inicio: arroba, termo };
}

/**
 * sugestoes devolve quem participa e combina com o termo — sem caixa e sem
 * acento, pelo começo de qualquer palavra do nome, ou do nome inteiro.
 */
export function sugestoes<T extends Mencionavel>(pessoas: T[], termo: string, limite = 6): T[] {
	const procurado = normalizar(termo);
	return pessoas
		.filter((p) => {
			const nome = normalizar(p.nome);
			if (procurado === '' || nome.startsWith(procurado)) return true;
			return nome
				.split(' ')
				.some((_, i, palavras) => palavras.slice(i).join(' ').startsWith(procurado));
		})
		.slice(0, limite);
}

/**
 * aplicarMencao troca o "@termo" digitado pelo "@Nome Completo" escolhido e
 * devolve o texto e a posição do cursor, logo depois do nome.
 *
 * Separa com um espaço quando o que vem depois é palavra (ou é o fim do texto),
 * e não quando é pontuação: "@Ana Souza, veja", e não "@Ana Souza , veja".
 */
export function aplicarMencao(
	texto: string,
	consulta: ConsultaDeMencao,
	cursor: number,
	nome: string
): { texto: string; cursor: number } {
	const depois = texto.slice(cursor);
	const espaco = depois === '' || /^[\p{L}\p{N}]/u.test(depois) ? ' ' : '';
	const inserido = `@${nome}${espaco}`;
	return {
		texto: texto.slice(0, consulta.inicio) + inserido + depois,
		cursor: consulta.inicio + inserido.length
	};
}

function normalizar(texto: string): string {
	return texto.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase().replace(/\s+/g, ' ').trim();
}
