<script lang="ts">
	import { tick, untrack } from 'svelte';
	import { confirmar } from '$lib/confirmar.svelte';
	import { goto, invalidateAll } from '$app/navigation';
	import {
		apagarBoard,
		criarColuna,
		renomearBoard,
		FUNDOS,
		type Cor,
		type Fundo
	} from '$lib/api/boards';
	import {
		apagarEtiqueta,
		criarEtiqueta,
		definirFundo,
		editarEtiqueta,
		moverCard,
		moverColuna
	} from '$lib/api/extras';
	import type { Card, Coluna } from '$lib/api/boards';
	import { ApiError } from '$lib/api/client';
	import { dndzone, type DndEvent } from 'svelte-dnd-action';
	import { flip } from 'svelte/animate';
	import {
		CLASSES_ALVO,
		DURACAO_MS,
		enfeitarArrastado,
		ESPERA_DE_TOQUE_MS,
		TIPO_COLUNA,
		vizinhosDe
	} from '$lib/arrastar';
	import {
		conectarAoQuadro,
		eventoExigeSaidaDoQuadro,
		pedidoDeRecargaPara,
		type EventoDoQuadro,
		type PedidoDeRecarga,
		type Presente
	} from '$lib/realtime/conexao.svelte';
	import {
		esperaParaReconciliar,
		executarDepoisDaAtual,
		executarPreservandoFalha,
		JANELA_DE_RAJADA_MS,
		reconciliarSnapshots,
		type RecarregarProjecaoAtiva
	} from '$lib/realtime/reconciliacao';
	import { iniciais } from '$lib/iniciais';
	import { sessao } from '$lib/stores/session.svelte';
	import {
		FILTRO_VAZIO,
		filtrando as estaFiltrando,
		passa,
		pessoasDoQuadro,
		type Filtro
	} from '$lib/filtro';
	import ColunaDoQuadro from '$lib/components/ColunaDoQuadro.svelte';
	import DialogoDeCompartilhamento from '$lib/components/DialogoDeCompartilhamento.svelte';
	import ModalDoCard from '$lib/components/ModalDoCard.svelte';
	import SeletorDeCor from '$lib/components/SeletorDeCor.svelte';

	let { data } = $props();
	// `data` inteiro muda a cada invalidateAll(), mesmo quando a navegação
	// continua no mesmo quadro. A derivação preserva a identidade primitiva e
	// impede que uma revisão nova reconstrua o WebSocket do mesmo quadro.
	const boardId = $derived(data.quadro.id);

	// O arraste precisa de uma cópia LOCAL: a lista que vem do load() é o estado
	// do servidor, e mexer nela direto faria a tela discordar do que a API sabe.
	// Esta cópia é a "atualização otimista" — o card se move na hora, e se a API
	// recusar ela volta a ser o que o servidor diz.
	let colunas = $state<Coluna[]>([]);

	// Re-sincroniza sempre que o load() traz dados novos (criar, apagar,
	// recarregar). Depende só de data.quadro, então mexer em `colunas` durante o
	// arraste não dispara isto de volta.
	$effect(() => {
		colunas = data.quadro.colunas.map((c) => ({ ...c, cards: [...c.cards] }));
	});

	// --- tempo real ---------------------------------------------------------
	//
	// A conexão nasce e morre com a página do quadro: $effect devolve a função
	// de limpeza, e o SvelteKit a chama ao navegar para fora. Sem isso, trocar
	// de quadro deixaria a sala antiga aberta e a pessoa receberia eventos de
	// um quadro que não está mais olhando.
	//
	// A dependência é `data.quadro.id`: mudar de quadro fecha uma conexão e abre
	// a outra, sem passar pelo estado "duas abertas".
	let conexao = $state<{
		readonly situacao: string;
		readonly revisaoConfirmada: number | undefined;
		confirmarRevisao: (revisao: number) => void;
		redefinirRevisaoAposSnapshot: (revisao: number) => void;
		anunciarEdicao: (colunaId: string | null) => void;
		fechar: () => void;
	} | null>(null);

	$effect(() => {
		const id = boardId;
		// A revisão do snapshot que já está na tela entra como cursor inicial:
		// a conexão pede ao servidor o que aconteceu DEPOIS dela, em vez de
		// recomeçar do zero ou aplicar por cima de um estado que não sabe de
		// onde veio. `untrack` é essencial: o ciclo de vida é do QUADRO, não do
		// snapshot. Sem ele cada revisão fecharia o socket e abriria outro.
		const revisaoInicial = untrack(() => data.quadro.revisao);
		const c = conectarAoQuadro(id, aoReceberEvento, revisaoInicial);
		conexao = c;
		return () => {
			c.fechar();
			if (conexao === c) conexao = null;
			presentes = [];
			// Uma recarga agendada que dispare depois da saída invalidaria dados
			// de um quadro que já não está na tela.
			if (recargaAgendada) {
				clearTimeout(recargaAgendada);
				recargaAgendada = null;
			}
			recargaForcada = false;
			revisaoPendente = undefined;
			redefinirCursorPendente = false;
			ultimaReconciliacaoEm = undefined;
			recargaEmCurso = null;
		};
	});

	// Toda mudança de outra pessoa recarrega o quadro.
	//
	// É deliberadamente grosseiro para a primeira versão: o servidor já manda o
	// tipo e o payload de cada evento, mas aplicar diferença aqui significaria
	// manter um caminho de aplicação por tipo — e qualquer um deles errado
	// deixaria a tela discordando do banco em silêncio, que é o pior defeito
	// possível num quadro colaborativo. Recarregar é sempre correto; o custo é
	// uma requisição.
	//
	// O evento da PRÓPRIA conta também chega aqui desde A3 — o servidor deixou
	// de filtrar por autor, porque filtrar por autor filtrava junto as outras
	// abas da mesma pessoa. A revisão permite dispensar o segundo GET apenas na
	// aba que já atualizou o snapshot pela resposta da mutação; a outra continua
	// recarregando normalmente.
	function aoReceberEvento(e: EventoDoQuadro) {
		// Um frame já enfileirado pelo socket anterior pode chegar durante a troca
		// de rota. Ele nunca deve alimentar o agendador do quadro novo.
		if (e.boardId !== boardId) return;

		// Exclusão é terminal: não existe snapshot para buscar. Fechar antes de
		// navegar impede a reconexão de assinar de novo uma sala cujo quadro já
		// responde 404.
		if (eventoExigeSaidaDoQuadro(e)) {
			conexao?.fechar();
			void goto('/painel');
			return;
		}

		// Presença é estado efêmero: aplica direto, sem ir ao banco. Recarregar o
		// quadro só porque alguém abriu a aba seria uma requisição por entrada e
		// saída de cada pessoa.
		if (e.tipo === 'presenca.alterada') {
			presentes = (e.dados as Presente[]) ?? [];
			return;
		}

		// `sincronizado` não confirma nada sozinho: confirmar é dizer "apliquei
		// até aqui", e só um snapshot permite essa afirmação. A decisão também
		// cobre o servidor anterior, que mandava `sincronizado` sem revisão e por
		// isso exige uma busca sem reconstruir a conexão.
		const pedido = pedidoDeRecargaPara(e, conexao?.revisaoConfirmada);
		if (pedido) agendarRecarga(pedido);
	}

	let recargaAgendada: ReturnType<typeof setTimeout> | null = null;
	let recargaForcada = false;
	let revisaoPendente: number | undefined;
	let redefinirCursorPendente = false;
	let recargaEmCurso: Promise<void> | null = null;
	let ultimaReconciliacaoEm: number | undefined;
	let recarregarProjecaoAtiva: RecarregarProjecaoAtiva | null = null;

	function registrarRecargaDoModal(recarregarModal: RecarregarProjecaoAtiva | null) {
		recarregarProjecaoAtiva = recarregarModal;
	}

	// Junta eventos próximos numa recarga só.
	//
	// A primeira rajada espera 150 ms. Sob carga contínua, a próxima execução
	// espera até completar 3 s desde a anterior. `invalidateAll` relê sessão e
	// quadro; com card, membros e histórico abertos são até cinco GETs por lote:
	// cerca de 100/min, ainda abaixo do teto de 120 da sessão. A contrapartida
	// explícita é até 3 s de latência visual sob mudanças sem parar.
	function agendarRecarga(pedido: PedidoDeRecarga) {
		recargaForcada ||= pedido.forcar;
		redefinirCursorPendente ||= pedido.redefinirCursor ?? false;
		if (pedido.revisao !== undefined) {
			revisaoPendente = Math.max(revisaoPendente ?? 0, pedido.revisao);
		}
		if (recargaAgendada) return;
		recargaAgendada = setTimeout(() => {
			recargaAgendada = null;
			void executarRecargaAgendada();
		}, JANELA_DE_RAJADA_MS);
	}

	async function executarRecargaAgendada() {
		const conexaoDestaExecucao = conexao;
		if (!conexaoDestaExecucao) return;

		const jaAplicou = () =>
			!recargaForcada &&
			revisaoPendente !== undefined &&
			conexao?.revisaoConfirmada !== undefined &&
			conexao.revisaoConfirmada >= revisaoPendente;

		// Uma mutação desta aba já costuma ter iniciado invalidateAll(). Esperar
		// esse snapshot antes de abrir outro elimina o GET duplicado; a revisão
		// ainda protege a outra aba, que não tem recarga própria para esperar.
		if (recargaEmCurso) {
			try {
				await recargaEmCurso;
				await tick();
			} catch {
				// A tentativa que falhou não prova que o evento foi aplicado. Continua
				// abaixo e faz a recarga pedida pelo tempo real.
			}
		}
		if (conexao !== conexaoDestaExecucao) return;

		if (jaAplicou()) {
			recargaForcada = false;
			revisaoPendente = undefined;
			redefinirCursorPendente = false;
			return;
		}

		const espera = esperaParaReconciliar(performance.now(), ultimaReconciliacaoEm);
		if (espera > 0) {
			// Os pedidos continuam nos acumuladores enquanto se espera; eventos que
			// chegarem neste intervalo entram no mesmo snapshot.
			recargaAgendada = setTimeout(() => {
				recargaAgendada = null;
				void executarRecargaAgendada();
			}, espera);
			return;
		}

		const pedidoEmExecucao: PedidoDeRecarga = {
			forcar: recargaForcada,
			revisao: revisaoPendente,
			redefinirCursor: redefinirCursorPendente
		};
		const conexaoDoPedido = conexao;
		recargaForcada = false;
		revisaoPendente = undefined;
		redefinirCursorPendente = false;
		try {
			await executarPreservandoFalha(
				pedidoEmExecucao,
				(pedido) => recarregar({ redefinirCursor: pedido.redefinirCursor }),
				(pedido) => {
					// Não ressuscita um timer do quadro anterior após a navegação.
					if (conexao === conexaoDoPedido && conexaoDoPedido) agendarRecarga(pedido);
				}
			);
		} catch (e) {
			tratar(e, 'não foi possível atualizar o quadro');
		}
	}

	// Quem está com este quadro aberto agora, incluindo você.
	let presentes = $state<Presente[]>([]);

	// Quem está editando cada coluna, por id de coluna.
	//
	// Derivado da presença, e não de um estado próprio: a presença já chega
	// completa a cada mudança, então não há o que sincronizar — e quem fecha a
	// aba sem avisar desaparece da lista pelo mesmo caminho que já fazia o
	// avatar sumir. Um mapa de "está editando" mantido à parte ficaria com
	// cadeados de gente que não está mais lá.
	//
	// O PRÓPRIO usuário sai de fora: ele sabe que está editando, está com o
	// formulário aberto na frente dele, e ver o próprio nome no rodapé só
	// confundiria.
	const editandoPorColuna = $derived.by(() => {
		const porColuna = new Map<string, string[]>();
		for (const p of presentes) {
			if (!p.editandoColunaId || p.id === sessao.usuario?.id) continue;
			porColuna.set(p.editandoColunaId, [...(porColuna.get(p.editandoColunaId) ?? []), p.nome]);
		}
		return porColuna;
	});

	// --- filtro ---------------------------------------------------------------
	//
	// Aplicado no cliente, sobre o quadro que já está carregado: ele veio inteiro
	// numa requisição só, então filtrar no servidor custaria uma ida e volta para
	// esconder o que a tela já tem na mão. A regra em si vive em $lib/filtro,
	// onde dá para testá-la — um predicado errado não quebra nada, só some com
	// cards, e quem olha conclui que foram apagados.
	let filtro = $state<Filtro>({ ...FILTRO_VAZIO });

	const filtrando = $derived(estaFiltrando(filtro));
	const pessoasNoQuadro = $derived(pessoasDoQuadro(colunas));

	// A lista filtrada é uma VISTA: `colunas` continua sendo o estado real, que é
	// o que o arraste manipula. Enquanto o filtro está ligado o arraste fica
	// travado, porque os vizinhos calculados a partir de uma lista incompleta
	// colocariam o card entre cards que não são os vizinhos de verdade.
	const colunasVisiveis = $derived(
		filtrando
			? colunas.map((c) => ({ ...c, cards: c.cards.filter((card) => passa(card, filtro)) }))
			: colunas
	);

	const cardsVisiveis = $derived(colunasVisiveis.reduce((total, c) => total + c.cards.length, 0));

	function limparFiltro() {
		filtro = { ...FILTRO_VAZIO };
	}

	let erro = $state('');
	let renomeando = $state(false);
	let titulo = $state('');
	let tituloDaColuna = $state('');
	let corDaColuna = $state<Cor | ''>('');
	let criandoColuna = $state(false);
	// Qual card está aberto no modal — null é modal fechado.
	let cardAberto = $state<string | null>(null);
	let painelDeAjustes = $state(false);
	let compartilhando = $state(false);
	let nomeDaEtiqueta = $state('');
	let corDaEtiqueta = $state<Cor>('azul');

	const podeEditar = $derived(data.quadro.papel === 'dono' || data.quadro.papel === 'editor');
	const podeAdministrar = $derived(data.quadro.papel === 'dono');

	const cores: Cor[] = ['cinza', 'vermelho', 'laranja', 'amarelo', 'verde', 'azul', 'roxo', 'rosa'];
	const nomeDoFundo: Record<Fundo, string> = {
		padrao: 'Padrão',
		ardosia: 'Ardósia',
		oceano: 'Oceano',
		floresta: 'Floresta',
		ameixa: 'Ameixa',
		brasa: 'Brasa'
	};

	// Recarrega todas as projeções visíveis em ordem. O cursor só é confirmado
	// depois do quadro e do modal ativo; a menor revisão entre ambos é o limite
	// que a tela realmente provou ter aplicado.
	function recarregar(opcoes: { redefinirCursor?: boolean } = {}): Promise<void> {
		if (recargaEmCurso) {
			const anterior = recargaEmCurso;
			const id = boardId;
			const conexaoAoSolicitar = conexao;
			return executarDepoisDaAtual(anterior, async () => {
				if (!conexaoAoSolicitar || conexao !== conexaoAoSolicitar || boardId !== id) return;
				await recarregar(opcoes);
			});
		}
		erro = '';
		const id = boardId;
		const conexaoDestaRecarga = conexao;
		ultimaReconciliacaoEm = performance.now();

		const execucao = reconciliarSnapshots({
			carregarQuadro: async () => {
				await invalidateAll();
				await tick();
				if (boardId !== id) return undefined;

				// Se o card sumiu no snapshot, o modal deixou de ser uma projeção
				// ativa. Fechá-lo evita transformar uma exclusão válida num 404 que
				// bloquearia o cursor indefinidamente.
				if (
					cardAberto &&
					!data.quadro.colunas.some((coluna) => coluna.cards.some((card) => card.id === cardAberto))
				) {
					cardAberto = null;
					await tick();
				}
				return data.quadro.revisao;
			},
			obterProjecaoAtiva: () => (boardId === id ? recarregarProjecaoAtiva : null),
			confirmar: (revisao) => {
				// Uma navegação não pode confirmar no socket antigo nem emprestar a
				// revisão do quadro anterior à conexão nova.
				if (boardId !== id || conexao !== conexaoDestaRecarga || !conexaoDestaRecarga) return;
				if (opcoes.redefinirCursor) {
					conexaoDestaRecarga.redefinirRevisaoAposSnapshot(revisao);
				} else {
					conexaoDestaRecarga.confirmarRevisao(revisao);
				}
			}
		}).then(() => undefined);
		const acompanhada = execucao.finally(() => {
			if (recargaEmCurso === acompanhada) recargaEmCurso = null;
		});
		recargaEmCurso = acompanhada;
		return acompanhada;
	}

	// --- arrastar e soltar --------------------------------------------------

	// Prévia do arraste: só espelha o que a biblioteca calculou, sem tocar na API.
	function espelharCards(colunaId: string, cards: Card[]) {
		colunas = colunas.map((c) => (c.id === colunaId ? { ...c, cards } : c));
	}

	// Ao soltar: aplica na tela, manda para a API e — se ela recusar — desfaz.
	// O `antes` é tirado do estado do SERVIDOR, e não da cópia local, que a esta
	// altura já foi mexida pela biblioteca.
	async function soltarCard(colunaId: string, cards: Card[], cardMovidoId: string | null) {
		espelharCards(colunaId, cards);
		if (!cardMovidoId) return;

		// A biblioteca dispara o finalize nas DUAS colunas envolvidas: na de
		// origem, o card já não está na lista, e é a de destino que manda.
		if (!cards.some((c) => c.id === cardMovidoId)) return;

		try {
			await moverCard(cardMovidoId, { colunaId, ...vizinhosDe(cards, cardMovidoId) });
		} catch (e) {
			tratar(e, 'não foi possível mover o card');
			await recarregar();
		}
	}

	function espelharColunas(novas: Coluna[]) {
		colunas = novas;
	}

	async function soltarColuna(novas: Coluna[], colunaMovidaId: string | null) {
		espelharColunas(novas);
		if (!colunaMovidaId) return;

		try {
			await moverColuna(colunaMovidaId, vizinhosDe(novas, colunaMovidaId));
		} catch (e) {
			tratar(e, 'não foi possível mover a coluna');
			await recarregar();
		}
	}

	function falhar(mensagem: string) {
		erro = mensagem;
	}

	function tratar(e: unknown, padrao: string) {
		falhar(e instanceof ApiError ? e.message : padrao);
	}

	async function renomear(evento: SubmitEvent) {
		evento.preventDefault();
		try {
			await renomearBoard(data.quadro.id, titulo);
			renomeando = false;
			await recarregar();
		} catch (e) {
			tratar(e, 'não foi possível renomear o quadro');
		}
	}

	async function adicionarColuna(evento: SubmitEvent) {
		evento.preventDefault();
		try {
			await criarColuna(data.quadro.id, tituloDaColuna, corDaColuna);
			tituloDaColuna = '';
			corDaColuna = '';
			criandoColuna = false;
			await recarregar();
		} catch (e) {
			tratar(e, 'não foi possível criar a coluna');
		}
	}

	async function apagar() {
		const ok = await confirmar({
			titulo: `Apagar "${data.quadro.titulo}"?`,
			detalhe: 'As colunas e os cards deste quadro vão junto.',
			acao: 'Apagar o quadro'
		});
		if (!ok) return;
		try {
			await apagarBoard(data.quadro.id);
			await goto('/painel');
		} catch (e) {
			tratar(e, 'não foi possível apagar o quadro');
		}
	}

	async function trocarFundo(fundo: Fundo) {
		try {
			await definirFundo(data.quadro.id, fundo);
			await recarregar();
		} catch (e) {
			tratar(e, 'não foi possível trocar o fundo');
		}
	}

	async function adicionarEtiqueta(evento: SubmitEvent) {
		evento.preventDefault();
		try {
			await criarEtiqueta(data.quadro.id, nomeDaEtiqueta, corDaEtiqueta);
			nomeDaEtiqueta = '';
			await recarregar();
		} catch (e) {
			tratar(e, 'não foi possível criar a etiqueta');
		}
	}

	async function trocarCor(etiquetaId: string, nome: string, cor: Cor) {
		try {
			await editarEtiqueta(etiquetaId, nome, cor);
			await recarregar();
		} catch (e) {
			tratar(e, 'não foi possível mudar a etiqueta');
		}
	}

	async function removerEtiquetaDoQuadro(etiquetaId: string, nome: string) {
		const ok = await confirmar({
			titulo: `Apagar a etiqueta "${nome}"?`,
			detalhe: 'Ela some de todos os cards que a usam.',
			acao: 'Apagar a etiqueta'
		});
		if (!ok) return;
		try {
			await apagarEtiqueta(etiquetaId);
			await recarregar();
		} catch (e) {
			tratar(e, 'não foi possível apagar a etiqueta');
		}
	}
