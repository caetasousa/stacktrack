<script lang="ts">
	import { confirmar } from '$lib/confirmar.svelte';
	// Uma coluna do quadro: título editável, os cards e o formulário de card
	// novo no pé.
	import {
		apagarColuna,
		criarCard,
		renomearColuna,
		type Card,
		type Coluna,
		type Cor,
		type Etiqueta
	} from '$lib/api/boards';
	import { ApiError } from '$lib/api/client';
	import { dndzone, type DndEvent } from 'svelte-dnd-action';
	import { flip } from 'svelte/animate';
	import {
		CLASSES_ALVO,
		cliqueSemArraste,
		DURACAO_MS,
		enfeitarArrastado,
		ESPERA_DE_TOQUE_MS,
		TIPO_CARD
	} from '$lib/arrastar';
	import CardDoQuadro from './CardDoQuadro.svelte';
	import SeletorDeCor from './SeletorDeCor.svelte';

	let {
		coluna,
		etiquetasDoQuadro,
		podeEditar,
		arrasteTravado = false,
		aoAbrirCard,
		aoMudar,
		aoFalhar,
		aoArrastarCards,
		aoSoltarCard,
		editandoAgora = [],
		aoEditar
	}: {
		coluna: Coluna;
		etiquetasDoQuadro: Etiqueta[];
		podeEditar: boolean;
		// Trava o arraste sem tirar a permissão de editar. É o que o filtro usa:
		// com a lista filtrada, os vizinhos que a página calcularia não seriam os
		// vizinhos reais, e o card pousaria no lugar errado.
		arrasteTravado?: boolean;
		aoAbrirCard: (cardId: string) => void;
		aoMudar: () => Promise<void>;
		aoFalhar: (mensagem: string) => void;
		// Durante o arraste (aoArrastarCards) a página só espelha a prévia; ao
		// soltar (aoSoltarCard) é que a mudança vai para a API.
		aoArrastarCards: (colunaId: string, cards: Card[]) => void;
		aoSoltarCard: (colunaId: string, cards: Card[], cardMovidoId: string | null) => void;
		// Quem MAIS está com o formulário desta coluna aberto agora. Vem da
		// presença em tempo real; a própria pessoa nunca aparece aqui.
		editandoAgora?: string[];
		// Avisa a sala que esta pessoa começou ou parou de editar a coluna.
		aoEditar?: (editando: boolean) => void;
	} = $props();

	let renomeando = $state(false);
	// Preenchido ao entrar em modo de renomear — ver o comentário em CardDoQuadro.
	let titulo = $state('');
	let corEmEdicao = $state<Cor | ''>('');
	let tituloDoCard = $state('');
	let criando = $state(false);

	function abrirRenomear() {
		if (!podeEditar) return;
		titulo = coluna.titulo;
		corEmEdicao = coluna.cor;
		renomeando = true;
	}

	// O anúncio segue o estado, e não os cliques.
	//
	// Ligá-lo em cada `onclick` significaria lembrar de desligar em TODOS os
	// caminhos de saída — salvar, cancelar, a tecla Esc, navegar para fora,
	// desmontar. Um esquecido deixa a coluna eternamente "sendo editada" por
	// alguém que já foi embora, e é o tipo de cadeado que só se descobre quando
	// incomoda. Com $effect, o retorno é a saída: ele roda quando `renomeando`
	// muda e quando o componente morre, aconteça o que acontecer no meio.
	$effect(() => {
		if (!renomeando) return;
		aoEditar?.(true);
		return () => aoEditar?.(false);
	});

	async function renomear(evento: SubmitEvent) {
		evento.preventDefault();
		try {
			await renomearColuna(coluna.id, titulo, corEmEdicao);
			renomeando = false;
			await aoMudar();
		} catch (e) {
			aoFalhar(e instanceof ApiError ? e.message : 'não foi possível renomear a coluna');
		}
	}

	async function apagar() {
		const ok = await confirmar({
			titulo: `Apagar a coluna "${coluna.titulo}"?`,
			detalhe: 'Os cards dela vão junto.',
			acao: 'Apagar a coluna'
		});
		if (!ok) return;
		try {
			await apagarColuna(coluna.id);
			await aoMudar();
		} catch (e) {
			aoFalhar(e instanceof ApiError ? e.message : 'não foi possível apagar a coluna');
		}
	}

	async function adicionarCard(evento: SubmitEvent) {
		evento.preventDefault();
		criando = true;
		try {
			// Sem cor: ela é propriedade do card e se escolhe dentro dele, junto
			// com etiqueta e prazo.
			await criarCard(coluna.id, tituloDoCard);
			tituloDoCard = '';
			await aoMudar();
		} catch (e) {
			aoFalhar(e instanceof ApiError ? e.message : 'não foi possível criar o card');
		} finally {
			criando = false;
		}
	}
