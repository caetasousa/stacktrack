// @vitest-environment jsdom

// Monta o componente de verdade para provar que a cor do card VIRA pixel. O
// teste existe porque essa ligação já se perdeu em silêncio uma vez: a cor
// atravessava domínio, banco e API corretamente, e a marcação que a pintava
// simplesmente não estava no arquivo. Nada acusava — nem o svelte-check, nem
// os testes de API, que só olham o JSON.
import { mount, unmount } from 'svelte';
import { afterEach, describe, expect, it } from 'vitest';
import CardDoQuadro from './CardDoQuadro.svelte';
import type { Card, Cor } from '$lib/api/boards';

let desmontar: (() => void) | null = null;

afterEach(() => {
	desmontar?.();
	desmontar = null;
	document.body.innerHTML = '';
});

function cardFalso(cor: Cor | ''): Card {
	return {
		id: 'card-1',
		colunaId: 'col-1',
		titulo: 'Migração',
		descricao: '',
		posicao: 1024,
		version: 1,
		prazo: null,
		vencido: false,
		cor,
		etiquetas: [],
		checklist: { concluidos: 0, total: 0 },
		qtdAnexos: 0
	};
}

function montar(cor: Cor | ''): HTMLElement {
	const alvo = document.createElement('div');
	document.body.appendChild(alvo);

	const componente = mount(CardDoQuadro, {
		target: alvo,
		props: {
			card: cardFalso(cor),
			etiquetasDoQuadro: [],
			podeEditar: true,
			aoAbrirCard: () => {},
			aoMudar: async () => {},
			aoFalhar: () => {}
		}
	});
	desmontar = () => unmount(componente);

	return alvo.querySelector('[role="button"]') as HTMLElement;
}

describe('CardDoQuadro', () => {
	it('pinta o card com a cor escolhida', () => {
		const elemento = montar('verde');

		expect(elemento.className).toContain('cor-verde');
		// A cor entra misturada na superfície, e não sobreposta: o card precisa
		// continuar opaco sobre uma coluna que também pode estar colorida.
		expect(elemento.getAttribute('style')).toContain('var(--etq-texto)');
		expect(elemento.getAttribute('style')).toContain('background-color');
	});

	it('não pinta nada quando o card está sem cor', () => {
		const elemento = montar('');

		expect(elemento.className).not.toContain('cor-');
		expect(elemento.className).toContain('bg-surface');
		expect(elemento.getAttribute('style')).toBeFalsy();
	});
});
