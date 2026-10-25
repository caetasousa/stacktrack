// O sino de quem está logado: quantos avisos pedem atenção, a lista deles e a
// conexão de tempo real que mantém os dois em dia. Uma instância para o
// aplicativo inteiro — o cabeçalho mostra, o painel e a página do link do email
// usam.
import {
	listarNotificacoes,
	marcarLida as marcarLidaNaApi,
	marcarTodasLidas as marcarTodasNaApi,
	type Notificacao
} from '$lib/api/notificacoes';
import { conectarAoSino, type SituacaoDoSino } from '$lib/realtime/notificacoes.svelte';

// Avisos próximos viram uma releitura só. O servidor já junta toques seguidos,
// mas cada aba relê por conta própria, e alguém atribuindo dez cards de uma vez
// não precisa custar dez listas.
const JANELA_DE_RELEITURA_MS = 300;

export class Sino {
	naoLidas = $state(0);
	itens = $state<Notificacao[]>([]);
	temMais = $state(false);
	/** A lista na tela é a do servidor — e não uma que ficou velha fechada. */
	carregada = $state(false);
	carregando = $state(false);
	aberto = $state(false);
	/**
	 * Sobe a cada aviso do servidor depois do primeiro. Quem precisa reagir a
	 * "chegou algo" sem olhar o número — o painel, que relê os convites —
	 * observa isto.
	 */
	versao = $state(0);

	#usuarioId: string | null = null;
	#conexao = $state<{ readonly situacao: SituacaoDoSino; fechar: () => void } | null>(null);
	#primeiroAviso = true;
	#releitura: ReturnType<typeof setTimeout> | null = null;

	/**
	 * iniciar liga o sino para a conta. Chamar de novo para a mesma conta não
	 * reabre nada; para outra (sair e entrar com outra conta na mesma aba),
	 * troca a conexão inteira.
	 */
	iniciar(usuarioId: string): void {
		if (this.#usuarioId === usuarioId && this.#conexao) return;
		this.parar();
		this.#usuarioId = usuarioId;
		this.#primeiroAviso = true;
		this.#conexao = conectarAoSino(
			(n) => this.#aoAvisar(n),
			() => this.parar()
		);
	}

	/** A situação da conexão de tempo real, lida ao vivo. */
	get situacao(): SituacaoDoSino {
		return this.#conexao?.situacao ?? 'desligado';
	}

	/** parar desliga a conexão e esquece o que era da conta. */
	parar(): void {
		this.#conexao?.fechar();
		this.#conexao = null;
		this.#usuarioId = null;
		if (this.#releitura) clearTimeout(this.#releitura);
		this.#releitura = null;
		this.naoLidas = 0;
		this.itens = [];
		this.temMais = false;
		this.carregada = false;
		this.aberto = false;
	}

	#aoAvisar(naoLidas: number | null) {
		if (naoLidas !== null) this.naoLidas = naoLidas;
		if (this.#primeiroAviso) {
			this.#primeiroAviso = false;
		} else {
			this.versao++;
		}
		// A lista aberta precisa acompanhar; a fechada só fica marcada como
		// velha, e é relida quando abrir. Formato desconhecido também relê: é a
		// API que diz o número certo.
		if (this.aberto || naoLidas === null) {
			this.#agendarReleitura();
		} else {
			this.carregada = false;
		}
	}

	#agendarReleitura() {
		if (this.#releitura) return;
		this.#releitura = setTimeout(() => {
			this.#releitura = null;
			void this.carregar();
		}, JANELA_DE_RELEITURA_MS);
	}

	/** carregar relê a primeira página e o número, da mesma resposta. */
	async carregar(): Promise<void> {
		if (!this.#usuarioId) return;
		const dono = this.#usuarioId;
		this.carregando = true;
		try {
			const pagina = await listarNotificacoes();
			if (this.#usuarioId !== dono) return;
			this.itens = pagina.notificacoes;
			this.naoLidas = pagina.naoLidas;
			this.temMais = pagina.temMais;
			this.carregada = true;
		} finally {
			this.carregando = false;
		}
	}

	/** carregarMais acrescenta a página seguinte, a partir do último aviso. */
	async carregarMais(): Promise<void> {
		const ultimo = this.itens.at(-1);
		if (!ultimo || !this.temMais) return;
		const pagina = await listarNotificacoes(ultimo.id);
		const vistos = new Set(this.itens.map((n) => n.id));
		this.itens = [...this.itens, ...pagina.notificacoes.filter((n) => !vistos.has(n.id))];
		this.temMais = pagina.temMais;
	}

	/**
	 * marcarLida marca na hora, na tela, e depois no servidor. O número só cai
	 * para o aviso que contava: lido ou encerrado, ele já não contava.
	 */
	async marcarLida(n: Notificacao): Promise<void> {
		if (!n.lida) {
			if (n.situacao === 'vigente') this.naoLidas = Math.max(0, this.naoLidas - 1);
			this.itens = this.itens.map((i) => (i.id === n.id ? { ...i, lida: true } : i));
		}
		await marcarLidaNaApi(n.id);
	}

	async marcarTodas(): Promise<void> {
		this.naoLidas = 0;
		this.itens = this.itens.map((i) => ({ ...i, lida: true }));
		await marcarTodasNaApi();
	}
}

export const sino = new Sino();
