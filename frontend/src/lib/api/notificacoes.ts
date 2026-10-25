// O sino: os avisos pessoais de quem está logado — o convite para um quadro, o
// card que passou a ser dela, a menção num comentário, o prazo chegando.
// Espelha os DTOs de backend/internal/adapter/http/dto/notificacao.go.
import { apiGet, apiPostVazio } from './client';

export type TipoDeNotificacao = 'convite' | 'atribuicao' | 'mencao' | 'prazo';

/**
 * Se o aviso ainda vale. "encerrada" é o fato que deixou de valer (a atribuição
 * desfeita, o prazo trocado); os quatro últimos são o desfecho do convite.
 */
export type SituacaoDaNotificacao =
	'vigente' | 'encerrada' | 'aceito' | 'recusado' | 'revogado' | 'vencido';

export interface Notificacao {
	id: string;
	tipo: TipoDeNotificacao;
	quadroId: string;
	quadro: string;
	cardId?: string;
	card?: string;
	/** Quem causou. Ausente no prazo, que não tem pessoa por trás. */
	autor?: string;
	/** O começo do comentário, na menção — lido do comentário de agora. */
	trecho?: string;
	/** A data que motivou o aviso de prazo. */
	prazo?: string;
	criadaEm: string;
	lida: boolean;
	situacao: SituacaoDaNotificacao;
}

export interface PaginaDeNotificacoes {
	notificacoes: Notificacao[];
	/** O número do sino, da MESMA leitura que montou a lista. */
	naoLidas: number;
	temMais: boolean;
}

/** listarNotificacoes devolve uma página; `antesDe` é o último aviso da anterior. */
export function listarNotificacoes(antesDe?: string): Promise<PaginaDeNotificacoes> {
	const consulta = antesDe ? `?antesDe=${encodeURIComponent(antesDe)}` : '';
	return apiGet<PaginaDeNotificacoes>(`/notificacoes${consulta}`);
}

/** detalharNotificacao é o que a página do link do email consulta. */
export function detalharNotificacao(id: string): Promise<Notificacao> {
	return apiGet<Notificacao>(`/notificacoes/${encodeURIComponent(id)}`);
}

/** marcarLida também cancela o email do aviso, se ainda não saiu. */
export function marcarLida(id: string): Promise<void> {
	return apiPostVazio(`/notificacoes/${encodeURIComponent(id)}/lida`);
}

export function marcarTodasLidas(): Promise<void> {
	return apiPostVazio('/notificacoes/lidas');
}
