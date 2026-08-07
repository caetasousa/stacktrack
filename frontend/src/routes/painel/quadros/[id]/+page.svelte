<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { apagarBoard, criarColuna, renomearBoard } from '$lib/api/boards';
	import { ApiError } from '$lib/api/client';
	import ColunaDoQuadro from '$lib/components/ColunaDoQuadro.svelte';

	let { data } = $props();

	let erro = $state('');
	let renomeando = $state(false);
	let titulo = $state('');
	let tituloDaColuna = $state('');

	const podeEditar = $derived(data.quadro.papel === 'dono' || data.quadro.papel === 'editor');
	const podeAdministrar = $derived(data.quadro.papel === 'dono');

	// Enquanto não há tempo real, é este recarregar que atualiza a tela — a
	// pessoa só vê o que ela mesma fez. A fase 5 troca isto por um evento vindo
	// do servidor, e é aí que o quadro vira colaborativo de verdade.
	async function recarregar() {
		erro = '';
		await invalidateAll();
	}

	function falhar(mensagem: string) {
		erro = mensagem;
	}

	async function renomear(evento: SubmitEvent) {
		evento.preventDefault();
		try {
			await renomearBoard(data.quadro.id, titulo);
			renomeando = false;
			await recarregar();
		} catch (e) {
			falhar(e instanceof ApiError ? e.message : 'não foi possível renomear o quadro');
		}
	}

	async function adicionarColuna(evento: SubmitEvent) {
		evento.preventDefault();
		try {
			await criarColuna(data.quadro.id, tituloDaColuna);
			tituloDaColuna = '';
			await recarregar();
		} catch (e) {
			falhar(e instanceof ApiError ? e.message : 'não foi possível criar a coluna');
		}
	}

	async function apagar() {
		if (!confirm(`Apagar "${data.quadro.titulo}"? As colunas e os cards vão junto.`)) return;
		try {
			await apagarBoard(data.quadro.id);
			await goto('/painel');
		} catch (e) {
			falhar(e instanceof ApiError ? e.message : 'não foi possível apagar o quadro');
		}
	}
</script>

<svelte:head><title>{data.quadro.titulo} · kanbanGo</title></svelte:head>

<div class="flex items-baseline justify-between gap-4">
	{#if renomeando}
		<form onsubmit={renomear} class="flex flex-1 gap-2">
			<input class="campo" bind:value={titulo} required maxlength="120" aria-label="Título do quadro" />
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
			<h1 class="text-xl font-semibold tracking-tight text-ink">{data.quadro.titulo}</h1>
			<p class="mt-0.5 text-sm text-mute">
				{data.quadro.colunas.length}
				{data.quadro.colunas.length === 1 ? 'coluna' : 'colunas'}
			</p>
		</div>
		<div class="flex shrink-0 items-center gap-3 text-xs text-mute">
			<span class="chip" class:chip-neutro={data.quadro.papel !== 'dono'}>
				<i class="size-1.5 rounded-full bg-current"></i>{data.quadro.papel}
			</span>
			<a href="/painel/quadros/{data.quadro.id}/membros" class="hover:text-ink">Membros</a>
			{#if podeAdministrar}
				<button
					onclick={() => ((titulo = data.quadro.titulo), (renomeando = true))}
					class="cursor-pointer hover:text-ink">Renomear</button
				>
				<button onclick={apagar} class="cursor-pointer hover:text-negativo">Apagar</button>
			{/if}
			<a href="/painel" class="hover:text-ink">Voltar</a>
		</div>
	{/if}
</div>

{#if erro}
	<p class="erro-form mt-4">{erro}</p>
{/if}

<div class="mt-8 flex items-start gap-4 overflow-x-auto pb-4">
	{#each data.quadro.colunas as coluna (coluna.id)}
		<ColunaDoQuadro {coluna} {podeEditar} aoMudar={recarregar} aoFalhar={falhar} />
	{/each}

	{#if podeEditar}
		<form onsubmit={adicionarColuna} class="w-72 shrink-0">
			<input
				class="campo"
				bind:value={tituloDaColuna}
				placeholder="+ nova coluna"
				required
				maxlength="120"
				aria-label="Título da nova coluna"
			/>
		</form>
	{/if}
</div>

{#if data.quadro.colunas.length === 0 && !podeEditar}
	<p class="mt-10 text-center text-sm text-mute">Este quadro ainda não tem colunas.</p>
{/if}
