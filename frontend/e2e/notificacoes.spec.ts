// Notificações: o sino do cabeçalho, o email de cada aviso e o link que o email
// leva. A notificação é gravada no banco e o stream (SSE) só avisa as telas
// abertas de que a lista mudou — estes testes conferem as duas pontas pela
// tela: o número que muda sem recarregar, e o email que chega ao Mailpit com um
// link que diz a verdade sobre o aviso.
//
// Os cenários estão em CENARIOS.md, área N.
import { test, expect, type APIRequestContext, type Browser, type Page } from '@playwright/test';
import {
	API,
	APP,
	ASSUNTO,
	abaDe,
	atribuir,
	cabecalhoDe,
	confirmarConta,
	convidar,
	convidarPelaApi,
	criarCard,
	criarColuna,
	criarConta,
	criarQuadro,
	definirPrazo,
	desatribuir,
	emailsPara,
	idDe,
	linkDaNotificacao,
	printFinal,
	TetoDeRequisicoes,
	type Conta
} from './apoio';

async function contaNova(req: APIRequestContext, nome: string): Promise<Conta> {
	try {
		return await criarConta(req, nome);
	} catch (e) {
		if (e instanceof TetoDeRequisicoes) test.skip(true, e.message);
		throw e;
	}
}

/**
 * quadroComBruno monta o cenário de quase todos os testes: um quadro da Ana,
 * com uma coluna e um card, e o Bruno dentro dele como editor.
 */
async function quadroComBruno(req: APIRequestContext, titulo: string) {
	const ana = await contaNova(req, 'Ana');
	const bruno = await contaNova(req, 'Bruno');
	const quadro = await criarQuadro(req, ana, `Quadro ${titulo}`);
	await convidar(req, ana, quadro.id, bruno);
	const coluna = await criarColuna(req, ana, quadro.id, 'A fazer');
	const card = await criarCard(req, ana, coluna.id, titulo);
	return { ana, bruno, brunoId: await idDe(req, bruno), quadro, coluna, card };
}

async function telaDe(navegador: Browser, conta: Conta, caminho = '/painel'): Promise<Page> {
	const contexto = await abaDe(navegador, conta);
	const pagina = await contexto.newPage();
	await pagina.goto(caminho);
	return pagina;
}

const sino = (pagina: Page) => pagina.getByRole('button', { name: /^Notificações/ });
const listaDoSino = (pagina: Page) => pagina.getByRole('dialog', { name: 'Notificações' });

test('[N1] a pessoa posta como responsável vê o sino mudar sem recarregar', async ({
	browser,
	playwright
}) => {
	const req = await playwright.request.newContext();
	const c = await quadroComBruno(req, 'Migrar o banco');

	const tela = await telaDe(browser, c.bruno);
	await expect(sino(tela)).toHaveAccessibleName('Notificações');

	await atribuir(req, c.ana, c.card.id, c.brunoId);
	await req.dispose();

	// Sem nenhum reload chamado pelo teste: quem muda o número é o stream.
	await expect(sino(tela)).toHaveAccessibleName('Notificações, 1 não lida');
	await sino(tela).click();
	await expect(
		listaDoSino(tela).getByText('pôs você como responsável por "Migrar o banco"')
	).toBeVisible();
	await expect(listaDoSino(tela).getByText('Ana', { exact: true })).toBeVisible();

	await printFinal(tela);
	await tela.context().close();
});

test('[N2] o email da atribuição chega com um link que abre o card', async ({
	browser,
	playwright
}) => {
	const req = await playwright.request.newContext();
	const c = await quadroComBruno(req, 'Revisar o contrato');
	await atribuir(req, c.ana, c.card.id, c.brunoId);
	const [email] = await emailsPara(req, c.bruno.email, ASSUNTO.atribuicao);
	await req.dispose();

	expect(email.assunto).toContain('Ana');
	expect(email.assunto).toContain('Revisar o contrato');

	const tela = await telaDe(browser, c.bruno);
	await expect(sino(tela)).toHaveAccessibleName('Notificações, 1 não lida');
	await tela.goto(linkDaNotificacao(email));

	// O card abre por cima do quadro, e o parâmetro sai da URL.
	await expect(tela).toHaveURL(new RegExp(`/painel/quadros/${c.quadro.id}$`));
	const modal = tela.getByRole('dialog', { name: 'Detalhes do card' });
	await expect(modal.getByRole('heading', { name: 'Revisar o contrato' })).toBeVisible();
	// Abrir pelo email é ler.
	await expect(sino(tela)).toHaveAccessibleName('Notificações');

	await printFinal(tela);
	await tela.context().close();
});

