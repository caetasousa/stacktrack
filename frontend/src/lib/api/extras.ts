// Tipos e chamadas do que pende do card: etiquetas, checklists e anexos.
// Espelham backend/internal/adapter/http/dto/board.go

import { apiDelete, apiGet, apiPatch, apiPost, BASE_URL } from './client';
import type { Card, Cor, Etiqueta, Fundo, Board } from './boards';
import type { Atividade } from '$lib/atividade';

export interface ItemDeChecklist {
	id: string;
	checklistId: string;
	texto: string;
	concluido: boolean;
	posicao: number;
}

export interface Checklist {
	id: string;
	cardId: string;
	titulo: string;
	posicao: number;
	itens: ItemDeChecklist[];
}

export type TipoDeAnexo = 'arquivo' | 'link';

export interface Anexo {
	id: string;
	cardId: string;
	tipo: TipoDeAnexo;
	nome: string;
	url?: string;
	tamanho?: number;
	mime?: string;
	criadoEm: string;
}

// CardDetalhado é o que o modal mostra: o card e tudo que pende dele, numa
// requisição só.
/** Uma mensagem na conversa de um card. */
export interface Comentario {
	id: string;
	cardId: string;
	autorId: string;
	autorNome: string;
	texto: string;
	criadoEm: string;
	// null enquanto nunca foi editado — é o que deixa a tela dizer "editado"
	// sem comparar datas que sempre diferem.
	editadoEm: string | null;
}

export interface CardDetalhado extends Card {
	boardId: string;
	comentarios: Comentario[];
	etiquetasDoCard: Etiqueta[];
	checklists: Checklist[];
	anexos: Anexo[];
}

export function detalharCard(cardId: string): Promise<CardDetalhado> {
	return apiGet<CardDetalhado>(`/cards/${cardId}`);
}

// definirPrazo marca a data; passar null limpa.
export function definirPrazo(cardId: string, prazo: string | null): Promise<Card> {
	return apiPatch<{ prazo: string | null }, Card>(`/cards/${cardId}/prazo`, { prazo });
}

export function definirFundo(boardId: string, fundo: Fundo): Promise<Board> {
	return apiPatch<{ fundo: Fundo }, Board>(`/boards/${boardId}/fundo`, { fundo });
}

// --- etiquetas ------------------------------------------------------------

export function criarEtiqueta(boardId: string, nome: string, cor: Cor): Promise<Etiqueta> {
	return apiPost<{ nome: string; cor: Cor }, Etiqueta>(`/boards/${boardId}/etiquetas`, {
		nome,
		cor
	});
}

export function editarEtiqueta(etiquetaId: string, nome: string, cor: Cor): Promise<Etiqueta> {
	return apiPatch<{ nome: string; cor: Cor }, Etiqueta>(`/etiquetas/${etiquetaId}`, { nome, cor });
}

export function apagarEtiqueta(etiquetaId: string): Promise<void> {
	return apiDelete(`/etiquetas/${etiquetaId}`);
}

// aplicarEtiqueta usa PUT porque é idempotente: aplicar duas vezes deixa o card
// no mesmo estado, e a API responde 204 nas duas.
export async function aplicarEtiqueta(cardId: string, etiquetaId: string): Promise<void> {
	const resposta = await fetch(`${BASE_URL}/cards/${cardId}/etiquetas/${etiquetaId}`, {
		method: 'PUT',
		credentials: 'include'
	});
	if (!resposta.ok) {
		throw await erroDaResposta(resposta);
	}
}

export function removerEtiqueta(cardId: string, etiquetaId: string): Promise<void> {
	return apiDelete(`/cards/${cardId}/etiquetas/${etiquetaId}`);
}

// --- responsáveis ---------------------------------------------------------

// atribuir usa PUT pelo mesmo motivo de aplicarEtiqueta: atribuir duas vezes
// deixa o card no mesmo estado, e a API responde 204 nas duas.
export async function atribuir(cardId: string, usuarioId: string): Promise<void> {
	const resposta = await fetch(`${BASE_URL}/cards/${cardId}/responsaveis/${usuarioId}`, {
		method: 'PUT',
		credentials: 'include'
	});
	if (!resposta.ok) {
		throw await erroDaResposta(resposta);
	}
}

export function desatribuir(cardId: string, usuarioId: string): Promise<void> {
	return apiDelete(`/cards/${cardId}/responsaveis/${usuarioId}`);
}

// --- histórico -------------------------------------------------------------

// atividadeDoCard busca o histórico sob demanda, e não junto do card: quase
// toda abertura de card é para ler ou mexer, não para auditar. Trazer o
// histórico sempre pagaria uma consulta a mais em todas elas.
export function atividadeDoCard(cardId: string): Promise<{ atividade: Atividade[] }> {
	return apiGet<{ atividade: Atividade[] }>(`/cards/${cardId}/atividade`);
}

/** O recorte da auditoria do quadro. */
export interface FiltroDeAuditoria {
	/** `true` (o padrão do servidor) traz só movimentações. */
	soMovimentacoes?: boolean;
	/** Vazio significa "de todo mundo". */
	autorId?: string;
	/**
	 * O seq da última linha já recebida — é o cursor da página seguinte.
	 *
	 * Cursor, e não número de página: o log recebe escrita o tempo todo, e
	 * paginar por deslocamento pularia linhas que entrassem entre um pedido e o
	 * outro. Numa auditoria, pular em silêncio é o pior defeito possível.
	 */
	antesDe?: number;
}

