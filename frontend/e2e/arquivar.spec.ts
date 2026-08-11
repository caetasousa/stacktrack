// Arquivar ponta a ponta: o que a pessoa faz, na stack inteira.
//
// A fase 13 existe porque apagar é definitivo — o DELETE de um card leva por
// cascata comentários, checklists, anexos, responsáveis e etiquetas aplicadas.
// Estes testes cobram o caminho de volta, que é a razão de a fase existir.
import { expect, test } from '@playwright/test';
import {
	abaDe,
	criarColuna,
	criarConta,
	criarQuadro,
	TetoDeRequisicoes,
	type Conta
} from './apoio';

const API = process.env.E2E_API_URL ?? 'http://localhost:8080';

let ana: Conta;
let quadroId: string;

test.beforeAll(async ({ playwright }) => {
	const req = await playwright.request.newContext();
	try {
		ana = await criarConta(req, 'ana-arquivo');
		const quadro = await criarQuadro(req, ana, 'Arquivo');
		quadroId = quadro.id;
		const coluna = await criarColuna(req, ana, quadroId, 'A fazer');
		await criarColuna(req, ana, quadroId, 'Depois');
		for (const titulo of ['Fica no quadro', 'Vai para o arquivo']) {
			const resp = await req.post(`${API}/colunas/${coluna.id}/cards`, {
				data: { titulo, descricao: '', cor: '' },
				headers: { Cookie: `${ana.cookie.name}=${ana.cookie.value}` }
			});
			if (!resp.ok()) throw new Error(`criar card: ${resp.status()}`);
		}
	} catch (e) {
		if (e instanceof TetoDeRequisicoes) test.skip(true, e.message);
		throw e;
	} finally {
		await req.dispose();
	}
});

test('arquivar tira o card do quadro, e a tela de arquivados o devolve ao mesmo lugar', async ({
	browser
}) => {
	const contexto = await abaDe(browser, ana);
	const pagina = await contexto.newPage();
	await pagina.goto(`/painel/quadros/${quadroId}`);

	const alvo = pagina.locator('li', { hasText: 'Vai para o arquivo' });
	await expect(alvo).toBeVisible();

	// Arquivar NÃO pergunta: é reversível, e a tela de arquivados é o desfazer.
	await alvo.getByLabel('Arquivar card').click();
	await expect(pagina.getByRole('alertdialog')).toBeHidden();
	await expect(alvo).toBeHidden();
	await expect(pagina.getByText('Fica no quadro')).toBeVisible();

	// O arquivo mostra o card com a coluna de ONDE ele veio — é onde ele cai
	// ao voltar.
	await pagina.getByRole('link', { name: 'Arquivados' }).click();
	const linha = pagina.locator('li', { hasText: 'Vai para o arquivo' });
	await expect(linha).toBeVisible();
	await expect(linha.getByText('de A fazer')).toBeVisible();

	await linha.getByRole('button', { name: 'Devolver ao quadro' }).click();
	await expect(linha).toBeHidden();

	// E voltou ao quadro, na coluna de origem.
	await pagina.goto(`/painel/quadros/${quadroId}`);
	await expect(pagina.getByText('Vai para o arquivo')).toBeVisible();
	await contexto.close();
});

test('arquivar a coluna leva os cards dela, e devolvê-la os traz de volta', async ({ browser }) => {
	const contexto = await abaDe(browser, ana);
	const pagina = await contexto.newPage();
	await pagina.goto(`/painel/quadros/${quadroId}`);

	const coluna = pagina.locator('section', { hasText: 'A fazer' }).first();
	await expect(coluna.getByText('Fica no quadro')).toBeVisible();

	await coluna.getByLabel('Arquivar coluna').click();
	await expect(pagina.getByText('Fica no quadro')).toBeHidden();
	// A outra coluna continua lá: arquivar uma não afeta as demais.
	await expect(pagina.getByRole('button', { name: 'Depois' })).toBeVisible();

	await pagina.goto(`/painel/quadros/${quadroId}/arquivados`);
	const linha = pagina.locator('li', { hasText: 'A fazer' });
	await expect(linha).toBeVisible();
	// Os cards de uma coluna arquivada NÃO aparecem na lista de cards: eles
	// voltam com ela, e listá-los sugeriria que precisam ser devolvidos um a um.
	await expect(pagina.locator('li', { hasText: 'Fica no quadro' })).toHaveCount(0);

	await linha.getByRole('button', { name: 'Devolver ao quadro' }).click();
	await pagina.goto(`/painel/quadros/${quadroId}`);
	await expect(pagina.getByText('Fica no quadro')).toBeVisible();
	await contexto.close();
});

// Apagar de vez é o único caminho sem volta, e o único que pergunta.
test('apagar de vez, a partir do arquivo, pede confirmação', async ({ browser }) => {
	const contexto = await abaDe(browser, ana);
	const pagina = await contexto.newPage();
	await pagina.goto(`/painel/quadros/${quadroId}`);

	const titulo = `Descartável ${Date.now()}`;
	const campoNovo = pagina.getByLabel('Título do novo card').first();
	await campoNovo.fill(titulo);
	await campoNovo.press('Enter');

	const card = pagina.locator('li', { hasText: titulo });
	await expect(card).toBeVisible();
	await card.getByLabel('Arquivar card').click();

	await pagina.goto(`/painel/quadros/${quadroId}/arquivados`);
	const linha = pagina.locator('li', { hasText: titulo });
	await expect(linha).toBeVisible();

	await linha.getByRole('button', { name: 'Apagar de vez' }).click();
	const confirmacao = pagina.getByRole('alertdialog');
	await expect(confirmacao).toBeVisible();
	await expect(confirmacao.getByText(/não tem desfazer/i)).toBeVisible();

	// Cancelar não apaga.
	await confirmacao.getByRole('button', { name: 'Cancelar' }).click();
	await expect(linha).toBeVisible();

	await linha.getByRole('button', { name: 'Apagar de vez' }).click();
	await pagina.getByRole('alertdialog').getByRole('button', { name: 'Apagar de vez' }).click();
	await expect(linha).toBeHidden();
	await contexto.close();
});
