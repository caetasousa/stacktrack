<script lang="ts">
	// Um card do quadro, com edição no lugar (clicar no título abre o
	// formulário) para não precisar de modal nesta fase.
	import { apagarCard, editarCard, type Card } from '$lib/api/boards';
	import { ApiError } from '$lib/api/client';

	let {
		card,
		podeEditar,
		aoMudar,
		aoFalhar
	}: {
		card: Card;
		podeEditar: boolean;
		aoMudar: () => Promise<void>;
		aoFalhar: (mensagem: string) => void;
	} = $props();

	let editando = $state(false);
	// Começam vazios e são preenchidos em abrir(): inicializá-los com o card
	// capturaria só o valor do primeiro render, e o formulário mostraria texto
	// velho depois que a lista fosse recarregada.
	let titulo = $state('');
	let descricao = $state('');
	let salvando = $state(false);

	function abrir() {
		if (!podeEditar) return;
		titulo = card.titulo;
		descricao = card.descricao;
		editando = true;
	}

	async function salvar(evento: SubmitEvent) {
		evento.preventDefault();
		salvando = true;
		try {
			await editarCard(card.id, titulo, descricao);
			editando = false;
			await aoMudar();
		} catch (e) {
			aoFalhar(e instanceof ApiError ? e.message : 'não foi possível salvar o card');
		} finally {
			salvando = false;
		}
	}

	async function apagar() {
		if (!confirm(`Apagar o card "${card.titulo}"?`)) return;
		try {
			await apagarCard(card.id);
			await aoMudar();
		} catch (e) {
			aoFalhar(e instanceof ApiError ? e.message : 'não foi possível apagar o card');
		}
	}
</script>

<li class="rounded-sm border border-hairline bg-surface p-3 shadow-ficha">
	{#if editando}
		<form onsubmit={salvar} class="space-y-2">
			<input class="campo text-sm" bind:value={titulo} required maxlength="200" aria-label="Título do card" />
			<textarea
				class="campo text-sm"
				bind:value={descricao}
				rows="3"
				maxlength="5000"
				placeholder="Descrição (opcional)"
				aria-label="Descrição do card"
			></textarea>
			<div class="flex gap-2">
				<button type="submit" class="botao w-auto px-3 py-1 text-xs" disabled={salvando}>
					{salvando ? 'Salvando…' : 'Salvar'}
				</button>
				<button
					type="button"
					onclick={() => (editando = false)}
					class="cursor-pointer px-2 text-xs text-mute hover:text-ink"
				>
					Cancelar
				</button>
			</div>
		</form>
	{:else}
		<div class="flex items-start justify-between gap-2">
			<button
				onclick={abrir}
				class="flex-1 text-left text-sm text-body {podeEditar ? 'cursor-pointer hover:text-ink' : ''}"
			>
				{card.titulo}
			</button>
			{#if podeEditar}
				<button
					onclick={apagar}
					class="cursor-pointer text-xs text-mute hover:text-negativo"
					aria-label="Apagar card"
				>
					×
				</button>
			{/if}
		</div>
		{#if card.descricao}
			<p class="mt-2 text-xs whitespace-pre-wrap text-mute">{card.descricao}</p>
		{/if}
	{/if}
</li>
