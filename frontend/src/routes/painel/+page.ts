import { exigirAutenticacao } from '$lib/auth-guard';
import { listarBoards, type Board } from '$lib/api/boards';
import { listarRecebidos, type ConviteRecebido } from '$lib/api/membros';
import type { SessaoResponse } from '$lib/api/auth';

// O cookie de sessão é HttpOnly e a API vive em outra origem, então o SSR
// nunca teria acesso a ele — a checagem de autenticação só pode rodar no
// browser.
export const ssr = false;

export async function load(): Promise<{
	usuario: SessaoResponse;
	boards: Board[];
	recebidos: ConviteRecebido[];
}> {
	// A autenticação vem primeiro e sozinha: listar em paralelo faria a
	// listagem falhar com 401 antes de o guard redirecionar, e o erro apareceria
	// na tela em vez do login.
	const usuario = await exigirAutenticacao();
	// Os convites só existem no painel de quem confirmou o email: sem isso a API
	// responde 403, e perguntar seria uma requisição para ouvir não.
	const [boards, recebidos] = await Promise.all([
		listarBoards(),
		usuario.emailConfirmado ? listarRecebidos() : Promise.resolve([])
	]);
	return { usuario, boards, recebidos };
}
