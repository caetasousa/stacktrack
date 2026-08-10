// Ajuda comum ao arrastar e soltar.

// TIPO_CARD e TIPO_COLUNA separam as duas zonas de arraste. Sem tipos
// distintos, o svelte-dnd-action deixaria soltar uma coluna dentro de outra.
export const TIPO_CARD = 'card';
export const TIPO_COLUNA = 'coluna';

// Duração da animação de reacomodação. Curta: o movimento existe para a pessoa
// entender que os itens se ajeitaram, não para ser assistido.
export const DURACAO_MS = 140;

export interface ComID {
	id: string;
}

// vizinhosDe descobre entre quem o item ficou depois do arraste.
//
// É o que a API espera: os IDS dos vizinhos, não a posição. Quem calcula o
// número é o servidor, que enxerga as posições reais — a cópia da tela pode
// estar velha.
export function vizinhosDe<T extends ComID>(
	lista: T[],
	itemID: string
): { anteriorId?: string; proximoId?: string } {
	const i = lista.findIndex((item) => item.id === itemID);
	if (i < 0) return {};

	return {
		anteriorId: lista[i - 1]?.id,
		proximoId: lista[i + 1]?.id
	};
}

// enfeitarArrastado dá ao item que segue o cursor a aparência de "na mão":
// inclinado e com sombra, descolado do quadro parado atrás. É o
// transformDraggedElement do svelte-dnd-action.
export function enfeitarArrastado(elemento?: HTMLElement) {
	elemento?.classList.add('item-arrastado');
}

// CLASSES_ALVO destaca a zona sob o cursor enquanto o item está no ar —
// responde a "esta coluna aceita o que estou segurando?".
export const CLASSES_ALVO = ['zona-alvo'];

// ESPERA_DE_TOQUE_MS é quanto o dedo precisa ficar parado antes de o gesto
// virar arraste. Vale só para toque — no mouse o arraste começa na hora.
//
// Com o padrão da biblioteca (`delayTouchStart: false`), o arraste começava no
// instante em que o dedo encostava. Efeito medido: arrastar o dedo para ROLAR a
// coluna levantava o card em vez de rolar, e o mesmo valia para o gesto lateral
// que percorre as colunas. Não havia como olhar uma lista longa no celular sem
// mover cards sem querer.
//
// Com a espera, o toque curto volta a ser toque e o arraste passa a ser um
// toque-e-segure — o mesmo gesto que o Trello e a tela inicial do celular usam
// para pegar algo.
//
// 250ms é deliberado: acima do tempo de um toque comum (que fica na casa dos
// 50-120ms), e abaixo do ponto em que segurar parece que não respondeu. O
// padrão da biblioteca quando ligada, 80ms, é curto demais — um toque sem
// pressa passaria dele e viraria arraste.
export const ESPERA_DE_TOQUE_MS = 250;

// DISTANCIA_DE_ARRASTE é quanto o ponteiro precisa andar para a interação
// contar como arraste, e não como clique.
//
// Sem esse limiar, um clique com a mão trêmula abriria o card E o moveria; ou
// pior, o clique abriria o modal logo depois de a pessoa ter arrastado.
export const DISTANCIA_DE_ARRASTE = 5;

// cliqueSemArraste devolve um trio de manipuladores para elementos que são
// clicáveis E arrastáveis: o clique só vale se o ponteiro praticamente não
// andou entre apertar e soltar.
//
// Quem mede é o `pointermove`, e não as coordenadas do `click`. Parece um
// detalhe e não é: o clique que o navegador SINTETIZA a partir de um toque
// chega sem clientX/clientY, então a conta dava NaN — e `NaN <= 5` é falso.
// Todo toque de dedo era classificado como arraste, e no celular nenhum card
// abria. Com `pointermove` a medida é o caminho de fato percorrido, que é o que
// a pergunta "isto foi um arraste?" quer saber, e existe nos dois casos.
//
// O toque ainda gera DOIS cliques (o sintetizado e o de compatibilidade). Quem
// resolve isso é `origem`, que o primeiro clique zera: o segundo cai fora e o
// modal não abre duas vezes.
export function cliqueSemArraste(aoClicar: () => void) {
	let origem: { x: number; y: number } | null = null;
	let percorreu = 0;

	return {
		onpointerdown(evento: PointerEvent) {
			origem = { x: evento.clientX, y: evento.clientY };
			percorreu = 0;
		},
		onpointermove(evento: PointerEvent) {
			if (!origem) return;
			percorreu = Math.max(
				percorreu,
				Math.hypot(evento.clientX - origem.x, evento.clientY - origem.y)
			);
		},
		onclick() {
			if (!origem) return;
			const andou = percorreu;
			origem = null;
			if (andou <= DISTANCIA_DE_ARRASTE) aoClicar();
		}
	};
}
