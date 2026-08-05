// Sondas de infraestrutura da API. Espelham os handlers health/ready de
// backend/cmd/api/main.go.

import { apiGet } from './client';

export interface SaudeResponse {
	status: string;
	erro?: string;
}

// verificarProcesso pergunta se a API está no ar (não toca em dependência).
export function verificarProcesso(): Promise<SaudeResponse> {
	return apiGet<SaudeResponse>('/health');
}

// verificarProntidao pergunta se a API consegue atender de fato — ela faz ping
// no Postgres. Responde 503 (ApiError) quando o banco está indisponível.
export function verificarProntidao(): Promise<SaudeResponse> {
	return apiGet<SaudeResponse>('/ready');
}
