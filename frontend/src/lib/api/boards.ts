// Tipos e chamadas da API de quadros, colunas e cards.
// Espelham backend/internal/adapter/http/dto/board.go

import { apiDelete, apiGet, apiPatch, apiPost } from './client';

// Papel de quem está olhando o quadro. A tela usa isso para não oferecer o que
// vai ser recusado — mas quem decide o que PODE é sempre o servidor.
export type Papel = 'dono' | 'editor' | 'leitor';

export interface Board {
	id: string;
	titulo: string;
	papel: Papel;
	fundo: Fundo;
	criadoEm: string;
}

export interface Progresso {
	concluidos: number;
	total: number;
}

export interface Card {
	id: string;
	colunaId: string;
	titulo: string;
	descricao: string;
	posicao: number;
	// version viaja para o cliente porque a partir da fase 6 ela volta no
	// update, como prova de qual versão a pessoa estava vendo.
	version: number;
	// prazo é null quando o card não tem data de entrega.
	prazo: string | null;
	// vencido vem calculado pelo servidor: o relógio do navegador pode estar
	// errado, e um card vermelho por engano confunde mais do que ajuda.
	vencido: boolean;
	// Só os ids: os dados da etiqueta vêm uma vez em BoardDetalhado.etiquetas.
	etiquetas: string[];
	checklist: Progresso;
	qtdAnexos: number;
}

export type Cor = 'cinza' | 'vermelho' | 'laranja' | 'amarelo' | 'verde' | 'azul' | 'roxo' | 'rosa';

export interface Etiqueta {
	id: string;
	nome: string;
	cor: Cor;
	posicao: number;
}

export type Fundo = 'padrao' | 'ardosia' | 'oceano' | 'floresta' | 'ameixa' | 'brasa';

export const FUNDOS: Fundo[] = ['padrao', 'ardosia', 'oceano', 'floresta', 'ameixa', 'brasa'];

export interface Coluna {
	id: string;
	boardId: string;
	titulo: string;
	posicao: number;
	cards: Card[];
}

export interface BoardDetalhado {
	id: string;
	titulo: string;
	papel: Papel;
	fundo: Fundo;
	colunas: Coluna[];
	// As etiquetas do quadro inteiro: o card carrega só os ids delas.
	etiquetas: Etiqueta[];
}

interface ListaBoards {
	boards: Board[];
}

export async function listarBoards(): Promise<Board[]> {
	const resposta = await apiGet<ListaBoards>('/boards');
	return resposta.boards;
}

export function criarBoard(titulo: string): Promise<Board> {
	return apiPost<{ titulo: string }, Board>('/boards', { titulo });
}

export function detalharBoard(id: string): Promise<BoardDetalhado> {
	return apiGet<BoardDetalhado>(`/boards/${id}`);
}

export function renomearBoard(id: string, titulo: string): Promise<Board> {
	return apiPatch<{ titulo: string }, Board>(`/boards/${id}`, { titulo });
}

export function apagarBoard(id: string): Promise<void> {
	return apiDelete(`/boards/${id}`);
}

export function criarColuna(boardId: string, titulo: string): Promise<Coluna> {
	return apiPost<{ titulo: string }, Coluna>(`/boards/${boardId}/colunas`, { titulo });
}

export function renomearColuna(id: string, titulo: string): Promise<Coluna> {
	return apiPatch<{ titulo: string }, Coluna>(`/colunas/${id}`, { titulo });
}

export function apagarColuna(id: string): Promise<void> {
	return apiDelete(`/colunas/${id}`);
}

export function criarCard(colunaId: string, titulo: string, descricao = ''): Promise<Card> {
	return apiPost<{ titulo: string; descricao: string }, Card>(`/colunas/${colunaId}/cards`, {
		titulo,
		descricao
	});
}

export function editarCard(id: string, titulo: string, descricao: string): Promise<Card> {
	return apiPatch<{ titulo: string; descricao: string }, Card>(`/cards/${id}`, {
		titulo,
		descricao
	});
}

export function apagarCard(id: string): Promise<void> {
	return apiDelete(`/cards/${id}`);
}
