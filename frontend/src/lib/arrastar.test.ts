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
	const evento = (x: number, y: number) =>
		({ clientX: x, clientY: y }) as PointerEvent & MouseEvent;

	it('conta como clique quando o ponteiro quase não andou', () => {
		let abriu = false;
		const { onpointerdown, onclick } = cliqueSemArraste(() => (abriu = true));

		onpointerdown(evento(100, 100));
		onclick(evento(102, 101));

		expect(abriu).toBe(true);
	});

	// Sem esse limiar, soltar um card depois de arrastá-lo abriria o modal em
	// cima do movimento que a pessoa acabou de fazer.
	it('não conta como clique depois de um arraste', () => {
		let abriu = false;
		const { onpointerdown, onclick } = cliqueSemArraste(() => (abriu = true));

		onpointerdown(evento(100, 100));
		onclick(evento(260, 340));

		expect(abriu).toBe(false);
	});

	it('ignora clique sem pointerdown antes', () => {
		let abriu = false;
		const { onclick } = cliqueSemArraste(() => (abriu = true));

		onclick(evento(100, 100));

		expect(abriu).toBe(false);
	});
});