// auditoriaDoQuadro responde "quem mexeu no quê" para o quadro inteiro. É o
// mesmo read model do histórico de um card, sem o recorte por card — a
// informação sempre esteve no log; o que faltava era alcançá-la sem abrir
// cinquenta cards.
export function auditoriaDoQuadro(
	boardId: string,
	filtro: FiltroDeAuditoria = {}
): Promise<{ atividade: Atividade[]; temMais?: boolean }> {
	const parametros = new URLSearchParams();
	if (filtro.soMovimentacoes === false) parametros.set('filtro', 'tudo');
	if (filtro.autorId) parametros.set('autor', filtro.autorId);
	if (filtro.antesDe) parametros.set('antesDe', String(filtro.antesDe));

	const consulta = parametros.toString();
	// `temMais` vem do servidor, e não é deduzido do tamanho do lote: o teto de
	// página é dele, e uma página cheia não significa que acabou. Sem isso a
	// tela ofereceria um "carregar mais" que devolve lista vazia.
	return apiGet<{ atividade: Atividade[]; temMais?: boolean }>(
		`/boards/${boardId}/atividade${consulta ? `?${consulta}` : ''}`
	);
}

// --- comentários ----------------------------------------------------------

export function comentar(cardId: string, texto: string): Promise<Comentario> {
	return apiPost<{ texto: string }, Comentario>(`/cards/${cardId}/comentarios`, { texto });
}

export function editarComentario(comentarioId: string, texto: string): Promise<Comentario> {
	return apiPatch<{ texto: string }, Comentario>(`/comentarios/${comentarioId}`, { texto });
}

export function apagarComentario(comentarioId: string): Promise<void> {
	return apiDelete(`/comentarios/${comentarioId}`);
}

// --- checklists -----------------------------------------------------------

export function criarChecklist(cardId: string, titulo: string): Promise<Checklist> {
	return apiPost<{ titulo: string }, Checklist>(`/cards/${cardId}/checklists`, { titulo });
}

export function renomearChecklist(checklistId: string, titulo: string): Promise<Checklist> {
	return apiPatch<{ titulo: string }, Checklist>(`/checklists/${checklistId}`, { titulo });
}

export function apagarChecklist(checklistId: string): Promise<void> {
	return apiDelete(`/checklists/${checklistId}`);
}

export function criarItem(checklistId: string, texto: string): Promise<ItemDeChecklist> {
	return apiPost<{ texto: string }, ItemDeChecklist>(`/checklists/${checklistId}/itens`, { texto });
}

// editarItem manda só o que mudou: marcar a caixa não reenvia o texto, e
// renomear não desmarca sem querer.
export function editarItem(
	itemId: string,
	mudanca: { texto?: string; concluido?: boolean }
): Promise<ItemDeChecklist> {
	return apiPatch<typeof mudanca, ItemDeChecklist>(`/itens/${itemId}`, mudanca);
}

export function apagarItem(itemId: string): Promise<void> {
	return apiDelete(`/itens/${itemId}`);
}

// --- anexos ---------------------------------------------------------------

export function anexarLink(cardId: string, url: string, nome: string): Promise<Anexo> {
	return apiPost<{ url: string; nome: string }, Anexo>(`/cards/${cardId}/anexos/link`, {
		url,
		nome
	});
}

// anexarArquivo manda multipart. Não passa pelo apiPost porque o corpo é
// FormData: definir Content-Type à mão aqui quebraria o boundary que o
// navegador precisa gerar.
export async function anexarArquivo(cardId: string, arquivo: File): Promise<Anexo> {
	const corpo = new FormData();
	corpo.append('arquivo', arquivo);

	const resposta = await fetch(`${BASE_URL}/cards/${cardId}/anexos/arquivo`, {
		method: 'POST',
		credentials: 'include',
		body: corpo
	});
	if (!resposta.ok) {
		throw await erroDaResposta(resposta);
	}
	return resposta.json() as Promise<Anexo>;
}

export function apagarAnexo(anexoId: string): Promise<void> {
	return apiDelete(`/anexos/${anexoId}`);
}

// urlDoAnexo é o endereço de download. Passa pela API, e não por um caminho
// estático, porque é lá que se confere se quem pede participa do quadro.
export function urlDoAnexo(anexoId: string): string {
	return `${BASE_URL}/anexos/${anexoId}`;
}

// erroDaResposta repete o tratamento do cliente HTTP para as duas chamadas que
// não passam por ele (PUT sem corpo e upload multipart).
async function erroDaResposta(resposta: Response): Promise<Error> {
	const { ApiError } = await import('./client');
	let mensagem = `erro ${resposta.status}`;
	try {
		const corpo = await resposta.json();
		if (corpo && typeof corpo.erro === 'string') {
			mensagem = corpo.erro;
		}
	} catch {
		// corpo não-JSON ou vazio: mantém a mensagem padrão
	}
	return new ApiError(resposta.status, mensagem);
}

// --- mover ----------------------------------------------------------------

// Vizinhos diz ONDE o item foi solto. Quem calcula a posição é o servidor, que
// enxerga as posições reais: a cópia do quadro na tela pode estar velha, e
// posição vinda do cliente embaralharia a ordem de um quadro inteiro.
// Vazio significa ponta — sem anterior é o topo, sem próximo é o fim.
export interface Vizinhos {
	colunaId?: string;
	anteriorId?: string;
	proximoId?: string;
}

export function moverCard(cardId: string, vizinhos: Vizinhos): Promise<Card> {
	return apiPatch<Vizinhos, Card>(`/cards/${cardId}/mover`, vizinhos);
}

export function moverColuna(colunaId: string, vizinhos: Vizinhos): Promise<unknown> {
	return apiPatch<Vizinhos, unknown>(`/colunas/${colunaId}/mover`, vizinhos);
}
