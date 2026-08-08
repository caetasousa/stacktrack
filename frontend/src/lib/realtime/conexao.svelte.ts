// Conexão de tempo real com o quadro.
//
// Abre um WebSocket ao entrar no quadro, avisa quem estiver ouvindo a cada
// evento e fecha ao sair. Reconecta sozinha com espera crescente — cair é o
// estado normal de uma conexão longa, não a exceção.

import { BASE_URL } from '$lib/api/client';

/** Tipos que o servidor emite (ver internal/domain/evento). */
export type TipoDeEvento =
	| 'coluna.criada'
	| 'coluna.alterada'
	| 'coluna.apagada'
	| 'coluna.movida'
	| 'card.criado'
	| 'card.alterado'
	| 'card.apagado'
	| 'card.movido'
	| 'quadro.alterado';

export interface EventoDoQuadro {
	tipo: TipoDeEvento;
	boardId: string;
	autorId: string;
	em: string;
	dados?: unknown;
}

/** Estado da conexão, para a tela poder dizer a verdade a quem usa. */
export type Situacao = 'conectando' | 'ao-vivo' | 'reconectando' | 'desligado';

// Espera antes de tentar de novo, em ms. Cresce até um teto: sem isso, um
// servidor fora do ar seria martelado por todas as abas abertas ao mesmo tempo.
const ESPERA_INICIAL = 500;
const ESPERA_MAXIMA = 15_000;

/**
 * enderecoDoSocket converte a base HTTP da API no endereço ws://.
 *
 * `BASE_URL` é relativa em produção (`/api`), porque front e API compartilham a
 * origem — então o esquema vem de `location`, e https vira wss. Em
 * desenvolvimento ela é absoluta (http://localhost:8080) e o http vira ws.
 */
export function enderecoDoSocket(base: string, origem: string, boardId: string): string {
	const absoluta = /^https?:\/\//.test(base);
	const url = new URL(absoluta ? base : `${origem}${base}`);
	url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
	url.pathname = `${url.pathname.replace(/\/$/, '')}/ws`;
	url.search = `?board=${encodeURIComponent(boardId)}`;
	return url.toString();
}

/**
 * conectarAoQuadro abre a conexão e devolve um objeto com a situação (reativa)
 * e o `fechar()` que a tela chama ao sair.
 *
 * `aoEvento` recebe cada evento. O servidor já filtra o eco do próprio autor,
 * então tudo que chega aqui foi feito por outra pessoa.
 */
export function conectarAoQuadro(
	boardId: string,
	aoEvento: (e: EventoDoQuadro) => void
): { readonly situacao: Situacao; fechar: () => void } {
	let situacao = $state<Situacao>('conectando');
	let socket: WebSocket | null = null;
	let espera = ESPERA_INICIAL;
	let agendado: ReturnType<typeof setTimeout> | null = null;
	let desligado = false;

	function abrir() {
		if (desligado) return;

		socket = new WebSocket(enderecoDoSocket(BASE_URL, location.origin, boardId));

		socket.onopen = () => {
			situacao = 'ao-vivo';
			// A espera só volta ao mínimo depois de uma conexão que ABRIU. Zerar
			// no início da tentativa faria um servidor que aceita e derruba na
			// hora ser martelado a cada 500ms.
			espera = ESPERA_INICIAL;
		};

		socket.onmessage = (msg) => {
			try {
				aoEvento(JSON.parse(msg.data) as EventoDoQuadro);
			} catch {
				// Frame que não é JSON não derruba a conexão: o resto continua
				// valendo, e insistir num evento ilegível não ajudaria ninguém.
			}
		};

		socket.onclose = () => {
			socket = null;
			if (desligado) return;
			situacao = 'reconectando';
			agendar();
		};

		// O erro é sempre seguido de close, que é quem reagenda — tratar aqui
		// também agendaria duas vezes.
		socket.onerror = () => {};
	}

	function agendar() {
		// Jitter: sem ele, cinquenta abas derrubadas juntas voltariam juntas, e
		// o servidor que acabou de subir levaria a mesma rajada que o derrubou.
		const atraso = espera * (0.5 + Math.random() / 2);
		espera = Math.min(espera * 2, ESPERA_MAXIMA);
		agendado = setTimeout(abrir, atraso);
	}

	abrir();

	return {
		get situacao() {
			return situacao;
		},
		fechar() {
			desligado = true;
			situacao = 'desligado';
			if (agendado) clearTimeout(agendado);
			// close() sem código, para o navegador enviar o fechamento normal —
			// é o que faz o servidor liberar a sala em vez de esperar o ping.
			socket?.close();
			socket = null;
		}
	};
}
