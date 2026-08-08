// Filtro do quadro.
//
// Vive fora do componente porque a regra é pura e o erro dela é silencioso: um
// predicado errado não quebra a tela, só some com cards — e quem está olhando
// conclui que o card foi apagado.

import type { Card } from '$lib/api/boards';

/** Critérios ativos. Vazio em todos os campos significa "mostrar tudo". */
export interface Filtro {
	responsavelId: string;
	etiquetaId: string;
	soVencidos: boolean;
}

export const FILTRO_VAZIO: Filtro = { responsavelId: '', etiquetaId: '', soVencidos: false };

/** filtrando informa se algum critério está ativo. */
export function filtrando(f: Filtro): boolean {
	return !!f.responsavelId || !!f.etiquetaId || f.soVencidos;
}

/**
 * passa informa se o card atende a TODOS os critérios ativos.
 *
 * Os critérios se somam em vez de se alternarem: "da Ana" e "urgente" quer
 * dizer os cards que são as duas coisas, que é como as pessoas leem dois
 * filtros ligados ao mesmo tempo.
 */
export function passa(card: Card, f: Filtro): boolean {
	if (f.responsavelId && !card.responsaveis.some((r) => r.usuarioId === f.responsavelId)) {
		return false;
	}
	if (f.etiquetaId && !card.etiquetas.includes(f.etiquetaId)) return false;
	if (f.soVencidos && !card.vencido) return false;
	return true;
}

/** Pessoa oferecida no seletor de responsável. */
export interface PessoaDoQuadro {
	usuarioId: string;
	nome: string;
}

/**
 * pessoasDoQuadro reúne quem aparece como responsável em algum card, sem
 * repetir e em ordem de nome.
 *
 * São essas, e não todos os membros: oferecer no filtro alguém que não responde
 * por card nenhum só levaria a um quadro vazio.
 */
export function pessoasDoQuadro(colunas: { cards: Card[] }[]): PessoaDoQuadro[] {
	const porId = new Map<string, string>();
	for (const coluna of colunas) {
		for (const card of coluna.cards) {
			for (const pessoa of card.responsaveis) porId.set(pessoa.usuarioId, pessoa.nome);
		}
	}
	return [...porId]
		.map(([usuarioId, nome]) => ({ usuarioId, nome }))
		.sort((a, b) => a.nome.localeCompare(b.nome));
}
