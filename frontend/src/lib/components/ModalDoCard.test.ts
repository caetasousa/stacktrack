// @vitest-environment jsdom

// Monta o modal de verdade para provar que o painel de HISTÓRICO abre e FICA
// aberto.
//
// O teste existe por causa de um defeito real, e sutil o bastante para ter
// passado por uma suíte inteira verde. O efeito que reinicia o modal quando o
// card muda chamava `carregar(id)` sem argumentos — e o valor padrão de um dos
// parâmetros é `cardId === id && historicoAberto`. Valores padrão são avaliados
// NA CHAMADA, então essa leitura acontecia dentro do efeito e o tornava
// dependente de `historicoAberto`. Abrir o histórico disparava o efeito, que
// executava `historicoAberto = false` e fechava o painel no mesmo quadro de
// animação: o botão simplesmente não funcionava.
//
// Nada acusava. O svelte-check não vê dependência reativa acidental, e os testes
// de API só olham o JSON. O que pega é montar o componente e clicar.
import { flushSync, mount, unmount } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import ModalDoCard from './ModalDoCard.svelte';
import type { CardDetalhado } from '$lib/api/extras';

const cardFalso: CardDetalhado = {
	id: 'card-1',
	boardId: 'quadro-1',
	colunaId: 'col-1',
	titulo: 'Migração',
	descricao: '',
	version: 1,
	revisao: 7,
	prazo: null,
	vencido: false,
	cor: '',
	// Os campos do resumo (`Card`) e os do detalhe convivem no mesmo tipo: o
	// modal recebe o card inteiro, e não só a parte que ele desenha.
	etiquetas: [],
	checklist: { concluidos: 0, total: 0 },
	qtdAnexos: 0,
	qtdComentarios: 0,
	ultimaMovimentacao: null,
	responsaveis: [],
	etiquetasDoCard: [],
	checklists: [],
	anexos: [],
	comentarios: []
};

// As chamadas de rede do modal são substituídas: o assunto aqui é o
// comportamento reativo do painel, não o transporte.
vi.mock('$lib/api/extras', async (original) => ({
	...(await original<typeof import('$lib/api/extras')>()),
	detalharCard: vi.fn(async () => cardFalso),
	atividadeDoCard: vi.fn(async () => ({
		atividade: [
			{
				seq: 1,
				tipo: 'comentario.criado',
				autorId: 'u-1',
				autorNome: 'Ana',
				autorEmail: 'ana@exemplo.com',
				dados: {},
				ocorridoEm: new Date().toISOString()
			}
		]
	}))
}));

vi.mock('$lib/api/membros', async (original) => ({
	...(await original<typeof import('$lib/api/membros')>()),
	listarParticipacao: vi.fn(async () => ({ membros: [], convites: [] }))
}));

let desmontar: (() => void) | null = null;

beforeEach(() => {
	const alvo = document.createElement('div');
	alvo.id = 'alvo';
	document.body.appendChild(alvo);
});

afterEach(() => {
	desmontar?.();
	desmontar = null;
	document.body.innerHTML = '';
	vi.clearAllMocks();
});

function montarModal() {
	const componente = mount(ModalDoCard, {
		target: document.getElementById('alvo')!,
		props: {
			cardId: 'card-1',
			etiquetasDoQuadro: [],
			podeEditar: true,
			aoFechar: () => {},
			aoMudar: async () => {},
			registrarRecarga: () => {}
		}
	});
	desmontar = () => unmount(componente);
}

/** botaoDoHistorico acha o botão pelo texto, como a pessoa acharia. */
function botaoDoHistorico(): HTMLButtonElement {
	const botoes = [...document.querySelectorAll('button')];
	const alvo = botoes.find((b) => /histórico/i.test(b.textContent ?? ''));
	if (!alvo) throw new Error('o botão de histórico não está na tela');
	return alvo as HTMLButtonElement;
}

/** esperar deixa as promessas pendentes (carregar, atividade) resolverem. */
async function esperar() {
	await Promise.resolve();
	await new Promise((resolva) => setTimeout(resolva, 0));
	flushSync();
}

describe('ModalDoCard — painel de histórico', () => {
	it('abre ao clicar e permanece aberto', async () => {
		montarModal();
		await esperar();

		const botao = botaoDoHistorico();
		expect(botao.getAttribute('aria-expanded')).toBe('false');

		botao.click();
		flushSync();
		expect(botaoDoHistorico().getAttribute('aria-expanded')).toBe('true');

		// O ponto do teste: depois de os efeitos rodarem, ele CONTINUA aberto.
		// Era aqui que o painel se fechava sozinho.
		await esperar();
		expect(botaoDoHistorico().getAttribute('aria-expanded')).toBe('true');
	});

	it('fecha ao clicar de novo', async () => {
		montarModal();
		await esperar();

		botaoDoHistorico().click();
		flushSync();
		await esperar();
		expect(botaoDoHistorico().getAttribute('aria-expanded')).toBe('true');

		botaoDoHistorico().click();
		flushSync();
		await esperar();
		expect(botaoDoHistorico().getAttribute('aria-expanded')).toBe('false');
	});

	it('mostra as linhas do histórico depois de abrir', async () => {
		montarModal();
		await esperar();

		botaoDoHistorico().click();
		flushSync();
		await esperar();

		expect(document.body.textContent).toMatch(/comentou/i);
	});
});
