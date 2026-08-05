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
