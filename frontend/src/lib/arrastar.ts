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

// DISTANCIA_DE_ARRASTE é quanto o ponteiro precisa andar para a interação
// contar como arraste, e não como clique.
//
// Sem esse limiar, um clique com a mão trêmula abriria o card E o moveria; ou
// pior, o clique abriria o modal logo depois de a pessoa ter arrastado.
export const DISTANCIA_DE_ARRASTE = 5;

// cliqueSemArraste devolve um par de manipuladores para elementos que são
// clicáveis E arrastáveis: o clique só vale se o ponteiro praticamente não
// andou entre apertar e soltar.
export function cliqueSemArraste(aoClicar: () => void) {
	let origem: { x: number; y: number } | null = null;

	return {
		onpointerdown(evento: PointerEvent) {
			origem = { x: evento.clientX, y: evento.clientY };
		},
		onclick(evento: MouseEvent) {
			if (!origem) return;
			const andou = Math.hypot(evento.clientX - origem.x, evento.clientY - origem.y);
			origem = null;
			if (andou <= DISTANCIA_DE_ARRASTE) aoClicar();
		}
	};
}
