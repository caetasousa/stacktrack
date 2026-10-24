// Tipos e chamadas da API de autenticação.
// Espelham backend/internal/adapter/http/dto/auth.go
//
// Nada de token por aqui: a sessão vive num cookie HttpOnly, que o navegador
// envia sozinho (ver credentials: 'include' no client) e o JavaScript não lê.

import { apiGet, apiPost, apiPostVazio } from './client';

export interface CadastroRequest {
	nome: string;
	email: string;
	senha: string;
}

export interface LoginRequest {
	email: string;
	senha: string;
}

// SessaoResponse é a conta autenticada — o mesmo corpo devolvido pelo
// cadastro, pelo login e pelo /auth/me.
export interface SessaoResponse {
	id: string;
	nome: string;
	email: string;
}

export function cadastrar(dados: CadastroRequest): Promise<SessaoResponse> {
	return apiPost<CadastroRequest, SessaoResponse>('/auth/cadastro', dados);
}

export function entrar(dados: LoginRequest): Promise<SessaoResponse> {
	return apiPost<LoginRequest, SessaoResponse>('/auth/login', dados);
}

export function sair(): Promise<void> {
	return apiPostVazio('/auth/logout');
}

export function me(): Promise<SessaoResponse> {
	return apiGet<SessaoResponse>('/auth/me');
}

// --- recuperação de senha ---------------------------------------------------

export interface RecuperacaoAberta {
	emailMascarado: string;
	expiraEm: string;
}

/**
 * pedirRecuperacao manda o link de senha nova. A resposta é a mesma para email
 * com conta e sem conta — a tela não tem como saber, e é de propósito.
 */
export function pedirRecuperacao(email: string): Promise<{ mensagem: string }> {
	return apiPost<{ email: string }, { mensagem: string }>('/auth/recuperacao', { email });
}

/** consultarRecuperacao diz de qual conta é o link; 404 quando ele não vale. */
export function consultarRecuperacao(token: string): Promise<RecuperacaoAberta> {
	return apiGet<RecuperacaoAberta>(`/auth/recuperacao/${encodeURIComponent(token)}`);
}

/** redefinirSenha troca a senha e já devolve a conta autenticada. */
export function redefinirSenha(token: string, senha: string): Promise<SessaoResponse> {
	return apiPost<{ senha: string }, SessaoResponse>(
		`/auth/recuperacao/${encodeURIComponent(token)}`,
		{ senha }
	);
}
