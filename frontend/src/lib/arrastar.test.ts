import { describe, expect, it } from 'vitest';
import { cliqueSemArraste, vizinhosDe } from './arrastar';

const lista = [{ id: 'a' }, { id: 'b' }, { id: 'c' }];

describe('vizinhosDe', () => {
	it('acha os dois vizinhos no meio', () => {
		expect(vizinhosDe(lista, 'b')).toEqual({ anteriorId: 'a', proximoId: 'c' });
	});

	// Ponta sem vizinho vira undefined, que o JSON omite — e o servidor lê como
	// "topo" ou "fim".
	it('no topo não há anterior', () => {
		expect(vizinhosDe(lista, 'a')).toEqual({ anteriorId: undefined, proximoId: 'b' });
	});

	it('no fim não há próximo', () => {
		expect(vizinhosDe(lista, 'c')).toEqual({ anteriorId: 'b', proximoId: undefined });
	});

	it('item sozinho não tem vizinho nenhum', () => {
		expect(vizinhosDe([{ id: 'unico' }], 'unico')).toEqual({
			anteriorId: undefined,
			proximoId: undefined
		});
	});

	it('item que não está na lista não tem vizinhos', () => {
		expect(vizinhosDe(lista, 'sumiu')).toEqual({});
	});
});

describe('cliqueSemArraste', () => {
	const evento = (x: number, y: number) => ({ clientX: x, clientY: y }) as PointerEvent;

	it('conta como clique quando o ponteiro quase não andou', () => {
		let abriu = false;
		const { onpointerdown, onpointermove, onclick } = cliqueSemArraste(() => (abriu = true));

		onpointerdown(evento(100, 100));
		onpointermove(evento(102, 101));
		onclick();

		expect(abriu).toBe(true);
	});

	// Sem esse limiar, soltar um card depois de arrastá-lo abriria o modal em
	// cima do movimento que a pessoa acabou de fazer.
	it('não conta como clique depois de um arraste', () => {
		let abriu = false;
		const { onpointerdown, onpointermove, onclick } = cliqueSemArraste(() => (abriu = true));

		onpointerdown(evento(100, 100));
		onpointermove(evento(260, 340));
		onclick();

		expect(abriu).toBe(false);
	});

	// O que quebrava o celular: o clique SINTETIZADO a partir de um toque chega
	// sem coordenada nenhuma. Medir pelo evento de clique dava NaN, e `NaN <= 5`
	// é falso — todo toque virava "arraste" e nenhum card abria.
	it('conta como clique um toque, que não move o ponteiro nenhuma vez', () => {
		let abriu = false;
		const { onpointerdown, onclick } = cliqueSemArraste(() => (abriu = true));

		onpointerdown(evento(100, 100));
		onclick();

		expect(abriu).toBe(true);
	});

	// Voltar ao ponto de partida não desfaz o arraste: o que interessa é o
	// caminho percorrido, não a distância entre as pontas.
	it('não conta como clique quando o ponteiro foi longe e voltou', () => {
		let abriu = false;
		const { onpointerdown, onpointermove, onclick } = cliqueSemArraste(() => (abriu = true));

		onpointerdown(evento(100, 100));
		onpointermove(evento(300, 100));
		onpointermove(evento(100, 100));
		onclick();

		expect(abriu).toBe(false);
	});

	// O toque gera DOIS cliques: o sintetizado e o de compatibilidade. Se os
	// dois valessem, o card abriria e o segundo clique cairia no fundo do modal,
	// fechando-o no mesmo gesto.
	it('só o primeiro dos dois cliques de um toque vale', () => {
		let vezes = 0;
		const { onpointerdown, onclick } = cliqueSemArraste(() => vezes++);

		onpointerdown(evento(100, 100));
		onclick();
		onclick();

		expect(vezes).toBe(1);
	});

	it('ignora clique sem pointerdown antes', () => {
		let abriu = false;
		const { onclick } = cliqueSemArraste(() => (abriu = true));

		onclick();

		expect(abriu).toBe(false);
	});

	// Um pointermove que chega depois do clique (ou sem aperto nenhum) não pode
	// contaminar a medida do gesto seguinte.
	it('não acumula movimento fora de um gesto', () => {
		let abriu = false;
		const { onpointerdown, onpointermove, onclick } = cliqueSemArraste(() => (abriu = true));

		onpointermove(evento(900, 900));
		onpointerdown(evento(100, 100));
		onclick();

		expect(abriu).toBe(true);
	});
});
