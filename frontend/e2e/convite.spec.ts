// O convite por email, de ponta a ponta: do formulário de quem convida até o
// quadro aparecer no painel de quem foi convidado.
//
// O email sai de verdade, pelo mesmo caminho da produção — outbox, worker,
// redator, SMTP —, e só o destino muda: o Mailpit da stack guarda a mensagem, e
// o teste a lê pela API dele. Um link que não chegasse, ou que chegasse
// quebrado, falha aqui mesmo que toda a API responda 201.
//
// Os cenários estão em CENARIOS.md, área E.
import { test, expect, type Browser, type Page } from '@playwright/test';
import {
	APP,
	abaDe,
	convidar,
	convidarPelaApi,
	criarConta,
	criarQuadro,
	emailsPara,
	linkDoConvite,
	novoEmail,
	printFinal,
	TetoDeRequisicoes,
	type Conta
} from './apoio';

const TITULO = 'Lançamento do app';

let ana: Conta;
let quadroId: string;

test.beforeAll(async ({ playwright }) => {
	const req = await playwright.request.newContext();
	try {
		ana = await criarConta(req, 'Ana');
		quadroId = (await criarQuadro(req, ana, TITULO)).id;
	} catch (e) {
		if (e instanceof TetoDeRequisicoes) test.skip(true, e.message);
		throw e;
	} finally {
		await req.dispose();
	}
});

/** telaDeMembros abre a tela de membros do quadro como a dona. */
async function telaDeMembros(navegador: Browser): Promise<Page> {
	const contexto = await abaDe(navegador, ana);
	const pagina = await contexto.newPage();
	await pagina.goto(`/painel/quadros/${quadroId}/membros`);
	return pagina;
}

/** convidarPelaTela preenche o formulário de convite e envia. */
async function convidarPelaTela(pagina: Page, email: string) {
	await pagina.getByLabel('Email de quem você quer convidar').fill(email);
	await pagina.getByRole('button', { name: 'Enviar convite' }).click();
}

/** abaSemConta é o navegador de quem recebeu o email e ainda não entrou em nada. */
async function abaSemConta(navegador: Browser): Promise<Page> {
	const contexto = await navegador.newContext({ baseURL: APP });
	return contexto.newPage();
}

