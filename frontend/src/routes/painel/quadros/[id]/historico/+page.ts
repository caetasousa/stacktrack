import { error } from '@sveltejs/kit';
import { exigirAutenticacao } from '$lib/auth-guard';
import { detalharBoard, type BoardDetalhado } from '$lib/api/boards';
import { auditoriaDoQuadro } from '$lib/api/extras';
import { listarParticipacao, type Membro } from '$lib/api/membros';
import { ApiError } from '$lib/api/client';
import type { Atividade } from '$lib/atividade';
import type { PageLoad } from './$types';

export const ssr = false;

// Três buscas em paralelo, e cada uma responde uma pergunta diferente:
//
//   quadro     o título, para o cabeçalho e o caminho de volta;
//   auditoria  o primeiro lote de linhas;
//   membros    nome e email de quem participa HOJE, para o seletor de pessoa.
//
// Os membros vêm de uma consulta própria porque o log só conhece quem já AGIU:
// quem entrou no quadro e ainda não mexeu em nada não apareceria no filtro. O
// caminho inverso também importa — quem agiu e depois saiu não está mais na
// lista de membros, e esse vem do log. As duas fontes se completam, e a tela as
// junta por id.
export const load: PageLoad = async ({
	params
}): Promise<{
	quadro: BoardDetalhado;
	atividade: Atividade[];
	temMais: boolean;
	membros: Membro[];
}> => {
	await exigirAutenticacao();

	try {
		const [quadro, auditoria, participacao] = await Promise.all([
			detalharBoard(params.id),
			auditoriaDoQuadro(params.id, { soMovimentacoes: false }),
			listarParticipacao(params.id)
		]);
		return {
			quadro,
			atividade: auditoria.atividade,
			temMais: auditoria.temMais ?? false,
			membros: participacao.membros
		};
	} catch (e) {
		// 404 tanto para quadro inexistente quanto para quadro de outra pessoa,
		// de propósito — a tela repete a indistinção da API.
		if (e instanceof ApiError && e.status === 404) {
			throw error(404, 'Quadro não encontrado');
		}
		throw e;
	}
};
