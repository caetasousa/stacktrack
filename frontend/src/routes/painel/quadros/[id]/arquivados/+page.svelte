<script lang="ts">
	// A tela de arquivados: o desfazer do quadro.
	//
	// É ela que torna arquivar seguro o bastante para não pedir confirmação —
	// tirar um card do quadro deixa de ser uma decisão irreversível e vira uma
	// que se volta atrás em dois cliques.
	import { invalidateAll } from '$app/navigation';
	import { ApiError } from '$lib/api/client';
	import { confirmar } from '$lib/confirmar.svelte';
	import { apagarCard, apagarColuna, desarquivarCard, desarquivarColuna } from '$lib/api/boards';

	let { data } = $props();

	let erro = $state('');
	const podeEditar = $derived(data.quadro.papel !== 'leitor');
	const vazio = $derived(data.arquivo.cards.length === 0 && data.arquivo.colunas.length === 0);

	function falhar(e: unknown, padrao: string) {
		erro = e instanceof ApiError ? e.message : padrao;
	}

	async function recarregar() {
		erro = '';
		await invalidateAll();
	}

	async function devolverCard(id: string) {
		try {
			await desarquivarCard(id);
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível devolver o card ao quadro');
		}
	}

	async function devolverColuna(id: string) {
		try {
			await desarquivarColuna(id);
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível devolver a coluna ao quadro');
		}
	}

	// Apagar daqui é o único caminho que NÃO tem volta, e por isso é o único
	// que pergunta.
	async function apagarDeVezCard(id: string, titulo: string) {
		const ok = await confirmar({
			titulo: `Apagar "${titulo}" de vez?`,
			detalhe: 'Comentários, checklists e anexos vão junto. Isto não tem desfazer.',
			acao: 'Apagar de vez'
		});
		if (!ok) return;
		try {
			await apagarCard(id);
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível apagar o card');
		}
	}

	async function apagarDeVezColuna(id: string, titulo: string) {
		const ok = await confirmar({
			titulo: `Apagar a coluna "${titulo}" de vez?`,
			detalhe: 'Os cards dela vão junto, com tudo que pende deles. Isto não tem desfazer.',
			acao: 'Apagar de vez'
		});
		if (!ok) return;
		try {
			await apagarColuna(id);
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível apagar a coluna');
		}
	}

	// Data curta: no arquivo o que importa é "quando saiu", não a precisão.
	function quando(iso: string): string {
		return new Date(iso).toLocaleString('pt-BR', {
			day: '2-digit',
			month: 'short',
			hour: '2-digit',
			minute: '2-digit'
		});
	}
</script>

<svelte:head><title>Arquivados · {data.quadro.titulo} · stacktrack</title></svelte:head>

<div class="flex flex-wrap items-baseline justify-between gap-4">
	<div>
		<h1 class="text-xl font-semibold tracking-tight text-ink">Arquivados</h1>
		<p class="mt-0.5 text-sm text-mute">{data.quadro.titulo}</p>
	</div>
	<a href="/painel/quadros/{data.quadro.id}" class="text-xs text-mute hover:text-ink">
		Voltar ao quadro
	</a>
</div>

{#if erro}
	<p class="mt-4 rounded-sm border border-negativo px-3 py-2 text-sm text-negativo" role="alert">
		{erro}
	</p>
{/if}

{#if vazio}
	<p class="mt-8 text-sm text-mute">
		Nada arquivado. O que sair do quadro aparece aqui e pode voltar ao mesmo lugar.
	</p>
{:else}
	{#if data.arquivo.colunas.length > 0}
		<section class="mt-8">
			<h2 class="text-xs font-semibold tracking-wide text-mute uppercase">Colunas</h2>
			<!-- A coluna vem antes do card de propósito: devolvê-la traz os cards
			     dela junto, e quem procura um card que "sumiu com a coluna" o
			     encontra voltando a coluna, não na lista de cards. -->
			<p class="mt-1 text-xs text-mute">
				Os cards de uma coluna arquivada não aparecem na lista abaixo — eles voltam com ela.
			</p>
			<ul class="mt-3 space-y-2">
				{#each data.arquivo.colunas as coluna (coluna.id)}
					<li
						class="flex flex-wrap items-center gap-3 rounded-sm border border-hairline bg-surface p-3"
					>
						<span class="flex-1 text-sm font-medium text-body">
							{#if coluna.cor}<i
									class="cor-{coluna.cor} mr-2 inline-block size-2 rounded-full bg-current"
								></i>{/if}{coluna.titulo}
						</span>
						<span class="text-xs tabular-nums text-mute">{quando(coluna.arquivadoEm)}</span>
						{#if podeEditar}
							<button
								class="botao-secundario w-auto px-3 py-1 text-xs"
								onclick={() => devolverColuna(coluna.id)}
							>
								Devolver ao quadro
							</button>
							<button
								class="cursor-pointer text-xs text-mute hover:text-negativo"
								onclick={() => apagarDeVezColuna(coluna.id, coluna.titulo)}
							>
								Apagar de vez
							</button>
						{/if}
					</li>
				{/each}
			</ul>
		</section>
	{/if}

	{#if data.arquivo.cards.length > 0}
		<section class="mt-8">
			<h2 class="text-xs font-semibold tracking-wide text-mute uppercase">Cards</h2>
			<ul class="mt-3 space-y-2">
				{#each data.arquivo.cards as card (card.id)}
					<li
						class="flex flex-wrap items-center gap-3 rounded-sm border border-hairline bg-surface p-3"
					>
						<span class="flex-1 text-sm font-medium text-body">
							{#if card.cor}<i
									class="cor-{card.cor} mr-2 inline-block size-2 rounded-full bg-current"
								></i>{/if}{card.titulo}
						</span>
						<!-- A coluna de origem responde ONDE o card cai ao voltar. -->
						<span class="text-xs text-mute">de {card.coluna}</span>
						<span class="text-xs tabular-nums text-mute">{quando(card.arquivadoEm)}</span>
						{#if podeEditar}
							<button
								class="botao-secundario w-auto px-3 py-1 text-xs"
								onclick={() => devolverCard(card.id)}
							>
								Devolver ao quadro
							</button>
							<button
								class="cursor-pointer text-xs text-mute hover:text-negativo"
								onclick={() => apagarDeVezCard(card.id, card.titulo)}
							>
								Apagar de vez
							</button>
						{/if}
					</li>
				{/each}
			</ul>
		</section>
	{/if}
{/if}
