import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
	conectarAoQuadro,
	enderecoDoSocket,
	eventoExigeSaidaDoQuadro,
	pedidoDeRecargaPara,
	type EventoDoQuadro
} from './conexao.svelte';

class WebSocketFalso {
	static readonly OPEN = 1;
	static readonly instancias: WebSocketFalso[] = [];

	readonly url: string;
	readyState = 0;
	onopen: (() => void) | null = null;
	onmessage: ((mensagem: { data: string }) => void) | null = null;
	onclose: (() => void) | null = null;
	onerror: (() => void) | null = null;
	send = vi.fn();
	close = vi.fn(() => {
		this.readyState = 3;
		this.onclose?.();
	});

	constructor(url: string | URL) {
		this.url = String(url);
		WebSocketFalso.instancias.push(this);
	}

	abrir() {
		this.readyState = WebSocketFalso.OPEN;
		this.onopen?.();
	}

	emitir(evento: EventoDoQuadro) {
		this.onmessage?.({ data: JSON.stringify(evento) });
	}

	cair() {
		this.readyState = 3;
		this.onclose?.();
	}
}

function evento(tipo: EventoDoQuadro['tipo'], revisao?: number): EventoDoQuadro {
	return {
		versao: 1,
		tipo,
		boardId: 'q1',
		autorId: 'u1',
		em: '2026-08-22T12:00:00Z',
		revisao
	};
}

describe('enderecoDoSocket', () => {
	it('converte http em ws e https em wss', () => {
		expect(enderecoDoSocket('http://localhost:8080', 'http://x', 'q1')).toBe(
			'ws://localhost:8080/ws?board=q1'
		);
		expect(enderecoDoSocket('/api', 'https://stacktrack.exemplo', 'q1')).toBe(
			'wss://stacktrack.exemplo/api/ws?board=q1'
		);
	});

	// Sem cursor, o servidor responde só a posição atual — sem história. É o que
	// obriga a tela a baixar o snapshot antes de aplicar qualquer evento.
	it('omite a revisão quando não há snapshot aplicado', () => {
		const url = enderecoDoSocket('/api', 'https://x', 'q1', undefined);
		expect(url).not.toContain('revisao=');
	});

	// Zero é diferente de ausente: significa "quadro sem mutação nenhuma", e
	// pede o replay desde o começo.
	it('envia revisão zero, que não é o mesmo que ausente', () => {
		expect(enderecoDoSocket('/api', 'https://x', 'q1', 0)).toContain('revisao=0');
	});

	it('envia a revisão confirmada como cursor', () => {
		expect(enderecoDoSocket('/api', 'https://x', 'q1', 42)).toContain('revisao=42');
	});

	// O cursor é a REVISÃO, não o seq: o seq registra a ordem de alocação do
	// número e não a de commit, e usá-lo como cursor pula para sempre o que
	// comitou tarde.
	it('não usa mais o seq como cursor', () => {
		expect(enderecoDoSocket('/api', 'https://x', 'q1', 42)).not.toContain('desde=');
	});

	it('escapa o id do quadro', () => {
		expect(enderecoDoSocket('/api', 'https://x', 'a b&c', 1)).toContain('board=a%20b%26c');
	});
});

describe('pedidoDeRecargaPara', () => {
	it('reconhece exclusão como saída terminal, não como pedido de snapshot', () => {
		const terminal = evento('quadro.apagado');
		expect(eventoExigeSaidaDoQuadro(terminal)).toBe(true);
		expect(pedidoDeRecargaPara(terminal, 12)).toBeNull();
		expect(eventoExigeSaidaDoQuadro(evento('card.apagado', 12))).toBe(false);
	});

	it('força snapshot ao falar com servidor antigo sem revisão', () => {
		expect(pedidoDeRecargaPara(evento('sincronizado'), 12)).toEqual({ forcar: true });
	});

	it('não recarrega um sincronizado igual ao snapshot já aplicado', () => {
		expect(pedidoDeRecargaPara(evento('sincronizado', 12), 12)).toBeNull();
	});

	it('força snapshot e pede redefinição do cursor depois de restore', () => {
		expect(pedidoDeRecargaPara(evento('sincronizado', 11), 12)).toEqual({
			forcar: true,
			revisao: 11,
			redefinirCursor: true
		});
		expect(pedidoDeRecargaPara(evento('sincronizado', 0), 100)).toEqual({
			forcar: true,
			revisao: 0,
			redefinirCursor: true
		});
	});

	it('pede a revisão nova sem forçar GET se outro snapshot já puder cobri-la', () => {
		expect(pedidoDeRecargaPara(evento('sincronizado', 13), 12)).toEqual({
			forcar: false,
			revisao: 13
		});
		expect(pedidoDeRecargaPara(evento('card.alterado', 13), 12)).toEqual({
			forcar: false,
			revisao: 13
		});
	});

	it('sempre força o snapshot quando o servidor manda recarregar tudo', () => {
		expect(pedidoDeRecargaPara(evento('recarregue.tudo', 12), 12)).toEqual({ forcar: true });
	});
});

