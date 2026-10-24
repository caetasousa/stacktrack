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

// EstadoDaEntrega é onde está o email de um convite: na fila, entregue ao
// provedor, desistido, ou sem propósito (cancelado com o convite, vencido).
export type EstadoDaEntrega = 'pendente' | 'enviado' | 'falhou' | 'cancelado' | 'expirado';

export interface ConvitePendente {
	id: string;
	email: string;
	papel: Papel;
	expiraEm: string;
	expirado: boolean;
	// O último email do convite. Ausente quando o envio está desligado ou o
	// convite nunca foi por email.
	entrega?: { estado: EstadoDaEntrega; enviadoEm?: string };
}

export interface Participacao {
	membros: Membro[];
	// Vem vazia para quem não é dono: a lista diz para quem o quadro foi
	// oferecido, o que não é da conta de quem só participa.
	convites: ConvitePendente[];
}

// Como o convite chega a quem foi convidado: por email, na fila; só pelo link,
// porque o envio está desligado neste ambiente; ou só pelo link, porque o teto
// de emails de convite foi atingido.
export type Entrega = 'email' | 'manual' | 'cota';

// ConviteCriado é o resultado de convidar ou reenviar. O `link` vem sempre —
// é o plano B de quem convida —, e só nesta resposta e no email: o banco
// guarda apenas o hash do token.
export interface ConviteCriado {
	convite: ConvitePendente;
	link: string;
	entrega: Entrega;
}

export interface ConviteDetalhe {
	quadro: string;
	// Mascarado (`a***@exemplo.com`) porque a rota é pública: quem tem o link
	// não é necessariamente quem foi convidado. Ver $lib/email.
	emailMascarado: string;
	papel: Papel;
	convidadoPor?: string;
	expiraEm: string;
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

// reenviarConvite dá ao convite um link novo e o manda de novo por email. O
// link anterior deixa de valer — inclusive o que estava no email anterior.
export function reenviarConvite(boardId: string, conviteId: string): Promise<ConviteCriado> {
	return apiPost<Record<string, never>, ConviteCriado>(
		`/boards/${boardId}/convites/${conviteId}/reenviar`,
		{}
	);
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
