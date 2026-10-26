// Empresas: a entrada pelo domínio do email. O dono abre o quadro a quem tem
// email confirmado do domínio dele, e a colega o encontra no painel e entra
// sem convite.
//
// Cada teste inventa uma empresa só dele (um domínio novo): os quadros abertos
// de um teste não podem aparecer no painel de outro.
//
// Os cenários estão em CENARIOS.md, área O.
import { test, expect, type APIRequestContext, type Browser, type Page } from '@playwright/test';
import {
	API,
	abaDe,
	cabecalhoDe,
	confirmarConta,
	criarConta,
	criarQuadro,
	idDe,
	printFinal,
	TetoDeRequisicoes,
	type Conta
} from './apoio';

function empresaNova(): string {
	return `empresa-${Math.random().toString(36).slice(2, 10)}.com.br`;
}

/** contaConfirmada cria a conta com o domínio pedido e confirma o email dela. */
async function contaConfirmada(
	req: APIRequestContext,
	nome: string,
	dominio: string
): Promise<Conta> {
	try {
		const conta = await criarConta(req, nome, dominio);
		await confirmarConta(req, conta);
		return conta;
	} catch (e) {
		if (e instanceof TetoDeRequisicoes) test.skip(true, e.message);
		throw e;
	}
}

async function paginaDe(navegador: Browser, conta: Conta, caminho: string): Promise<Page> {
	const pagina = await (await abaDe(navegador, conta)).newPage();
	await pagina.goto(caminho);
	return pagina;
}

/** abrirPelaTela abre o quadro à empresa na tela de membros, com o papel pedido. */
async function abrirPelaTela(pagina: Page, papel: 'Editor' | 'Leitor', dominio: string) {
	await pagina.getByLabel('Papel de quem entra pela empresa').selectOption({ label: papel });
	await pagina.getByRole('button', { name: `Abrir para @${dominio}` }).click();
	await expect(pagina.getByText(`@${dominio}`, { exact: false }).first()).toBeVisible();
	await expect(pagina.getByRole('button', { name: 'Fechar à empresa' })).toBeVisible();
}

async function abrirPelaApi(req: APIRequestContext, dono: Conta, boardId: string) {
	const resp = await req.put(`${API}/boards/${boardId}/dominio`, {
		data: { papel: 'editor' },
		headers: cabecalhoDe(dono)
	});
	expect(resp.status()).toBe(200);
}

async function entrarPelaApi(req: APIRequestContext, quem: Conta, boardId: string) {
	return req.post(`${API}/boards/${boardId}/entrar`, { headers: cabecalhoDe(quem) });
}

test('[O1][O2] a colega encontra o quadro aberto no painel e entra com o papel escolhido', async ({
	browser,
	playwright
}) => {
	const empresa = empresaNova();
	const req = await playwright.request.newContext();
	const ana = await contaConfirmada(req, 'Ana', empresa);
	const bruno = await contaConfirmada(req, 'Bruno', empresa);
	const quadro = await criarQuadro(req, ana, 'Vendas da empresa');

	const membrosDaAna = await paginaDe(browser, ana, `/painel/quadros/${quadro.id}/membros`);
	await abrirPelaTela(membrosDaAna, 'Leitor', empresa);

	const painelDoBruno = await paginaDe(browser, bruno, '/painel');
	const daEmpresa = painelDoBruno.getByRole('region', { name: 'Da sua empresa' });
	await expect(daEmpresa.getByText('Vendas da empresa')).toBeVisible();
	await expect(daEmpresa.getByText('você entra como leitor')).toBeVisible();
	await daEmpresa.getByRole('button', { name: 'Entrar' }).click();
	await expect(painelDoBruno).toHaveURL(new RegExp(`/painel/quadros/${quadro.id}$`));

	// O papel gravado é o que a dona escolheu — e é o que a API devolve a ele.
	const detalhe = await req.get(`${API}/boards/${quadro.id}`, { headers: cabecalhoDe(bruno) });
	expect(((await detalhe.json()) as { papel: string }).papel).toBe('leitor');

	// A dona vê quem entrou, e por onde.
	await membrosDaAna.reload();
	await expect(membrosDaAna.getByText(bruno.email, { exact: false })).toBeVisible();
	const historico = await paginaDe(browser, ana, `/painel/quadros/${quadro.id}/historico`);
	await expect(historico.getByText(`entrou pelo domínio @${empresa} como leitor`)).toBeVisible();

	await printFinal(painelDoBruno);
	await req.dispose();
});