test('[N3] desfazer a atribuição tira do sino, e o link do email avisa que não vale mais', async ({
	browser,
	playwright
}) => {
	const req = await playwright.request.newContext();
	const c = await quadroComBruno(req, 'Atualizar a documentação');
	await atribuir(req, c.ana, c.card.id, c.brunoId);
	const [email] = await emailsPara(req, c.bruno.email, ASSUNTO.atribuicao);

	const tela = await telaDe(browser, c.bruno);
	await expect(sino(tela)).toHaveAccessibleName('Notificações, 1 não lida');

	await desatribuir(req, c.ana, c.card.id, c.brunoId);
	await req.dispose();

	await expect(sino(tela)).toHaveAccessibleName('Notificações');
	await sino(tela).click();
	await expect(
		listaDoSino(tela).getByText('Não vale mais: você não responde mais por este card.')
	).toBeVisible();

	await tela.goto(linkDaNotificacao(email));
	await expect(tela.getByRole('heading', { name: 'Este aviso não vale mais' })).toBeVisible();
	await expect(tela.getByText(/você não responde mais por este card/)).toBeVisible();

	await printFinal(tela);
	await tela.context().close();
});

test('[N4] a menção escrita com o autocompletar chega a quem foi chamado', async ({
	browser,
	playwright
}) => {
	const req = await playwright.request.newContext();
	const c = await quadroComBruno(req, 'Planejar a migração');
	await req.dispose();

	const telaBruno = await telaDe(browser, c.bruno);
	const telaAna = await telaDe(browser, c.ana, `/painel/quadros/${c.quadro.id}`);

	await telaAna.getByText('Planejar a migração', { exact: true }).click();
	const campo = telaAna.getByLabel('Novo comentário');
	await campo.click();
	await campo.pressSequentially('@Bru');
	const pessoas = telaAna.getByRole('listbox', { name: 'Pessoas para mencionar' });
	await expect(pessoas.getByRole('option', { name: 'Bruno' })).toBeVisible();
	await campo.press('Enter');
	await campo.pressSequentially('pode revisar até sexta?');
	await expect(campo).toHaveValue('@Bruno pode revisar até sexta?');
	await telaAna.getByRole('button', { name: 'Comentar' }).click();
	await expect(telaAna.getByText('@Bruno pode revisar até sexta?')).toBeVisible();

	await expect(sino(telaBruno)).toHaveAccessibleName('Notificações, 1 não lida');
	await sino(telaBruno).click();
	const lista = listaDoSino(telaBruno);
	await expect(lista.getByText('mencionou você em "Planejar a migração"')).toBeVisible();
	await expect(lista.getByText(/pode revisar até sexta/)).toBeVisible();

	await printFinal(telaBruno);
	await telaAna.context().close();
	await telaBruno.context().close();
});

