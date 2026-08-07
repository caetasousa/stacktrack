<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import {
		apagarBoard,
		criarColuna,
		renomearBoard,
		FUNDOS,
		type Cor,
		type Fundo
	} from '$lib/api/boards';
	import { apagarEtiqueta, criarEtiqueta, definirFundo, editarEtiqueta } from '$lib/api/extras';
	import { ApiError } from '$lib/api/client';
	import ColunaDoQuadro from '$lib/components/ColunaDoQuadro.svelte';
	import ModalDoCard from '$lib/components/ModalDoCard.svelte';

	let { data } = $props();

	let erro = $state('');
	let renomeando = $state(false);
	let titulo = $state('');
	let tituloDaColuna = $state('');
	// Qual card está aberto no modal — null é modal fechado.
	let cardAberto = $state<string | null>(null);
	let painelDeEtiquetas = $state(false);
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
			await criarColuna(data.quadro.id, tituloDaColuna);
			tituloDaColuna = '';
			await recarregar();
		} catch (e) {
			tratar(e, 'não foi possível criar a coluna');
		}
	}

	async function apagar() {
		if (!confirm(`Apagar "${data.quadro.titulo}"? As colunas e os cards vão junto.`)) return;
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
		if (!confirm(`Apagar a etiqueta "${nome}"? Ela some de todos os cards.`)) return;
		try {
			await apagarEtiqueta(etiquetaId);
			await recarregar();
		} catch (e) {
			tratar(e, 'não foi possível apagar a etiqueta');
		}
	}
</script>

<svelte:head><title>{data.quadro.titulo} · kanbanGo</title></svelte:head>

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
			<h1 class="text-xl font-semibold tracking-tight text-ink">{data.quadro.titulo}</h1>
			<p class="mt-0.5 text-sm text-mute">
				{data.quadro.colunas.length}
				{data.quadro.colunas.length === 1 ? 'coluna' : 'colunas'}
			</p>
		</div>
		<div class="flex shrink-0 flex-wrap items-center gap-3 text-xs text-mute">
			<span class="chip" class:chip-neutro={data.quadro.papel !== 'dono'}>
				<i class="size-1.5 rounded-full bg-current"></i>{data.quadro.papel}
			</span>
			{#if podeEditar}
				<button
					onclick={() => (painelDeEtiquetas = !painelDeEtiquetas)}
					class="cursor-pointer hover:text-ink"
					aria-expanded={painelDeEtiquetas}
				>
					Etiquetas
				</button>
			{/if}
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

{#if painelDeEtiquetas && podeEditar}
	<section class="mt-5 rounded-lg border border-hairline bg-surface p-5 shadow-ficha">
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
			<select class="campo w-auto py-1 text-xs" bind:value={corDaEtiqueta} aria-label="Cor">
				{#each cores as cor (cor)}
					<option value={cor}>{cor}</option>
				{/each}
			</select>
			<button type="submit" class="botao w-auto px-4 py-1 text-xs">Criar</button>
		</form>
	</section>
{/if}

{#if podeAdministrar}
	<div class="mt-4 flex flex-wrap items-center gap-2 text-xs text-mute">
		<span>Fundo:</span>
		{#each FUNDOS as fundo (fundo)}
			<button
				class="cursor-pointer rounded-sm border px-2 py-0.5 {data.quadro.fundo === fundo
					? 'border-accent text-accent-texto'
					: 'border-hairline-strong hover:text-ink'}"
				onclick={() => trocarFundo(fundo)}
			>
				{nomeDoFundo[fundo]}
			</button>
		{/each}
	</div>
{/if}

<div class="mt-6 -mx-6 rounded-lg px-6 py-4 fundo-{data.quadro.fundo}">
	<div class="flex items-start gap-4 overflow-x-auto pb-4">
		{#each data.quadro.colunas as coluna (coluna.id)}
			<ColunaDoQuadro
				{coluna}
				etiquetasDoQuadro={data.quadro.etiquetas}
				{podeEditar}
				aoAbrirCard={(id) => (cardAberto = id)}
				aoMudar={recarregar}
				aoFalhar={falhar}
			/>
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
		<p class="py-8 text-center text-sm text-mute">Este quadro ainda não tem colunas.</p>
	{/if}
</div>

{#if cardAberto}
	<ModalDoCard
		cardId={cardAberto}
		etiquetasDoQuadro={data.quadro.etiquetas}
		{podeEditar}
		aoFechar={() => (cardAberto = null)}
		aoMudar={recarregar}
	/>
{/if}