test('[O3] outro domínio e email não confirmado não veem nem entram', async ({
	browser,
	playwright
}) => {
	const empresa = empresaNova();
	const req = await playwright.request.newContext();
	const ana = await contaConfirmada(req, 'Ana', empresa);
	const carla = await contaConfirmada(req, 'Carla', empresaNova());
	let davi: Conta;
	try {
		davi = await criarConta(req, 'Davi', empresa);
	} catch (e) {
		if (e instanceof TetoDeRequisicoes) test.skip(true, e.message);
		throw e;
	}
	const quadro = await criarQuadro(req, ana, 'Quadro só da empresa');
	await abrirPelaApi(req, ana, quadro.id);

	for (const conta of [carla, davi]) {
		const painel = await paginaDe(browser, conta, '/painel');
		await expect(painel.getByRole('heading', { name: 'Quadros', exact: true })).toBeVisible();
		await expect(painel.getByText('Quadro só da empresa')).toHaveCount(0);
		expect((await entrarPelaApi(req, conta, quadro.id)).status()).toBe(404);
		if (conta === davi) await printFinal(painel);
	}
	await req.dispose();
});

test('[O4] o quadro de quem só tem email pessoal não pode ser aberto', async ({
	browser,
	playwright
}) => {
	const req = await playwright.request.newContext();
	const fabi = await contaConfirmada(req, 'Fabi', 'gmail.com');
	const quadro = await criarQuadro(req, fabi, 'Quadro pessoal');

	const membros = await paginaDe(browser, fabi, `/painel/quadros/${quadro.id}/membros`);
	await expect(
		membros.getByText('Emails pessoais (Gmail, Outlook…) não podem ser abertos')
	).toBeVisible();
	await expect(membros.getByRole('button', { name: /Abrir para/ })).toHaveCount(0);

	const resp = await req.put(`${API}/boards/${quadro.id}/dominio`, {
		data: { papel: 'editor' },
		headers: cabecalhoDe(fabi)
	});
	expect(resp.status()).toBe(422);

	await printFinal(membros);
	await req.dispose();
});

test('[O5] quem a dona removeu não volta sozinho pelo domínio', async ({ browser, playwright }) => {
	const empresa = empresaNova();
	const req = await playwright.request.newContext();
	const ana = await contaConfirmada(req, 'Ana', empresa);
	const bruno = await contaConfirmada(req, 'Bruno', empresa);
	const quadro = await criarQuadro(req, ana, 'Quadro com saída');
	await abrirPelaApi(req, ana, quadro.id);
	expect((await entrarPelaApi(req, bruno, quadro.id)).status()).toBe(200);

	const remocao = await req.delete(`${API}/boards/${quadro.id}/membros/${await idDe(req, bruno)}`, {
		headers: cabecalhoDe(ana)
	});
	expect(remocao.status()).toBe(204);

	const painel = await paginaDe(browser, bruno, '/painel');
	await expect(painel.getByRole('heading', { name: 'Quadros', exact: true })).toBeVisible();
	await expect(painel.getByText('Quadro com saída')).toHaveCount(0);
	expect((await entrarPelaApi(req, bruno, quadro.id)).status()).toBe(404);

	await printFinal(painel);
	await req.dispose();
});

test('[O6] fechar o quadro à empresa o esconde de quem não entrou', async ({
	browser,
	playwright
}) => {
	const empresa = empresaNova();
	const req = await playwright.request.newContext();
	const ana = await contaConfirmada(req, 'Ana', empresa);
	const carla = await contaConfirmada(req, 'Carla', empresa);
	const quadro = await criarQuadro(req, ana, 'Quadro que fecha');
	await abrirPelaApi(req, ana, quadro.id);

	const membros = await paginaDe(browser, ana, `/painel/quadros/${quadro.id}/membros`);
	await membros.getByRole('button', { name: 'Fechar à empresa' }).click();
	await membros
		.getByRole('alertdialog')
		.getByRole('button', { name: 'Fechar', exact: true })
		.click();
	await expect(membros.getByRole('button', { name: `Abrir para @${empresa}` })).toBeVisible();

	const painel = await paginaDe(browser, carla, '/painel');
	await expect(painel.getByRole('heading', { name: 'Quadros', exact: true })).toBeVisible();
	await expect(painel.getByText('Quadro que fecha')).toHaveCount(0);
	expect((await entrarPelaApi(req, carla, quadro.id)).status()).toBe(404);

	await printFinal(membros);
	await req.dispose();
});
