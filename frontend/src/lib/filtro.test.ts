import { describe, expect, it } from 'vitest';
import { FILTRO_VAZIO, filtrando, passa, pessoasDoQuadro, type Filtro } from './filtro';
import type { Card } from '$lib/api/boards';

function card(ajustes: Partial<Card> = {}): Card {
	return {
		id: 'card-1',
		colunaId: 'col-1',
		titulo: 'Migração',
		descricao: '',
		posicao: 1024,
		version: 1,
		prazo: null,
		vencido: false,
		cor: '',
		responsaveis: [],
		etiquetas: [],
		checklist: { concluidos: 0, total: 0 },
		qtdAnexos: 0,
		...ajustes
	};
}

const comFiltro = (ajustes: Partial<Filtro>): Filtro => ({ ...FILTRO_VAZIO, ...ajustes });

const ana = { usuarioId: 'u-ana', nome: 'Ana' };
const bruno = { usuarioId: 'u-bruno', nome: 'Bruno' };

describe('filtrando', () => {
	it('é falso sem nenhum critério', () => {
		expect(filtrando(FILTRO_VAZIO)).toBe(false);
	});

	it('é verdadeiro com qualquer critério ligado', () => {
		expect(filtrando(comFiltro({ responsavelId: 'u-ana' }))).toBe(true);
		expect(filtrando(comFiltro({ etiquetaId: 'e-1' }))).toBe(true);
		expect(filtrando(comFiltro({ soVencidos: true }))).toBe(true);
	});
});

describe('passa', () => {
	it('deixa tudo passar sem filtro', () => {
		expect(passa(card(), FILTRO_VAZIO)).toBe(true);
	});

	it('filtra por responsável', () => {
		const meu = card({ responsaveis: [ana] });
		const alheio = card({ responsaveis: [bruno] });
		const semDono = card();

		const f = comFiltro({ responsavelId: ana.usuarioId });
		expect(passa(meu, f)).toBe(true);
		expect(passa(alheio, f)).toBe(false);
		expect(passa(semDono, f)).toBe(false);
	});

	it('encontra o card quando a pessoa é um de vários responsáveis', () => {
		const compartilhado = card({ responsaveis: [bruno, ana] });

		expect(passa(compartilhado, comFiltro({ responsavelId: ana.usuarioId }))).toBe(true);
	});

	it('filtra por etiqueta', () => {
		const comEtiqueta = card({ etiquetas: ['e-1', 'e-2'] });

		expect(passa(comEtiqueta, comFiltro({ etiquetaId: 'e-2' }))).toBe(true);
		expect(passa(comEtiqueta, comFiltro({ etiquetaId: 'e-9' }))).toBe(false);
		expect(passa(card(), comFiltro({ etiquetaId: 'e-1' }))).toBe(false);
	});

	it('filtra por vencido', () => {
		expect(passa(card({ vencido: true }), comFiltro({ soVencidos: true }))).toBe(true);
		expect(passa(card({ vencido: false }), comFiltro({ soVencidos: true }))).toBe(false);
	});

	// Os critérios se somam: dois filtros ligados querem os cards que são as
	// duas coisas, não a união deles.
	it('exige TODOS os critérios ativos ao mesmo tempo', () => {
		const f = comFiltro({ responsavelId: ana.usuarioId, etiquetaId: 'e-1' });

		expect(passa(card({ responsaveis: [ana], etiquetas: ['e-1'] }), f)).toBe(true);
		expect(passa(card({ responsaveis: [ana], etiquetas: ['e-2'] }), f)).toBe(false);
		expect(passa(card({ responsaveis: [bruno], etiquetas: ['e-1'] }), f)).toBe(false);
	});
});

describe('pessoasDoQuadro', () => {
	it('reúne quem aparece nos cards, sem repetir e em ordem de nome', () => {
		const colunas = [
			{ cards: [card({ responsaveis: [bruno] }), card({ responsaveis: [ana, bruno] })] },
			{ cards: [card({ responsaveis: [ana] })] }
		];

		expect(pessoasDoQuadro(colunas)).toEqual([ana, bruno]);
	});

	it('devolve lista vazia quando ninguém responde por nada', () => {
		expect(pessoasDoQuadro([{ cards: [card(), card()] }])).toEqual([]);
	});
});
