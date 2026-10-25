// Semeadura dos testes de ponta a ponta.
//
// Contas, quadro e convite são criados pela API, não pela tela. Passar por
// formulário de cadastro em todo teste tornaria cada um lento e — pior —
// dependente de telas que não são o assunto dele: um teste de tempo real que
// quebra porque o botão de cadastro mudou de rótulo aponta para o lugar errado.

import {
	test,
	type APIRequestContext,
	type Browser,
	type BrowserContext,
	type Page
} from '@playwright/test';

export const API = process.env.E2E_API_URL ?? 'http://localhost:8080';
export const APP = process.env.E2E_BASE_URL ?? 'http://localhost:5173';
// O Mailpit da stack de desenvolvimento faz o papel do provedor de email: a API
// manda por SMTP de verdade, e ele guarda a mensagem em vez de entregá-la.
export const MAILPIT = process.env.E2E_MAILPIT_URL ?? 'http://localhost:8025';

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

/** novoEmail devolve um endereço que nenhuma conta usa — o de quem ainda vai ser convidado. */
export function novoEmail(nome: string): string {
	return `${nome}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}@teste.dev`;
}

/** Um email como o Mailpit o guardou. */
export interface EmailRecebido {
	assunto: string;
	texto: string;
	html: string;
}

/** Os assuntos dos emails do sistema, para cada teste ler só o que é dele. */
export const ASSUNTO = {
	convite: 'convidou você',
	recuperacao: 'senha nova',
	confirmacao: 'Confirme seu email',
	atribuicao: 'pôs você como responsável',
	mencao: 'mencionou você',
	prazo: 'O prazo de'
} as const;

/**
 * emailsPara espera até chegarem `quantidade` mensagens para o endereço — com o
 * assunto informado — e as devolve da mais antiga para a mais nova.
 *
 * O assunto filtra porque uma conta recebe mais de um tipo de email: o cadastro
 * já manda o de confirmação, e um teste de convite que contasse "o primeiro
 * email da Bia" leria o errado.
 *
 * Espera, e não lê uma vez só: quem manda o email é o worker do outbox, depois
 * do commit — a resposta do convite volta antes de a mensagem sair, e é assim
 * que tem de ser.
 */
export async function emailsPara(
	req: APIRequestContext,
	endereco: string,
	assunto: string,
	quantidade = 1,
	prazoMs = 30_000
): Promise<EmailRecebido[]> {
	const fim = Date.now() + prazoMs;
	for (;;) {
		const busca = await req.get(`${MAILPIT}/api/v1/search`, {
			params: { query: `to:"${endereco}" subject:"${assunto}"` }
		});
		if (!busca.ok()) {
			throw new Error(`Mailpit respondeu ${busca.status()} — a stack subiu o serviço mailpit?`);
		}
		const { messages } = (await busca.json()) as { messages: { ID: string; Created: string }[] };
		if (messages.length >= quantidade) {
			const emOrdem = [...messages].sort((a, b) => a.Created.localeCompare(b.Created));
			return Promise.all(
				emOrdem.map(async ({ ID }) => {
					const resp = await req.get(`${MAILPIT}/api/v1/message/${ID}`);
					const m = (await resp.json()) as { Subject: string; Text: string; HTML: string };
					return { assunto: m.Subject, texto: m.Text, html: m.HTML };
				})
			);
		}
		if (Date.now() > fim) {
			throw new Error(
				`chegaram ${messages.length} de ${quantidade} email(s) para ${endereco} em ${prazoMs / 1000}s`
			);
		}
		await new Promise((resolver) => setTimeout(resolver, 500));
	}
}

/** linkDeConfirmacao tira do texto do email o link que confirma a conta. */
export function linkDeConfirmacao(email: EmailRecebido): string {
	const achado = /https?:\/\/\S+?\/confirmar-email\/[A-Za-z0-9_-]+/.exec(email.texto);
	if (!achado) throw new Error(`o email "${email.assunto}" não traz link de confirmação`);
	return achado[0];
}

/**
 * confirmarConta confirma o email da conta pelo link que o cadastro mandou —
 * pela API, porque o assunto dos testes que a usam é o que vem DEPOIS de a
 * conta estar confirmada.
 */
export async function confirmarConta(req: APIRequestContext, conta: Conta): Promise<void> {
	const [email] = await emailsPara(req, conta.email, ASSUNTO.confirmacao);
	const token = linkDeConfirmacao(email).split('/').pop();
	const resp = await req.post(`${API}/auth/confirmacao/${token}`);
	if (!resp.ok())
		throw new Error(`confirmar ${conta.email}: ${resp.status()} ${await resp.text()}`);
}

