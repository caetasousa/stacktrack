// Tipos e chamadas da API de participação e convites.
// Espelham backend/internal/adapter/http/dto/membro.go

import { apiDelete, apiGet, apiPatch, apiPost } from './client';
import type { Board, Papel } from './boards';

export interface Membro {
	usuarioId: string;
	nome: string;
	email: string;
	papel: Papel;
	desdeEm: string;
}

export interface ConvitePendente {
	id: string;
	email: string;
	papel: Papel;
	expiraEm: string;
	expirado: boolean;
}

export interface Participacao {
	membros: Membro[];
	// Vem vazia para quem não é dono: a lista diz para quem o quadro foi
	// oferecido, o que não é da conta de quem só participa.
	convites: ConvitePendente[];
}

// ConviteCriado é o resultado de convidar. Quando `adicionado` é true a pessoa
// já tinha conta e entrou agora; caso contrário vem o `link`, que a API devolve
// NESTA resposta e em nenhuma outra — o banco guarda só o hash do token.
export interface ConviteCriado {
	adicionado: boolean;
	membro?: Membro;
	convite?: ConvitePendente;
	link?: string;
}

export interface ConviteDetalhe {
	quadro: string;
	email: string;
	papel: Papel;
	convidadoPor?: string;
}

export function listarParticipacao(boardId: string): Promise<Participacao> {
	return apiGet<Participacao>(`/boards/${boardId}/membros`);
}

export function convidar(boardId: string, email: string, papel: Papel): Promise<ConviteCriado> {
	return apiPost<{ email: string; papel: Papel }, ConviteCriado>(`/boards/${boardId}/membros`, {
		email,
		papel
	});
}

export function alterarPapel(boardId: string, usuarioId: string, papel: Papel): Promise<Membro> {
	return apiPatch<{ papel: Papel }, Membro>(`/boards/${boardId}/membros/${usuarioId}`, { papel });
}

export function removerMembro(boardId: string, usuarioId: string): Promise<void> {
	return apiDelete(`/boards/${boardId}/membros/${usuarioId}`);
}

export function revogarConvite(boardId: string, conviteId: string): Promise<void> {
	return apiDelete(`/boards/${boardId}/convites/${conviteId}`);
}

// detalharConvite não exige sessão: quem foi convidado costuma ainda não ter
// conta, e precisa ver de que quadro se trata antes de criar uma.
export function detalharConvite(token: string): Promise<ConviteDetalhe> {
	return apiGet<ConviteDetalhe>(`/convites/${encodeURIComponent(token)}`);
}

// aceitarConvite manda um corpo vazio porque a rota não lê nada dele — o token
// vem no caminho. Usa apiPost, e não apiPostVazio, porque a resposta traz o
// quadro para onde levar a pessoa em seguida.
export function aceitarConvite(token: string): Promise<Board> {
	return apiPost<Record<string, never>, Board>(
		`/convites/${encodeURIComponent(token)}/aceitar`,
		{}
	);
}
