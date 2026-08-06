import { error } from '@sveltejs/kit';
import { exigirAutenticacao } from '$lib/auth-guard';
import { detalharBoard, type BoardDetalhado } from '$lib/api/boards';
import { ApiError } from '$lib/api/client';
import type { PageLoad } from './$types';

export const ssr = false;

export const load: PageLoad = async ({ params }): Promise<{ quadro: BoardDetalhado }> => {
	await exigirAutenticacao();

	try {
		return { quadro: await detalharBoard(params.id) };
	} catch (e) {
		// A API responde 404 tanto para quadro inexistente quanto para quadro de
		// outra pessoa, de propósito — e a tela repete essa indistinção em vez
		// de dizer "sem permissão", que confirmaria a existência do quadro.
		if (e instanceof ApiError && e.status === 404) {
			throw error(404, 'Quadro não encontrado');
		}
		throw e;
	}
};
