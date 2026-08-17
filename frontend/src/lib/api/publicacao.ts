// O link público de acompanhamento de um quadro.
// Espelha backend/internal/adapter/http/dto/publicacao.go
//
// São duas conversas diferentes com a API, e o arquivo as mantém à vista uma da
// outra de propósito: as três primeiras são do DONO e viajam com a sessão; a
// última é a de quem chegou pelo link e não tem conta nenhuma.

import { apiDelete, apiGet, BASE_URL, ApiError } from './client';
import type { Cor, Fundo } from './boards';

/** O estado do link público, como o dono o vê. */
export interface Publicacao {
	publicado: boolean;
	/** A URL para copiar e mandar. Ausente quando o quadro não está publicado. */
	url?: string;
	criadoEm?: string;
}

/** Um rótulo no card público: já resolvido em nome e cor, sem id para cruzar. */
export interface EtiquetaPublica {
	nome: string;
	cor: Cor;
}

/**
 * Um card como quem acompanha de fora o enxerga.
 *
 * Sem responsáveis, sem comentários, sem anexos e sem id — e isso não é um
 * recorte da tela, é o que a API devolve. Ver ucboard.QuadroPublico no backend
 * para o motivo de cada ausência.
 */
export interface CardPublico {
	titulo: string;
	descricao: string;
	cor: Cor | '';
	prazo: string | null;
	vencido: boolean;
	etiquetas: EtiquetaPublica[];
	checklist: { concluidos: number; total: number };
}

export interface ColunaPublica {
	titulo: string;
	cor: Cor | '';
	cards: CardPublico[];
}

export interface QuadroPublico {
	titulo: string;
	fundo: Fundo;
	atualizadoEm: string;
	colunas: ColunaPublica[];
}

export function obterPublicacao(boardId: string): Promise<Publicacao> {
	return apiGet<Publicacao>(`/boards/${boardId}/publicacao`);
}

/**
 * publicar liga o link e devolve a URL.
 *
 * PUT, e não POST: repetir devolve o MESMO link em vez de criar um segundo.
 * Sem essa garantia, abrir a tela de compartilhamento duas vezes invalidaria em
 * silêncio o endereço já enviado às pessoas.
 */
export async function publicar(boardId: string): Promise<Publicacao> {
	const resposta = await fetch(`${BASE_URL}/boards/${boardId}/publicacao`, {
		method: 'PUT',
		credentials: 'include'
	});
	if (!resposta.ok) {
		throw new ApiError(resposta.status, 'não foi possível publicar o quadro');
	}
	return resposta.json() as Promise<Publicacao>;
}

/** despublicar derruba o link na hora. Religar depois gera um endereço NOVO. */
export function despublicar(boardId: string): Promise<void> {
	return apiDelete(`/boards/${boardId}/publicacao`);
}

/**
 * verQuadroPublico busca o quadro pelo token do link, SEM mandar cookie.
 *
 * `credentials: 'omit'` é deliberado, e é a única chamada do aplicativo que o
 * usa. A rota não olha sessão nenhuma, então mandar o cookie de quem por acaso
 * está logado só exporia a credencial dele numa requisição que não precisa
 * dela — inclusive quando a página é aberta a partir de um link recebido de
 * terceiros.
 */
export async function verQuadroPublico(token: string): Promise<QuadroPublico> {
	const resposta = await fetch(`${BASE_URL}/publico/${encodeURIComponent(token)}`, {
		credentials: 'omit'
	});
	if (!resposta.ok) {
		throw new ApiError(resposta.status, 'link inválido ou desativado');
	}
	return resposta.json() as Promise<QuadroPublico>;
}
