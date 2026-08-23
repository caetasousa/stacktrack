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
	convidar,
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

		// convidar() cria o convite E o aceita: convidar deixou de pôr ninguém no
		// quadro, e sem o aceite o Bruno não conseguiria mover card nenhum.
		await convidar(req, ana, quadroId, bruno);

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

test('o histórico lista quem mexeu em quê', async ({ playwright, browser }) => {
	const req = await playwright.request.newContext();
	await mover(req, ana, aFazer);
	await req.dispose();

	const contexto = await abaDe(browser, ana);
	const pagina = await contexto.newPage();
	await pagina.goto(`/painel/quadros/${quadroId}`);
	await pagina.getByRole('link', { name: 'Histórico' }).click();

	await expect(pagina.getByRole('heading', { name: 'Histórico do quadro' })).toBeVisible();
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
	await pagina.goto(`/painel/quadros/${quadroId}/historico`);
	await pagina.getByLabel('Só movimentações de card').check();
	await expect(pagina.getByText('de A fazer para Pronto')).toBeVisible();

	// A opção é escolhida pelo VALOR, e não pelo rótulo: o rótulo passou a levar
	// o email junto — que é o ponto do campo, desempatar homônimos — e um email
	// gerado com timestamp não dá para escrever no teste.
	const valorDoBruno = await pagina
		.locator('option', { hasText: 'bruno-auditoria' })
		.getAttribute('value');
	await pagina.getByLabel('Pessoa').selectOption(valorDoBruno);

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
	await convidar(req, ana, quadroId, carla);
	await req.dispose();

	const contexto = await abaDe(browser, carla);
	const pagina = await contexto.newPage();
	await pagina.goto(`/painel/quadros/${quadroId}/historico`);

	await expect(pagina.getByText('moveu "Trocar o gateway"').first()).toBeVisible();

	await contexto.close();
});

// A prova do que o histórico passou a ser: um quadro onde se mexe em tudo, e
// TUDO aparece.
//
// Antes, doze ações diferentes — etiqueta, anexo, checklist, responsável,
// renomear, fundo — chegavam ao log como um "quadro.alterado" sem payload, e a
// tela as descartava em silêncio. Este teste falha se qualquer uma delas voltar
// a ser invisível.
test('o histórico mostra o que aconteceu com tudo, não só com os cards', async ({
	playwright,
	browser
}) => {
	const req = await playwright.request.newContext();
	const ck = `${ana.cookie.name}=${ana.cookie.value}`;

	const etq = await req.post(`${API}/boards/${quadroId}/etiquetas`, {
		data: { nome: 'urgente', cor: 'vermelho' },
		headers: { Cookie: ck }
	});
	const etqId = ((await etq.json()) as { id: string }).id;
	await req.put(`${API}/cards/${cardID}/etiquetas/${etqId}`, { headers: { Cookie: ck } });
	await req.post(`${API}/cards/${cardID}/checklists`, {
		data: { titulo: 'Passos' },
		headers: { Cookie: ck }
	});
	await req.post(`${API}/cards/${cardID}/anexos/link`, {
		data: { url: 'https://exemplo.com/contrato', nome: 'contrato' },
		headers: { Cookie: ck }
	});
	await req.patch(`${API}/boards/${quadroId}`, {
		data: { titulo: 'Quadro auditado (renomeado)' },
		headers: { Cookie: ck }
	});
	await req.patch(`${API}/boards/${quadroId}/fundo`, {
		data: { fundo: 'oceano' },
		headers: { Cookie: ck }
	});
	await req.dispose();

	const contexto = await abaDe(browser, ana);
	const pagina = await contexto.newPage();
	await pagina.goto(`/painel/quadros/${quadroId}/historico`);

	await expect(pagina.getByText('criou a etiqueta "urgente"')).toBeVisible();
	await expect(pagina.getByText('marcou "Trocar o gateway" com "urgente"')).toBeVisible();
	await expect(pagina.getByText('criou a checklist "Passos"')).toBeVisible();
	await expect(pagina.getByText('anexou "contrato"')).toBeVisible();
	await expect(pagina.getByText('renomeou o quadro')).toBeVisible();
	await expect(pagina.getByText('mudou o fundo do quadro para oceano')).toBeVisible();
	await expect(pagina.getByText('criou a coluna "A fazer"')).toBeVisible();

	await contexto.close();
});

