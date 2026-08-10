// O quadro num CELULAR: dedo, tela estreita, sem cursor.
//
// Estes testes existem porque o quadro estava inteiramente quebrado no celular
// sem nenhum teste reclamar. A suíte roda em viewport de desktop com mouse, e
// o mouse não passa por nenhum dos caminhos que estavam com defeito — foram
// TRÊS, independentes, e cada um sozinho já bastava para inutilizar a tela:
//
//   1. o clique que o navegador sintetiza a partir de um toque chega SEM
//      coordenadas, e o guarda de "isto foi arraste?" media justamente por
//      elas: a conta dava NaN e todo toque era descartado como arraste;
//   2. um toque gera DOIS cliques. Corrigido o item 1, o primeiro abria o
//      modal e o segundo caía no fundo escuro que acabara de cobrir o card,
//      fechando-o no mesmo gesto;
//   3. o arraste começava no primeiro contato do dedo, então rolar a lista
//      levantava o card.
//
// Cada teste aqui morre se a sua correção for desfeita — verificado desfazendo
// as três, uma de cada vez.
import { test, expect, devices } from '@playwright/test';
import {
	abaDe,
	criarColuna,
	criarConta,
	criarQuadro,
	TetoDeRequisicoes,
	type Conta
} from './apoio';

// Um aparelho real, com `hasTouch` — é o que faz o Playwright emitir eventos de
// toque em vez de mouse, e sem isso o teste passaria sem tocar no assunto.
test.use({ ...devices['Pixel 5'] });

let ana: Conta;
let quadroId: string;

test.beforeAll(async ({ playwright }) => {
	const req = await playwright.request.newContext();
	try {
		ana = await criarConta(req, 'ana-celular');
		const quadro = await criarQuadro(req, ana, 'Quadro no bolso');
		quadroId = quadro.id;
		// Três colunas: o quadro precisa ser mais largo que a tela para o gesto
		// lateral ter o que rolar.
		const coluna = await criarColuna(req, ana, quadroId, 'A fazer');
		await criarColuna(req, ana, quadroId, 'Fazendo');
		await criarColuna(req, ana, quadroId, 'Feito');
		for (const titulo of ['Primeiro card', 'Segundo card']) {
			const resp = await req.post(
				`${process.env.E2E_API_URL ?? 'http://localhost:8080'}/colunas/${coluna.id}/cards`,
				{
					data: { titulo, descricao: '', cor: '' },
					headers: { Cookie: `${ana.cookie.name}=${ana.cookie.value}` }
				}
			);
			if (!resp.ok()) throw new Error(`criar card: ${resp.status()}`);
		}
	} catch (e) {
		if (e instanceof TetoDeRequisicoes) test.skip(true, e.message);
		throw e;
	} finally {
		await req.dispose();
	}
});

test('um toque no card abre o modal', async ({ browser }) => {
	const contexto = await abaDe(browser, ana);
	const pagina = await contexto.newPage();
	await pagina.goto(`/painel/quadros/${quadroId}`);

	const card = pagina.getByText('Primeiro card');
	await expect(card).toBeVisible();

	// `tap` e não `click`: o clique do Playwright dispara eventos de mouse, que
	// não passam pelo caminho quebrado. O toque passa.
	await card.tap();

	await expect(pagina.getByRole('dialog')).toBeVisible();
	await contexto.close();
});

