// Conexão de tempo real com o sino: um EventSource (SSE) que recebe quantos
// avisos a pessoa tem sem ler.
//
// SSE, e não o WebSocket do quadro: aqui o servidor só avisa, e a tela relê —
// marcar como lido é um POST comum. O EventSource já reabre sozinho quando a
// rede cai; o que fica por conta deste módulo é o caso em que o NAVEGADOR
// desiste (uma resposta que não é 200, como o 503 de teto de conexões) e o
// encerramento de sessão, em que reabrir não adianta.

import { BASE_URL } from '$lib/api/client';

/** A versão do envelope que este cliente sabe ler (ver adapter/http/sse). */
export const VERSAO_DO_ENVELOPE = 1;

export type SituacaoDoSino = 'conectando' | 'ao-vivo' | 'reconectando' | 'desligado';

// Espera antes de reabrir depois de o navegador desistir. Cresce até um teto,
// com sorteio: abas derrubadas juntas não voltam juntas.
const ESPERA_INICIAL = 1_000;
const ESPERA_MAXIMA = 30_000;

/** enderecoDoStream monta o endereço do stream sobre a base da API. */
export function enderecoDoStream(base: string): string {
	return `${base.replace(/\/$/, '')}/notificacoes/eventos`;
}

interface DadosDoStream {
	versao?: number;
	naoLidas?: number;
	motivo?: string;
}

function ler(bruto: string): DadosDoStream | null {
	try {
		return JSON.parse(bruto) as DadosDoStream;
	} catch {
		return null;
	}
}

/**
 * conectarAoSino abre o stream e devolve a situação (reativa) e o `fechar()`.
 *
 * `aoMudar` recebe a contagem a cada aviso do servidor — ou `null`, quando a
 * mensagem veio num formato que este cliente não conhece e a contagem precisa
 * ser relida pela API. `aoEncerrarSessao` é chamado quando o servidor diz que a
 * sessão acabou: a conexão fecha de vez.
 *
 * `criar` existe para os testes trocarem o EventSource.
 */
export function conectarAoSino(
	aoMudar: (naoLidas: number | null) => void,
	aoEncerrarSessao: () => void,
	criar: (url: string) => EventSource = (url) => new EventSource(url, { withCredentials: true })
): { readonly situacao: SituacaoDoSino; fechar: () => void } {
	let situacao = $state<SituacaoDoSino>('conectando');
	let fonte: EventSource | null = null;
	let espera = ESPERA_INICIAL;
	let agendado: ReturnType<typeof setTimeout> | null = null;
	let desligado = false;

	function abrir() {
		if (desligado) return;
		// withCredentials: em desenvolvimento a API é outra origem
		// (localhost:8080), e sem ele o cookie de sessão não vai junto.
		const atual = criar(enderecoDoStream(BASE_URL));
		fonte = atual;

		atual.onopen = () => {
			situacao = 'ao-vivo';
			// A espera só volta ao mínimo depois de uma conexão que ABRIU.
			espera = ESPERA_INICIAL;
		};

		atual.addEventListener('notificacoes', (e) => {
			const dados = ler((e as MessageEvent<string>).data);
			if (
				!dados ||
				(dados.versao ?? 1) > VERSAO_DO_ENVELOPE ||
				typeof dados.naoLidas !== 'number'
			) {
				aoMudar(null);
				return;
			}
			situacao = 'ao-vivo';
			aoMudar(dados.naoLidas);
		});

		atual.addEventListener('encerrada', (e) => {
			const dados = ler((e as MessageEvent<string>).data);
			if (dados?.motivo === 'sessao') {
				fechar();
				aoEncerrarSessao();
			}
			// Servidor reiniciando: o navegador reabre sozinho, depois do
			// `retry:` que o próprio stream informou.
		});

		atual.onerror = () => {
			if (desligado || fonte !== atual) return;
			if (atual.readyState === 2) {
				// CLOSED: o navegador desistiu e não vai tentar de novo.
				atual.close();
				fonte = null;
				situacao = 'reconectando';
				agendar();
				return;
			}
			situacao = 'reconectando';
		};
	}

	function agendar() {
		const atraso = espera * (0.5 + Math.random() / 2);
		espera = Math.min(espera * 2, ESPERA_MAXIMA);
		agendado = setTimeout(abrir, atraso);
	}

	function fechar() {
		desligado = true;
		situacao = 'desligado';
		if (agendado) clearTimeout(agendado);
		fonte?.close();
		fonte = null;
	}

	abrir();

	return {
		get situacao() {
			return situacao;
		},
		fechar
	};
}
