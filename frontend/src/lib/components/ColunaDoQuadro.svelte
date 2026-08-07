<script lang="ts">
	// Uma coluna do quadro: título editável, os cards e o formulário de card
	// novo no pé.
	import {
		apagarColuna,
		criarCard,
		renomearColuna,
		type Card,
		type Coluna,
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
		TIPO_CARD
	} from '$lib/arrastar';
	import CardDoQuadro from './CardDoQuadro.svelte';

	let {
		coluna,
		etiquetasDoQuadro,
		podeEditar,
		aoAbrirCard,
		aoMudar,
		aoFalhar,
		aoArrastarCards,
		aoSoltarCard
	}: {
		coluna: Coluna;
		etiquetasDoQuadro: Etiqueta[];
		podeEditar: boolean;
		aoAbrirCard: (cardId: string) => void;
		aoMudar: () => Promise<void>;
		aoFalhar: (mensagem: string) => void;
		// Durante o arraste (aoArrastarCards) a página só espelha a prévia; ao
		// soltar (aoSoltarCard) é que a mudança vai para a API.
		aoArrastarCards: (colunaId: string, cards: Card[]) => void;
		aoSoltarCard: (colunaId: string, cards: Card[], cardMovidoId: string | null) => void;
	} = $props();

	let renomeando = $state(false);
	// Preenchido ao entrar em modo de renomear — ver o comentário em CardDoQuadro.
	let titulo = $state('');
	let tituloDoCard = $state('');
	let criando = $state(false);

	function abrirRenomear() {
		if (!podeEditar) return;
		titulo = coluna.titulo;
		renomeando = true;
	}

	async function renomear(evento: SubmitEvent) {
		evento.preventDefault();
		try {
			await renomearColuna(coluna.id, titulo);
			renomeando = false;
			await aoMudar();
		} catch (e) {
			aoFalhar(e instanceof ApiError ? e.message : 'não foi possível renomear a coluna');
		}
	}

	async function apagar() {
		if (!confirm(`Apagar a coluna "${coluna.titulo}"? Os cards dela vão junto.`)) return;
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

<section class="flex w-72 shrink-0 flex-col rounded-lg border border-hairline bg-surface-elevated">
	<header class="flex items-center gap-2 border-b border-hairline px-4 py-3">
		{#if podeEditar && !renomeando}
			<!-- Pista visual de que a coluna se arrasta. Não é botão nem alça: a
			     coluna inteira é a área de arraste, e pegar num card move só o
			     card, porque a zona dos cards interrompe o evento antes que ele
			     chegue aqui. -->
			<span class="text-mute" aria-hidden="true" title="Arraste a coluna para reordenar">⠿</span>
		{/if}
		{#if renomeando}
			<!-- svelte-ignore a11y_autofocus -->
			<form onsubmit={renomear} class="flex-1">
				<input
					class="campo text-sm"
					bind:value={titulo}
					required
					maxlength="120"
					autofocus
					onblur={() => (renomeando = false)}
					aria-label="Título da coluna"
				/>
			</form>
		{:else}
			<button
				{...cliqueSemArraste(abrirRenomear)}
				class="flex-1 text-left text-sm font-medium text-ink {podeEditar ? 'cursor-pointer' : ''}"
			>
				{coluna.titulo}
			</button>
			<div class="flex items-center gap-2">
				<span
					class="rounded-full border border-hairline bg-surface px-1.5 text-xs tabular-nums text-mute"
				>
					{coluna.cards.length}
				</span>
				{#if podeEditar}
					<button
						{...cliqueSemArraste(apagar)}
						class="cursor-pointer text-xs text-mute hover:text-negativo"
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
			dragDisabled: !podeEditar,
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

	{#if podeEditar}
		<form onsubmit={adicionarCard} class="border-t border-hairline p-3">
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
