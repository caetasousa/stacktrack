// Recuperar a senha, de ponta a ponta: do "esqueci minha senha" no login até
// entrar com a senha nova.
//
// O email sai de verdade, pelo mesmo caminho do convite — outbox, worker,
// redator, SMTP —, e o teste o lê pela API do Mailpit. Um link que não chegasse,
// ou que chegasse quebrado, falha aqui mesmo que toda a API responda 202.
//
// Os cenários estão em CENARIOS.md, área F.
import { test, expect, type APIRequestContext, type Browser, type Page } from '@playwright/test';
import {
	API,
	ASSUNTO,
	APP,
	abaDe,
	criarConta,
	emailsPara,
	MAILPIT,
	novoEmail,
	printFinal,
	TetoDeRequisicoes,
	type Conta,
	type EmailRecebido
} from './apoio';

const SENHA_NOVA = 'uma senha nova que só eu saberia';

/** linkDeRecuperacao tira do texto do email o link que abre a tela de senha nova. */
function linkDeRecuperacao(email: EmailRecebido): string {
	const achado = /https?:\/\/\S+?\/redefinir-senha\/[A-Za-z0-9_-]+/.exec(email.texto);
	if (!achado) throw new Error(`o email "${email.assunto}" não traz link de recuperação`);
	return achado[0];
}

/** pedirPelaApi pede o link sem passar pela tela, devolvendo a resposta crua. */
async function pedirPelaApi(req: APIRequestContext, email: string) {
	const resp = await req.post(`${API}/auth/recuperacao`, { data: { email } });
	return { status: resp.status(), corpo: await resp.text() };
}

/** entra tenta o login pela API e devolve o status. */
async function entra(req: APIRequestContext, email: string, senha: string): Promise<number> {
	return (await req.post(`${API}/auth/login`, { data: { email, senha } })).status();
}

/** abaSemConta é o navegador de quem esqueceu a senha: ninguém logado nele. */
async function abaSemConta(navegador: Browser): Promise<Page> {
	const contexto = await navegador.newContext({ baseURL: APP });
	return contexto.newPage();
}

/** contaNova cria uma conta só para o teste, pulando o teste se o teto por IP barrar. */
async function contaNova(req: APIRequestContext, nome: string): Promise<Conta> {
	try {
		return await criarConta(req, nome);
	} catch (e) {
		if (e instanceof TetoDeRequisicoes) test.skip(true, e.message);
		throw e;
	}
}

