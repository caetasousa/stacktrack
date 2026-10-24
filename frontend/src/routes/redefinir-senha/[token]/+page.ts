import { consultarRecuperacao, type RecuperacaoAberta } from '$lib/api/auth';
import { ApiError } from '$lib/api/client';
import type { PageLoad } from './$types';

export const ssr = false;

// Sem guard de autenticação: quem chega aqui esqueceu a senha. O link inválido
// não vira página de erro — vira a mesma tela, com o caminho para pedir outro.
export const load: PageLoad = async ({
	params
}): Promise<{ token: string; recuperacao: RecuperacaoAberta | null }> => {
	try {
		return { token: params.token, recuperacao: await consultarRecuperacao(params.token) };
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) {
			return { token: params.token, recuperacao: null };
		}
		throw e;
	}
};
