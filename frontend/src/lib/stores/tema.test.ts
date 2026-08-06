// @vitest-environment jsdom
import { describe, expect, it, beforeEach, vi, afterEach } from 'vitest';
import { ControleTema, CHAVE_TEMA, TEMA_PADRAO } from './tema.svelte';

beforeEach(() => {
	document.documentElement.removeAttribute('data-theme');
	localStorage.clear();
});

afterEach(() => {
	vi.restoreAllMocks();
});

describe('sincronizar', () => {
	// A store nunca decide o tema inicial: quem decidiu foi o static/tema.js,
	// antes da primeira pintura. Se ela decidisse de novo, poderia discordar
	// do que já está na tela.
	it('lê o tema que já está aplicado no documento', () => {
		document.documentElement.setAttribute('data-theme', 'light');
		const tema = new ControleTema();

		tema.sincronizar();

		expect(tema.atual).toBe('light');
	});

	it('cai no padrão quando o atributo está ausente ou é lixo', () => {
		const tema = new ControleTema();

		tema.sincronizar();
		expect(tema.atual).toBe(TEMA_PADRAO);

		document.documentElement.setAttribute('data-theme', 'roxo');
		tema.sincronizar();
		expect(tema.atual).toBe(TEMA_PADRAO);
	});
});

describe('definir', () => {
	it('aplica no documento e grava a preferência', () => {
		const tema = new ControleTema();

		tema.definir('light');

		expect(document.documentElement.getAttribute('data-theme')).toBe('light');
		expect(localStorage.getItem(CHAVE_TEMA)).toBe('light');
		expect(tema.persistido).toBe(true);
	});

	// Navegação privada e cookies bloqueados fazem o localStorage lançar. A
	// troca de tema precisa continuar valendo na sessão — perder o tema é
	// pior do que perder a memória dele.
	it('continua trocando o tema quando o localStorage falha', () => {
		vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
			throw new Error('acesso negado');
		});
		const tema = new ControleTema();

		expect(() => tema.definir('light')).not.toThrow();

		expect(document.documentElement.getAttribute('data-theme')).toBe('light');
		expect(tema.atual).toBe('light');
		expect(tema.persistido).toBe(false);
	});
});

describe('alternar', () => {
	it('vai e volta entre escuro e claro', () => {
		const tema = new ControleTema();
		tema.sincronizar();

		expect(tema.atual).toBe('dark');
		tema.alternar();
		expect(tema.atual).toBe('light');
		tema.alternar();
		expect(tema.atual).toBe('dark');
		expect(localStorage.getItem(CHAVE_TEMA)).toBe('dark');
	});
});

describe('padrão do projeto', () => {
	it('é o escuro', () => {
		expect(TEMA_PADRAO).toBe('dark');
	});
});
