import { describe, expect, it, vi, afterEach } from 'vitest';
import { cadastrar, entrar, me, sair } from './auth';
import { BASE_URL } from './client';

function respostaFalsa(corpo: unknown, status = 200) {
	return {
		ok: status >= 200 && status < 300,
		status,
		json: async () => corpo
	} as Response;
}

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('cadastrar', () => {
	it('envia nome, email e senha e devolve a conta', async () => {
		const conta = { id: '1', nome: 'Ana', email: 'ana@exemplo.com' };
		const fetchFalso = vi.fn().mockResolvedValue(respostaFalsa(conta, 201));
		vi.stubGlobal('fetch', fetchFalso);

		await expect(
			cadastrar({ nome: 'Ana', email: 'ana@exemplo.com', senha: 'senha-boa-de-teste-123' })
		).resolves.toEqual(conta);
		expect(fetchFalso).toHaveBeenCalledWith(`${BASE_URL}/auth/cadastro`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			credentials: 'include',
			body: JSON.stringify({
				nome: 'Ana',
				email: 'ana@exemplo.com',
				senha: 'senha-boa-de-teste-123'
			})
		});
	});

	it('propaga a mensagem da API quando o email já existe', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn().mockResolvedValue(respostaFalsa({ erro: 'já existe uma conta com este email' }, 409))
		);

		await expect(
			cadastrar({ nome: 'Ana', email: 'ana@exemplo.com', senha: 'senha-boa-de-teste-123' })
		).rejects.toThrowError(
			expect.objectContaining({ status: 409, message: 'já existe uma conta com este email' })
		);
	});
});

describe('entrar', () => {
	it('chama /auth/login com as credenciais', async () => {
		const fetchFalso = vi
			.fn()
			.mockResolvedValue(respostaFalsa({ id: '1', nome: 'Ana', email: 'ana@exemplo.com' }));
		vi.stubGlobal('fetch', fetchFalso);

		await entrar({ email: 'ana@exemplo.com', senha: 'senha-boa-de-teste-123' });

		expect(fetchFalso.mock.calls[0][0]).toBe(`${BASE_URL}/auth/login`);
	});

	it('propaga o 401 de credenciais inválidas', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn().mockResolvedValue(respostaFalsa({ erro: 'credenciais inválidas' }, 401))
		);

		await expect(entrar({ email: 'ana@exemplo.com', senha: 'errada' })).rejects.toThrowError(
			expect.objectContaining({ status: 401 })
		);
	});
});

describe('sair', () => {
	// O logout responde 204: ler o corpo com json() quebraria, por isso ele usa
	// apiPostVazio e não apiPost.
	it('não tenta interpretar corpo na resposta 204', async () => {
		const fetchFalso = vi.fn().mockResolvedValue({
			ok: true,
			status: 204,
			json: async () => {
				throw new Error('não deveria ler o corpo de um 204');
			}
		} as unknown as Response);
		vi.stubGlobal('fetch', fetchFalso);

		await expect(sair()).resolves.toBeUndefined();
		expect(fetchFalso).toHaveBeenCalledWith(`${BASE_URL}/auth/logout`, {
			method: 'POST',
			credentials: 'include'
		});
	});
});

describe('me', () => {
	it('consulta /auth/me enviando o cookie de sessão', async () => {
		const fetchFalso = vi
			.fn()
			.mockResolvedValue(respostaFalsa({ id: '1', nome: 'Ana', email: 'ana@exemplo.com' }));
		vi.stubGlobal('fetch', fetchFalso);

		await me();

		expect(fetchFalso).toHaveBeenCalledWith(`${BASE_URL}/auth/me`, { credentials: 'include' });
	});
});
