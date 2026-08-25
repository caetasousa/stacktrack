import { describe, expect, it } from 'vitest';
import { CORES_AUTOMATICAS, corDoCard, proximaCor } from './paleta';
import { CORES } from '$lib/api/boards';

describe('CORES_AUTOMATICAS', () => {
	it('usa a paleta inteira, sem sobra nem repetição', () => {
		expect([...CORES_AUTOMATICAS].sort()).toEqual([...CORES].sort());
	});
});

describe('proximaCor', () => {
	it('dá a primeira do rodízio ao quadro vazio', () => {
		expect(proximaCor([])).toBe('azul');
	});

	it('pula as cores que já estão em uso', () => {
		expect(proximaCor(['azul', 'verde'])).toBe('roxo');
	});

	it('ignora coluna sem cor', () => {
		expect(proximaCor(['', ''])).toBe('azul');
	});

	it('aproveita o buraco deixado por uma coluna apagada', () => {
		expect(proximaCor(['azul', 'roxo'])).toBe('verde');
	});

	it('recomeça quando a paleta acaba', () => {
		expect(proximaCor(CORES_AUTOMATICAS)).toBe('azul');
		expect(proximaCor([...CORES_AUTOMATICAS, 'azul'])).toBe('azul');
	});
});

describe('corDoCard', () => {
	it('herda a cor da coluna quando o card não tem a sua', () => {
		expect(corDoCard('', 'verde')).toBe('verde');
	});

	it('a cor do próprio card vence a da coluna', () => {
		expect(corDoCard('vermelho', 'verde')).toBe('vermelho');
	});

	it('fica sem cor quando nem card nem coluna têm', () => {
		expect(corDoCard('', '')).toBe('');
	});
});
