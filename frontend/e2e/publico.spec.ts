// O link público de acompanhamento, de ponta a ponta.
//
// É o único teste da suíte que abre uma aba SEM cookie nenhum, e é isso que ele
// existe para provar. Todos os outros partem de `abaDe`, que já injeta a sessão;
// aqui um contexto limpo é o cenário, não um detalhe — se a página só
// funcionasse por causa de um cookie que o navegador já tinha, nada nos testes
// de unidade ou de handler apontaria isso.
//
// Os três fatos cercados são os que decidem se a funcionalidade é segura:
//
//   1. quem tem o link entra sem conta;
//   2. o que ele vê NÃO inclui comentário nem o nome de quem trabalha no card;
//   3. revogar derruba o endereço na hora, e religar não o ressuscita.
import { test, expect, type Browser } from '@playwright/test';
import {
	API,
	APP,
	abaDe,
	criarColuna,
	criarConta,
	criarQuadro,
	TetoDeRequisicoes,
	type Conta
} from './apoio';

let ana: Conta;
let bob: Conta;
let quadroId: string;
let cookieDaAna: string;

/** urlPublica liga o link do quadro e devolve o endereço. */
async function publicar(req: import('@playwright/test').APIRequestContext, id: string) {
	const resp = await req.put(`${API}/boards/${id}/publicacao`, {
		headers: { Cookie: cookieDaAna }
	});
	if (!resp.ok()) throw new Error(`publicar: ${resp.status()} ${await resp.text()}`);
	return ((await resp.json()) as { url: string }).url;
}

/** abaAnonima abre um contexto SEM cookie nenhum — o navegador de quem recebeu o link. */
async function abaAnonima(navegador: Browser) {
	return navegador.newContext({ baseURL: APP });
}

test.beforeAll(async ({ playwright }) => {
	const req = await playwright.request.newContext();
	try {
		ana = await criarConta(req, 'ana-publico');
		bob = await criarConta(req, 'bob-publico');
		cookieDaAna = `${ana.cookie.name}=${ana.cookie.value}`;

		const quadro = await criarQuadro(req, ana, 'Migração do checkout');
		quadroId = quadro.id;
		const coluna = await criarColuna(req, ana, quadroId, 'Em andamento');

		const card = await req.post(`${API}/colunas/${coluna.id}/cards`, {
			data: { titulo: 'Trocar o gateway', descricao: 'combinar a janela com o time', cor: '' },
			headers: { Cookie: cookieDaAna }
		});
		if (!card.ok()) throw new Error(`criar card: ${card.status()} ${await card.text()}`);
		const cardId = ((await card.json()) as { id: string }).id;

		// O quadro ganha um segundo membro, um responsável e um comentário: é o
		// que o teste de vazamento precisa ter para poder não encontrar.
		await req.post(`${API}/boards/${quadroId}/membros`, {
			data: { email: bob.email, papel: 'editor' },
			headers: { Cookie: cookieDaAna }
		});
		// A atribuição é por id de usuário, não por email: quem responde por um
		// card é uma conta, e o email é só como se chega a ela.
		const me = await req.get(`${API}/auth/me`, {
			headers: { Cookie: `${bob.cookie.name}=${bob.cookie.value}` }
		});
		const bobID = ((await me.json()) as { id: string }).id;
		await req.put(`${API}/cards/${cardId}/responsaveis/${bobID}`, {
			headers: { Cookie: cookieDaAna }
		});
		await req.post(`${API}/cards/${cardId}/comentarios`, {
			data: { texto: 'a janela combinada é sexta de madrugada' },
			headers: { Cookie: cookieDaAna }
		});
	} catch (e) {
		if (e instanceof TetoDeRequisicoes) test.skip(true, e.message);
		throw e;
	} finally {
		await req.dispose();
	}
});