test('[N5] o convite para uma conta confirmada aparece no sino e leva aos convites', async ({
	browser,
	playwright
}) => {
	const req = await playwright.request.newContext();
	const ana = await contaNova(req, 'Ana');
	const julia = await contaNova(req, 'Julia');
	await confirmarConta(req, julia);
	const quadro = await criarQuadro(req, ana, 'Roteiro da Julia');

	const tela = await telaDe(browser, julia);
	await expect(tela.getByRole('region', { name: 'Convites para você' })).toHaveCount(0);

	await convidarPelaApi(req, ana, quadro.id, julia.email);
	await req.dispose();

	// O sino muda, e o painel mostra o convite — os dois sem recarregar.
	await expect(sino(tela)).toHaveAccessibleName('Notificações, 1 não lida');
	await expect(
		tela.getByRole('region', { name: 'Convites para você' }).getByText('Roteiro da Julia')
	).toBeVisible();

	// De outra página, o clique no aviso leva aos convites do painel.
	await tela.goto('/');
	await sino(tela).click();
	await listaDoSino(tela).getByText('convidou você para o quadro "Roteiro da Julia"').click();
	await expect(tela).toHaveURL(/\/painel#convites$/);
	await expect(tela.getByRole('region', { name: 'Convites para você' })).toBeVisible();

	await printFinal(tela);
	await tela.context().close();
});

test('[N6] o convite recusado deixa de contar no sino', async ({ browser, playwright }) => {
	const req = await playwright.request.newContext();
	const ana = await contaNova(req, 'Ana');
	const kleber = await contaNova(req, 'Kleber');
	await confirmarConta(req, kleber);
	const quadro = await criarQuadro(req, ana, 'Reforma do Kleber');
	await convidarPelaApi(req, ana, quadro.id, kleber.email);
	await req.dispose();

	const tela = await telaDe(browser, kleber);
	await expect(sino(tela)).toHaveAccessibleName('Notificações, 1 não lida');

	const convites = tela.getByRole('region', { name: 'Convites para você' });
	await convites.getByRole('button', { name: 'Recusar' }).click();
	await tela.getByRole('alertdialog').getByRole('button', { name: 'Recusar o convite' }).click();

	await expect(sino(tela)).toHaveAccessibleName('Notificações');
	await sino(tela).click();
	await expect(listaDoSino(tela).getByText('Não vale mais: você recusou.')).toBeVisible();

	await printFinal(tela);
	await tela.context().close();
});

test('[N7] ler numa aba apaga o número nas outras abas da mesma conta', async ({
	browser,
	playwright
}) => {
	const req = await playwright.request.newContext();
	const c = await quadroComBruno(req, 'Card das duas abas');

	const contexto = await abaDe(browser, c.bruno);
	const abaA = await contexto.newPage();
	const abaB = await contexto.newPage();
	await abaA.goto('/painel');
	await abaB.goto(`/painel/quadros/${c.quadro.id}`);

	await atribuir(req, c.ana, c.card.id, c.brunoId);
	await req.dispose();
	await expect(sino(abaA)).toHaveAccessibleName('Notificações, 1 não lida');
	await expect(sino(abaB)).toHaveAccessibleName('Notificações, 1 não lida');

	await sino(abaA).click();
	await listaDoSino(abaA).getByRole('button', { name: 'Marcar todas como lidas' }).click();

	await expect(sino(abaA)).toHaveAccessibleName('Notificações');
	await expect(sino(abaB)).toHaveAccessibleName('Notificações');

	await printFinal(abaB);
	await contexto.close();
});

test('[N8] o prazo chegando avisa quem responde pelo card', async ({ browser, playwright }) => {
	const req = await playwright.request.newContext();
	const c = await quadroComBruno(req, 'Entregar o relatório');
	await atribuir(req, c.ana, c.card.id, c.brunoId);

	const tela = await telaDe(browser, c.bruno);
	await sino(tela).click();
	await listaDoSino(tela).getByRole('button', { name: 'Marcar todas como lidas' }).click();
	await expect(sino(tela)).toHaveAccessibleName('Notificações');

	// Daqui a seis horas: dentro da véspera, seja qual for a hora da execução.
	await definirPrazo(req, c.ana, c.card.id, new Date(Date.now() + 6 * 3_600_000).toISOString());

	await expect(sino(tela)).toHaveAccessibleName('Notificações, 1 não lida');
	await expect(
		listaDoSino(tela).getByText(/O prazo de "Entregar o relatório" vence (hoje|amanhã)/)
	).toBeVisible();

	const [email] = await emailsPara(req, c.bruno.email, ASSUNTO.prazo);
	await req.dispose();
	expect(email.assunto).toContain('Entregar o relatório');

	await printFinal(tela);
	await tela.context().close();
});

test('[N9] só o dono troca a retenção das notificações, e ela fica gravada e no histórico', async ({
	browser,
	playwright
}) => {
	const req = await playwright.request.newContext();
	const c = await quadroComBruno(req, 'Card qualquer');

	// O editor não tem como mudar — nem pela tela, nem pela API.
	const telaBruno = await telaDe(browser, c.bruno, `/painel/quadros/${c.quadro.id}`);
	await telaBruno.getByRole('button', { name: 'Ajustes' }).click();
	await expect(telaBruno.getByText('Etiquetas do quadro')).toBeVisible();
	await expect(telaBruno.getByLabel('Por quantos dias guardar as notificações lidas')).toHaveCount(
		0
	);
	const recusa = await req.patch(`${API}/boards/${c.quadro.id}/retencao-notificacoes`, {
		data: { dias: 7 },
		headers: cabecalhoDe(c.bruno)
	});
	expect(recusa.status()).toBe(403);
	await req.dispose();

	const telaAna = await telaDe(browser, c.ana, `/painel/quadros/${c.quadro.id}`);
	await telaAna.getByRole('button', { name: 'Ajustes' }).click();
	const retencao = telaAna.getByLabel('Por quantos dias guardar as notificações lidas');
	await expect(retencao).toHaveValue('30');
	await retencao.selectOption('7');

	await telaAna.reload();
	await telaAna.getByRole('button', { name: 'Ajustes' }).click();
	await expect(telaAna.getByLabel('Por quantos dias guardar as notificações lidas')).toHaveValue(
		'7'
	);

	await telaAna.goto(`/painel/quadros/${c.quadro.id}/historico`);
	await expect(
		telaAna.getByText('passou a guardar as notificações lidas por 7 dias (antes, 30)')
	).toBeVisible();

	await printFinal(telaAna);
	await telaAna.context().close();
	await telaBruno.context().close();
});

test('[N10] o link do email aberto sem sessão passa pelo login e volta ao aviso', async ({
	browser,
	playwright
}) => {
	const req = await playwright.request.newContext();
	const c = await quadroComBruno(req, 'Fechar o trimestre');
	await atribuir(req, c.ana, c.card.id, c.brunoId);
	const [email] = await emailsPara(req, c.bruno.email, ASSUNTO.atribuicao);
	await req.dispose();

	const contexto = await browser.newContext({ baseURL: APP });
	const tela = await contexto.newPage();
	await tela.goto(linkDaNotificacao(email));
	await expect(tela).toHaveURL(/\/login\?voltar=%2Fnotificacoes%2F/);

	await tela.getByLabel('Email', { exact: true }).fill(c.bruno.email);
	await tela.getByLabel('Senha', { exact: true }).fill(c.bruno.senha);
	await tela.getByRole('button', { name: 'Entrar' }).click();

	await expect(tela).toHaveURL(new RegExp(`/painel/quadros/${c.quadro.id}$`));
	await expect(
		tela
			.getByRole('dialog', { name: 'Detalhes do card' })
			.getByRole('heading', { name: 'Fechar o trimestre' })
	).toBeVisible();

	await printFinal(tela);
	await contexto.close();
});
