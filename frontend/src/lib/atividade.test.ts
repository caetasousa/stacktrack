import { describe, expect, it } from 'vitest';
import { descritas, frase, quando, type Atividade } from './atividade';

function entrada(ajustes: Partial<Atividade> = {}): Atividade {
	return {
		seq: 1,
		tipo: 'card.criado',
		autorId: 'u-ana',
		autorNome: 'Ana',
		ocorridoEm: '2026-08-08T19:00:00Z',
		...ajustes
	};
}

describe('frase', () => {
	it('diz de qual coluna para qual no movimento', () => {
		const a = entrada({ tipo: 'card.movido', dados: { deColuna: 'A fazer', coluna: 'Pronto' } });

		expect(frase(a)).toBe('moveu de A fazer para Pronto');
	});

	// Mover dentro da MESMA coluna é reordenar, não mudar de etapa. Dizer "moveu
	// de A fazer para A fazer" seria ruído.
	it('chama de reordenação o movimento dentro da mesma coluna', () => {
		const a = entrada({ tipo: 'card.movido', dados: { deColuna: 'A fazer', coluna: 'A fazer' } });

		expect(frase(a)).toBe('reordenou em A fazer');
	});

	// Eventos gravados antes de o payload carregar a origem não têm `deColuna`.
	// A frase encolhe em vez de inventar de onde o card veio.
	it('encolhe a frase quando o evento antigo não sabe a origem', () => {
		const a = entrada({ tipo: 'card.movido', dados: { coluna: 'Pronto' } });

		expect(frase(a)).toBe('reordenou em Pronto');
		expect(frase(entrada({ tipo: 'card.movido', dados: {} }))).toBe('moveu este card');
	});

	it('diz o nome anterior ao renomear', () => {
		const a = entrada({ tipo: 'card.alterado', dados: { tituloAnterior: 'Nome velho' } });

		expect(frase(a)).toBe('renomeou de "Nome velho"');
	});

	// Editar sem trocar o título não é renomear.
	it('não alega renomeação quando só a descrição mudou', () => {
		expect(frase(entrada({ tipo: 'card.alterado', dados: {} }))).toBe('editou este card');
	});

	it('guarda o nome do card apagado', () => {
		const a = entrada({ tipo: 'card.apagado', dados: { titulo: 'Some daqui' } });

		expect(frase(a)).toBe('apagou "Some daqui"');
	});

	it('descreve a criação com a coluna', () => {
		expect(frase(entrada({ tipo: 'card.criado', dados: { coluna: 'A fazer' } }))).toBe(
			'criou este card em A fazer'
		);
		expect(frase(entrada({ tipo: 'card.criado', dados: {} }))).toBe('criou este card');
	});

	// Um tipo de evento que a tela ainda não sabe descrever vira silêncio, e não
	// uma linha com "undefined" no meio do histórico.
	it('devolve vazio para o que não sabe descrever', () => {
		expect(frase(entrada({ tipo: 'inventado.agora' }))).toBe('');
	});
});

describe('descritas', () => {
	it('omite as entradas que não sabe descrever', () => {
		const lista = [
			entrada({ seq: 3, tipo: 'card.criado', dados: { coluna: 'A fazer' } }),
			entrada({ seq: 2, tipo: 'inventado.agora' }),
			entrada({ seq: 1, tipo: 'comentario.alterado' })
		];

		const fora = descritas(lista);
		expect(fora).toHaveLength(2);
		expect(fora.map((l) => l.atividade.seq)).toEqual([3, 1]);
	});
});

describe('quando', () => {
	it('devolve vazio para data ilegível, em vez de "Invalid Date"', () => {
		expect(quando('não é data')).toBe('');
	});

	it('formata uma data válida', () => {
		expect(quando('2026-08-08T19:00:00Z')).not.toBe('');
	});
});
