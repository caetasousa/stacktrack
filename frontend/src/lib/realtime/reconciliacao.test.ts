import { describe, expect, it, vi } from 'vitest';
import {
	esperaParaReconciliar,
	executarDepoisDaAtual,
	executarPreservandoFalha,
	INTERVALO_SUSTENTADO_MS,
	reconciliarSnapshots,
	type RecarregarProjecaoAtiva
} from './reconciliacao';

function pendente<T>() {
	let resolver!: (valor: T) => void;
	let rejeitar!: (erro: unknown) => void;
	const promessa = new Promise<T>((resolve, reject) => {
		resolver = resolve;
		rejeitar = reject;
	});
	return { promessa, resolver, rejeitar };
}

describe('reconciliarSnapshots', () => {
	it('confirma somente depois do quadro e da projeção ativa', async () => {
		const quadro = pendente<number>();
		const modal = pendente<number>();
		const ordem: string[] = [];
		const confirmar = vi.fn((revisao: number) => ordem.push(`confirmar:${revisao}`));
		const recarregarModal = vi.fn(async () => {
			ordem.push('modal:inicio');
			const revisao = await modal.promessa;
			ordem.push('modal:fim');
			return revisao;
		});

		const execucao = reconciliarSnapshots({
			carregarQuadro: async () => {
				ordem.push('quadro:inicio');
				const revisao = await quadro.promessa;
				ordem.push('quadro:fim');
				return revisao;
			},
			obterProjecaoAtiva: () => recarregarModal,
			confirmar
		});

		expect(ordem).toEqual(['quadro:inicio']);
		quadro.resolver(12);
		await vi.waitFor(() => expect(ordem).toContain('modal:inicio'));
		expect(confirmar).not.toHaveBeenCalled();

		modal.resolver(10);
		await expect(execucao).resolves.toBe(10);
		expect(ordem).toEqual([
			'quadro:inicio',
			'quadro:fim',
			'modal:inicio',
			'modal:fim',
			'confirmar:10'
		]);
	});

	it('relê a projeção que substituiu o modal durante o interleaving', async () => {
		const primeira = pendente<number>();
		const recarregarPrimeira = vi.fn(async () => primeira.promessa);
		const segunda = vi.fn(async () => 14);
		let ativa: RecarregarProjecaoAtiva | null = recarregarPrimeira;
		const confirmar = vi.fn();

		const execucao = reconciliarSnapshots({
			carregarQuadro: async () => 15,
			obterProjecaoAtiva: () => ativa,
			confirmar
		});
		await vi.waitFor(() => expect(recarregarPrimeira).toHaveBeenCalledOnce());
		ativa = segunda;
		primeira.resolver(13);

		await expect(execucao).resolves.toBe(14);
		expect(segunda).toHaveBeenCalledOnce();
		expect(confirmar).toHaveBeenCalledOnce();
		expect(confirmar).toHaveBeenCalledWith(14);
	});

	it('não confirma quando a projeção ativa falha', async () => {
		const confirmar = vi.fn();
		const falha = new Error('modal indisponível');

		await expect(
			reconciliarSnapshots({
				carregarQuadro: async () => 9,
				obterProjecaoAtiva: () => async () => {
					throw falha;
				},
				confirmar
			})
		).rejects.toBe(falha);
		expect(confirmar).not.toHaveBeenCalled();
	});

	it('não confirma se o histórico aberto no interleaving falha', async () => {
		const card = pendente<number>();
		const recarregarCard = vi.fn(async () => card.promessa);
		const falhaDoHistorico = new Error('histórico indisponível');
		let ativa: RecarregarProjecaoAtiva | null = recarregarCard;
		const confirmar = vi.fn();

		const execucao = reconciliarSnapshots({
			carregarQuadro: async () => 21,
			obterProjecaoAtiva: () => ativa,
			confirmar
		});
		await vi.waitFor(() => expect(recarregarCard).toHaveBeenCalledOnce());
		// Abrir o histórico troca a identidade registrada pelo modal. A primeira
		// carga já não basta; a nova projeção precisa terminar com sucesso.
		ativa = async () => {
			throw falhaDoHistorico;
		};
		card.resolver(21);

		await expect(execucao).rejects.toBe(falhaDoHistorico);
		expect(confirmar).not.toHaveBeenCalled();
	});

	it('não confirma revisão que um servidor antigo não informou', async () => {
		const confirmar = vi.fn();
		await expect(
			reconciliarSnapshots({
				carregarQuadro: async () => undefined,
				obterProjecaoAtiva: () => null,
				confirmar
			})
		).resolves.toBeUndefined();
		expect(confirmar).not.toHaveBeenCalled();
	});
});

describe('esperaParaReconciliar', () => {
	it('libera a primeira rajada e limita carga contínua ao intervalo sustentado', () => {
		expect(esperaParaReconciliar(10_000, undefined)).toBe(0);
		expect(esperaParaReconciliar(10_150, 10_000)).toBe(INTERVALO_SUSTENTADO_MS - 150);
		expect(esperaParaReconciliar(10_000 + INTERVALO_SUSTENTADO_MS, 10_000)).toBe(0);
	});
});

describe('executarPreservandoFalha', () => {
	it('reagenda o restore completo e permite reset somente no retry bem-sucedido', async () => {
		const pedido = { forcar: true, revisao: 90, redefinirCursor: true };
		const fila: (typeof pedido)[] = [];
		const redefinir = vi.fn();

		await expect(
			executarPreservandoFalha(
				pedido,
				async () => {
					throw new Error('primeiro snapshot falhou');
				},
				(repetir) => fila.push(repetir)
			)
		).rejects.toThrow('primeiro snapshot falhou');
		expect(redefinir).not.toHaveBeenCalled();
		expect(fila).toEqual([pedido]);

		await executarPreservandoFalha(
			fila.shift()!,
			async (repetido) => redefinir(repetido.revisao),
			(repetir) => fila.push(repetir)
		);
		expect(redefinir).toHaveBeenCalledOnce();
		expect(redefinir).toHaveBeenCalledWith(90);
		expect(fila).toHaveLength(0);
	});
});

describe('executarDepoisDaAtual', () => {
	it('inicia o snapshot pós-mutação somente depois do snapshot antigo', async () => {
		const antiga = pendente<void>();
		const ordem = ['A:inicio'];
		const nova = vi.fn(async () => {
			ordem.push('B:inicio');
		});

		const execucao = executarDepoisDaAtual(antiga.promessa, nova);
		await Promise.resolve();
		expect(nova).not.toHaveBeenCalled();

		ordem.push('A:fim');
		antiga.resolver();
		await execucao;
		expect(ordem).toEqual(['A:inicio', 'A:fim', 'B:inicio']);
	});

	it('a recarga nova ainda roda quando a antiga falhou', async () => {
		const antiga = Promise.reject(new Error('snapshot antigo falhou'));
		const nova = vi.fn(async () => {});

		await expect(executarDepoisDaAtual(antiga, nova)).resolves.toBeUndefined();
		expect(nova).toHaveBeenCalledOnce();
	});
});