</script>

<svelte:head><title>{data.quadro.titulo} · stacktrack</title></svelte:head>

<div class="flex flex-wrap items-baseline justify-between gap-4">
	{#if renomeando}
		<form onsubmit={renomear} class="flex flex-1 gap-2">
			<input
				class="campo"
				bind:value={titulo}
				required
				maxlength="120"
				aria-label="Título do quadro"
			/>
			<button type="submit" class="botao w-auto px-4">Salvar</button>
			<button
				type="button"
				onclick={() => (renomeando = false)}
				class="cursor-pointer px-2 text-sm text-mute hover:text-ink"
			>
				Cancelar
			</button>
		</form>
	{:else}
		<div>
			<!-- "Voltar" ficava no fim da fileira de ações, com o mesmo peso de
			     "Apagar". Voltar é navegação, e navegação se lê ANTES do título. -->
			<a
				href="/painel"
				class="inline-flex items-center gap-1 text-xs text-mute transition-colors hover:text-ink"
			>
				<svg
					class="size-3"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="2.5"
					stroke-linecap="round"
					stroke-linejoin="round"
					aria-hidden="true"
				>
					<path d="M15 5l-7 7 7 7" />
				</svg>
				Seus quadros
			</a>
			<h1 class="mt-1 text-2xl font-semibold tracking-tight text-ink">{data.quadro.titulo}</h1>
			<p class="mt-0.5 text-sm text-mute">
				{data.quadro.colunas.length}
				{data.quadro.colunas.length === 1 ? 'coluna' : 'colunas'}
			</p>
		</div>
		<div class="flex flex-wrap items-center gap-3 text-xs text-mute">
			<!-- Quem está no quadro agora. A lista vem do mapa de conexões do
			     hub, não do banco: é estado que só existe enquanto a aba está
			     aberta. -->
			{#if presentes.length > 0}
				<div class="flex items-center -space-x-1.5" aria-label="Quem está neste quadro agora">
					{#each presentes as pessoa (pessoa.id)}
						<span
							class="flex size-6 items-center justify-center rounded-full border border-surface bg-accent-suave text-[10px] font-semibold text-accent-texto"
							title={pessoa.nome}
						>
							{iniciais(pessoa.nome)}
						</span>
					{/each}
				</div>
			{/if}
			<span class="chip" class:chip-neutro={data.quadro.papel !== 'dono'}>
				<i class="size-1.5 rounded-full bg-current"></i>{data.quadro.papel}
			</span>
			<!-- O aviso vai para TODO membro, e não só para quem publicou: quem
			     escreve num card precisa saber que aquilo está à vista de fora
			     ANTES de escrever, não depois. É o selo, e não o botão, que carrega
			     essa informação — o botão só o dono vê. -->
			{#if data.quadro.publico}
				<span class="chip chip-aviso" title="Existe um link de acompanhamento ativo neste quadro">
					<svg
						class="size-3"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2.2"
						stroke-linecap="round"
						stroke-linejoin="round"
						aria-hidden="true"
					>
						<circle cx="12" cy="12" r="9" />
						<path d="M3 12h18M12 3c2.5 2.7 2.5 15.3 0 18M12 3c-2.5 2.7-2.5 15.3 0 18" />
					</svg>
					público
				</span>
			{/if}
			<!-- A situação da conexão fica à vista: quando ela cai, quem está
			     olhando precisa saber que a tela parou de se atualizar sozinha —
			     senão vai confiar num quadro velho. Ao vivo não vira selo verde
			     de propósito: o normal não precisa de aviso. -->
			{#if conexao && conexao.situacao !== 'ao-vivo'}
				<span class="chip chip-neutro" title="A tela não está se atualizando sozinha">
					<i class="size-1.5 rounded-full bg-aviso"></i>
					{conexao.situacao === 'conectando' ? 'conectando' : 'reconectando'}
				</span>
			{/if}
			<!-- Daqui para a direita são AÇÕES; à esquerda, o estado do quadro. O
			     traço separa os dois, que antes se misturavam numa fileira só. -->
			<span class="hidden h-4 w-px bg-hairline-strong sm:block" aria-hidden="true"></span>

			{#if podeEditar}
				<button
					onclick={() => (painelDeAjustes = !painelDeAjustes)}
					class="cursor-pointer rounded px-2 py-1 transition-colors hover:bg-surface-elevated hover:text-ink"
					aria-expanded={painelDeAjustes}
				>
					Ajustes
				</button>
			{/if}
			<a
				href="/painel/quadros/{data.quadro.id}/membros"
				class="rounded px-2 py-1 transition-colors hover:bg-surface-elevated hover:text-ink"
			>
				Membros
			</a>
			<!-- A auditoria fica ao lado de Membros, e não escondida em Ajustes:
			     as duas respondem "quem", e é aí que se procura. Qualquer membro
			     entra — ver o que aconteceu é ver, não mexer. -->
			<a
				href="/painel/quadros/{data.quadro.id}/historico"
				class="rounded px-2 py-1 transition-colors hover:bg-surface-elevated hover:text-ink"
			>
				Histórico
			</a>
			{#if podeAdministrar}
				<button
					onclick={() => (compartilhando = true)}
					class="cursor-pointer rounded px-2 py-1 transition-colors hover:bg-surface-elevated hover:text-ink"
				>
					Compartilhar
				</button>
				<button
					onclick={() => ((titulo = data.quadro.titulo), (renomeando = true))}
					class="cursor-pointer rounded px-2 py-1 transition-colors hover:bg-surface-elevated hover:text-ink"
					>Renomear</button
				>
				<!-- Apagar é o único que não tem volta, e por isso é o único que sai
				     do agrupamento: separado por traço e com hover vermelho, para não
				     ser vizinho indistinguível de "Renomear". -->
				<span class="h-4 w-px bg-hairline-strong" aria-hidden="true"></span>
				<button
					onclick={apagar}
					class="cursor-pointer rounded px-2 py-1 transition-colors hover:bg-surface-elevated hover:text-negativo"
					>Apagar</button
				>
			{/if}
			{#if podeEditar}
				<button
					onclick={() => (criandoColuna = !criandoColuna)}
					class="botao ml-1 w-auto px-3 py-1.5 text-xs"
					aria-expanded={criandoColuna}
				>
					+ Nova coluna
				</button>
			{/if}
		</div>
	{/if}
</div>

{#if erro}
	<p class="erro-form mt-4">{erro}</p>
{/if}

<!-- O formulário mora aqui, no cabeçalho, e não no fim da faixa de colunas:
     lá ele saía da tela junto com a rolagem horizontal assim que o quadro
     ganhava colunas demais. -->
{#if criandoColuna && podeEditar}
	<section class="mt-4 rounded-lg border border-hairline bg-surface p-4 shadow-ficha">
		<form onsubmit={adicionarColuna} class="flex flex-wrap items-center gap-3">
			<input
				class="campo min-w-0 flex-1 text-sm"
				bind:value={tituloDaColuna}
				placeholder="Nome da coluna"
				required
				maxlength="120"
				aria-label="Título da nova coluna"
			/>
			<SeletorDeCor bind:cor={corDaColuna} rotulo="Cor da nova coluna" />
			<button type="submit" class="botao w-auto px-4 py-1 text-xs">Criar</button>
			<button
				type="button"
				class="cursor-pointer px-2 text-xs text-mute hover:text-ink"
				onclick={() => (criandoColuna = false)}>Cancelar</button
			>
		</form>
	</section>
{/if}

{#if painelDeAjustes && podeEditar}
	<section class="mt-5 space-y-6 rounded-lg border border-hairline bg-surface p-5 shadow-ficha">
		<div>
			<h2 class="text-sm font-semibold text-ink">Etiquetas do quadro</h2>
			<p class="mt-1 text-xs text-mute">
				Valem para todos os cards. Renomear ou trocar a cor muda em todos de uma vez.
			</p>

			<ul class="mt-4 space-y-2">
				{#each data.quadro.etiquetas as etiqueta (etiqueta.id)}
					<li class="flex flex-wrap items-center gap-2">
						<span class="etiqueta cor-{etiqueta.cor} min-w-24">{etiqueta.nome}</span>
						<div class="flex gap-1">
							{#each cores as cor (cor)}
								<button
									class="etiqueta-barra cor-{cor} cursor-pointer {etiqueta.cor === cor
										? 'ring-2 ring-accent ring-offset-1 ring-offset-surface'
										: 'opacity-50'}"
									onclick={() => trocarCor(etiqueta.id, etiqueta.nome, cor)}
									aria-label="Mudar {etiqueta.nome} para {cor}"
								></button>
							{/each}
						</div>
						<button
							class="ml-auto cursor-pointer text-xs text-mute hover:text-negativo"
							onclick={() => removerEtiquetaDoQuadro(etiqueta.id, etiqueta.nome)}
						>
							Apagar
						</button>
					</li>
				{/each}
				{#if data.quadro.etiquetas.length === 0}
					<li class="text-xs text-mute">Nenhuma etiqueta ainda.</li>
				{/if}
			</ul>

			<form onsubmit={adicionarEtiqueta} class="mt-4 flex flex-wrap gap-2">
				<input
					class="campo min-w-0 flex-1 py-1 text-xs"
					bind:value={nomeDaEtiqueta}
					placeholder="Nome da nova etiqueta"
					required
					maxlength="60"
					aria-label="Nome da nova etiqueta"
				/>
				<select
					class="campo py-1 text-xs font-semibold"
					style="color: var(--cor-{corDaEtiqueta})"
					bind:value={corDaEtiqueta}
					aria-label="Cor"
				>
					{#each cores as cor (cor)}
						<option value={cor} style="color: var(--cor-{cor})">{cor}</option>
					{/each}
				</select>
				<button type="submit" class="botao w-auto px-4 py-1 text-xs">Criar</button>
			</form>
		</div>

		{#if podeAdministrar}
			<div class="border-t border-hairline pt-5">
				<h2 class="text-sm font-semibold text-ink">Fundo do quadro</h2>
				<div class="mt-3 flex flex-wrap items-center gap-2 text-xs">
					{#each FUNDOS as fundo (fundo)}
						<button
							class="fundo-{fundo} flex cursor-pointer items-center gap-1.5 rounded-sm border px-2 py-1"
							style="color: var(--fundo-cor); border-color: color-mix(in srgb, var(--fundo-cor) {data
								.quadro.fundo === fundo
								? '70%'
								: '25%'}, transparent)"
							onclick={() => trocarFundo(fundo)}
							aria-pressed={data.quadro.fundo === fundo}
						>
							<i
								class="size-2.5 rounded-full"
								style="background-color: color-mix(in srgb, var(--fundo-cor) {data.quadro.fundo ===
								fundo
									? '100%'
									: '45%'}, transparent)"
							></i>
							{nomeDoFundo[fundo]}
						</button>
					{/each}
				</div>
			</div>
		{/if}
	</section>
{/if}

<!-- Filtro. Fica fora do painel do quadro de propósito: ele age SOBRE o quadro,
     e some junto com a barra quando não há nada para filtrar. -->
{#if pessoasNoQuadro.length > 0 || data.quadro.etiquetas.length > 0}
	<div class="mt-6 flex flex-wrap items-center gap-2 text-xs">
		<span class="font-semibold tracking-widest text-mute uppercase">Filtrar</span>

		{#if pessoasNoQuadro.length > 0}
			<select
				bind:value={filtro.responsavelId}
				class="campo w-auto py-1 text-xs"
				aria-label="Responsável"
			>
				<option value="">Qualquer responsável</option>
				{#each pessoasNoQuadro as pessoa (pessoa.usuarioId)}
					<option value={pessoa.usuarioId}>{pessoa.nome}</option>
				{/each}
			</select>
		{/if}

		{#if data.quadro.etiquetas.length > 0}
			<select
				bind:value={filtro.etiquetaId}
				class="campo w-auto py-1 text-xs"
				aria-label="Etiqueta"
			>
				<option value="">Qualquer etiqueta</option>
				{#each data.quadro.etiquetas as etiqueta (etiqueta.id)}
					<option value={etiqueta.id}>{etiqueta.nome}</option>
				{/each}
			</select>
		{/if}

		<label class="flex cursor-pointer items-center gap-1.5 text-mute">
			<input type="checkbox" bind:checked={filtro.soVencidos} class="cursor-pointer" />
			Só vencidos
		</label>

		{#if filtrando}
			<span class="text-mute tabular-nums">
				{cardsVisiveis}
				{cardsVisiveis === 1 ? 'card' : 'cards'}
			</span>
			<button onclick={limparFiltro} class="cursor-pointer underline hover:text-body">
				limpar
			</button>
			<!-- Dizer POR QUE o arraste parou: sem isto, a pessoa tenta mover um
			     card, nada acontece, e ela conclui que a tela travou. -->
			<span class="text-mute">· arraste desativado enquanto filtra</span>
		{/if}
	</div>
{/if}

<div class="painel-fundo fundo-{data.quadro.fundo} mt-3 rounded-lg p-4">
	<!-- Zona das colunas. Ela ENVOLVE a zona dos cards, e é o aninhamento que
	     faz o arraste começar na zona certa: um card é filho do <ul> de cards,
	     não desta lista.
	     A grade quebra linha em vez de rolar para o lado: duas colunas por linha
	     no celular, quatro a partir do desktop, e o excedente empilha para baixo
	     com a rolagem normal da página. A faixa horizontal escondia as colunas
	     seguintes atrás de uma barra que ninguém vê sem procurar. -->
	<div
		class="grid grid-cols-2 items-start gap-4 pb-4 lg:grid-cols-4"
		use:dndzone={{
			items: colunasVisiveis,
			type: TIPO_COLUNA,
			flipDurationMs: DURACAO_MS,
			dragDisabled: !podeEditar || filtrando,
			delayTouchStart: ESPERA_DE_TOQUE_MS,
			dropTargetStyle: {},
			dropTargetClasses: CLASSES_ALVO,
			transformDraggedElement: enfeitarArrastado
		}}
		onconsider={(e: CustomEvent<DndEvent<Coluna>>) => espelharColunas(e.detail.items)}
		onfinalize={(e: CustomEvent<DndEvent<Coluna>>) =>
			soltarColuna(e.detail.items, e.detail.info.id)}
	>
		{#each colunasVisiveis as coluna (coluna.id)}
			<!-- min-w-0 no item da grade: sem ele o conteúdo largo (título comprido,
			     nome de anexo) força a célula a crescer e estoura a linha inteira. -->
			<div class="min-w-0" animate:flip={{ duration: DURACAO_MS }}>
				<ColunaDoQuadro
					{coluna}
					etiquetasDoQuadro={data.quadro.etiquetas}
					{podeEditar}
					arrasteTravado={filtrando}
					editandoAgora={editandoPorColuna.get(coluna.id) ?? []}
					aoEditar={(editando) => conexao?.anunciarEdicao(editando ? coluna.id : null)}
					aoAbrirCard={(id) => (cardAberto = id)}
					aoMudar={recarregar}
					aoFalhar={falhar}
					aoArrastarCards={espelharCards}
					aoSoltarCard={soltarCard}
				/>
			</div>
		{/each}
	</div>

	{#if colunas.length === 0}
		<p class="py-8 text-center text-sm text-mute">
			{podeEditar
				? 'Este quadro ainda não tem colunas — comece por "+ Nova coluna", ali em cima.'
				: 'Este quadro ainda não tem colunas.'}
		</p>
	{/if}
</div>

{#if compartilhando && podeAdministrar}
	<DialogoDeCompartilhamento
		boardId={data.quadro.id}
		aoFechar={() => (compartilhando = false)}
		aoMudar={recarregar}
	/>
{/if}

{#if cardAberto}
	<ModalDoCard
		cardId={cardAberto}
		etiquetasDoQuadro={data.quadro.etiquetas}
		{podeEditar}
		{podeAdministrar}
		aoFechar={() => (cardAberto = null)}
		aoMudar={recarregar}
		registrarRecarga={registrarRecargaDoModal}
	/>
{/if}
