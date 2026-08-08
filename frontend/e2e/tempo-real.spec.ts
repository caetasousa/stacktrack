// O teste que define o projeto: duas pessoas, o mesmo quadro, navegadores
// separados. Uma age e a outra vê — sem recarregar.
//
// O `duas_abas_test.go` do backend já prova o protocolo (handshake, autorização,
// origem, entrega). O que ele NÃO prova é que a tela reage: se o cliente parar
// de aplicar o evento, aquele teste continua verde. É esse pedaço que vive aqui.

import { expect, test, type BrowserContext, type Page } from '@playwright/test';
import { abaDe, convidar, criarColuna, criarConta, criarQuadro, type Conta } from './apoio';

let ana: Conta;
let bruno: Conta;
let boardId: string;
let abaAna: BrowserContext;
let abaBruno: BrowserContext;
let telaAna: Page;
let telaBruno: Page;

// O cenário é montado UMA vez para o arquivo inteiro.
//
// Não é só velocidade: cadastro e login têm teto por IP, e criar contas a cada
// teste derruba a suíte com 429 no meio do caminho — que parece defeito do
// produto e é o rate limiter fazendo o trabalho dele.
test.beforeAll(async ({ browser, playwright }) => {
	const req = await playwright.request.newContext();
	ana = await criarConta(req, 'ana');
	bruno = await criarConta(req, 'bruno');

	const quadro = await criarQuadro(req, ana, 'Tempo real');
	boardId = quadro.id;
	await convidar(req, ana, boardId, bruno);
	await criarColuna(req, ana, boardId, 'A fazer');
	await req.dispose();

	abaAna = await abaDe(browser, ana);
	abaBruno = await abaDe(browser, bruno);
	telaAna = await abaAna.newPage();
	telaBruno = await abaBruno.newPage();

	await telaAna.goto(`/painel/quadros/${boardId}`);
	await telaBruno.goto(`/painel/quadros/${boardId}`);
	// Espera as duas telas terminarem de carregar o quadro.
	await expect(telaAna.getByText('A fazer')).toBeVisible();
	await expect(telaBruno.getByText('A fazer')).toBeVisible();
});

test.afterAll(async () => {
	await abaAna?.close();
	await abaBruno?.close();
});

// A conexão fica de pé sozinha. O indicador só aparece quando ela NÃO está no
// ar, então a ausência dele é a prova de que o WebSocket conectou.
test('a conexão de tempo real fica no ar', async () => {
	await expect(telaBruno.getByText('reconectando')).toHaveCount(0);
	await expect(telaBruno.getByText('conectando')).toHaveCount(0);
});

test('ana cria um card e bruno vê sem recarregar', async () => {
	const titulo = `Card da ana ${Date.now()}`;

	// Bruno não pode ter isso na tela ainda.
	await expect(telaBruno.getByText(titulo)).toHaveCount(0);

	const campo = telaAna.getByLabel('Título do novo card').first();
	await campo.fill(titulo);
	await campo.press('Enter');

	// Quem age vê pelo próprio caminho.
	await expect(telaAna.getByText(titulo)).toBeVisible();

	// E quem assiste vê pelo evento. Nenhum reload é chamado aqui — se o
	// WebSocket não entregasse, isto expiraria.
	await expect(telaBruno.getByText(titulo)).toBeVisible();
});

test('bruno cria uma coluna e ana vê sem recarregar', async () => {
	const titulo = `Coluna do bruno ${Date.now()}`;

	await expect(telaAna.getByText(titulo)).toHaveCount(0);

	await telaBruno.getByRole('button', { name: '+ Nova coluna' }).click();
	const campo = telaBruno.getByLabel('Título da nova coluna');
	await campo.fill(titulo);
	await campo.press('Enter');

	await expect(telaBruno.getByText(titulo)).toBeVisible();
	await expect(telaAna.getByText(titulo)).toBeVisible();
});

// A queda é o estado normal de uma conexão longa. O que não pode é a tela
// mentir: se ela parou de se atualizar sozinha, quem está olhando precisa
// saber, senão confia num quadro velho.
//
// A queda é simulada interceptando o WebSocket com routeWebSocket: o teste
// vira o servidor, deixa a conexão passar para o de verdade e depois a fecha.
// `setOffline` não serve aqui — ele não derruba um socket já estabelecido, que
// só perceberia a rede ausente no ping seguinte, 30 segundos depois.
test('a queda da conexão aparece na tela, e ela volta sozinha', async ({ browser }) => {
	const aba = await abaDe(browser, bruno);
	const tela = await aba.newPage();

	let servidor: { close: () => void } | null = null;
	await tela.routeWebSocket(/\/ws\?board=/, (ws) => {
		// Repassa para a API de verdade: o teste observa a conexão, não a
		// substitui.
		const real = ws.connectToServer();
		servidor = { close: () => real.close() };
	});

	await tela.goto(`/painel/quadros/${boardId}`);
	await expect(tela.getByText('A fazer')).toBeVisible();
	await expect(tela.getByText(/reconectando|conectando/)).toHaveCount(0);

	// O servidor "cai".
	expect(servidor).not.toBeNull();
	servidor!.close();

	await expect(tela.getByText(/reconectando|conectando/)).toBeVisible();

	// E a reconexão traz a tela de volta sozinha — sem F5.
	await expect(tela.getByText(/reconectando|conectando/)).toHaveCount(0, { timeout: 30_000 });

	await aba.close();
});
