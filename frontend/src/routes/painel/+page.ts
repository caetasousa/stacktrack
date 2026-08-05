import { exigirAutenticacao } from '$lib/auth-guard';
import type { SessaoResponse } from '$lib/api/auth';

// O cookie de sessão é HttpOnly e a API vive em outra origem, então o SSR
// nunca teria acesso a ele — a checagem de autenticação só pode rodar no
// browser.
export const ssr = false;

export async function load(): Promise<{ usuario: SessaoResponse }> {
	return { usuario: await exigirAutenticacao() };
}
