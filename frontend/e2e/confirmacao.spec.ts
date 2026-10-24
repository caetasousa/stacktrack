// Confirmação de email e convites na conta: o convite deixa de existir só no
// email e aparece no painel de quem foi convidado — mas só depois de a conta
// provar que o email é dela.
//
// A ordem importa, e é o que estes testes guardam. Sem confirmação, qualquer um
// cria uma conta com o email de outra pessoa; se o painel mostrasse os convites
// daquele email, bastaria isso para entrar no quadro dela sem nunca ter visto o
// link.
//
// Os cenários estão em CENARIOS.md, área G.
import { test, expect, type APIRequestContext, type Browser, type Page } from '@playwright/test';
import {
	API,
	APP,
	ASSUNTO,
	abaDe,
	confirmarConta,
	convidarPelaApi,
	criarConta,
	criarQuadro,
	emailsPara,
	linkDeConfirmacao,
	linkDoConvite,
	novoEmail,
	printFinal,
	TetoDeRequisicoes,
	type Conta
} from './apoio';

let ana: Conta;

test.beforeAll(async ({ playwright }) => {
	const req = await playwright.request.newContext();
	try {
		ana = await criarConta(req, 'Ana');
	} catch (e) {
		if (e instanceof TetoDeRequisicoes) test.skip(true, e.message);
		throw e;
	} finally {
		await req.dispose();
	}
});

async function contaNova(req: APIRequestContext, nome: string): Promise<Conta> {
	try {
		return await criarConta(req, nome);
	} catch (e) {
		if (e instanceof TetoDeRequisicoes) test.skip(true, e.message);
		throw e;
	}
}

/** quadroDaAnaComConviteA cria um quadro da Ana e convida o email informado. */
async function quadroDaAnaComConviteA(req: APIRequestContext, email: string, titulo: string) {
	const quadro = await criarQuadro(req, ana, titulo);
	await convidarPelaApi(req, ana, quadro.id, email);
	return quadro;
}

async function painelDe(navegador: Browser, conta: Conta): Promise<Page> {
	const contexto = await abaDe(navegador, conta);
	const pagina = await contexto.newPage();
	await pagina.goto('/painel');
	return pagina;
}

test('[G1] o cadastro manda o email de confirmação, e o link confirma a conta', async ({
	browser,
	playwright
}) => {
	const email = novoEmail('helena');
	const contexto = await browser.newContext({ baseURL: APP });
	const pagina = await contexto.newPage();
	await pagina.goto('/cadastro');
	await pagina.getByLabel('Nome', { exact: true }).fill('Helena');
	await pagina.getByLabel('Email', { exact: true }).fill(email);
	await pagina.getByLabel('Senha', { exact: true }).fill('uma senha comprida da Helena');
	await pagina.getByRole('button', { name: 'Criar conta' }).click();
	await expect(pagina).toHaveURL(/\/painel$/);
	await expect(pagina.getByText('Confirme seu email')).toBeVisible();

	const req = await playwright.request.newContext();
	const [mensagem] = await emailsPara(req, email, ASSUNTO.confirmacao);
	await req.dispose();

	await pagina.goto(linkDeConfirmacao(mensagem));
	await expect(pagina.getByRole('heading', { name: 'Email confirmado' })).toBeVisible();
	await pagina.getByRole('link', { name: 'Ir para o painel' }).click();
	await expect(pagina).toHaveURL(/\/painel$/);
	await expect(pagina.getByText('Confirme seu email')).toHaveCount(0);

	await printFinal(pagina);
	await contexto.close();
});

test('[G2] a conta que não confirmou o email não vê os convites para ele', async ({
	browser,
	playwright
}) => {
	const req = await playwright.request.newContext();
	const igor = await contaNova(req, 'Igor');
	await quadroDaAnaComConviteA(req, igor.email, 'Quadro que o Igor não pode ver');
	await req.dispose();

	const pagina = await painelDe(browser, igor);
	await expect(pagina.getByText('Confirme seu email')).toBeVisible();
	await expect(pagina.getByRole('region', { name: 'Convites para você' })).toHaveCount(0);
	await expect(pagina.getByText('Quadro que o Igor não pode ver')).toHaveCount(0);

	await printFinal(pagina);
	await pagina.context().close();
});

