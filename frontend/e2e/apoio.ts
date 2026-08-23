// Semeadura dos testes de ponta a ponta.
//
// Contas, quadro e convite são criados pela API, não pela tela. Passar por
// formulário de cadastro em todo teste tornaria cada um lento e — pior —
// dependente de telas que não são o assunto dele: um teste de tempo real que
// quebra porque o botão de cadastro mudou de rótulo aponta para o lugar errado.

import type { APIRequestContext, Browser, BrowserContext } from '@playwright/test';

export const API = process.env.E2E_API_URL ?? 'http://localhost:8080';
export const APP = process.env.E2E_BASE_URL ?? 'http://localhost:5173';

/**
 * TetoDeRequisicoes é o 429 do rate limiter, e NÃO um defeito do produto: é o
 * teto por IP fazendo o trabalho dele, e a suíte inteira divide o mesmo IP.
 *
 * Tem tipo próprio para o beforeAll poder distinguir "não deu para montar o
 * cenário" de "o produto está quebrado" — o primeiro vira skip, o segundo
 * vermelho.
 */
export class TetoDeRequisicoes extends Error {
	constructor(quem: string) {
		super(`teto por IP do rate limiter atingido ao criar ${quem} — espere um minuto`);
		this.name = 'TetoDeRequisicoes';
	}
}

export interface Conta {
	nome: string;
	email: string;
	senha: string;
	cookie: { name: string; value: string };
}

/** criarConta cadastra e autentica, devolvendo o cookie de sessão. */
export async function criarConta(req: APIRequestContext, nome: string): Promise<Conta> {
	const email = `${nome}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}@teste.dev`;
	const senha = 'Senha-de-teste!12345';

	const cadastro = await req.post(`${API}/auth/cadastro`, { data: { nome, email, senha } });
	if (!cadastro.ok()) {
		if (cadastro.status() === 429) throw new TetoDeRequisicoes(nome);
		throw new Error(`cadastro de ${nome} falhou: ${cadastro.status()} ${await cadastro.text()}`);
	}

	const login = await req.post(`${API}/auth/login`, { data: { email, senha } });
	if (!login.ok()) {
		if (login.status() === 429) throw new TetoDeRequisicoes(nome);
		throw new Error(`login de ${nome} falhou: ${login.status()} ${await login.text()}`);
	}

	const sessao = (await login.headersArray())
		.filter((h) => h.name.toLowerCase() === 'set-cookie')
		.map((h) => h.value)
		.find((v) => v.includes('stacktrack_session'));
	if (!sessao) throw new Error(`login de ${nome} não devolveu cookie de sessão`);

	const [par] = sessao.split(';');
	const corte = par.indexOf('=');
	return {
		nome,
		email,
		senha,
		cookie: { name: par.slice(0, corte).trim(), value: par.slice(corte + 1).trim() }
	};
}

/** criarQuadro cria um quadro pertencente à conta informada. */
export async function criarQuadro(req: APIRequestContext, dono: Conta, titulo: string) {
	const resp = await req.post(`${API}/boards`, {
		data: { titulo },
		headers: { Cookie: `${dono.cookie.name}=${dono.cookie.value}` }
	});
	if (!resp.ok()) throw new Error(`criar quadro: ${resp.status()} ${await resp.text()}`);
	return (await resp.json()) as { id: string; titulo: string };
}

/**
 * convidar cria o convite e JÁ O ACEITA com a conta convidada, deixando a
 * pessoa dentro do quadro como editora.
 *
 * São dois passos porque o produto tem dois passos: convidar deixou de pôr
 * ninguém no quadro. Conhecer o email de alguém não concede participação — o
 * acesso nasce de um token que a pessoa certa apresenta estando logada na conta
 * daquele email. Antes esta função fazia só o POST, e a pessoa já saía membro.
 */
export async function convidar(req: APIRequestContext, dono: Conta, boardId: string, quem: Conta) {
	const resp = await req.post(`${API}/boards/${boardId}/membros`, {
		data: { email: quem.email, papel: 'editor' },
		headers: { Cookie: `${dono.cookie.name}=${dono.cookie.value}` }
	});
	if (!resp.ok()) throw new Error(`convidar ${quem.nome}: ${resp.status()} ${await resp.text()}`);

	const { link } = (await resp.json()) as { link: string };
	const token = link.split('/convite/')[1];
	if (!token) throw new Error(`link de convite em formato inesperado: ${link}`);

	const aceite = await req.post(`${API}/convites/${token}/aceitar`, {
		headers: { Cookie: `${quem.cookie.name}=${quem.cookie.value}` }
	});
	if (!aceite.ok()) {
		throw new Error(`aceitar convite de ${quem.nome}: ${aceite.status()} ${await aceite.text()}`);
	}
}

/** criarColuna cria uma coluna pela API — usado para preparar o cenário. */
export async function criarColuna(
	req: APIRequestContext,
	quem: Conta,
	boardId: string,
	titulo: string
) {
	const resp = await req.post(`${API}/boards/${boardId}/colunas`, {
		data: { titulo, cor: '' },
		headers: { Cookie: `${quem.cookie.name}=${quem.cookie.value}` }
	});
	if (!resp.ok()) throw new Error(`criar coluna: ${resp.status()} ${await resp.text()}`);
	return (await resp.json()) as { id: string };
}

/**
 * abaDe abre um BrowserContext já autenticado como a conta informada.
 *
 * Um contexto por pessoa é o que torna o teste de duas abas possível: contextos
 * têm cookies e armazenamento independentes, então duas contas convivem no
 * mesmo navegador sem uma derrubar a sessão da outra.
 */
export async function abaDe(navegador: Browser, conta: Conta): Promise<BrowserContext> {
	const contexto = await navegador.newContext({ baseURL: APP });

	// O `secure` vem do ESQUEMA, e não é opcional: contra um ambiente HTTPS o
	// cookie de sessão leva o prefixo `__Host-`, e esse prefixo exige Secure.
	// Sem ele o navegador recusa o cookie inteiro com "Invalid cookie fields", e
	// a suíte falha na primeira aba autenticada — foi o que aconteceu ao mirar
	// produção pela primeira vez.
	//
	// `domain` + `path` (e não `url`): o Playwright aceita um OU outro, nunca os
	// dois. Com `domain` sem ponto inicial o cookie nasce host-only, que é o que
	// o `__Host-` também exige.
	await contexto.addCookies([
		{
			name: conta.cookie.name,
			value: conta.cookie.value,
			domain: new URL(APP).hostname,
			path: '/',
			httpOnly: true,
			secure: new URL(APP).protocol === 'https:',
			sameSite: 'Lax'
		}
	]);
	return contexto;
}
