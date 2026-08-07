import { error } from '@sveltejs/kit';
import { detalharConvite, type ConviteDetalhe } from '$lib/api/membros';
import { me, type SessaoResponse } from '$lib/api/auth';
import { ApiError } from '$lib/api/client';
import { sessao } from '$lib/stores/session.svelte';
import type { PageLoad } from './$types';

export const ssr = false;

// Esta página NÃO usa o guard de autenticação: quem foi convidado costuma ainda
// não ter conta, e precisa ver de que quadro se trata antes de criar uma. Ela
// só pergunta quem está logado, para saber o que oferecer.
export const load: PageLoad = async ({
	params
}): Promise<{ token: string; convite: ConviteDetalhe; usuario: SessaoResponse | null }> => {
	let convite: ConviteDetalhe;
	try {
		convite = await detalharConvite(params.token);
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) {
			throw error(404, 'Convite inválido ou expirado');
		}
		throw e;
	}

	let usuario: SessaoResponse | null = null;
	try {
		usuario = await me();
		sessao.definir(usuario);
	} catch {
		// Sem sessão é o caso normal aqui, não um erro.
	}

	return { token: params.token, convite, usuario };
};
