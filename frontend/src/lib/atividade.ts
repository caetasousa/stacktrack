// O histórico de um card, em frases.
//
// A tradução vive aqui, e não no servidor, por uma razão: a API devolve o que
// ACONTECEU (tipo + dados), e a frase é apresentação. Montar o texto em Go
// congelaria idioma e redação no backend, e mudar "moveu" para "arrastou"
// viraria deploy de API.
//
// E vive fora do componente porque é lógica pura com muitos casos — é onde um
// `else` esquecido produz uma linha em branco no meio do histórico sem quebrar
// nada, que é o defeito mais fácil de não notar.

/** O que o servidor grava no payload dos eventos de card. */
export interface DadosDoCard {
	cardId?: string;
	titulo?: string;
	tituloAnterior?: string;
	coluna?: string;
	deColuna?: string;
}

export interface Atividade {
	seq: number;
	tipo: string;
	autorId: string;
	autorNome: string;
	dados?: DadosDoCard;
	ocorridoEm: string;
}

/**
 * frase descreve uma entrada do histórico, já sem o nome de quem agiu — a tela
 * mostra o autor em negrito e a frase logo depois.
 *
 * Devolve string vazia para o que não sabe descrever, e quem chama omite a
 * linha. É deliberado: um tipo de evento novo aparece como silêncio, e não como
 * "undefined" no meio da conversa.
 */
export function frase(a: Atividade): string {
	const d = a.dados ?? {};

	switch (a.tipo) {
		case 'card.criado':
			return d.coluna ? `criou este card em ${d.coluna}` : 'criou este card';

		case 'card.movido':
			// O "de onde" é o que dá sentido ao movimento. Quando ele falta — em
			// eventos gravados antes de o payload carregar a origem — a frase
			// encolhe em vez de mentir.
			if (d.deColuna && d.coluna && d.deColuna !== d.coluna) {
				return `moveu de ${d.deColuna} para ${d.coluna}`;
			}
			return d.coluna ? `reordenou em ${d.coluna}` : 'moveu este card';

		case 'card.alterado':
			return d.tituloAnterior ? `renomeou de "${d.tituloAnterior}"` : 'editou este card';

		case 'card.apagado':
			return d.titulo ? `apagou "${d.titulo}"` : 'apagou este card';

		case 'comentario.alterado':
			return 'comentou';

		default:
			return '';
	}
}

/** Data curta: num histórico o que importa é quando, não a precisão. */
export function quando(iso: string): string {
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return '';
	return d.toLocaleString('pt-BR', {
		day: '2-digit',
		month: 'short',
		hour: '2-digit',
		minute: '2-digit'
	});
}

/** Entradas que a tela sabe descrever — o resto é omitido. */
export function descritas(lista: Atividade[]): { atividade: Atividade; texto: string }[] {
	return lista
		.map((atividade) => ({ atividade, texto: frase(atividade) }))
		.filter((linha) => linha.texto !== '');
}
