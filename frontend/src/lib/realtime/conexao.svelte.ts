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
	| 'quadro.criado'
	| 'quadro.renomeado'
	| 'quadro.fundo'
	| 'quadro.apagado'
	| 'quadro.publicado'
	| 'quadro.publicacao_revogada'
	| 'etiqueta.criada'
	| 'etiqueta.alterada'
	| 'etiqueta.apagada'
	| 'etiqueta.aplicada'
	| 'etiqueta.retirada'
	| 'checklist.criada'
	| 'checklist.alterada'
	| 'checklist.apagada'
	| 'item.criado'
	| 'item.alterado'
	| 'item.apagado'
	| 'anexo.adicionado'
	| 'anexo.removido'
	| 'responsavel.atribuido'
	| 'responsavel.removido'
	| 'comentario.criado'
	| 'comentario.editado'
	| 'comentario.apagado'
	| 'membro.entrou'
	| 'membro.papel'
	| 'membro.removido'
	| 'convite.criado'
	| 'convite.revogado'
	// Manutenção: o comando que redistribui chaves de ordenação duplicadas.
	// Chega como qualquer mutação e a tela reconcilia — o autor vem vazio
	// porque não há pessoa por trás dele.
	| 'ordenacao.reparada'
	// membro.adicionado passou a ser LEGADO: o caminho de pôr no quadro, na
	// hora, quem já tinha conta foi removido em A1. Ninguém o produz mais, e as
	// linhas antigas do log continuam precisando ser lidas.
	| 'membro.adicionado'
	// Tipos legados: o log é append-only e a reconexão ainda pode entregá-los.
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
	/**
	 * A coluna que esta pessoa está editando agora, quando está editando alguma.
	 *
	 * Vem junto da presença, e não num evento próprio: é a mesma pergunta —
	 * "quem está aqui, e fazendo o quê" — e um segundo canal teria de repetir a
	 * entrada, a saída e a deduplicação por pessoa.
	 */
	editandoColunaId?: string;
}

/**
 * Versão do envelope que este cliente sabe ler.
 *
 * Mensagem com versão maior NÃO é aplicada nem confirmada: o significado dos
 * campos pode ter mudado, e aplicar pela metade é pior que não aplicar. O
 * cliente busca um snapshot, que é sempre correto.
 */
export const VERSAO_DO_ENVELOPE = 1;

export interface EventoDoQuadro {
	/** Versão do formato. Ausente nas mensagens do servidor anterior. */
	versao?: number;
	/**
	 * Identidade global e imutável do evento.
	 *
	 * ⚠️ NÃO é cursor. Sendo BIGSERIAL no banco, ele registra a ordem de
	 * ALOCAÇÃO do número e não a de COMMIT: duas escritas concorrentes pegam 42
	 * e 43 nessa ordem e podem comitar na inversa. Quem avança o cursor por seq
	 * recebe o 43, passa a ignorar tudo abaixo dele, e nunca mais vê o 42.
	 * O cursor é `revisao`.
	 */
	seq?: number;
	/** Posição na história DAQUELE quadro: contígua e na ordem de commit. */
	revisao?: number;
	/** Posição dentro da revisão, e quantos eventos a formam. */
	indice?: number;
	quantidade?: number;
	tipo: TipoDeEvento;
	boardId: string;
	/** Card afetado, quando o evento pertence a uma projeção de card. */
	cardId?: string;
	autorId: string;
	em: string;
	dados?: unknown;
}

/**
 * Identifica o único sinal terminal do quadro.
 *
 * Ele não pede snapshot: o recurso já não existe e insistir no GET criaria um
 * ciclo de 404/reconexão. A página fecha o socket e volta ao painel.
 */
export function eventoExigeSaidaDoQuadro(evento: EventoDoQuadro): boolean {
	return evento.tipo === 'quadro.apagado';
}

/**
 * O que a tela precisa fazer para transformar um evento em estado aplicado.
 *
 * Quando há revisão, a recarga pode ser dispensada se outro caminho (por
 * exemplo, a resposta de uma mutação feita nesta aba) já trouxe um snapshot
 * igual ou mais novo. Sem revisão não existe essa prova: o único caminho
 * seguro é buscar o snapshot.
 */
export interface PedidoDeRecarga {
	forcar: boolean;
	revisao?: number;
	/**
	 * Autoriza substituir o cursor, mas somente depois de um novo snapshot.
	 *
	 * Aparece quando o servidor informa uma revisão menor que a confirmada — o
	 * caso esperado depois de restaurar um backup. Não se pode simplesmente
	 * ignorar a mensagem, nem fazer o cursor andar para trás antes da leitura:
	 * em ambos os casos abre-se uma janela em que eventos podem sumir.
	 */
	redefinirCursor?: boolean;
}

