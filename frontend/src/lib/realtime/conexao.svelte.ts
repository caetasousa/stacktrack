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
	| 'quadro.alterado'
	| 'comentario.alterado'
	| 'membros.alterados'
	| 'presenca.alterada'
	// Controle da reconexão: não descrevem mudança no quadro, dizem em que
	// ponto da história o cliente está.
	| 'sincronizado'
	| 'recarregue.tudo';

/** Quem está com o quadro aberto agora. Estado efêmero: não existe no banco. */
export interface Presente {
	id: string;
	nome: string;
}

export interface EventoDoQuadro {
	/** Posição na história do quadro. Ausente nos eventos que não são gravados. */
	seq?: number;
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
export function enderecoDoSocket(
	base: string,
	origem: string,
	boardId: string,
	desde?: number
): string {
	const absoluta = /^https?:\/\//.test(base);
	const url = new URL(absoluta ? base : `${origem}${base}`);
	url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
	url.pathname = `${url.pathname.replace(/\/$/, '')}/ws`;
	url.search = `?board=${encodeURIComponent(boardId)}`;
	// `desde` só vai numa RECONEXÃO. Na primeira conexão não há o que repor, e
	// pedir "desde 0" traria a história inteira do quadro para jogar fora.
	if (desde !== undefined) url.search += `&desde=${desde}`;
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
	// Até onde a tela já aplicou. É o que vai em ?desde= na reconexão, e é o
	// que torna reaplicar inofensivo: evento com seq menor ou igual já foi
	// visto e é descartado.
	//
	// undefined = nunca conectou. Zero seria diferente: significaria "quadro
	// sem história", e mandaria o servidor repor desde o começo.
	let ultimoSeq: number | undefined;
	let espera = ESPERA_INICIAL;
	let agendado: ReturnType<typeof setTimeout> | null = null;
	let desligado = false;

	function abrir() {
		if (desligado) return;

		socket = new WebSocket(enderecoDoSocket(BASE_URL, location.origin, boardId, ultimoSeq));

		socket.onopen = () => {
			situacao = 'ao-vivo';
			// A espera só volta ao mínimo depois de uma conexão que ABRIU. Zerar
			// no início da tentativa faria um servidor que aceita e derruba na
			// hora ser martelado a cada 500ms.
			espera = ESPERA_INICIAL;
		};

		socket.onmessage = (msg) => {
			let e: EventoDoQuadro;
			try {
				e = JSON.parse(msg.data) as EventoDoQuadro;
			} catch {
				// Frame ilegível não derruba a conexão: o resto continua valendo.
				return;
			}

			// IDEMPOTÊNCIA. Um evento já aplicado chega de novo quando o backlog
			// e o ao vivo se sobrepõem — e vai chegar, porque a assinatura
			// acontece antes da reposição, de propósito: é o que garante que
			// nada se perca ENTRE os dois. Preferimos repetir a arriscar buraco,
			// e o seq torna a repetição inofensiva.
			if (e.seq !== undefined && ultimoSeq !== undefined && e.seq <= ultimoSeq) return;
			if (e.seq !== undefined) ultimoSeq = e.seq;

			aoEvento(e);
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
