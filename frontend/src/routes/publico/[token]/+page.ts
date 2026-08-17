import { error } from '@sveltejs/kit';
import { verQuadroPublico, type QuadroPublico } from '$lib/api/publicacao';
import { ApiError } from '$lib/api/client';
import type { PageLoad } from './$types';

export const ssr = false;

// Esta página NÃO usa o guard de autenticação, e é a única do produto que
// mostra conteúdo de quadro sem ele. Quem chega aqui costuma não ter conta e
// não vai criar uma — é o ponto da funcionalidade.
//
// Ela também não pergunta quem está logado, ao contrário da tela de convite: o
// que se vê aqui é o mesmo para todo mundo, e uma consulta a /auth/me só
// serviria para o cabeçalho trocar "Entrar" por um nome.
export const load: PageLoad = async ({ params }): Promise<{ quadro: QuadroPublico }> => {
	try {
		return { quadro: await verQuadroPublico(params.token) };
	} catch (e) {
		// Link inventado, revogado e de quadro apagado respondem os três 404 na
		// API — e a tela repete a indistinção. Dizer "este quadro existe, mas o
		// compartilhamento foi desligado" contaria a quem testa links algo sobre
		// um quadro que não é dele.
		if (e instanceof ApiError && e.status === 404) {
			throw error(404, 'Este link não está mais disponível');
		}
		throw e;
	}
};