test('[G3][G4] a conta confirmada vê o convite no painel e entra no quadro ao aceitar', async ({
	browser,
	playwright
}) => {
	const req = await playwright.request.newContext();
	const julia = await contaNova(req, 'Julia');
	await confirmarConta(req, julia);
	const quadro = await quadroDaAnaComConviteA(req, julia.email, 'Planejamento da Julia');
	await req.dispose();

	const pagina = await painelDe(browser, julia);
	const convites = pagina.getByRole('region', { name: 'Convites para você' });
	await expect(convites.getByText('Planejamento da Julia')).toBeVisible();
	await expect(convites.getByText(/de Ana/)).toBeVisible();

	await convites.getByRole('button', { name: 'Aceitar' }).click();
	await expect(pagina).toHaveURL(new RegExp(`/painel/quadros/${quadro.id}`));

	await pagina.goto('/painel');
	await expect(
		pagina
			.getByRole('region', { name: 'Compartilhados com você' })
			.getByText('Planejamento da Julia')
	).toBeVisible();
	await expect(pagina.getByRole('region', { name: 'Convites para você' })).toHaveCount(0);

	await printFinal(pagina);
	await pagina.context().close();
});

test('[G5][G7] recusar tira o convite do painel, mata o link do email e aparece para quem convidou', async ({
	browser,
	playwright
}) => {
	const req = await playwright.request.newContext();
	const kleber = await contaNova(req, 'Kleber');
	await confirmarConta(req, kleber);
	const quadro = await quadroDaAnaComConviteA(req, kleber.email, 'Reforma do Kleber');
	const [emailDoConvite] = await emailsPara(req, kleber.email, ASSUNTO.convite);
	await req.dispose();

	const pagina = await painelDe(browser, kleber);
	const convites = pagina.getByRole('region', { name: 'Convites para você' });
	await convites.getByRole('button', { name: 'Recusar' }).click();
	await pagina.getByRole('alertdialog').getByRole('button', { name: 'Recusar o convite' }).click();
	await expect(pagina.getByRole('region', { name: 'Convites para você' })).toHaveCount(0);

	// O link do email morreu junto.
	await pagina.goto(linkDoConvite(emailDoConvite));
	await expect(pagina.getByText('Convite inválido ou expirado')).toBeVisible();

	// E quem convidou fica sabendo, pelo histórico do quadro.
	const daAna = await abaDe(browser, ana);
	const historico = await daAna.newPage();
	await historico.goto(`/painel/quadros/${quadro.id}/historico`);
	await expect(historico.getByText(/recusou o convite/)).toBeVisible();

	await printFinal(historico);
	await daAna.close();
	await pagina.context().close();
});

test('[G6] o convite revogado sai do painel de quem foi convidado', async ({
	browser,
	playwright
}) => {
	const req = await playwright.request.newContext();
	const lia = await contaNova(req, 'Lia');
	await confirmarConta(req, lia);
	const quadro = await criarQuadro(req, ana, 'Quadro revogado da Lia');
	await convidarPelaApi(req, ana, quadro.id, lia.email);

	const pagina = await painelDe(browser, lia);
	await expect(
		pagina.getByRole('region', { name: 'Convites para você' }).getByText('Quadro revogado da Lia')
	).toBeVisible();

	// A Ana revoga.
	const lista = await req.get(`${API}/boards/${quadro.id}/membros`, {
		headers: { Cookie: `${ana.cookie.name}=${ana.cookie.value}` }
	});
	const { convites } = (await lista.json()) as { convites: { id: string }[] };
	const revogar = await req.delete(`${API}/boards/${quadro.id}/convites/${convites[0].id}`, {
		headers: { Cookie: `${ana.cookie.name}=${ana.cookie.value}` }
	});
	expect(revogar.status()).toBe(204);
	await req.dispose();

	await pagina.reload();
	await expect(pagina.getByText('Quadro revogado da Lia')).toHaveCount(0);

	await printFinal(pagina);
	await pagina.context().close();
});
