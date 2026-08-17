// @vitest-environment jsdom

// Monta o componente de verdade para provar que a cor do card VIRA pixel. O
// teste existe porque essa ligação já se perdeu em silêncio uma vez: a cor
// atravessava domínio, banco e API corretamente, e a marcação que a pintava
// simplesmente não estava no arquivo. Nada acusava — nem o svelte-check, nem
// os testes de API, que só olham o JSON.
import { mount, unmount } from 'svelte';
import { afterEach, describe, expect, it } from 'vitest';
import CardDoQuadro from './CardDoQuadro.svelte';
import type { Card, Cor, Etiqueta, Responsavel } from '$lib/api/boards';

let desmontar: (() => void) | null = null;

afterEach(() => {
	desmontar?.();
	desmontar = null;
	document.body.innerHTML = '';
});

function cardFalso(ajustes: Partial<Card> = {}): Card {
	return {
		id: 'card-1',
		colunaId: 'col-1',
		titulo: 'Migração',
		descricao: '',
		version: 1,
		prazo: null,
		vencido: false,
		cor: '',
		responsaveis: [],
		etiquetas: [],
		checklist: { concluidos: 0, total: 0 },
		qtdAnexos: 0,
		qtdComentarios: 0,
		ultimaMovimentacao: null,
		...ajustes
	};
}

function montar(card: Card, etiquetasDoQuadro: Etiqueta[] = []): HTMLElement {
	const alvo = document.createElement('div');
	document.body.appendChild(alvo);

	const componente = mount(CardDoQuadro, {
		target: alvo,
		props: {
			card,
			etiquetasDoQuadro,
			podeEditar: true,
			aoAbrirCard: () => {},
			aoMudar: async () => {},
			aoFalhar: () => {}
		}
	});
	desmontar = () => unmount(componente);

	return alvo.querySelector('[role="button"]') as HTMLElement;
}

function comCor(cor: Cor | ''): HTMLElement {
	return montar(cardFalso({ cor }));
}

const urgente: Etiqueta = { id: 'e-1', nome: 'Urgente', cor: 'vermelho', posicao: 1024 };

describe('CardDoQuadro', () => {
	it('pinta o card com a cor escolhida', () => {
		const elemento = comCor('verde');

		expect(elemento.className).toContain('cor-verde');
		// A cor entra misturada na superfície, e não sobreposta: o card precisa
		// continuar opaco sobre uma coluna que também pode estar colorida.
		expect(elemento.getAttribute('style')).toContain('var(--etq-texto)');
		expect(elemento.getAttribute('style')).toContain('background-color');
	});

	it('não pinta nada quando o card está sem cor', () => {
		const elemento = comCor('');

		expect(elemento.className).not.toContain('cor-');
		expect(elemento.className).toContain('bg-surface');
		expect(elemento.getAttribute('style')).toBeFalsy();
	});

	// A etiqueta mostra o NOME, e não só a cor. A cor sozinha exigia decorar a
	// convenção do quadro, e não dizia nada para quem não a distingue.
	it('mostra o nome da etiqueta, e não só a cor', () => {
		const elemento = montar(cardFalso({ etiquetas: [urgente.id] }), [urgente]);

		const selo = elemento.querySelector('.etiqueta-selo');
		expect(selo).not.toBeNull();
		expect(selo?.textContent?.trim()).toBe('Urgente');
		// A cor continua ali, agora como reforço do nome.
		expect(selo?.className).toContain('cor-vermelho');
	});

	it('desenha um avatar por responsável, com as iniciais', () => {
		const responsaveis: Responsavel[] = [
			{ usuarioId: 'u-1', nome: 'Ana Souza' },
			{ usuarioId: 'u-2', nome: 'Bruno' }
		];
		const elemento = montar(cardFalso({ responsaveis }));

		const avatares = elemento.querySelectorAll('[data-responsavel]');
		expect(avatares).toHaveLength(2);
		expect(avatares[0].textContent?.trim()).toBe('AS');
		expect(avatares[1].textContent?.trim()).toBe('BR');
	});

	it('não desenha avatar nenhum quando o card não tem responsável', () => {
		const elemento = montar(cardFalso());

		expect(elemento.querySelectorAll('[data-responsavel]')).toHaveLength(0);
	});
	// O selo de auditoria. A lição do botão de arquivar vale aqui: um seletor
	// enxerga o que o olho não vê, então o teste confere o TEXTO renderizado —
	// o nome de quem moveu tem de estar legível no card, não só num atributo.
	it('mostra quem moveu o card por último', () => {
		const elemento = montar(
			cardFalso({
				ultimaMovimentacao: {
					autorId: 'u-9',
					autorNome: 'Roberto Silva',
					de: 'A fazer',
					para: 'Pronto',
					ocorridoEm: new Date().toISOString()
				}
			})
		);

		expect(elemento.textContent).toContain('Roberto Silva');
		expect(elemento.textContent).toContain('agora');
	});

	// O trajeto e a data exata não cabem no card, mas quem audita precisa deles
	// sem sair da tela.
	it('guarda o trajeto e a data completos no title', () => {
		const elemento = montar(
			cardFalso({
				ultimaMovimentacao: {
					autorId: 'u-9',
					autorNome: 'Roberto Silva',
					de: 'A fazer',
					para: 'Pronto',
					ocorridoEm: '2026-08-17T12:00:00Z'
				}
			})
		);

		const linha = [...elemento.querySelectorAll('[title]')].find((e) =>
			e.getAttribute('title')?.includes('Roberto Silva')
		);
		expect(linha?.getAttribute('title')).toContain('moveu de A fazer para Pronto');
	});

	// Card recém-criado não ganha linha nenhuma: ausência é a informação certa.
	// Inventar "movido por quem criou" transformaria auditoria em ficção.
	it('não mostra selo de movimentação no card que nunca foi movido', () => {
		const elemento = montar(cardFalso());

		expect(elemento.textContent).not.toContain('agora');
		expect(
			[...elemento.querySelectorAll('[title]')].some((e) =>
				e.getAttribute('title')?.includes('moveu')
			)
		).toBe(false);
	});
});
