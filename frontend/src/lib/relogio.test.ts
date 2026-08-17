// O relógio existe por causa de um defeito que teste nenhum pegava: o card
// mostrava "há 39 min" e continuava mostrando "há 39 min" meia hora depois.
//
// O que este teste tranca é justamente o que faltava — que o valor ANDE
// sozinho. Um relógio que devolve a hora certa uma vez e congela passa em
// qualquer asserção feita logo depois de lê-lo.
import { afterEach, describe, expect, it, vi } from 'vitest';
import { agoraReativo, pararRelogio } from './relogio.svelte';
import { haQuanto } from './atividade';

afterEach(() => {
	pararRelogio();
	vi.useRealTimers();
});

describe('agoraReativo', () => {
	// O primeiro tique é esperado de propósito antes de medir: o `$state` do
	// módulo nasce na IMPORTAÇÃO, com o relógio real, e só entra no tempo falso
	// depois que o intervalo roda uma vez. Comparar contra o valor de importação
	// mediria a diferença entre dois relógios, não a passagem do tempo.
	it('avança sozinho com o tempo', () => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date('2026-08-17T12:00:00Z'));
		agoraReativo();
		vi.advanceTimersByTime(30_000);

		const primeiro = agoraReativo();
		vi.advanceTimersByTime(120_000);
		const depois = agoraReativo();

		expect(depois.getTime() - primeiro.getTime()).toBe(120_000);
	});

	// O ponto do relógio não é dizer as horas: é fazer o texto relativo mudar.
	// Este teste percorre o caminho inteiro, do tique até a frase.
	it('faz o tempo relativo envelhecer', () => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date('2026-08-17T12:00:00Z'));
		const movidoEm = '2026-08-17T11:58:00Z';
		agoraReativo();
		vi.advanceTimersByTime(30_000);

		expect(haQuanto(movidoEm, agoraReativo())).toBe('há 2 min');

		vi.advanceTimersByTime(10 * 60_000);

		expect(haQuanto(movidoEm, agoraReativo())).toBe('há 12 min');
	});

	// O temporizador nasce na primeira leitura, e não na importação: um teste
	// que só use `haQuanto` não pode deixar um intervalo pendurado sem nunca
	// ter pedido as horas.
	it('não cria temporizador antes de alguém pedir as horas', () => {
		vi.useFakeTimers();
		pararRelogio();
		expect(vi.getTimerCount()).toBe(0);

		agoraReativo();

		expect(vi.getTimerCount()).toBe(1);
	});

	// Duas leituras não podem virar dois temporizadores: um quadro com cinquenta
	// cards criaria cinquenta, fazendo a mesma conta em momentos diferentes.
	it('usa um temporizador só, por mais que seja lido', () => {
		vi.useFakeTimers();
		pararRelogio();

		agoraReativo();
		agoraReativo();
		agoraReativo();

		expect(vi.getTimerCount()).toBe(1);
	});
});
