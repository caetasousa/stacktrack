/** Recarga da projeção que não faz parte do snapshot principal do quadro. */
export type RecarregarProjecaoAtiva = () => Promise<number | undefined>;

export interface ReconciliacaoDeSnapshots {
	/** Atualiza o quadro e devolve a revisão realmente lida. */
	carregarQuadro: () => Promise<number | undefined>;
	/**
	 * Lê a projeção ativa no momento da chamada.
	 *
	 * É um getter, e não um valor capturado, porque o modal pode abrir, fechar ou
	 * trocar de card enquanto a requisição do quadro está em andamento.
	 */
	obterProjecaoAtiva: () => RecarregarProjecaoAtiva | null;
	confirmar: (revisao: number) => void;
}

/**
 * Aplica todos os snapshots visíveis antes de confirmar o cursor.
 *
 * A revisão confirmada é a menor revisão comprovadamente coberta pelas
 * projeções. Se o modal trocar enquanto sua leitura está em andamento, a nova
 * projeção também é carregada antes do ack. Qualquer falha rejeita a operação
 * e, portanto, não chama `confirmar`.
 */
export async function reconciliarSnapshots({
	carregarQuadro,
	obterProjecaoAtiva,
	confirmar
}: ReconciliacaoDeSnapshots): Promise<number | undefined> {
	const revisaoDoQuadro = await carregarQuadro();
	let revisaoAplicada = revisaoDoQuadro;

	while (true) {
		const recarregarProjecao = obterProjecaoAtiva();
		if (!recarregarProjecao) break;

		const revisaoDaProjecao = await recarregarProjecao();
		// A projeção mudou durante a leitura. O resultado continua válido para a
		// antiga, mas não diz nada sobre a que agora está visível.
		if (obterProjecaoAtiva() !== recarregarProjecao) continue;

		if (revisaoDaProjecao === undefined) {
			revisaoAplicada = undefined;
		} else if (revisaoAplicada !== undefined) {
			revisaoAplicada = Math.min(revisaoAplicada, revisaoDaProjecao);
		}
		break;
	}

	// Compatibilidade de rollout: um servidor anterior atualiza a tela, mas não
	// fornece revisão. Sem a prova numérica, a conexão conserva o cursor antigo.
	if (revisaoAplicada !== undefined) confirmar(revisaoAplicada);
	return revisaoAplicada;
}

/** Espera curta para colapsar a rajada inicial de eventos. */
export const JANELA_DE_RAJADA_MS = 150;

/**
 * Intervalo mínimo sustentado entre snapshots.
 *
 * Neste projeto, `invalidateAll` relê autenticação e quadro; com modal e
 * histórico abertos a reconciliação chega a cinco GETs. A cada 3 s são no
 * máximo 100 GETs/min, deixando margem sob o teto de 120.
 */
export const INTERVALO_SUSTENTADO_MS = 3_000;

/** Quanto ainda falta para uma nova reconciliação poder começar. */
export function esperaParaReconciliar(
	agora: number,
	ultimaReconciliacaoEm: number | undefined
): number {
	if (ultimaReconciliacaoEm === undefined) return 0;
	return Math.max(0, ultimaReconciliacaoEm + INTERVALO_SUSTENTADO_MS - agora);
}

/**
 * Executa um pedido consumido da fila e o devolve inteiro quando há falha.
 *
 * É especialmente importante no restore: perder `redefinirCursor` após a
 * primeira tentativa deixaria todas as tentativas seguintes presas ao cursor
 * monotônico anterior ao backup.
 */
export async function executarPreservandoFalha<T>(
	pedido: T,
	executar: (pedido: T) => Promise<void>,
	reagendar: (pedido: T) => void
): Promise<void> {
	try {
		await executar(pedido);
	} catch (erro) {
		reagendar(pedido);
		throw erro;
	}
}

/**
 * Serializa uma recarga explícita atrás da que já estava em andamento.
 *
 * A falha anterior não cancela a nova: uma mutação pode ter comitado enquanto
 * o snapshot antigo falhava, e ainda precisa de sua própria leitura posterior.
 */
export async function executarDepoisDaAtual(
	atual: Promise<void>,
	executar: () => Promise<void>
): Promise<void> {
	try {
		await atual;
	} catch {
		// A próxima tentativa continua sendo necessária — e pode se recuperar.
	}
	await executar();
}