/**
 * Decide se um evento exige snapshot sem confundir "recebi" com "apliquei".
 *
 * Também mantém compatibilidade com o servidor anterior, cujo `sincronizado`
 * não carregava revisão. Nesse caso a tela precisa buscar uma vez o estado;
 * confirmar ou ignorar a mensagem deixaria o cliente novo sem saber em que
 * ponto da história está.
 */
export function pedidoDeRecargaPara(
	evento: EventoDoQuadro,
	revisaoConfirmada: number | undefined
): PedidoDeRecarga | null {
	if (eventoExigeSaidaDoQuadro(evento)) return null;
	if (evento.tipo === 'presenca.alterada') return null;

	if (evento.tipo === 'recarregue.tudo') return { forcar: true };

	if (evento.tipo === 'sincronizado') {
		if (evento.revisao === undefined) return { forcar: true };
		if (revisaoConfirmada !== undefined && evento.revisao < revisaoConfirmada) {
			return { forcar: true, revisao: evento.revisao, redefinirCursor: true };
		}
		if (evento.revisao === revisaoConfirmada) return null;
		return { forcar: false, revisao: evento.revisao };
	}

	// Eventos do protocolo anterior não tinham revisão. O seq não substitui o
	// cursor por quadro, portanto não há deduplicação segura além do snapshot.
	if (evento.revisao === undefined) return { forcar: true };

	return { forcar: false, revisao: evento.revisao };
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
	revisao?: number
): string {
	const absoluta = /^https?:\/\//.test(base);
	const url = new URL(absoluta ? base : `${origem}${base}`);
	url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
	url.pathname = `${url.pathname.replace(/\/$/, '')}/ws`;
	url.search = `?board=${encodeURIComponent(boardId)}`;
	// A REVISÃO é o cursor, e vai a que a tela CONFIRMOU — a do último snapshot
	// aplicado com sucesso, nunca um número que o cliente tenha incrementado por
	// conta própria. Confirmar o que não se aplicou deixaria o cursor à frente
	// do estado real, e o que faltou nunca mais seria entregue.
	//
	// `undefined` é a conexão sem snapshot: o servidor responde só a posição
	// atual, sem história, o que obriga a tela a baixar o estado antes de
	// aplicar qualquer coisa.
	if (revisao !== undefined) url.search += `&revisao=${revisao}`;
	return url.toString();
}

/**
 * conectarAoQuadro abre a conexão e devolve um objeto com a situação (reativa)
 * e o `fechar()` que a tela chama ao sair.
 *
 * `aoEvento` recebe cada evento — INCLUSIVE os causados pela própria conta.
 * O servidor deixou de filtrar o eco por autor em A3, porque filtrar por autor
 * filtrava também as OUTRAS abas da mesma pessoa: em dois monitores, a segunda
 * tela parava de receber o que a primeira fazia. A revisão deixa a aba autora
 * dispensar a recarga se a resposta da própria mutação já atualizou o snapshot,
 * sem dispensá-la na outra aba.
 *
 * `revisaoInicial` é a revisão do snapshot que a tela já tem em mãos. Sem ela,
 * a conexão começa sem cursor e o servidor manda buscar o estado antes de
 * aplicar qualquer coisa.
 *
 * `anunciarEdicao` é o único caminho de volta: o cliente fala com o servidor
 * por aqui, e só para dizer o que está editando.
 */
