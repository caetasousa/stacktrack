// A conexão do sino (SSE).
//
// Como isto quebra:
//   - a contagem que o servidor manda não chega a quem a pediu;
//   - uma versão de envelope que este cliente não conhece é interpretada pela
//     metade, em vez de virar "releia pela API";
//   - a sessão encerrada no servidor deixa o navegador reabrindo o stream para
//     sempre;
//   - o navegador desiste (resposta que não é 200: 503, 429) e a tela fica
//     calada até alguém recarregar;
//   - reabrir depois de desistir martela o servidor sem espera nem sorteio.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { conectarAoSino, enderecoDoStream } from './notificacoes.svelte';

type Ouvinte = (e: { data: string }) => void;

class EventSourceFalso {
	static readonly CONNECTING = 0;
	static readonly OPEN = 1;
	static readonly CLOSED = 2;
	static readonly instancias: EventSourceFalso[] = [];

	readonly url: string;
	readonly comCredenciais: boolean;
	readyState = EventSourceFalso.CONNECTING;
	onopen: (() => void) | null = null;
	onerror: (() => void) | null = null;
	ouvintes = new Map<string, Ouvinte>();
	close = vi.fn(() => {
		this.readyState = EventSourceFalso.CLOSED;
	});

	constructor(url: string, opcoes?: { withCredentials?: boolean }) {
		this.url = url;
		this.comCredenciais = opcoes?.withCredentials ?? false;
		EventSourceFalso.instancias.push(this);
	}

	addEventListener(nome: string, ouvinte: Ouvinte) {
		this.ouvintes.set(nome, ouvinte);
	}

	abrir() {
		this.readyState = EventSourceFalso.OPEN;
		this.onopen?.();
	}

	emitir(nome: string, dados: unknown) {
		this.ouvintes.get(nome)?.({ data: JSON.stringify(dados) });
	}

	/** O navegador desistiu: resposta que não é 200. Ele NÃO reabre sozinho. */
	desistir() {
		this.readyState = EventSourceFalso.CLOSED;
		this.onerror?.();
	}

	/** A rede caiu: o navegador está reabrindo sozinho. */
	oscilar() {
		this.readyState = EventSourceFalso.CONNECTING;
		this.onerror?.();
	}
}

const criar = (url: string) =>
	new EventSourceFalso(url, { withCredentials: true }) as unknown as EventSource;

beforeEach(() => {
	EventSourceFalso.instancias.length = 0;
	vi.useFakeTimers();
});

afterEach(() => {
	vi.useRealTimers();
});

describe('enderecoDoStream', () => {
	it('monta o caminho sobre a base da API, absoluta ou relativa', () => {
		expect(enderecoDoStream('http://localhost:8080')).toBe(
			'http://localhost:8080/notificacoes/eventos'
		);
		expect(enderecoDoStream('/api')).toBe('/api/notificacoes/eventos');
	});
});

describe('conectarAoSino', () => {
	it('entrega a contagem e diz que está ao vivo', () => {
		const contagens: (number | null)[] = [];
		const sino = conectarAoSino((n) => contagens.push(n), vi.fn(), criar);
		const fonte = EventSourceFalso.instancias[0];
		expect(fonte.comCredenciais).toBe(true);

		fonte.abrir();
		fonte.emitir('notificacoes', { versao: 1, naoLidas: 3 });
		expect(sino.situacao).toBe('ao-vivo');
		expect(contagens).toEqual([3]);
	});

	it('versão de envelope desconhecida vira "releia pela API"', () => {
		const contagens: (number | null)[] = [];
		conectarAoSino((n) => contagens.push(n), vi.fn(), criar);
		EventSourceFalso.instancias[0].emitir('notificacoes', { versao: 2, naoLidas: 3, novo: true });
		expect(contagens).toEqual([null]);
	});

	it('sessão encerrada fecha de vez e avisa quem chamou', () => {
		const sessaoAcabou = vi.fn();
		const sino = conectarAoSino(vi.fn(), sessaoAcabou, criar);
		const fonte = EventSourceFalso.instancias[0];
		fonte.emitir('encerrada', { versao: 1, motivo: 'sessao' });

		expect(fonte.close).toHaveBeenCalled();
		expect(sessaoAcabou).toHaveBeenCalled();
		expect(sino.situacao).toBe('desligado');
		vi.advanceTimersByTime(60_000);
		expect(EventSourceFalso.instancias).toHaveLength(1);
	});

	it('servidor reiniciando deixa o navegador reabrir sozinho', () => {
		const sino = conectarAoSino(vi.fn(), vi.fn(), criar);
		const fonte = EventSourceFalso.instancias[0];
		fonte.abrir();
		fonte.emitir('encerrada', { versao: 1, motivo: 'servidor' });
		fonte.oscilar();
		expect(fonte.close).not.toHaveBeenCalled();
		expect(sino.situacao).toBe('reconectando');
	});

	it('quando o navegador desiste, reabre com espera crescente', () => {
		// O sorteio fixo no meio: a espera vira 3/4 do teto de cada tentativa.
		vi.spyOn(Math, 'random').mockReturnValue(0.5);
		const sino = conectarAoSino(vi.fn(), vi.fn(), criar);
		EventSourceFalso.instancias[0].desistir();
		expect(sino.situacao).toBe('reconectando');
		expect(EventSourceFalso.instancias).toHaveLength(1);

		// A primeira espera é de no máximo 1 s (com sorteio); a segunda, maior.
		vi.advanceTimersByTime(1_000);
		expect(EventSourceFalso.instancias).toHaveLength(2);
		EventSourceFalso.instancias[1].desistir();
		vi.advanceTimersByTime(1_000);
		expect(EventSourceFalso.instancias).toHaveLength(2);
		vi.advanceTimersByTime(1_000);
		expect(EventSourceFalso.instancias).toHaveLength(3);
	});

	it('fechar encerra e não reabre', () => {
		const sino = conectarAoSino(vi.fn(), vi.fn(), criar);
		const fonte = EventSourceFalso.instancias[0];
		sino.fechar();
		expect(fonte.close).toHaveBeenCalled();
		fonte.desistir();
		vi.advanceTimersByTime(60_000);
		expect(EventSourceFalso.instancias).toHaveLength(1);
	});
});
