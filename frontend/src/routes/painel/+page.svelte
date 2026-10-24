<script lang="ts">
	import { confirmar } from '$lib/confirmar.svelte';
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
		const ok = await confirmar({
			titulo: `Apagar "${nome}"?`,
			detalhe: 'As colunas e os cards deste quadro vão junto.',
			acao: 'Apagar o quadro'
		});
		if (!ok) return;
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

	// Os quadros da pessoa e os que outra pessoa compartilhou, separados: um
	// quadro em que ela entrou por convite é de outro dono, e a tela não pode
	// misturá-lo com os dela. Quem foi convidado como DONO divide o quadro de
	// verdade, e ele fica entre os seus.
	const seus = $derived(data.boards.filter((b) => b.papel === 'dono'));
	const compartilhados = $derived(data.boards.filter((b) => b.papel !== 'dono'));
</script>

<svelte:head><title>Painel · stacktrack</title></svelte:head>

<div class="flex flex-wrap items-end justify-between gap-4">
	<div>
		<h1 class="text-2xl font-semibold tracking-tight text-ink">Quadros</h1>
		<p class="mt-1 text-sm text-mute">
			{data.boards.length}
			{data.boards.length === 1 ? 'quadro' : 'quadros'} · {data.usuario.nome}
		</p>
	</div>

	<!-- O formulário ocupava a largura inteira e vinha ANTES dos quadros, o que
	     punha um campo de texto vazio como assunto principal de uma página cujo
	     assunto são os quadros que já existem. Aqui ele é uma ação do cabeçalho,
	     com largura limitada. -->
	<form onsubmit={criar} class="flex w-full max-w-sm gap-2">
		<input
			class="campo min-w-0 flex-1"
			bind:value={titulo}
			placeholder="Nome do novo quadro"
			required
			maxlength="120"
			aria-label="Nome do novo quadro"
		/>
		<button type="submit" class="botao w-auto shrink-0 px-4" disabled={enviando}>
			{enviando ? 'Criando…' : 'Criar'}
		</button>
	</form>
</div>

{#if erro}
	<p class="erro-form mt-4">{erro}</p>
{/if}

<section class="mt-8" aria-labelledby="titulo-seus">
	<h2 id="titulo-seus" class="text-xs font-semibold tracking-widest text-mute uppercase">
		Seus quadros
	</h2>

	{#if seus.length === 0}
		<div
			class="mt-3 flex flex-col items-center gap-2 rounded-lg border border-dashed border-hairline-strong px-6 py-12 text-center"
		>
			<b class="text-sm font-semibold text-ink">Nenhum quadro seu ainda</b>
			<p class="max-w-sm text-sm text-mute">
				Um quadro guarda as colunas e os cards de um fluxo de trabalho. Dê um nome ao primeiro no
				campo acima.
			</p>
		</div>
	{:else}
		<ul class="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
			{#each seus as board (board.id)}
				<!-- O cartão inteiro é o alvo do clique, e não só o título: `relative` no
				     item mais o `after:` do link cobrem a ficha toda. O botão de apagar
				     sobe de camada para continuar clicável por cima dele. -->
				<li
					class="group relative flex flex-col justify-between gap-6 rounded-lg border border-hairline bg-surface p-5 shadow-ficha transition-colors hover:border-hairline-strong focus-within:border-accent"
				>
					<div>
						<a
							href="/painel/quadros/{board.id}"
							class="text-sm font-semibold text-ink after:absolute after:inset-0 after:content-[''] focus-visible:outline-none"
						>
							{board.titulo}
						</a>
						<p class="mt-1 text-xs text-mute">criado em {formatarData(board.criadoEm)}</p>
					</div>

					<div class="flex items-center justify-between">
						<span class="chip"><i class="size-1.5 rounded-full bg-current"></i>{board.papel}</span>
						<button
							onclick={() => apagar(board.id, board.titulo)}
							class="relative cursor-pointer rounded px-2 py-1 text-xs text-mute transition-colors hover:bg-surface-elevated hover:text-negativo"
						>
							Apagar
						</button>
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</section>

{#if compartilhados.length > 0}
	<section class="mt-10" aria-labelledby="titulo-compartilhados">
		<h2
			id="titulo-compartilhados"
			class="text-xs font-semibold tracking-widest text-mute uppercase"
		>
			Compartilhados com você
		</h2>
		<p class="mt-1 text-xs text-mute">
			Quadros de outras pessoas, em que você entrou por convite. Você vê e mexe só no quadro
			compartilhado, conforme o seu papel nele.
		</p>

		<ul class="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
			{#each compartilhados as board (board.id)}
				<li
					class="group relative flex flex-col justify-between gap-6 rounded-lg border border-hairline bg-surface p-5 shadow-ficha transition-colors hover:border-hairline-strong focus-within:border-accent"
				>
					<div>
						<a
							href="/painel/quadros/{board.id}"
							class="text-sm font-semibold text-ink after:absolute after:inset-0 after:content-[''] focus-visible:outline-none"
						>
							{board.titulo}
						</a>
						<p class="mt-1 text-xs text-mute">
							{board.dono ? `de ${board.dono}` : 'de outra pessoa'}
						</p>
					</div>

					<div class="flex items-center justify-between">
						<span class="chip chip-neutro">
							<i class="size-1.5 rounded-full bg-current"></i>{board.papel}
						</span>
					</div>
				</li>
			{/each}
		</ul>
	</section>
{/if}
