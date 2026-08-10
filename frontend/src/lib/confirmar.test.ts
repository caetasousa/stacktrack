import { describe, expect, it } from 'vitest';
import { confirmar, pedidoAberto, responder } from './confirmar.svelte';

describe('confirmar', () => {
	it('não resolve enquanto ninguém responde', async () => {
		let resolvida = false;
		const p = confirmar({ titulo: 'Apagar?', acao: 'Apagar' });
		p.then(() => (resolvida = true));

		// Uma volta no laço de eventos: tempo de sobra para uma promessa já
		// resolvida se acusar.
		await Promise.resolve();

		expect(resolvida).toBe(false);
		expect(pedidoAberto()?.titulo).toBe('Apagar?');

		responder(false);
		await p;
	});

	it('resolve true ao confirmar e fecha o diálogo', async () => {
		const p = confirmar({ titulo: 'Apagar?', acao: 'Apagar' });
		responder(true);

		expect(await p).toBe(true);
		expect(pedidoAberto()).toBe(null);
	});

	it('resolve false ao cancelar', async () => {
		const p = confirmar({ titulo: 'Apagar?', acao: 'Apagar' });
		responder(false);

		expect(await p).toBe(false);
		expect(pedidoAberto()).toBe(null);
	});

	// A guarda que sustenta os pontos de chamada: eles são todos
	// `if (!(await confirmar(...))) return;`. Uma promessa que ficasse pendente
	// para sempre travaria a ação sem nenhum sinal na tela.
	it('um pedido novo cancela o anterior em vez de enfileirar', async () => {
		const primeira = confirmar({ titulo: 'Apagar o card?', acao: 'Apagar' });
		const segunda = confirmar({ titulo: 'Apagar a coluna?', acao: 'Apagar' });

		expect(await primeira).toBe(false);
		expect(pedidoAberto()?.titulo).toBe('Apagar a coluna?');

		responder(true);
		expect(await segunda).toBe(true);
	});

	it('responder sem diálogo aberto não quebra', () => {
		expect(() => responder(true)).not.toThrow();
		expect(pedidoAberto()).toBe(null);
	});

	// O detalhe é opcional, e é ele que diz o que vai junto — a informação que
	// muda a decisão de quem está prestes a apagar uma coluna cheia.
	it('leva título, detalhe e rótulo da ação para o diálogo', async () => {
		const p = confirmar({
			titulo: 'Apagar a coluna "A fazer"?',
			detalhe: 'Os cards dela vão junto.',
			acao: 'Apagar a coluna'
		});

		expect(pedidoAberto()).toMatchObject({
			titulo: 'Apagar a coluna "A fazer"?',
			detalhe: 'Os cards dela vão junto.',
			acao: 'Apagar a coluna'
		});

		responder(false);
		await p;
	});
});
