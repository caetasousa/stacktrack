// Cliente HTTP fino sobre fetch para falar com a API Go.
// A URL base vem de PUBLIC_API_URL (Vite), com fallback para o dev local.

export const BASE_URL = import.meta.env.PUBLIC_API_URL ?? 'http://localhost:8080';

// ApiError carrega o status HTTP e a mensagem de erro devolvida pela API
// (campo `erro` do corpo JSON, quando presente).
export class ApiError extends Error {
	constructor(
		public readonly status: number,
		message: string
	) {
		super(message);
		this.name = 'ApiError';
	}
}

async function parseErro(resposta: Response): Promise<never> {
	let mensagem = `erro ${resposta.status}`;
	try {
		const corpo = await resposta.json();
		if (corpo && typeof corpo.erro === 'string') {
			mensagem = corpo.erro;
		}
	} catch {
		// corpo não-JSON ou vazio: mantém a mensagem padrão
	}
	throw new ApiError(resposta.status, mensagem);
}

// credentials: 'include' em todas as chamadas porque a sessão (fase 1) viaja em
// cookie, e o navegador não envia cookie para outra origem sem isso — em
// desenvolvimento o frontend (5173) e a API (8080) são origens diferentes.
export async function apiGet<TRes>(caminho: string): Promise<TRes> {
	const resposta = await fetch(`${BASE_URL}${caminho}`, {
		credentials: 'include'
	});

	if (!resposta.ok) {
		return parseErro(resposta);
	}

	return resposta.json() as Promise<TRes>;
}

export async function apiPost<TReq, TRes>(caminho: string, corpo: TReq): Promise<TRes> {
	const resposta = await fetch(`${BASE_URL}${caminho}`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		credentials: 'include',
		body: JSON.stringify(corpo)
	});

	if (!resposta.ok) {
		return parseErro(resposta);
	}

	return resposta.json() as Promise<TRes>;
}

// apiPostVazio é para rotas que respondem sem corpo (ex: 204 No Content).
// Chamar apiPost nelas quebraria no resposta.json(), que não tem o que ler.
export async function apiPostVazio(caminho: string): Promise<void> {
	const resposta = await fetch(`${BASE_URL}${caminho}`, {
		method: 'POST',
		credentials: 'include'
	});

	if (!resposta.ok) {
		return parseErro(resposta);
	}
}
