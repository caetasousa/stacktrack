// A auditoria: quem moveu o quê.
//
// O caso que a motiva é concreto — alguém arrasta trinta cards e ninguém sabe
// quem foi. A informação sempre esteve no log de eventos; o que faltava era
// alcançá-la sem abrir card por card.
//
// Este teste usa DUAS contas de propósito. Com uma só, "quem moveu" seria
// sempre a mesma pessoa, e um selo que mostrasse o autor errado — o dono do
// quadro em vez de quem arrastou, por exemplo — passaria despercebido.
import { test, expect } from '@playwright/test';
import {
	API,
	abaDe,
	criarColuna,
	criarConta,
	criarQuadro,
	TetoDeRequisicoes,
	type Conta
} from './apoio';

let ana: Conta;
let bruno: Conta;
let quadroId: string;
let aFazer: string;
let pronto: string;
let cardID: string;

test.beforeAll(async ({ playwright }) => {
	const req = await playwright.request.newContext();
	try {
		ana = await criarConta(req, 'ana-auditoria');
		bruno = await criarConta(req, 'bruno-auditoria');
		const cookieDaAna = `${ana.cookie.name}=${ana.cookie.value}`;

		const quadro = await criarQuadro(req, ana, 'Quadro auditado');
		quadroId = quadro.id;
		aFazer = (await criarColuna(req, ana, quadroId, 'A fazer')).id;
		pronto = (await criarColuna(req, ana, quadroId, 'Pronto')).id;

		await req.post(`${API}/boards/${quadroId}/membros`, {
			data: { email: bruno.email, papel: 'editor' },
			headers: { Cookie: cookieDaAna }
		});

		const card = await req.post(`${API}/colunas/${aFazer}/cards`, {
			data: { titulo: 'Trocar o gateway', descricao: '', cor: '' },
			headers: { Cookie: cookieDaAna }
		});
		if (!card.ok()) throw new Error(`criar card: ${card.status()} ${await card.text()}`);
		cardID = ((await card.json()) as { id: string }).id;
	} catch (e) {
		if (e instanceof TetoDeRequisicoes) test.skip(true, e.message);
		throw e;
	} finally {
		await req.dispose();
	}
});

/** mover arrasta o card pela API, como a conta informada. */
async function mover(
	req: import('@playwright/test').APIRequestContext,
	quem: Conta,
	colunaId: string
) {
	const resp = await req.patch(`${API}/cards/${cardID}/mover`, {
		data: { colunaId },
		headers: { Cookie: `${quem.cookie.name}=${quem.cookie.value}` }
	});
	if (!resp.ok()) throw new Error(`mover: ${resp.status()} ${await resp.text()}`);
}

// O selo tem de nomear QUEM ARRASTOU, e não quem é dono do quadro nem quem
// criou o card. As três pessoas são diferentes aqui de propósito.
test('o card mostra quem o moveu por último, e não quem o criou', async ({
	playwright,
	browser
}) => {
	const req = await playwright.request.newContext();
	await mover(req, bruno, pronto);
	await req.dispose();

	// A ANA é quem olha o quadro — o selo é sobre o bruno.
	const contexto = await abaDe(browser, ana);
	const pagina = await contexto.newPage();
	await pagina.goto(`/painel/quadros/${quadroId}`);

	const card = pagina.locator('[role="button"]').filter({ hasText: 'Trocar o gateway' });
	await expect(card).toContainText('bruno-auditoria');
	// O nome de quem só criou o card não pode aparecer no selo de movimentação.
	await expect(card.locator('..')).not.toContainText('ana-auditoria moveu');

	await contexto.close();
});

test('a tela de movimentações lista quem mexeu em quê', async ({ playwright, browser }) => {
	const req = await playwright.request.newContext();
	await mover(req, ana, aFazer);
	await req.dispose();

	const contexto = await abaDe(browser, ana);
	const pagina = await contexto.newPage();
	await pagina.goto(`/painel/quadros/${quadroId}`);
	await pagina.getByRole('link', { name: 'Movimentações' }).click();

	await expect(pagina.getByRole('heading', { name: 'Movimentações' })).toBeVisible();
	// Os dois movimentos aparecem, cada um com o nome de quem o fez.
	await expect(pagina.getByText('moveu "Trocar o gateway" de A fazer para Pronto')).toBeVisible();
	await expect(pagina.getByText('moveu "Trocar o gateway" de Pronto para A fazer')).toBeVisible();

	await contexto.close();
});

// O filtro por pessoa é o que transforma a lista em auditoria: a pergunta não é
// "o que aconteceu", é "o que ESTA pessoa fez".
test('o filtro por pessoa isola o que cada uma fez', async ({ browser }) => {
	const contexto = await abaDe(browser, ana);
	const pagina = await contexto.newPage();
	await pagina.goto(`/painel/quadros/${quadroId}/movimentacoes`);
	await expect(pagina.getByText('de A fazer para Pronto')).toBeVisible();

	await pagina.getByLabel('Pessoa').selectOption({ label: 'bruno-auditoria' });

	// Só o movimento do bruno sobra: ele levou o card para Pronto, a ana o trouxe
	// de volta.
	await expect(pagina.getByText('de A fazer para Pronto')).toBeVisible();
	await expect(pagina.getByText('de Pronto para A fazer')).toHaveCount(0);

	await contexto.close();
});

// Um leitor audita. Acompanhar o que aconteceu é ver, não mexer — e um leitor
// que não pudesse auditar teria de virar editor para descobrir quem bagunçou,
// que é o contrário do que se quer.
test('quem só lê o quadro também consegue auditar', async ({ playwright, browser }) => {
	const req = await playwright.request.newContext();
	let carla: Conta;
	try {
		carla = await criarConta(req, 'carla-auditoria');
	} catch (e) {
		await req.dispose();
		if (e instanceof TetoDeRequisicoes) test.skip(true, e.message);
		throw e;
	}
	await req.post(`${API}/boards/${quadroId}/membros`, {
		data: { email: carla.email, papel: 'leitor' },
		headers: { Cookie: `${ana.cookie.name}=${ana.cookie.value}` }
	});
	await req.dispose();

	const contexto = await abaDe(browser, carla);
	const pagina = await contexto.newPage();
	await pagina.goto(`/painel/quadros/${quadroId}/movimentacoes`);

	await expect(pagina.getByText('moveu "Trocar o gateway"').first()).toBeVisible();

	await contexto.close();
});
