import { confirmarEmail } from '$lib/api/auth';
import { ApiError } from '$lib/api/client';
import type { PageLoad } from './$types';

export const ssr = false;

// Sem guard de autenticação: o link prova a caixa de entrada, e quem o abre pode
// estar noutro aparelho, sem sessão nenhuma. Confirmar é o próprio carregamento
// da página — não há o que a pessoa decidir antes.
export const load: PageLoad = async ({ params }): Promise<{ confirmado: boolean }> => {
	try {
		await confirmarEmail(params.token);
		return { confirmado: true };
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) return { confirmado: false };
		throw e;
	}
};