test('quem tem o link vê o quadro sem ter conta', async ({ playwright, browser }) => {
	const req = await playwright.request.newContext();
	const url = await publicar(req, quadroId);
	await req.dispose();

	const contexto = await abaAnonima(browser);
	const pagina = await contexto.newPage();
	await pagina.goto(url);

	await expect(pagina.getByRole('heading', { name: 'Migração do checkout' })).toBeVisible();
	await expect(pagina.getByText('Em andamento')).toBeVisible();
	await expect(pagina.getByText('Trocar o gateway')).toBeVisible();
	// Dito com todas as letras, e não deduzido da ausência de botões: quem nunca
	// viu o produto não sabe o que está faltando.
	await expect(pagina.getByText('somente leitura', { exact: true })).toBeVisible();

	await contexto.close();
});

test('a página pública não mostra comentário nem quem trabalha no card', async ({
	playwright,
	browser
}) => {
	const req = await playwright.request.newContext();
	const url = await publicar(req, quadroId);
	await req.dispose();

	const contexto = await abaAnonima(browser);
	const pagina = await contexto.newPage();
	await pagina.goto(url);
	await expect(pagina.getByText('Trocar o gateway')).toBeVisible();

	// O HTML inteiro, e não um seletor: um vazamento não vem com um rótulo que
	// se possa consultar. Se o nome ou o comentário aparecerem em QUALQUER lugar
	// da página — inclusive num atributo `title` —, o teste cai.
	const html = await pagina.content();
	expect(html).not.toContain('a janela combinada é sexta de madrugada');
	expect(html).not.toContain(bob.nome);
	expect(html).not.toContain(bob.email);

	await contexto.close();
});

test('revogar derruba o link, e republicar não o ressuscita', async ({ playwright, browser }) => {
	const req = await playwright.request.newContext();
	const antigo = await publicar(req, quadroId);

	const revogar = await req.delete(`${API}/boards/${quadroId}/publicacao`, {
		headers: { Cookie: cookieDaAna }
	});
	expect(revogar.status()).toBe(204);

	const contexto = await abaAnonima(browser);
	const pagina = await contexto.newPage();
	await pagina.goto(antigo);
	await expect(pagina.getByText('Este link não está mais disponível')).toBeVisible();

	// Religar dá um endereço NOVO. O antigo continua morto — é o que separa
	// revogar de esconder.
	const novo = await publicar(req, quadroId);
	expect(novo).not.toBe(antigo);

	await pagina.goto(antigo);
	await expect(pagina.getByText('Este link não está mais disponível')).toBeVisible();

	await pagina.goto(novo);
	await expect(pagina.getByRole('heading', { name: 'Migração do checkout' })).toBeVisible();

	await contexto.close();
	await req.dispose();
});

test('o dono liga e desliga o compartilhamento pela tela', async ({ playwright, browser }) => {
	// Estado conhecido: o teste anterior pode ter deixado o quadro publicado.
	const req = await playwright.request.newContext();
	await req.delete(`${API}/boards/${quadroId}/publicacao`, { headers: { Cookie: cookieDaAna } });
	await req.dispose();

	const contexto = await abaDe(browser, ana);
	const pagina = await contexto.newPage();
	await pagina.goto(`/painel/quadros/${quadroId}`);

	await pagina.getByRole('button', { name: 'Compartilhar' }).click();
	await pagina.getByRole('button', { name: 'Ativar o link público' }).click();

	const endereco = pagina.getByLabel('Endereço público do quadro');
	await expect(endereco).toBeVisible();
	await expect(endereco).toHaveValue(/\/publico\/.+/);

	// Fechado o painel, o quadro precisa AVISAR que está à vista de fora — é o
	// que quem escreve num card tem de saber antes de escrever.
	await pagina.getByRole('button', { name: 'Fechar' }).click();
	await expect(pagina.getByText('público', { exact: true })).toBeVisible();

	await pagina.getByRole('button', { name: 'Compartilhar' }).click();
	await pagina.getByRole('button', { name: 'Desativar o link público' }).click();
	await pagina.getByRole('button', { name: 'Desativar o link', exact: true }).click();
	await expect(pagina.getByRole('button', { name: 'Ativar o link público' })).toBeVisible();

	await contexto.close();
});
