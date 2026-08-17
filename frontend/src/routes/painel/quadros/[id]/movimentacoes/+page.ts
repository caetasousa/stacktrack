import { error } from '@sveltejs/kit';
import { exigirAutenticacao } from '$lib/auth-guard';
import { detalharBoard, type BoardDetalhado } from '$lib/api/boards';
import { auditoriaDoQuadro } from '$lib/api/extras';
import { ApiError } from '$lib/api/client';
import type { Atividade } from '$lib/atividade';
import type { PageLoad } from './$types';

export const ssr = false;

// O quadro vem junto da auditoria, e não só o título: a tela precisa da lista
// de PESSOAS para montar o filtro, e quem participa do quadro se descobre pelos
// responsáveis e pelos membros — não pelo log, que só conhece quem já agiu.
//
// Uma pessoa que entrou no quadro e ainda não mexeu em nada não tem por que
// aparecer no filtro; uma que mexeu e depois saiu, tem — e essa vem do próprio
// log. Por isso as duas fontes.
export const load: PageLoad = async ({
	params
}): Promise<{ quadro: BoardDetalhado; atividade: Atividade[]; temMais: boolean }> => {
	await exigirAutenticacao();

	try {
		const [quadro, auditoria] = await Promise.all([
			detalharBoard(params.id),
			auditoriaDoQuadro(params.id)
		]);
		return { quadro, atividade: auditoria.atividade, temMais: auditoria.temMais ?? false };
	} catch (e) {
		// 404 tanto para quadro inexistente quanto para quadro de outra pessoa,
		// de propósito — a tela repete a indistinção da API.
		if (e instanceof ApiError && e.status === 404) {
			throw error(404, 'Quadro não encontrado');
		}
		throw e;
	}
};
