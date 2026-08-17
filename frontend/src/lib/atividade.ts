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

/**
 * fraseNoQuadro descreve a mesma entrada para a AUDITORIA do quadro, onde a
 * frase precisa dizer em qual card a coisa aconteceu.
 *
 * No histórico de um card o contexto é a própria tela ("moveu de A para B" —
 * moveu este card, o que está aberto). Numa lista que mistura o quadro inteiro,
 * a mesma frase fica sem sujeito: cinquenta linhas dizendo "moveu de A para B"
 * não deixam auditar nada.
 *
 * O título vem do payload do evento, e é o que o card tinha NA HORA. Um card
 * renomeado depois — ou apagado — continua identificável pela linha do log.
 */
export function fraseNoQuadro(a: Atividade): string {
	const d = a.dados ?? {};
	const card = d.titulo ? `"${d.titulo}"` : 'um card';

	switch (a.tipo) {
		case 'card.movido':
			if (d.deColuna && d.coluna && d.deColuna !== d.coluna) {
				return `moveu ${card} de ${d.deColuna} para ${d.coluna}`;
			}
			return d.coluna ? `reordenou ${card} em ${d.coluna}` : `moveu ${card}`;

		case 'card.criado':
			return d.coluna ? `criou ${card} em ${d.coluna}` : `criou ${card}`;

		case 'card.alterado':
			return d.tituloAnterior ? `renomeou "${d.tituloAnterior}" para ${card}` : `editou ${card}`;

		case 'card.apagado':
			return `apagou ${card}`;

		case 'comentario.alterado':
			return `comentou em ${card}`;

		// Os eventos de coluna e de quadro não têm card, e o payload deles é
		// outro. Ficam de fora em vez de virarem uma frase torta — a auditoria
		// existe para as movimentações, e o resto é acompanhamento.
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

/**
 * haQuanto devolve "agora", "há 5 min", "há 3 h", "há 2 d" — o tempo relativo
 * que cabe no rodapé de um card.
 *
 * Relativo, e não a data: no card o que importa é se aquilo foi AGORA HÁ POUCO
 * ou faz tempo, e "17 de ago., 14:32" obriga quem lê a fazer a conta. A data
 * exata continua disponível no `title` e na tela de auditoria.
 *
 * Acima de uma semana volta a ser data: "há 43 d" não diz nada a ninguém.
 */
export function haQuanto(iso: string, agora: Date = new Date()): string {
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return '';

	const segundos = Math.floor((agora.getTime() - d.getTime()) / 1000);
	// Relógio do cliente adiantado em relação ao servidor produz futuro por
	// alguns segundos. "em 3 s" seria absurdo num histórico — vira "agora".
	if (segundos < 60) return 'agora';
	if (segundos < 3600) return `há ${Math.floor(segundos / 60)} min`;
	if (segundos < 86400) return `há ${Math.floor(segundos / 3600)} h`;
	if (segundos < 7 * 86400) return `há ${Math.floor(segundos / 86400)} d`;
	return quando(iso);
}

/** Entradas que a tela sabe descrever — o resto é omitido. */
export function descritas(lista: Atividade[]): { atividade: Atividade; texto: string }[] {
	return lista
		.map((atividade) => ({ atividade, texto: frase(atividade) }))
		.filter((linha) => linha.texto !== '');
}
