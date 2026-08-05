import { describe, expect, it, vi, beforeEach } from 'vitest';
import { Sessao } from './session.svelte';
import { ApiError } from '$lib/api/client';

const conta = { id: '1', nome: 'Ana', email: 'ana@exemplo.com' };

const api = vi.hoisted(() => ({
	me: vi.fn(),
	sair: vi.fn()
}));

vi.mock('$lib/api/auth', () => ({
	me: api.me,
	sair: api.sair
}));

beforeEach(() => {
	api.me.mockReset();
	api.sair.mockReset();
});

describe('carregar', () => {
	it('popula o usuário quando /auth/me responde', async () => {
		api.me.mockResolvedValue(conta);
		const sessao = new Sessao();

		await sessao.carregar();

		expect(sessao.usuario).toEqual(conta);
		expect(sessao.carregando).toBe(false);
	});

	// Qualquer falha vira "deslogado", inclusive rede fora: o header aparece em
	// toda página e não pode quebrar porque a API caiu.
	it('trata 401 e falha de rede como deslogado', async () => {
		for (const falha of [new ApiError(401, 'não autenticado'), new Error('rede fora')]) {
			api.me.mockRejectedValueOnce(falha);
			const sessao = new Sessao();

			await sessao.carregar();

			expect(sessao.usuario).toBeNull();
			expect(sessao.carregando).toBe(false);
		}
	});

	it('não repete a chamada quando a store já foi populada pelo guard', async () => {
		const sessao = new Sessao();
		sessao.definir(conta);

		await sessao.carregar();

		expect(api.me).not.toHaveBeenCalled();
	});
});

describe('encerrar', () => {
	it('limpa o estado local depois de sair na API', async () => {
		api.sair.mockResolvedValue(undefined);
		const sessao = new Sessao();
		sessao.definir(conta);

		await sessao.encerrar();

		expect(api.sair).toHaveBeenCalled();
		expect(sessao.usuario).toBeNull();
	});

	// Manter a tela dizendo "logado" depois do clique em sair é pior do que
	// perder a confirmação do servidor.
	it('limpa o estado local mesmo se a API falhar', async () => {
		api.sair.mockRejectedValue(new Error('rede fora'));
		const sessao = new Sessao();
		sessao.definir(conta);

		await expect(sessao.encerrar()).rejects.toThrow();

		expect(sessao.usuario).toBeNull();
	});
});
