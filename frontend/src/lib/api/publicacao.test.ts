import { afterEach, describe, expect, it, vi } from 'vitest';
import { despublicar, obterPublicacao, publicar, verQuadroPublico } from './publicacao';
import { ApiError, BASE_URL } from './client';

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

describe('verQuadroPublico', () => {
	// O teste que mais importa deste arquivo. A rota não olha sessão nenhuma, e
	// mandar o cookie de quem por acaso está logado exporia a credencial dele
	// numa requisição que não precisa dela — ainda mais numa página aberta a
	// partir de um link recebido de terceiros.
	it('não manda o cookie de sessão', async () => {
		const fetchFalso = vi.fn().mockResolvedValue(respostaFalsa({ titulo: 'Roadmap', colunas: [] }));
		vi.stubGlobal('fetch', fetchFalso);

		await verQuadroPublico('um-token');

		expect(fetchFalso).toHaveBeenCalledWith(`${BASE_URL}/publico/um-token`, {
			credentials: 'omit'
		});
	});

	// Sem isto, um token com barra ou interrogação montaria outra rota — e a
	// página responderia 404 por um defeito de montagem de URL, não por o link
	// ser inválido.
	it('escapa o token na URL', async () => {
		const fetchFalso = vi.fn().mockResolvedValue(respostaFalsa({ titulo: '', colunas: [] }));
		vi.stubGlobal('fetch', fetchFalso);

		await verQuadroPublico('a/b?c');

		expect(fetchFalso.mock.calls[0][0]).toBe(`${BASE_URL}/publico/a%2Fb%3Fc`);
	});

	it('vira ApiError 404 quando o link não vale mais', async () => {
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue(respostaFalsa({ erro: 'x' }, 404)));

		await expect(verQuadroPublico('revogado')).rejects.toMatchObject({
			name: 'ApiError',
			status: 404
		});
	});
});

describe('publicar', () => {
	// PUT, e não POST: repetir devolve o MESMO link. Um POST prometeria criar um
	// segundo, e a tela do dono chama isto toda vez que o painel abre com o
	// botão de ativar.
	it('usa PUT e viaja com a sessão', async () => {
		const fetchFalso = vi
			.fn()
			.mockResolvedValue(respostaFalsa({ publicado: true, url: 'http://x/publico/t' }));
		vi.stubGlobal('fetch', fetchFalso);

		await publicar('quadro-1');

		expect(fetchFalso).toHaveBeenCalledWith(`${BASE_URL}/boards/quadro-1/publicacao`, {
			method: 'PUT',
			credentials: 'include'
		});
	});

	it('propaga a recusa como ApiError', async () => {
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue(respostaFalsa({}, 403)));

		await expect(publicar('quadro-1')).rejects.toBeInstanceOf(ApiError);
	});
});

describe('obterPublicacao', () => {
	it('lê o estado do link do quadro', async () => {
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue(respostaFalsa({ publicado: false })));

		await expect(obterPublicacao('quadro-1')).resolves.toEqual({ publicado: false });
	});
});

describe('despublicar', () => {
	it('chama DELETE na publicação do quadro', async () => {
		const fetchFalso = vi.fn().mockResolvedValue({ ok: true, status: 204 } as Response);
		vi.stubGlobal('fetch', fetchFalso);

		await despublicar('quadro-1');

		expect(fetchFalso).toHaveBeenCalledWith(`${BASE_URL}/boards/quadro-1/publicacao`, {
			method: 'DELETE',
			credentials: 'include'
		});
	});
});
