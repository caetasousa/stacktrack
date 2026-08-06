<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { apagarBoard, criarBoard } from '$lib/api/boards';
	import { ApiError } from '$lib/api/client';

	let { data } = $props();

	let titulo = $state('');
	let erro = $state('');
	let enviando = $state(false);

	async function criar(evento: SubmitEvent) {
		evento.preventDefault();
		erro = '';
		enviando = true;
		try {
			await criarBoard(titulo);
			titulo = '';
			// Recarrega o load() da rota. É a "recarga manual" desta fase: a
			// tela só sabe do que mudou porque perguntou de novo. Na fase 5 o
			// servidor passa a avisar sozinho, e a diferença fica evidente.
			await invalidateAll();
		} catch (e) {
			erro = e instanceof ApiError ? e.message : 'não foi possível criar o quadro';
		} finally {
			enviando = false;
		}
	}

	async function apagar(id: string, nome: string) {
		if (!confirm(`Apagar "${nome}"? As colunas e os cards vão junto.`)) return;
		erro = '';
		try {
			await apagarBoard(id);
			await invalidateAll();
		} catch (e) {
			erro = e instanceof ApiError ? e.message : 'não foi possível apagar o quadro';
		}
	}

	const formatarData = (iso: string) =>
		new Date(iso).toLocaleDateString('pt-BR', { day: '2-digit', month: 'short', year: 'numeric' });
</script>

<svelte:head><title>Painel · kanbanGo</title></svelte:head>

<div class="flex flex-wrap items-baseline justify-between gap-3">
	<div>
		<h1 class="text-xl font-semibold tracking-tight text-ink">Seus quadros</h1>
		<p class="mt-0.5 text-sm text-mute">
			{data.boards.length}
			{data.boards.length === 1 ? 'quadro' : 'quadros'} · {data.usuario.nome}
		</p>
	</div>
</div>

<!-- min-w-0 no campo: sem ele o tamanho mínimo automático do flex é o
     conteúdo, e o input se recusa a encolher abaixo disso. flex-1 faz ele
     ficar com todo o espaço que o botão não usa. -->
<form onsubmit={criar} class="mt-6 flex flex-wrap gap-2">
	<input
		class="campo min-w-0 flex-1"
		bind:value={titulo}
		placeholder="Nome do novo quadro"
		required
		maxlength="120"
		aria-label="Nome do novo quadro"
	/>
	<button type="submit" class="botao w-auto shrink-0 px-5" disabled={enviando}>
		{enviando ? 'Criando…' : 'Criar quadro'}
	</button>
</form>

{#if erro}
	<p class="erro-form mt-4">{erro}</p>
{/if}

{#if data.boards.length === 0}
	<p class="mt-10 rounded-lg border border-dashed border-hairline-strong p-8 text-center text-sm text-mute">
		Nenhum quadro ainda. Crie o primeiro acima.
	</p>
{:else}
	<ul class="mt-8 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
		{#each data.boards as board (board.id)}
			<li class="rounded-lg border border-hairline bg-surface p-5 shadow-ficha">
				<a href="/painel/quadros/{board.id}" class="block">
					<h2 class="text-sm font-semibold text-ink">{board.titulo}</h2>
					<p class="mt-1 text-xs text-mute">criado em {formatarData(board.criadoEm)}</p>
				</a>
				<div class="mt-4 flex items-center justify-between">
					<span class="chip" class:chip-neutro={board.papel !== 'dono'}>
						<i class="size-1.5 rounded-full bg-current"></i>{board.papel}
					</span>
					{#if board.papel === 'dono'}
						<button
							onclick={() => apagar(board.id, board.titulo)}
							class="cursor-pointer text-xs text-mute hover:text-negativo"
						>
							Apagar
						</button>
					{/if}
				</div>
			</li>
		{/each}
	</ul>
{/if}
