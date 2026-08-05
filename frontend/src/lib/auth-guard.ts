// Guard de autenticação usado pelo load() das páginas autenticadas: consulta
// /auth/me, trata 401 (limpa a store e manda para o login) e popula a sessão.
import { redirect } from '@sveltejs/kit';
import { ApiError } from '$lib/api/client';
import { me, type SessaoResponse } from '$lib/api/auth';
import { sessao } from '$lib/stores/session.svelte';

export async function exigirAutenticacao(): Promise<SessaoResponse> {
	let usuario: SessaoResponse;
	try {
		usuario = await me();
	} catch (e) {
		if (e instanceof ApiError && e.status === 401) {
			sessao.limpar();
			throw redirect(302, '/login');
		}
		// Erro que não é 401 (API fora do ar, rede caída) sobe para a página de
		// erro: mandar para o login sugeriria que o problema é a credencial.
		throw e;
	}

	sessao.definir(usuario);
	return usuario;
}