test('arrastar o dedo pela lista não levanta o card', async ({ browser }) => {
	const contexto = await abaDe(browser, ana);
	const pagina = await contexto.newPage();
	await pagina.goto(`/painel/quadros/${quadroId}`);
	const card = pagina.getByText('Primeiro card');
	await expect(card).toBeVisible();

	const caixa = await card.boundingBox();
	if (!caixa) throw new Error('o card não tem caixa');
	const x = caixa.x + caixa.width / 2;
	const y = caixa.y + caixa.height / 2;

	// Toque de verdade, pelo protocolo do navegador: a API pública do Playwright
	// só sabe TOCAR, e o que está em jogo aqui é o ARRASTAR do dedo.
	const cdp = await contexto.newCDPSession(pagina);
	const dedo = (type: string, ty: number) =>
		cdp.send('Input.dispatchTouchEvent', {
			type,
			touchPoints: type === 'touchEnd' ? [] : [{ x, y: ty }]
		});

	await dedo('touchStart', y);
	// Rápido e longo, como quem rola a lista — bem antes da espera que converte
	// o gesto em arraste.
	for (let i = 1; i <= 6; i++) await dedo('touchMove', y - i * 20);

	// A verificação acontece com o dedo AINDA na tela: é enquanto o gesto corre
	// que o card estaria levantado, e depois do touchEnd a classe já saiu — um
	// teste que só olhasse no fim passaria com o defeito de volta.
	await expect(pagina.locator('.item-arrastado')).toHaveCount(0);

	await dedo('touchEnd', y - 120);
	await expect(pagina.getByText('Primeiro card')).toBeVisible();
	await contexto.close();
});

// A outra metade do estrago do clique fantasma: o modal ABRIA e fechava no
// mesmo toque, porque o segundo clique caía no fundo escuro que acabara de
// cobrir o card. Um toque tem de deixar o modal aberto — e continuar aberto.
test('o modal aberto por toque não se fecha sozinho', async ({ browser }) => {
	const contexto = await abaDe(browser, ana);
	const pagina = await contexto.newPage();
	await pagina.goto(`/painel/quadros/${quadroId}`);
	await pagina.getByText('Segundo card').tap();

	const modal = pagina.getByRole('dialog');
	await expect(modal).toBeVisible();
	await pagina.waitForTimeout(400);
	await expect(modal).toBeVisible();

	// E fechar pelo fundo continua funcionando — a correção não podia trancar
	// ninguém dentro do modal.
	await pagina.touchscreen.tap(10, 10);
	await expect(modal).toBeHidden();
	await contexto.close();
});

// A zona das COLUNAS é outra, com a mesma armadilha: o gesto lateral que
// percorre o quadro passa por cima da alça de arraste da coluna. Sem a espera,
// ele reordenava o quadro em vez de rolar — e o teste do card, que exercita só
// a zona de dentro, não diria nada sobre isto.
//
// O alvo é a ALÇA, e não o cabeçalho: a biblioteca se recusa a arrastar quando
// o toque começa num elemento com `value` — e `<button>` tem. Um teste mirando
// o título da coluna passaria sem nunca chegar perto do defeito.
test('arrastar o dedo pela alça da coluna rola o quadro, não reordena', async ({ browser }) => {
	const contexto = await abaDe(browser, ana);
	const pagina = await contexto.newPage();
	await pagina.goto(`/painel/quadros/${quadroId}`);

	const alca = pagina.locator('[title="Arraste a coluna para reordenar"]').first();
	await expect(alca).toBeVisible();
	const caixa = await alca.boundingBox();
	if (!caixa) throw new Error('a alça não tem caixa');
	const y = caixa.y + caixa.height / 2;
	const x = caixa.x + caixa.width / 2;

	const ordem = () => pagina.locator('section header button').first().innerText();
	const antes = await ordem();

	const cdp = await contexto.newCDPSession(pagina);
	const dedo = (type: string, tx: number) =>
		cdp.send('Input.dispatchTouchEvent', {
			type,
			touchPoints: type === 'touchEnd' ? [] : [{ x: tx, y }]
		});

	await dedo('touchStart', x);
	for (let i = 1; i <= 6; i++) await dedo('touchMove', x - i * 25);

	// Com o dedo ainda na tela: é durante o gesto que a coluna estaria no ar.
	await expect(pagina.locator('.item-arrastado')).toHaveCount(0);
	// E o quadro rolou de fato — o gesto fez o que devia, e não só deixou de
	// fazer o que não devia.
	expect(
		await pagina.evaluate(() => document.querySelector('.overflow-x-auto')!.scrollLeft)
	).toBeGreaterThan(0);

	await dedo('touchEnd', x - 150);
	expect(await ordem()).toBe(antes);
	await contexto.close();
});
