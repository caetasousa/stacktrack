import { error } from '@sveltejs/kit';
import { exigirAutenticacao } from '$lib/auth-guard';
import { detalharBoard, type BoardDetalhado } from '$lib/api/boards';
import { listarParticipacao, type Participacao } from '$lib/api/membros';
import { ApiError } from '$lib/api/client';
import type { PageLoad } from './$types';

export const ssr = false;

export const load: PageLoad = async ({
	params
}): Promise<{ quadro: BoardDetalhado; participacao: Participacao; usuarioId: string }> => {
	const usuario = await exigirAutenticacao();

	try {
		// As duas em paralelo: nenhuma depende da outra, e a autenticação já
		// passou antes — o guard sozinho decide o redirecionamento.
		const [quadro, participacao] = await Promise.all([
			detalharBoard(params.id),
			listarParticipacao(params.id)
		]);
		return { quadro, participacao, usuarioId: usuario.id };
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) {
			throw error(404, 'Quadro não encontrado');
		}
		throw e;
	}
};
