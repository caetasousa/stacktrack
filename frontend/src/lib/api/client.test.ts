import { describe, expect, it, vi, afterEach } from 'vitest';
import { ApiError, apiGet, apiPost, BASE_URL } from './client';

function respostaFalsa(corpo: unknown, init: { status?: number; json?: boolean } = {}) {
	const { status = 200, json = true } = init;
	return {
		ok: status >= 200 && status < 300,
		status,
		json: json ? async () => corpo : async () => Promise.reject(new Error('não é JSON'))
	} as Response;
}

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('apiGet', () => {
	it('devolve o corpo JSON quando a resposta é de sucesso', async () => {
		const fetchFalso = vi.fn().mockResolvedValue(respostaFalsa({ status: 'ok' }));
		vi.stubGlobal('fetch', fetchFalso);

		await expect(apiGet('/health')).resolves.toEqual({ status: 'ok' });
		expect(fetchFalso).toHaveBeenCalledWith(`${BASE_URL}/health`, { credentials: 'include' });
	});

	// A sessão da fase 1 vive num cookie, e o navegador não o envia para outra
	// origem sem credentials: 'include' — em dev, front (5173) e API (8080) são
	// origens diferentes. Sem isto, tudo responderia 401 sem explicação.
	it('sempre envia as credenciais', async () => {
		const fetchFalso = vi.fn().mockResolvedValue(respostaFalsa({}));
		vi.stubGlobal('fetch', fetchFalso);

		await apiGet('/ready');

		expect(fetchFalso.mock.calls[0][1]).toMatchObject({ credentials: 'include' });
	});

	it('lança ApiError com a mensagem do campo `erro` da API', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn().mockResolvedValue(respostaFalsa({ erro: 'banco indisponível' }, { status: 503 }))
		);

		await expect(apiGet('/ready')).rejects.toThrowError(
			expect.objectContaining({
				name: 'ApiError',
				status: 503,
				message: 'banco indisponível'
			})
		);
	});

	it('cai numa mensagem padrão quando o corpo do erro não é JSON', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn().mockResolvedValue(respostaFalsa(null, { status: 502, json: false }))
		);

		await expect(apiGet('/health')).rejects.toThrowError(new ApiError(502, 'erro 502'));
	});
});

describe('apiPost', () => {
	it('serializa o corpo como JSON e marca o content-type', async () => {
		const fetchFalso = vi.fn().mockResolvedValue(respostaFalsa({ id: '1' }));
		vi.stubGlobal('fetch', fetchFalso);

		await apiPost('/boards', { titulo: 'Estudos' });

		expect(fetchFalso).toHaveBeenCalledWith(`${BASE_URL}/boards`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			credentials: 'include',
			body: JSON.stringify({ titulo: 'Estudos' })
		});
	});
});