</script>

<!-- A cor tinge a coluna inteira, e não só o cabeçalho: é o que dá significado
     à etapa de relance — verde no começo, amarelo no meio, azul no fim. Os
     cards têm fundo opaco e continuam saltando por cima dela. -->
<section
	class="flex w-full min-w-0 flex-col rounded-lg border {coluna.cor
		? `cor-${coluna.cor}`
		: 'border-hairline bg-surface-elevated'}"
	style={coluna.cor
		? 'background-color: color-mix(in srgb, var(--etq-texto) 16%, var(--surface-elevated));' +
			'border-color: color-mix(in srgb, var(--etq-texto) 32%, transparent)'
		: ''}
>
	<header
		class="flex items-center gap-2 border-b px-3 py-3 sm:px-4 {coluna.cor ? '' : 'border-hairline'}"
		style={coluna.cor
			? 'background-color: color-mix(in srgb, var(--etq-texto) 26%, var(--surface-elevated));' +
				'border-bottom-color: color-mix(in srgb, var(--etq-texto) 30%, transparent);' +
				'border-top: 3px solid var(--etq-texto)'
			: ''}
	>
		{#if podeEditar && !renomeando}
			<!-- Pista visual de que a coluna se arrasta. Não é botão nem alça: a
			     coluna inteira é a área de arraste, e pegar num card move só o
			     card, porque a zona dos cards interrompe o evento antes que ele
			     chegue aqui. -->
			<span class="text-mute" aria-hidden="true" title="Arraste a coluna para reordenar">⠿</span>
		{/if}
		{#if renomeando}
			<!-- Enquanto o formulário está aberto, o gesto é dele: sem isto, apertar
			     uma bolinha de cor e escorregar 3px começaria a arrastar a coluna
			     em vez de escolher a cor. -->
			<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
			<form onsubmit={renomear} onmousedown={(e) => e.stopPropagation()} class="flex-1 space-y-2">
				<!-- svelte-ignore a11y_autofocus -->
				<input
					class="campo text-sm"
					bind:value={titulo}
					required
					maxlength="120"
					autofocus
					aria-label="Título da coluna"
				/>
				<SeletorDeCor bind:cor={corEmEdicao} rotulo="Cor da coluna" />
				<div class="flex gap-2">
					<button type="submit" class="botao w-auto px-3 py-1 text-xs">Salvar</button>
					<button
						type="button"
						class="cursor-pointer px-2 text-xs text-mute hover:text-ink"
						onclick={() => (renomeando = false)}>Cancelar</button
					>
				</div>
			</form>
		{:else}
			<!-- min-w-0 + wrap-anywhere: sem os dois, um título de palavra longa
			     recusa encolher, empurra a contagem e o × para fora da coluna e
			     estoura a caixa — o que se vê primeiro no celular, onde cabem
			     duas colunas por linha. -->
			<button
				{...cliqueSemArraste(abrirRenomear)}
				class="min-w-0 flex-1 text-left text-sm font-medium wrap-anywhere text-ink {podeEditar
					? 'cursor-pointer'
					: ''}"
			>
				{coluna.titulo}
			</button>
			<div class="flex shrink-0 items-center gap-1">
				<span
					class="rounded-full border border-hairline bg-surface px-1.5 text-xs tabular-nums text-mute"
				>
					{coluna.cards.length}
				</span>
				{#if podeEditar}
					<!-- O alvo de toque é o quadrado de 1.5rem, e não o glifo: no
					     celular, um × de 8px de largura só se acerta por sorte. -->
					<button
						{...cliqueSemArraste(apagar)}
						class="flex size-6 shrink-0 cursor-pointer items-center justify-center rounded text-sm leading-none text-mute hover:bg-surface hover:text-negativo"
						aria-label="Apagar coluna"
					>
						×
					</button>
				{/if}
			</div>
		{/if}
	</header>

	<!-- A zona de arraste dos cards é compartilhada entre as colunas (mesmo
	     `type`), e é isso que permite arrastar de uma para a outra. -->
	<ul
		class="min-h-24 flex-1 space-y-2 p-3"
		use:dndzone={{
			items: coluna.cards,
			type: TIPO_CARD,
			flipDurationMs: DURACAO_MS,
			dragDisabled: !podeEditar || arrasteTravado,
			delayTouchStart: ESPERA_DE_TOQUE_MS,
			// Sem estilo inline: a classe usa os tokens do tema, e o amarelo
			// padrão da biblioteca não pertence a esta paleta.
			dropTargetStyle: {},
			dropTargetClasses: CLASSES_ALVO,
			transformDraggedElement: enfeitarArrastado
		}}
		onconsider={(e: CustomEvent<DndEvent<Card>>) => aoArrastarCards(coluna.id, e.detail.items)}
		onfinalize={(e: CustomEvent<DndEvent<Card>>) =>
			aoSoltarCard(coluna.id, e.detail.items, e.detail.info.id)}
	>
		{#each coluna.cards as card (card.id)}
			<li animate:flip={{ duration: DURACAO_MS }}>
				<CardDoQuadro {card} {etiquetasDoQuadro} {podeEditar} {aoAbrirCard} {aoMudar} {aoFalhar} />
			</li>
		{/each}
	</ul>

	{#if coluna.cards.length === 0}
		<!-- Coluna vazia precisa de alvo: uma lista sem altura é quase
		     impossível de acertar com um card na mão. -->
		<p class="pointer-events-none -mt-2 px-3 pb-3 text-center text-xs text-mute">
			solte um card aqui
		</p>
	{/if}

	<!-- Quem mais está mexendo nesta coluna. Fica no RODAPÉ, e não no cabeçalho:
	     lá em cima ele empurraria o título e a contagem a cada vez que alguém
	     começasse a digitar, e um cabeçalho que dança é pior do que o aviso é
	     bom. Aqui ele aparece e some sem mover mais nada. -->
	{#if editandoAgora.length > 0}
		<p
			class="flex items-center gap-1.5 border-t border-hairline px-3 py-2 text-[0.6875rem] text-aviso"
			aria-live="polite"
		>
			<i class="size-1.5 shrink-0 animate-pulse rounded-full bg-current"></i>
			<span class="truncate">
				{editandoAgora.join(', ')}
				{editandoAgora.length === 1 ? 'está editando' : 'estão editando'}
			</span>
		</p>
	{/if}

	{#if podeEditar}
		<form
			onsubmit={adicionarCard}
			class="space-y-2 border-t p-3 {coluna.cor ? '' : 'border-hairline'}"
			style={coluna.cor
				? 'border-top-color: color-mix(in srgb, var(--etq-texto) 26%, transparent)'
				: ''}
		>
			<input
				class="campo text-sm"
				bind:value={tituloDoCard}
				placeholder="+ novo card"
				required
				maxlength="200"
				disabled={criando}
				aria-label="Título do novo card"
			/>
		</form>
	{/if}
</section>
