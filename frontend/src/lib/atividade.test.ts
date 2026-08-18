import { describe, expect, it } from 'vitest';
import { descritas, frase, fraseNoQuadro, haQuanto, quando, type Atividade } from './atividade';

function entrada(ajustes: Partial<Atividade> = {}): Atividade {
	return {
		seq: 1,
		tipo: 'card.criado',
		autorId: 'u-ana',
		autorNome: 'Ana',
		autorEmail: 'ana@exemplo.com',
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

// A frase da AUDITORIA do quadro precisa dizer em qual card a coisa aconteceu.
// No histórico de um card o contexto é a tela; numa lista que mistura o quadro
// inteiro, cinquenta linhas dizendo "moveu de A para B" não auditam nada.
describe('fraseNoQuadro', () => {
	it('nomeia o card no movimento', () => {
		const a = entrada({
			tipo: 'card.movido',
			dados: { titulo: 'Migração', deColuna: 'A fazer', coluna: 'Pronto' }
		});

		expect(fraseNoQuadro(a)).toBe('moveu "Migração" de A fazer para Pronto');
	});

	it('chama de reordenação o movimento dentro da mesma coluna', () => {
		const a = entrada({
			tipo: 'card.movido',
			dados: { titulo: 'Migração', deColuna: 'A fazer', coluna: 'A fazer' }
		});

		expect(fraseNoQuadro(a)).toBe('reordenou "Migração" em A fazer');
	});

	// O título vem do payload — o que o card tinha NA HORA. Quando falta, a
	// frase encolhe em vez de mentir ou de sair pela metade.
	it('encolhe quando o payload não traz o título', () => {
		const a = entrada({ tipo: 'card.movido', dados: { deColuna: 'A fazer', coluna: 'Pronto' } });

		expect(fraseNoQuadro(a)).toBe('moveu um card de A fazer para Pronto');
	});

	// A auditoria descreve o quadro INTEIRO, e não só os cards: coluna, etiqueta,
	// anexo, checklist, responsável, participação. Foi o pedido que originou a
	// separação dos tipos no servidor — antes, doze ações diferentes chegavam
	// aqui como um "quadro.alterado" sem payload, e a tela as descartava.
	it.each([
		['coluna.criada', { titulo: 'A fazer' }, 'criou a coluna "A fazer"'],
		['coluna.apagada', { titulo: 'A fazer' }, 'apagou a coluna "A fazer"'],
		['coluna.movida', { titulo: 'A fazer' }, 'reordenou a coluna "A fazer"'],
		[
			'quadro.renomeado',
			{ titulo: 'Novo', tituloAnterior: 'Velho' },
			'renomeou o quadro de "Velho" para "Novo"'
		],
		['quadro.fundo', { fundo: 'oceano' }, 'mudou o fundo do quadro para oceano'],
		['etiqueta.criada', { nome: 'urgente' }, 'criou a etiqueta "urgente"'],
		['etiqueta.aplicada', { titulo: 'Card', alvo: 'urgente' }, 'marcou "Card" com "urgente"'],
		['anexo.adicionado', { titulo: 'Card', alvo: 'nota.pdf' }, 'anexou "nota.pdf" em "Card"'],
		[
			'responsavel.atribuido',
			{ titulo: 'Card', alvo: 'Ana' },
			'pôs "Ana" como responsável por "Card"'
		],
		['item.criado', { titulo: 'Card', alvo: 'passo 1' }, 'acrescentou "passo 1" a "Card"'],
		['membro.adicionado', { nome: 'Ana', papel: 'editor' }, 'adicionou Ana ao quadro como editor'],
		['membro.entrou', { papel: 'editor' }, 'entrou no quadro como editor'],
		[
			'membro.papel',
			{ nome: 'Ana', papel: 'leitor', papelAnterior: 'editor' },
			'mudou Ana de editor para leitor'
		],
		['membro.removido', { nome: 'Ana' }, 'removeu Ana do quadro']
	])('descreve %s', (tipo, dados, esperado) => {
		expect(fraseNoQuadro(entrada({ tipo, dados }))).toBe(esperado);
	});

	// Os tipos genéricos de ANTES da separação continuam no log — append-only —,
	// e some-los seria pior do que uma frase vaga: a auditoria mostraria um
	// buraco onde houve atividade.
	it('ainda descreve, mesmo que vagamente, os eventos antigos', () => {
		expect(fraseNoQuadro(entrada({ tipo: 'quadro.alterado' }))).toBe('mexeu no quadro');
		expect(fraseNoQuadro(entrada({ tipo: 'membros.alterados' }))).toBe(
			'mexeu na participação do quadro'
		);
	});

	// Silêncio, e não uma frase torta: a tela omite a linha inteira.
	it('devolve vazio para um tipo que ainda não existe', () => {
		expect(fraseNoQuadro(entrada({ tipo: 'inventado.agora' }))).toBe('');
	});
});

describe('haQuanto', () => {
	const agora = new Date('2026-08-17T12:00:00Z');

	it.each([
		['2026-08-17T11:59:30Z', 'agora'],
		['2026-08-17T11:45:00Z', 'há 15 min'],
		['2026-08-17T09:00:00Z', 'há 3 h'],
		['2026-08-15T12:00:00Z', 'há 2 d']
	])('%s vira %s', (iso, esperado) => {
		expect(haQuanto(iso, agora)).toBe(esperado);
	});

	// Acima de uma semana volta a ser data: "há 43 d" não diz nada a ninguém.
	it('volta a mostrar a data depois de uma semana', () => {
		expect(haQuanto('2026-07-01T12:00:00Z', agora)).toBe(quando('2026-07-01T12:00:00Z'));
	});

	// O relógio do cliente pode estar adiantado em relação ao servidor. "em 3 s"
	// seria absurdo num histórico.
	it('não mostra futuro quando o relógio do cliente está adiantado', () => {
		expect(haQuanto('2026-08-17T12:00:30Z', agora)).toBe('agora');
	});

	it('devolve vazio para data ilegível', () => {
		expect(haQuanto('nem data é', agora)).toBe('');
	});
});