test.describe.serial('quem ainda não tem conta', () => {
	const bia = {
		nome: 'Bia',
		email: novoEmail('bia'),
		senha: 'uma frase comprida que só a Bia diria'
	};
	let link = '';
	let sessaoDaBia: Awaited<ReturnType<import('@playwright/test').BrowserContext['storageState']>>;

	test('[E13] a tela de membros explica o que acontece depois de convidar', async ({ browser }) => {
		const pagina = await telaDeMembros(browser);

		await expect(pagina.getByRole('heading', { name: 'Convidar por email' })).toBeVisible();
		await expect(pagina.getByText('A pessoa recebe um email com o link do convite')).toBeVisible();
		// A frase que confundia: prometia um atalho que o produto não tem desde que
		// conhecer um email deixou de pôr alguém no quadro.
		await expect(pagina.getByText('entra na hora')).toHaveCount(0);

		await printFinal(pagina);
		await pagina.context().close();
	});

	test('[E1] a dona convida pelo email e a mensagem chega', async ({ browser, playwright }) => {
		const pagina = await telaDeMembros(browser);
		await convidarPelaTela(pagina, bia.email);
		await expect(pagina.getByText(`Convite enviado para ${bia.email}`)).toBeVisible();

		const req = await playwright.request.newContext();
		const [email] = await emailsPara(req, bia.email);
		await req.dispose();

		// O assunto é o que decide se a pessoa abre o email: quem chama e para quê.
		expect(email.assunto).toContain('Ana');
		expect(email.assunto).toContain(TITULO);
		link = linkDoConvite(email);

		await printFinal(pagina);
		await pagina.context().close();
	});

	test('[E9] a lista de convites diz que o email saiu', async ({ browser }) => {
		const pagina = await telaDeMembros(browser);
		const convite = pagina.getByRole('listitem').filter({ hasText: bia.email });

		// O worker marca o envio logo depois de o servidor SMTP aceitar; a tela só
		// sabe disso quando pergunta de novo.
		await expect(async () => {
			await pagina.reload();
			await expect(convite.getByText(/email enviado/)).toBeVisible({ timeout: 1_000 });
		}).toPass({ timeout: 20_000 });

		await printFinal(pagina);
		await pagina.context().close();
	});

	test('[E2] o link do email abre o convite', async ({ browser }) => {
		const pagina = await abaSemConta(browser);
		await pagina.goto(link);

		await expect(pagina.getByRole('heading', { name: 'Ana convidou você' })).toBeVisible();
		await expect(pagina.getByText(TITULO)).toBeVisible();
		// Quem chega pelo email precisa saber até quando pode decidir.
		await expect(pagina.getByText('Vale até')).toBeVisible();

		await printFinal(pagina);
		await pagina.context().close();
	});

	test('[E10] a página do convite não mostra o email inteiro de quem foi convidado', async ({
		browser
	}) => {
		const pagina = await abaSemConta(browser);
		await pagina.goto(link);

		await expect(pagina.getByText('b***@teste.dev').first()).toBeVisible();
		await expect(pagina.getByText(bia.email)).toHaveCount(0);

		await printFinal(pagina);
		await pagina.context().close();
	});

	test('[E3] quem não tem conta cria a conta pelo convite e volta a ele', async ({ browser }) => {
		const pagina = await abaSemConta(browser);
		await pagina.goto(link);

		await pagina.getByRole('main').getByRole('link', { name: 'Criar conta' }).click();
		await expect(pagina.getByText('Você está aceitando um convite')).toBeVisible();

		await pagina.getByLabel('Nome', { exact: true }).fill(bia.nome);
		await pagina.getByLabel('Email', { exact: true }).fill(bia.email);
		await pagina.getByLabel('Senha', { exact: true }).fill(bia.senha);
		await pagina.getByRole('button', { name: 'Criar conta' }).click();

		// De volta ao convite, e não ao painel vazio de uma conta recém-criada.
		await expect(pagina).toHaveURL(/\/convite\//);
		await pagina.getByRole('button', { name: 'Aceitar convite' }).click();

		await expect(pagina).toHaveURL(new RegExp(`/painel/quadros/${quadroId}`));
		await expect(pagina.getByRole('heading', { name: TITULO })).toBeVisible();

		sessaoDaBia = await pagina.context().storageState();
		await printFinal(pagina);
		await pagina.context().close();
	});

	test('[E5] o quadro aceito aparece em "Compartilhados com você", com o nome da dona', async ({
		browser
	}) => {
		const contexto = await browser.newContext({ baseURL: APP, storageState: sessaoDaBia });
		const pagina = await contexto.newPage();
		await pagina.goto('/painel');

		const compartilhados = pagina.getByRole('region', { name: 'Compartilhados com você' });
		await expect(compartilhados.getByText(TITULO)).toBeVisible();
		await expect(compartilhados.getByText('de Ana')).toBeVisible();
		// Isolado dos quadros dela: o quadro é da Ana, e a tela não pode sugerir
		// outra coisa.
		await expect(
			pagina.getByRole('region', { name: 'Seus quadros' }).getByText(TITULO)
		).toHaveCount(0);

		await printFinal(pagina);
		await contexto.close();
	});

	test('[E11] convidar e entrar ficam no histórico do quadro', async ({ browser }) => {
		const contexto = await abaDe(browser, ana);
		const pagina = await contexto.newPage();
		await pagina.goto(`/painel/quadros/${quadroId}/historico`);

		await expect(pagina.getByText('convidou b***@teste.dev como editor')).toBeVisible();
		await expect(pagina.getByText('entrou no quadro como editor')).toBeVisible();

		await printFinal(pagina);
		await contexto.close();
	});
});

test('[E4] quem já tem conta vai do cadastro para o login e volta ao convite', async ({
	browser,
	playwright
}) => {
	const req = await playwright.request.newContext();
	const carlos = await criarConta(req, 'Carlos');
	await convidarPelaApi(req, ana, quadroId, carlos.email);
	const [email] = await emailsPara(req, carlos.email);
	await req.dispose();

	const pagina = await abaSemConta(browser);
	await pagina.goto(linkDoConvite(email));

	// Começa pelo caminho errado, como acontece: clica em criar conta e percebe
	// que já tem uma.
	await pagina.getByRole('main').getByRole('link', { name: 'Criar conta' }).click();
	await pagina.getByRole('main').getByRole('link', { name: 'Entrar' }).click();
	await expect(pagina.getByText('Entre com c***@teste.dev para aceitar o convite')).toBeVisible();

	await pagina.getByLabel('Email', { exact: true }).fill(carlos.email);
	await pagina.getByLabel('Senha', { exact: true }).fill(carlos.senha);
	await pagina.getByRole('button', { name: 'Entrar' }).click();

	await expect(pagina).toHaveURL(/\/convite\//);
	await pagina.getByRole('button', { name: 'Aceitar convite' }).click();
	await expect(pagina).toHaveURL(new RegExp(`/painel/quadros/${quadroId}`));

	await printFinal(pagina);
	await pagina.context().close();
});

test('[E6] logado com outra conta, o convite não é aceito e a tela oferece trocar', async ({
	browser,
	playwright
}) => {
	const req = await playwright.request.newContext();
	const davi = await criarConta(req, 'Davi');
	const link = await convidarPelaApi(req, ana, quadroId, novoEmail('erica'));
	await req.dispose();

	const contexto = await abaDe(browser, davi);
	const pagina = await contexto.newPage();
	await pagina.goto(link);

	await expect(pagina.getByText(`Você está com a conta ${davi.email}`)).toBeVisible();
	await expect(pagina.getByRole('button', { name: 'Aceitar convite' })).toHaveCount(0);

	await pagina.getByRole('button', { name: 'Sair e entrar com outra conta' }).click();
	await expect(pagina).toHaveURL(/\/login\?voltar=%2Fconvite%2F/);

	await printFinal(pagina);
	await contexto.close();
});

test('[E7] revogar derruba o link que já foi por email', async ({ browser, playwright }) => {
	const fabio = novoEmail('fabio');
	const req = await playwright.request.newContext();
	await convidarPelaApi(req, ana, quadroId, fabio);
	const [email] = await emailsPara(req, fabio);
	await req.dispose();

	const pagina = await telaDeMembros(browser);
	const convite = pagina.getByRole('listitem').filter({ hasText: fabio });
	await convite.getByRole('button', { name: 'Revogar' }).click();
	// A confirmação é um alertdialog: ação destrutiva, que interrompe.
	await pagina.getByRole('alertdialog').getByRole('button', { name: 'Revogar' }).click();
	await expect(convite).toHaveCount(0);
	await pagina.context().close();

	const anonima = await abaSemConta(browser);
	await anonima.goto(linkDoConvite(email));
	await expect(anonima.getByText('Convite inválido ou expirado')).toBeVisible();

	await printFinal(anonima);
	await anonima.context().close();
});

test('[E8] reenviar manda email novo, e o link anterior deixa de valer', async ({
	browser,
	playwright
}) => {
	const gabi = novoEmail('gabi');
	const req = await playwright.request.newContext();
	await convidarPelaApi(req, ana, quadroId, gabi);
	const [primeiro] = await emailsPara(req, gabi);

	const pagina = await telaDeMembros(browser);
	const convite = pagina.getByRole('listitem').filter({ hasText: gabi });
	await convite.getByRole('button', { name: 'Reenviar' }).click();
	await expect(pagina.getByText(`Convite reenviado para ${gabi}`)).toBeVisible();
	await expect(pagina.getByText('O link anterior deixou de valer')).toBeVisible();
	await pagina.context().close();

	const [, segundo] = await emailsPara(req, gabi, 2);
	await req.dispose();
	const linkAntigo = linkDoConvite(primeiro);
	const linkNovo = linkDoConvite(segundo);
	expect(linkNovo).not.toBe(linkAntigo);

	const anonima = await abaSemConta(browser);
	await anonima.goto(linkAntigo);
	await expect(anonima.getByText('Convite inválido ou expirado')).toBeVisible();
	await anonima.goto(linkNovo);
	await expect(anonima.getByRole('heading', { name: 'Ana convidou você' })).toBeVisible();

	await printFinal(anonima);
	await anonima.context().close();
});

test('[E12] convidar de novo quem já está no quadro, ou já tem convite em aberto, diz o motivo', async ({
	browser,
	playwright
}) => {
	const req = await playwright.request.newContext();
	const ivo = await criarConta(req, 'Ivo');
	await convidar(req, ana, quadroId, ivo);
	await req.dispose();
	const hugo = novoEmail('hugo');

	const pagina = await telaDeMembros(browser);
	await convidarPelaTela(pagina, hugo);
	await expect(pagina.getByText(`Convite enviado para ${hugo}`)).toBeVisible();

	await convidarPelaTela(pagina, hugo);
	await expect(pagina.getByText('já existe um convite pendente para este email')).toBeVisible();

	await convidarPelaTela(pagina, ivo.email);
	await expect(pagina.getByText('esta pessoa já participa do quadro')).toBeVisible();

	// Um convite para o Hugo, e não dois.
	await expect(pagina.getByRole('listitem').filter({ hasText: hugo })).toHaveCount(1);

	await printFinal(pagina);
	await pagina.context().close();
});

// A primeira tela que alguém vê antes de chamar a equipe. Ela descrevia o
// convite da versão anterior — "quem já tem conta entra direto", "convite por
// link" —, o mesmo atalho que a E13 tirou da tela de membros: desde que conhecer
// um email deixou de pôr alguém no quadro, todo mundo entra pelo convite.
test('[E14] a página inicial descreve o convite como ele é', async ({ browser }) => {
	const pagina = await abaSemConta(browser);
	await pagina.goto('/');

	await expect(pagina.getByText('O convite chega por email')).toBeVisible();
	await expect(pagina.getByText('entra direto')).toHaveCount(0);
	await expect(pagina.getByText('Convite por link')).toHaveCount(0);

	await printFinal(pagina);
	await pagina.context().close();
});