export function conectarAoQuadro(
	boardId: string,
	aoEvento: (e: EventoDoQuadro) => void,
	revisaoInicial?: number
): {
	readonly situacao: Situacao;
	readonly revisaoConfirmada: number | undefined;
	confirmarRevisao: (revisao: number) => void;
	redefinirRevisaoAposSnapshot: (revisao: number) => void;
	anunciarEdicao: (colunaId: string | null) => void;
	fechar: () => void;
} {
	let situacao = $state<Situacao>('conectando');
	let socket: WebSocket | null = null;

	// A revisão que a tela CONSEGUIU APLICAR — nada além disso.
	//
	// Ela avança por um caminho só: `confirmarRevisao`, chamada pela tela depois
	// de um snapshot baixado e aplicado com sucesso. Nenhum evento a incrementa
	// sozinho, e essa é a regra que sustenta o protocolo: o servidor repõe tudo
	// a partir daqui, então confirmar o que não se aplicou abre um buraco
	// permanente — o que faltou fica abaixo do cursor e nunca mais é entregue.
	//
	// undefined = nenhum snapshot aplicado ainda. É diferente de zero, que
	// significa "quadro sem mutação nenhuma" e faria o servidor repor a história
	// inteira.
	let revisaoConfirmada = $state<number | undefined>(revisaoInicial);
	// Fica ativo entre o `sincronizado` regressivo e a aplicação do snapshot.
	// Nesse intervalo os eventos do servidor restaurado também têm revisão
	// menor que o cursor antigo e, por isso, não podem cair na deduplicação.
	let restauracaoPendente = false;
	let espera = ESPERA_INICIAL;
	let agendado: ReturnType<typeof setTimeout> | null = null;
	let desligado = false;

	function abrir() {
		if (desligado) return;

		socket = new WebSocket(enderecoDoSocket(BASE_URL, location.origin, boardId, revisaoConfirmada));

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

			// VERSÃO DESCONHECIDA não é aplicada nem confirmada. O envelope pode
			// ter mudado o significado de um campo, e aplicar metade de um
			// formato que não se entende é pior que não aplicar: a tela ficaria
			// com um estado que ninguém consegue reproduzir. `recarregue.tudo`
			// é a saída sempre correta.
			if (e.versao !== undefined && e.versao > VERSAO_DO_ENVELOPE) {
				aoEvento({ ...e, tipo: 'recarregue.tudo' });
				return;
			}

			const mensagemDeControle = e.tipo === 'sincronizado' || e.tipo === 'recarregue.tudo';
			if (
				e.tipo === 'sincronizado' &&
				e.revisao !== undefined &&
				revisaoConfirmada !== undefined &&
				e.revisao < revisaoConfirmada
			) {
				restauracaoPendente = true;
			}

			// IDEMPOTÊNCIA. Um evento já aplicado chega de novo quando o replay
			// e o ao vivo se sobrepõem — e vai chegar, porque a assinatura
			// acontece antes da reposição, de propósito: é o que garante que
			// nada se perca ENTRE os dois. Preferimos repetir a arriscar buraco,
			// e a revisão torna a repetição inofensiva.
			//
			// Repare que isto NÃO avança o cursor: descartar um evento é
			// diferente de aplicá-lo. Quem avança é `confirmarRevisao`.
			if (
				!mensagemDeControle &&
				!restauracaoPendente &&
				e.revisao !== undefined &&
				e.revisao > 0 &&
				revisaoConfirmada !== undefined &&
				e.revisao <= revisaoConfirmada
			) {
				return;
			}

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
		get revisaoConfirmada() {
			return revisaoConfirmada;
		},
		/**
		 * confirmarRevisao registra até onde a tela CONSEGUIU aplicar.
		 *
		 * Chamada pela tela depois de um snapshot baixado e aplicado — nunca ao
		 * receber um evento. É essa separação que impede o cursor de passar à
		 * frente do estado real: o servidor repõe a partir daqui, e o que ficar
		 * abaixo do cursor não é entregue de novo.
		 *
		 * Nunca anda para trás: um snapshot mais velho chegando depois de um
		 * mais novo (duas requisições concorrentes, a lenta respondendo por
		 * último) não pode desfazer o que já foi confirmado.
		 */
		confirmarRevisao(revisao: number) {
			if (revisaoConfirmada === undefined || revisao > revisaoConfirmada) {
				revisaoConfirmada = revisao;
			}
		},
		/**
		 * Substitui o cursor depois de um restore confirmado por snapshot.
		 *
		 * O recuo só é aceito se esta conexão recebeu antes um `sincronizado`
		 * regressivo. Isso mantém o caminho normal monotônico e impede que uma
		 * resposta HTTP antiga faça o cursor andar para trás por acidente.
		 */
		redefinirRevisaoAposSnapshot(revisao: number) {
			if (!restauracaoPendente) return;
			revisaoConfirmada = revisao;
			restauracaoPendente = false;
		},
		/**
		 * anunciarEdicao avisa a sala de que esta pessoa começou (ou parou, com
		 * `null`) de editar uma coluna.
		 *
		 * Silencioso quando o socket não está aberto: quem está editando durante
		 * uma queda de conexão simplesmente não aparece para os outros, e forçar
		 * um erro aqui transformaria uma informação acessória num problema no
		 * meio da digitação. Ao reconectar, o `$effect` da coluna reanuncia.
		 */
		anunciarEdicao(colunaId: string | null) {
			if (socket?.readyState !== WebSocket.OPEN) return;
			socket.send(JSON.stringify({ tipo: 'foco', colunaId: colunaId ?? '' }));
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