// O CRUD INTEIRO, de ponta a ponta: coluna e card, do nascimento à exclusão.
//
// Este teste existe por um relato concreto — "criei uma coluna e não apareceu,
// apaguei e não apareceu, renomeei e não apareceu, criei um card e não
// apareceu". As quatro eram invisíveis, por duas razões somadas: a tela abria
// filtrada em movimentações de card, e coluna não tinha frase nenhuma.
//
// Ele confere cada operação pela SUA FRASE, e não pela contagem de linhas: uma
// regressão que devolvesse qualquer uma ao tipo genérico ainda produziria uma
// linha ("mexeu no quadro") e passaria por qualquer asserção de quantidade.
test('o histórico registra o CRUD inteiro de coluna e de card', async ({ playwright, browser }) => {
	const req = await playwright.request.newContext();
	let dono: Conta;
	try {
		dono = await criarConta(req, 'dona-crud');
	} catch (e) {
		await req.dispose();
		if (e instanceof TetoDeRequisicoes) test.skip(true, e.message);
		throw e;
	}
	const ck = `${dono.cookie.name}=${dono.cookie.value}`;
	const quadro = await criarQuadro(req, dono, 'CRUD auditado');

	const criarCol = async (titulo: string) => {
		const r = await req.post(`${API}/boards/${quadro.id}/colunas`, {
			data: { titulo, cor: '' },
			headers: { Cookie: ck }
		});
		if (!r.ok()) throw new Error(`criar coluna: ${r.status()} ${await r.text()}`);
		return ((await r.json()) as { id: string }).id;
	};

	const origem = await criarCol('Coluna A');
	const destino = await criarCol('Coluna B');
	await req.patch(`${API}/colunas/${origem}`, {
		data: { titulo: 'Coluna A renomeada', cor: 'verde' },
		headers: { Cookie: ck }
	});

	const rk = await req.post(`${API}/colunas/${origem}/cards`, {
		data: { titulo: 'Tarefa', descricao: '', cor: '' },
		headers: { Cookie: ck }
	});
	const card = ((await rk.json()) as { id: string }).id;

	// Atribuir e depois desatribuir: é o caso que o relato citou como motivo de
	// precisar da auditoria — "atribuir uma tarefa e depois editar para culpar
	// alguém". As duas pontas precisam ficar registradas.
	const me = await req.get(`${API}/auth/me`, { headers: { Cookie: ck } });
	const donoID = ((await me.json()) as { id: string }).id;
	await req.put(`${API}/cards/${card}/responsaveis/${donoID}`, { headers: { Cookie: ck } });
	await req.delete(`${API}/cards/${card}/responsaveis/${donoID}`, { headers: { Cookie: ck } });

	await req.patch(`${API}/cards/${card}/mover`, {
		data: { colunaId: destino },
		headers: { Cookie: ck }
	});
	await req.patch(`${API}/cards/${card}`, {
		data: { titulo: 'Tarefa renomeada', descricao: 'x', cor: '', version: 0 },
		headers: { Cookie: ck }
	});
	await req.delete(`${API}/cards/${card}`, { headers: { Cookie: ck } });
	await req.delete(`${API}/colunas/${origem}`, { headers: { Cookie: ck } });
	await req.dispose();

	const contexto = await abaDe(browser, dono);
	const pagina = await contexto.newPage();
	await pagina.goto(`/painel/quadros/${quadro.id}/historico`);
	await expect(pagina.getByRole('heading', { name: 'Histórico do quadro' })).toBeVisible();

	for (const frase of [
		'criou a coluna "Coluna A"',
		'criou a coluna "Coluna B"',
		'renomeou a coluna "Coluna A" para "Coluna A renomeada"',
		'apagou a coluna "Coluna A renomeada"',
		'criou "Tarefa" em Coluna A renomeada',
		'pôs "dona-crud" como responsável por "Tarefa"',
		'tirou "dona-crud" da responsabilidade de "Tarefa"',
		'moveu "Tarefa" de Coluna A renomeada para Coluna B',
		'renomeou "Tarefa" para "Tarefa renomeada"',
		'apagou "Tarefa renomeada"'
	]) {
		await expect(pagina.getByText(frase), `faltou no histórico: ${frase}`).toBeVisible();
	}

	await contexto.close();
});

// O histórico guarda o que era verdade NA HORA — e é isso que o torna prova.
//
// Sem essa garantia, renomear depois reescreveria o passado: a linha "criou X"
// passaria a mostrar o nome novo, e apagar o próprio rastro seria só renomear.
test('renomear depois não reescreve o que o histórico já registrou', async ({
	playwright,
	browser
}) => {
	const req = await playwright.request.newContext();
	let dono: Conta;
	try {
		dono = await criarConta(req, 'dona-passado');
	} catch (e) {
		await req.dispose();
		if (e instanceof TetoDeRequisicoes) test.skip(true, e.message);
		throw e;
	}
	const ck = `${dono.cookie.name}=${dono.cookie.value}`;
	const quadro = await criarQuadro(req, dono, 'Passado');
	const col = await criarColuna(req, dono, quadro.id, 'A fazer');
	const rk = await req.post(`${API}/colunas/${col.id}/cards`, {
		data: { titulo: 'Nome original', descricao: '', cor: '' },
		headers: { Cookie: ck }
	});
	const card = ((await rk.json()) as { id: string }).id;
	await req.patch(`${API}/cards/${card}`, {
		data: { titulo: 'Nome trocado', descricao: '', cor: '', version: 0 },
		headers: { Cookie: ck }
	});
	await req.dispose();

	const contexto = await abaDe(browser, dono);
	const pagina = await contexto.newPage();
	await pagina.goto(`/painel/quadros/${quadro.id}/historico`);

	// A criação continua falando do nome ANTIGO, e a renomeação mostra os dois.
	await expect(pagina.getByText('criou "Nome original" em A fazer')).toBeVisible();
	await expect(pagina.getByText('renomeou "Nome original" para "Nome trocado"')).toBeVisible();

	await contexto.close();
});
