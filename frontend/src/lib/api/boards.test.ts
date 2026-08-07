import { describe, expect, it, vi, afterEach } from 'vitest';
import {
	apagarCard,
	criarBoard,
	criarCard,
	detalharBoard,
	listarBoards,
	renomearColuna
} from './boards';
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

describe('listarBoards', () => {
	// A API envelopa a lista num objeto para caber paginação depois; o cliente
	// desembrulha para as telas não conhecerem esse detalhe.
	it('desembrulha o envelope { boards }', async () => {
		const boards = [
			{ id: '1', titulo: 'Estudos', papel: 'dono', criadoEm: '2026-01-01T00:00:00Z' }
		];
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue(respostaFalsa({ boards })));

		await expect(listarBoards()).resolves.toEqual(boards);
	});
});

describe('criarBoard', () => {
	it('envia o título e devolve o quadro criado', async () => {
		const fetchFalso = vi
			.fn()
			.mockResolvedValue(
				respostaFalsa({ id: '1', titulo: 'Estudos', papel: 'dono', criadoEm: '' }, 201)
			);
		vi.stubGlobal('fetch', fetchFalso);

		await criarBoard('Estudos');

		expect(fetchFalso).toHaveBeenCalledWith(`${BASE_URL}/boards`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			credentials: 'include',
			body: JSON.stringify({ titulo: 'Estudos' })
		});
	});
});

describe('detalharBoard', () => {
	it('propaga o 404 de quadro que não é seu', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn().mockResolvedValue(respostaFalsa({ erro: 'quadro não encontrado' }, 404))
		);

		await expect(detalharBoard('de-outro')).rejects.toThrowError(
			expect.objectContaining({ status: 404, message: 'quadro não encontrado' })
		);
	});
});

describe('criarCard', () => {
	// Descrição e cor são opcionais na tela, mas o corpo precisa levá-las
	// sempre: sem o campo, o JSON omitiria a string vazia e a API receberia
	// undefined.
	it('manda descrição e cor vazias quando não informadas', async () => {
		const fetchFalso = vi.fn().mockResolvedValue(respostaFalsa({}, 201));
		vi.stubGlobal('fetch', fetchFalso);

		await criarCard('col-1', 'Tarefa');

		expect(fetchFalso.mock.calls[0][1].body).toBe(
			JSON.stringify({ titulo: 'Tarefa', descricao: '', cor: '' })
		);
	});

	it('manda a cor escolhida', async () => {
		const fetchFalso = vi.fn().mockResolvedValue(respostaFalsa({}, 201));
		vi.stubGlobal('fetch', fetchFalso);

		await criarCard('col-1', 'Tarefa', '', 'verde');

		expect(fetchFalso.mock.calls[0][1].body).toBe(
			JSON.stringify({ titulo: 'Tarefa', descricao: '', cor: 'verde' })
		);
	});
});

describe('renomearColuna', () => {
	it('usa PATCH na rota da coluna', async () => {
		const fetchFalso = vi.fn().mockResolvedValue(respostaFalsa({}));
		vi.stubGlobal('fetch', fetchFalso);

		await renomearColuna('col-1', 'Fazendo');

		expect(fetchFalso.mock.calls[0][0]).toBe(`${BASE_URL}/colunas/col-1`);
		expect(fetchFalso.mock.calls[0][1].method).toBe('PATCH');
	});
});

describe('apagarCard', () => {
	it('não tenta ler corpo na resposta 204', async () => {
		const fetchFalso = vi.fn().mockResolvedValue({
			ok: true,
			status: 204,
			json: async () => {
				throw new Error('não deveria ler o corpo de um 204');
			}
		} as unknown as Response);
		vi.stubGlobal('fetch', fetchFalso);

		await expect(apagarCard('card-1')).resolves.toBeUndefined();
		expect(fetchFalso.mock.calls[0][1].method).toBe('DELETE');
	});
});
