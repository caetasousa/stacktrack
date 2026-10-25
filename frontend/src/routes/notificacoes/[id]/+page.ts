// A página que o link de um email de notificação abre. Confere, na hora do
// clique, se o aviso ainda vale: o email pode ter saído antes de a atribuição
// ser desfeita, e o link precisa dizer isso em vez de abrir o card como se nada
// tivesse mudado.
import { redirect } from '@sveltejs/kit';
import { me } from '$lib/api/auth';
import { ApiError } from '$lib/api/client';
import { detalharNotificacao, type Notificacao } from '$lib/api/notificacoes';
import { sessao } from '$lib/stores/session.svelte';
import type { PageLoad } from './$types';

export const ssr = false;

export const load: PageLoad = async ({
	params,
	url
}): Promise<{ notificacao: Notificacao | null }> => {
	// Quem abre o email sem estar logado entra e volta para cá — e não para o
	// painel, onde não saberia o que o email queria mostrar.
	try {
		sessao.definir(await me());
	} catch (e) {
		if (e instanceof ApiError && e.status === 401) {
			sessao.limpar();
			throw redirect(302, `/login?voltar=${encodeURIComponent(url.pathname)}`);
		}
		throw e;
	}

	try {
		return { notificacao: await detalharNotificacao(params.id) };
	} catch (e) {
		// 404 é o aviso que não existe mais (o card apagado, a menção retirada),
		// o de outra conta, e o de um quadro de que a pessoa saiu — os três
		// iguais, de propósito. 400 é um id que nem é id.
		if (e instanceof ApiError && (e.status === 404 || e.status === 400)) {
			return { notificacao: null };
		}
		throw e;
	}
};