test.describe.serial('quem esqueceu a senha', () => {
	let carla: Conta;
	let link = '';

	test.beforeAll(async ({ playwright }) => {
		const req = await playwright.request.newContext();
		carla = await contaNova(req, 'Carla');
		await req.dispose();
	});

	test('[F1] o login leva à recuperação, e o email com o link chega', async ({
		browser,
		playwright
	}) => {
		const pagina = await abaSemConta(browser);
		await pagina.goto('/login');
		await pagina.getByRole('link', { name: 'Esqueci minha senha' }).click();

		await expect(pagina.getByRole('heading', { name: 'Recuperar a senha' })).toBeVisible();
		await pagina.getByLabel('Email').fill(carla.email);
		await pagina.getByRole('button', { name: 'Enviar link' }).click();
		await expect(pagina.getByText('Se existe uma conta com esse email')).toBeVisible();

		const req = await playwright.request.newContext();
		const [email] = await emailsPara(req, carla.email, ASSUNTO.recuperacao);
		await req.dispose();
		expect(email.assunto).toContain('senha');
		link = linkDeRecuperacao(email);

		await printFinal(pagina);
		await pagina.context().close();
	});

	test('[F8] a senha nova segue as regras do cadastro, e a recusa não gasta o link', async ({
		browser
	}) => {
		const pagina = await abaSemConta(browser);
		await pagina.goto(link);
		await pagina.getByLabel('Senha nova').fill('curta');
		await pagina.getByRole('button', { name: 'Salvar a senha nova' }).click();
		await expect(pagina.getByText('a senha precisa ter ao menos 15 caracteres')).toBeVisible();

		// Continua na mesma tela, com o link ainda valendo.
		await expect(pagina.getByRole('heading', { name: 'Criar uma senha nova' })).toBeVisible();

		await printFinal(pagina);
		await pagina.context().close();
	});

	test('[F2][F3][F6] o link abre a tela, a senha nova vale e as outras sessões caem', async ({
		browser,
		playwright
	}) => {
		// Uma sessão aberta antes, noutro aparelho: é ela que precisa cair.
		const outroAparelho = await abaDe(browser, carla);
		const req = await playwright.request.newContext();

		const pagina = await abaSemConta(browser);
		await pagina.goto(link);
		await expect(pagina.getByRole('heading', { name: 'Criar uma senha nova' })).toBeVisible();
		// A tela diz de qual conta é a senha, sem mostrar o email inteiro.
		await expect(pagina.getByText(/c\*\*\*@teste\.dev/)).toBeVisible();

		await pagina.getByLabel('Senha nova').fill(SENHA_NOVA);
		await pagina.getByRole('button', { name: 'Salvar a senha nova' }).click();
		await expect(pagina).toHaveURL(/\/painel$/);
		await expect(pagina.getByRole('heading', { name: 'Quadros', exact: true })).toBeVisible();

		expect(await entra(req, carla.email, carla.senha), 'a senha antiga ainda entra').toBe(401);
		expect(await entra(req, carla.email, SENHA_NOVA), 'a senha nova não entra').toBe(200);

		// A sessão do outro aparelho foi encerrada no servidor.
		const antiga = await outroAparelho.newPage();
		await antiga.goto('/painel');
		await expect(antiga).toHaveURL(/\/login/);

		await req.dispose();
		await outroAparelho.close();
		await printFinal(pagina);
		await pagina.context().close();
	});

	test('[F4] o mesmo link não vale uma segunda vez', async ({ browser }) => {
		const pagina = await abaSemConta(browser);
		await pagina.goto(link);
		await expect(pagina.getByText('Este link é inválido ou já venceu')).toBeVisible();
		await expect(pagina.getByLabel('Senha nova')).toHaveCount(0);

		await printFinal(pagina);
		await pagina.context().close();
	});
});

test('[F5] pedir com um email sem conta responde igual, e nenhum email sai', async ({
	browser,
	playwright
}) => {
	const req = await playwright.request.newContext();
	const dani = await contaNova(req, 'Dani');
	const ninguem = novoEmail('ninguem');

	const comConta = await pedirPelaApi(req, dani.email);
	const semConta = await pedirPelaApi(req, ninguem);
	expect(semConta).toEqual(comConta);
	expect(comConta.status).toBe(202);

	// O da Dani chega; esperado ele, o do endereço sem conta já teria chegado.
	await emailsPara(req, dani.email, ASSUNTO.recuperacao);
	const busca = await req.get(`${MAILPIT}/api/v1/search`, {
		params: { query: `to:"${ninguem}"` }
	});
	expect(((await busca.json()) as { messages: unknown[] }).messages).toHaveLength(0);
	await req.dispose();

	// E a tela diz a mesma coisa nos dois casos.
	const pagina = await abaSemConta(browser);
	await pagina.goto('/recuperar-senha');
	await pagina.getByLabel('Email').fill(ninguem);
	await pagina.getByRole('button', { name: 'Enviar link' }).click();
	await expect(pagina.getByText('Se existe uma conta com esse email')).toBeVisible();

	await printFinal(pagina);
	await pagina.context().close();
});

test('[F7] o link inventado diz que não vale e oferece pedir outro', async ({ browser }) => {
	const pagina = await abaSemConta(browser);
	await pagina.goto('/redefinir-senha/um-link-que-ninguem-mandou');
	await expect(pagina.getByText('Este link é inválido ou já venceu')).toBeVisible();

	await pagina.getByRole('link', { name: 'Pedir outro link' }).click();
	await expect(pagina.getByRole('heading', { name: 'Recuperar a senha' })).toBeVisible();

	await printFinal(pagina);
	await pagina.context().close();
});