describe('ciclo de vida da conexão', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		WebSocketFalso.instancias.length = 0;
		vi.stubGlobal('WebSocket', WebSocketFalso);
		vi.stubGlobal('location', { origin: 'https://stacktrack.exemplo' });
	});

	afterEach(() => {
		vi.useRealTimers();
		vi.restoreAllMocks();
		vi.unstubAllGlobals();
	});

	it('abre uma vez e avançar o snapshot não reconstrói o socket', () => {
		const conexao = conectarAoQuadro('q1', vi.fn(), 7);

		expect(WebSocketFalso.instancias).toHaveLength(1);
		expect(WebSocketFalso.instancias[0].url).toContain('board=q1');
		expect(WebSocketFalso.instancias[0].url).toContain('revisao=7');

		conexao.confirmarRevisao(8);
		conexao.confirmarRevisao(9);

		expect(conexao.revisaoConfirmada).toBe(9);
		expect(WebSocketFalso.instancias).toHaveLength(1);
		expect(WebSocketFalso.instancias[0].close).not.toHaveBeenCalled();

		conexao.fechar();
	});

	it('usa a revisão confirmada mais nova somente quando uma queda exige reconexão', () => {
		vi.spyOn(Math, 'random').mockReturnValue(0);
		const conexao = conectarAoQuadro('q1', vi.fn(), 3);
		const primeira = WebSocketFalso.instancias[0];
		primeira.abrir();
		conexao.confirmarRevisao(8);

		primeira.cair();
		expect(conexao.situacao).toBe('reconectando');
		expect(WebSocketFalso.instancias).toHaveLength(1);

		// Jitter mínimo: 50% da espera inicial de 500ms.
		vi.advanceTimersByTime(250);

		expect(WebSocketFalso.instancias).toHaveLength(2);
		expect(WebSocketFalso.instancias[1].url).toContain('revisao=8');
		conexao.fechar();
	});

	it('cancelar a conexão também cancela uma reconexão já agendada', () => {
		vi.spyOn(Math, 'random').mockReturnValue(0);
		const conexao = conectarAoQuadro('q1', vi.fn(), 1);
		WebSocketFalso.instancias[0].cair();

		conexao.fechar();
		vi.advanceTimersByTime(10_000);

		expect(conexao.situacao).toBe('desligado');
		expect(WebSocketFalso.instancias).toHaveLength(1);
	});

	it('ignora revisão já aplicada sem avançar o cursor ao receber evento', () => {
		const aoEvento = vi.fn();
		const conexao = conectarAoQuadro('q1', aoEvento, 5);
		const socket = WebSocketFalso.instancias[0];

		socket.emitir(evento('card.alterado', 5));
		socket.emitir(evento('card.alterado', 6));

		expect(aoEvento).toHaveBeenCalledTimes(1);
		expect(aoEvento).toHaveBeenCalledWith(evento('card.alterado', 6));
		expect(conexao.revisaoConfirmada).toBe(5);
		conexao.fechar();
	});

	it('não descarta eventos do servidor restaurado enquanto o snapshot está pendente', () => {
		const aoEvento = vi.fn();
		const conexao = conectarAoQuadro('q1', aoEvento, 20);
		const socket = WebSocketFalso.instancias[0];

		socket.emitir(evento('sincronizado', 10));
		socket.emitir(evento('card.alterado', 11));

		expect(aoEvento).toHaveBeenNthCalledWith(1, evento('sincronizado', 10));
		expect(aoEvento).toHaveBeenNthCalledWith(2, evento('card.alterado', 11));
		expect(conexao.revisaoConfirmada).toBe(20);
		conexao.fechar();
	});

	it('só permite recuar o cursor após sincronizado regressivo e snapshot', () => {
		const conexao = conectarAoQuadro('q1', vi.fn(), 20);

		conexao.redefinirRevisaoAposSnapshot(8);
		expect(conexao.revisaoConfirmada).toBe(20);

		WebSocketFalso.instancias[0].emitir(evento('sincronizado', 10));
		conexao.confirmarRevisao(9);
		expect(conexao.revisaoConfirmada).toBe(20);

		conexao.redefinirRevisaoAposSnapshot(10);
		expect(conexao.revisaoConfirmada).toBe(10);

		// Consumida a autorização do restore, o caminho explícito volta a ser
		// monotônico e não aceita um segundo recuo acidental.
		conexao.redefinirRevisaoAposSnapshot(7);
		expect(conexao.revisaoConfirmada).toBe(10);
		conexao.confirmarRevisao(11);
		expect(conexao.revisaoConfirmada).toBe(11);
		conexao.fechar();
	});

	it('redefine de 100 para zero após restore de um quadro sem eventos', () => {
		const aoEvento = vi.fn();
		const conexao = conectarAoQuadro('q1', aoEvento, 100);

		WebSocketFalso.instancias[0].emitir(evento('sincronizado', 0));
		expect(aoEvento).toHaveBeenCalledWith(evento('sincronizado', 0));
		expect(conexao.revisaoConfirmada).toBe(100);

		conexao.redefinirRevisaoAposSnapshot(0);
		expect(conexao.revisaoConfirmada).toBe(0);
		conexao.fechar();
	});

	it('converte envelope futuro em ordem de snapshot', () => {
		const aoEvento = vi.fn();
		const conexao = conectarAoQuadro('q1', aoEvento, 5);
		WebSocketFalso.instancias[0].emitir({ ...evento('card.alterado', 6), versao: 2 });

		expect(aoEvento).toHaveBeenCalledWith(
			expect.objectContaining({ tipo: 'recarregue.tudo', revisao: 6 })
		);
		conexao.fechar();
	});

	it('só anuncia edição quando o socket está aberto', () => {
		const conexao = conectarAoQuadro('q1', vi.fn(), 1);
		const socket = WebSocketFalso.instancias[0];

		conexao.anunciarEdicao('c1');
		expect(socket.send).not.toHaveBeenCalled();

		socket.abrir();
		conexao.anunciarEdicao('c1');
		expect(socket.send).toHaveBeenCalledWith(JSON.stringify({ tipo: 'foco', colunaId: 'c1' }));
		conexao.fechar();
	});
});