/** linkDoConvite tira do texto do email o link que abre o convite. */
export function linkDoConvite(email: EmailRecebido): string {
	const achado = /https?:\/\/\S+?\/convite\/[A-Za-z0-9_-]+/.exec(email.texto);
	if (!achado) throw new Error(`o email "${email.assunto}" não traz link de convite`);
	return achado[0];
}

/** cabecalhoDe é o cookie de sessão da conta, para as chamadas pela API. */
export function cabecalhoDe(conta: Conta) {
	return { Cookie: `${conta.cookie.name}=${conta.cookie.value}` };
}

/** idDe devolve o id da conta, que as rotas de responsável pedem na URL. */
export async function idDe(req: APIRequestContext, conta: Conta): Promise<string> {
	const resp = await req.get(`${API}/auth/me`, { headers: cabecalhoDe(conta) });
	if (!resp.ok()) throw new Error(`/auth/me de ${conta.nome}: ${resp.status()}`);
	return ((await resp.json()) as { id: string }).id;
}

/** criarCard cria um card no fim da coluna, pela API. */
export async function criarCard(
	req: APIRequestContext,
	quem: Conta,
	colunaId: string,
	titulo: string
): Promise<{ id: string }> {
	const resp = await req.post(`${API}/colunas/${colunaId}/cards`, {
		data: { titulo, descricao: '', cor: '' },
		headers: cabecalhoDe(quem)
	});
	if (!resp.ok()) throw new Error(`criar card: ${resp.status()} ${await resp.text()}`);
	return (await resp.json()) as { id: string };
}

/** atribuir põe `alvoId` como responsável pelo card, pela API. */
export async function atribuir(
	req: APIRequestContext,
	quem: Conta,
	cardId: string,
	alvoId: string
) {
	const resp = await req.put(`${API}/cards/${cardId}/responsaveis/${alvoId}`, {
		headers: cabecalhoDe(quem)
	});
	if (!resp.ok()) throw new Error(`atribuir: ${resp.status()} ${await resp.text()}`);
}

/** desatribuir tira `alvoId` da responsabilidade do card, pela API. */
export async function desatribuir(
	req: APIRequestContext,
	quem: Conta,
	cardId: string,
	alvoId: string
) {
	const resp = await req.delete(`${API}/cards/${cardId}/responsaveis/${alvoId}`, {
		headers: cabecalhoDe(quem)
	});
	if (!resp.ok()) throw new Error(`desatribuir: ${resp.status()} ${await resp.text()}`);
}

/** definirPrazo marca a data de entrega do card, pela API. */
export async function definirPrazo(
	req: APIRequestContext,
	quem: Conta,
	cardId: string,
	prazo: string
) {
	const resp = await req.patch(`${API}/cards/${cardId}/prazo`, {
		data: { prazo },
		headers: cabecalhoDe(quem)
	});
	if (!resp.ok()) throw new Error(`prazo: ${resp.status()} ${await resp.text()}`);
}

/** linkDaNotificacao tira do texto do email o link do aviso. */
export function linkDaNotificacao(email: EmailRecebido): string {
	const achado = /https?:\/\/\S+?\/notificacoes\/[0-9a-f-]{36}/.exec(email.texto);
	if (!achado) throw new Error(`o email "${email.assunto}" não traz o link da notificação`);
	return achado[0];
}

/**
 * convidarPelaApi cria o convite SEM aceitá-lo e devolve o link da resposta —
 * para os testes cujo assunto é o que acontece depois do convite, e não o
 * formulário de convidar.
 */
export async function convidarPelaApi(
	req: APIRequestContext,
	dono: Conta,
	boardId: string,
	email: string
): Promise<string> {
	const resp = await req.post(`${API}/boards/${boardId}/membros`, {
		data: { email, papel: 'editor' },
		headers: { Cookie: `${dono.cookie.name}=${dono.cookie.value}` }
	});
	if (!resp.ok()) throw new Error(`convidar ${email}: ${resp.status()} ${await resp.text()}`);
	return ((await resp.json()) as { link: string }).link;
}

/**
 * printFinal guarda a tela como está no fim do teste e a anexa ao relatório.
 *
 * O `screenshot: 'on'` da configuração só fotografa a página padrão do
 * Playwright, e estes testes abrem contextos próprios — um por pessoa. Sem isto
 * o resumo da execução sairia sem o print de quase todos os cenários.
 */
export async function printFinal(pagina: Page) {
	const caminho = test.info().outputPath('final.png');
	await pagina.screenshot({ path: caminho, fullPage: true });
	await test.info().attach('screenshot', { path: caminho, contentType: 'image/png' });
}
