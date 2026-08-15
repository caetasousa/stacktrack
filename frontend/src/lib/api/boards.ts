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
	// version viaja para o cliente porque a partir da fase 6 ela volta no
	// update, como prova de qual versão a pessoa estava vendo.
	version: number;
	// prazo é null quando o card não tem data de entrega.
	prazo: string | null;
	// vencido vem calculado pelo servidor: o relógio do navegador pode estar
	// errado, e um card vermelho por engano confunde mais do que ajuda.
	vencido: boolean;
	// Cor opcional do card: vira uma tarja na lateral. Vazio é o visual padrão.
	cor: Cor | '';
	// Os responsáveis vêm com nome, e não só o id: o avatar precisa das
	// iniciais, e resolvê-las aqui exigiria uma segunda requisição.
	responsaveis: Responsavel[];
	// Só os ids: os dados da etiqueta vêm uma vez em BoardDetalhado.etiquetas.
	etiquetas: string[];
	checklist: Progresso;
	qtdAnexos: number;
	qtdComentarios: number;
}

export type Cor = 'cinza' | 'vermelho' | 'laranja' | 'amarelo' | 'verde' | 'azul' | 'roxo' | 'rosa';

/** Quem responde por um card. Só o necessário para desenhar o avatar. */
export interface Responsavel {
	usuarioId: string;
	nome: string;
}

export interface Etiqueta {
	id: string;
	nome: string;
	cor: Cor;
	posicao: number;
}

export type Fundo = 'padrao' | 'ardosia' | 'oceano' | 'floresta' | 'ameixa' | 'brasa';

export const FUNDOS: Fundo[] = ['padrao', 'ardosia', 'oceano', 'floresta', 'ameixa', 'brasa'];

// CORES é a paleta compartilhada por etiqueta, coluna e card — a mesma do
// backend (internal/domain/cor).
export const CORES: Cor[] = [
	'cinza',
	'vermelho',
	'laranja',
	'amarelo',
	'verde',
	'azul',
	'roxo',
	'rosa'
];

export interface Coluna {
	id: string;
	boardId: string;
	titulo: string;
	// Cor opcional da coluna: tinge o cabeçalho, para a etapa ter significado
	// de relance — verde no começo, amarelo no meio, azul no fim.
	cor: Cor | '';
	// A ordem é a da lista: o servidor já devolve cards e colunas ordenados
	// pela chave textual, e a chave em si não sobe — o cliente não decide
	// ordem, só a mostra. Ver backend/internal/domain/ordem.
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

export function criarColuna(boardId: string, titulo: string, cor: Cor | '' = ''): Promise<Coluna> {
	return apiPost<{ titulo: string; cor: string }, Coluna>(`/boards/${boardId}/colunas`, {
		titulo,
		cor
	});
}

export function renomearColuna(id: string, titulo: string, cor: Cor | '' = ''): Promise<Coluna> {
	return apiPatch<{ titulo: string; cor: string }, Coluna>(`/colunas/${id}`, { titulo, cor });
}

export function apagarColuna(id: string): Promise<void> {
	return apiDelete(`/colunas/${id}`);
}

export function criarCard(
	colunaId: string,
	titulo: string,
	descricao = '',
	cor: Cor | '' = ''
): Promise<Card> {
	return apiPost<{ titulo: string; descricao: string; cor: string }, Card>(
		`/colunas/${colunaId}/cards`,
		{ titulo, descricao, cor }
	);
}

/**
 * editarCard grava título, descrição e cor.
 *
 * `version` é a versão que a tela estava mostrando. O servidor a compara com a
 * atual e responde **409** se outra pessoa gravou nesse meio-tempo — em vez de
 * apagar o trabalho dela em silêncio. Zero desliga a conferência.
 */
export function editarCard(
	id: string,
	titulo: string,
	descricao: string,
	cor: Cor | '' = '',
	version = 0
): Promise<Card> {
	return apiPatch<{ titulo: string; descricao: string; cor: string; version: number }, Card>(
		`/cards/${id}`,
		{ titulo, descricao, cor, version }
	);
}

export function apagarCard(id: string): Promise<void> {
	return apiDelete(`/cards/${id}`);
}
