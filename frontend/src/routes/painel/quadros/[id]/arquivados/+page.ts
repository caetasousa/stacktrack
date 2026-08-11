import { error } from '@sveltejs/kit';
import { exigirAutenticacao } from '$lib/auth-guard';
import { arquivados, detalharBoard, type Arquivados, type BoardDetalhado } from '$lib/api/boards';
import { ApiError } from '$lib/api/client';
import type { PageLoad } from './$types';

export const ssr = false;

export const load: PageLoad = async ({
	params
}): Promise<{ quadro: BoardDetalhado; arquivo: Arquivados }> => {
	await exigirAutenticacao();

	try {
		// As duas em paralelo: nenhuma depende da outra. O quadro vem junto pelo
		// título e pelo papel — desarquivar exige papel de edição, e o botão não
		// pode aparecer para quem só lê.
		const [quadro, arquivo] = await Promise.all([detalharBoard(params.id), arquivados(params.id)]);
		return { quadro, arquivo };
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) {
			throw error(404, 'Quadro não encontrado');
		}
		throw e;
	}
};
