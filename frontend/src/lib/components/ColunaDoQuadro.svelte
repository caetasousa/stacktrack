<script lang="ts">
	// Uma coluna do quadro: título editável, os cards e o formulário de card
	// novo no pé.
	import { apagarColuna, criarCard, renomearColuna, type Coluna } from '$lib/api/boards';
	import { ApiError } from '$lib/api/client';
	import CardDoQuadro from './CardDoQuadro.svelte';

	let {
		coluna,
		podeEditar,
		aoMudar,
		aoFalhar
	}: {
		coluna: Coluna;
		podeEditar: boolean;
		aoMudar: () => Promise<void>;
		aoFalhar: (mensagem: string) => void;
	} = $props();

	let renomeando = $state(false);
	// Preenchido ao entrar em modo de renomear — ver o comentário em CardDoQuadro.
	let titulo = $state('');
	let tituloDoCard = $state('');
	let criando = $state(false);

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
	<header class="flex items-center justify-between gap-2 border-b border-hairline px-4 py-3">
		{#if renomeando}
			<form onsubmit={renomear} class="flex-1">
				<!-- svelte-ignore a11y_autofocus -->
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
				onclick={() => podeEditar && ((titulo = coluna.titulo), (renomeando = true))}
				class="text-sm font-medium text-ink {podeEditar ? 'cursor-pointer' : ''}"
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
						onclick={apagar}
						class="cursor-pointer text-xs text-mute hover:text-negativo"
						aria-label="Apagar coluna"
					>
						×
					</button>
				{/if}
			</div>
		{/if}
	</header>

	<ul class="flex-1 space-y-2 p-3">
		{#each coluna.cards as card (card.id)}
			<CardDoQuadro {card} {podeEditar} {aoMudar} {aoFalhar} />
		{/each}
	</ul>

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
