<script lang="ts">
	// A página de erro do aplicativo inteiro. Sem ela, um endereço errado ou uma
	// falha da API caía na página crua do SvelteKit — um número e uma frase em
	// inglês, sem caminho de volta.
	//
	// Quando o `load` explica o 404 ("Convite inválido ou expirado", "Este link
	// não está mais disponível"), a explicação é o título: é ela que diz à pessoa
	// o que aconteceu. A genérica fica para o endereço que não existe.
	import { page } from '$app/state';
	import { sessao } from '$lib/stores/session.svelte';

	const naoEncontrada = $derived(page.status === 404);
	const mensagemDoLoad = $derived(
		page.error?.message && page.error.message !== 'Not Found' ? page.error.message : ''
	);
	const titulo = $derived(
		naoEncontrada ? mensagemDoLoad || 'Página não encontrada' : 'Algo deu errado'
	);
</script>

<svelte:head>
	<title>{titulo} · stacktrack</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<div
	class="mx-auto my-auto w-full max-w-sm rounded-lg border border-hairline bg-surface p-7 text-center shadow-flutuante"
>
	<p class="text-xs font-semibold tracking-widest text-mute uppercase">Erro {page.status}</p>
	<h1 class="mt-2 text-xl font-semibold tracking-tight text-ink">{titulo}</h1>
	{#if naoEncontrada}
		{#if !mensagemDoLoad}
			<!-- O 404 vale também para o quadro de que a pessoa não participa: a API
			     não diferencia os dois casos, de propósito, e a tela também não. -->
			<p class="mt-2 text-sm text-mute">
				O endereço não existe, foi apagado, ou é de um quadro de que você não participa.
			</p>
		{/if}
	{:else}
		<p class="mt-2 text-sm text-mute">
			Não foi possível carregar esta página agora. Tente de novo em alguns instantes.
		</p>
	{/if}
	<div class="mt-6 flex flex-col gap-2">
		{#if sessao.usuario}
			<a href="/painel" class="botao inline-flex justify-center">Ir para o painel</a>
		{:else}
			<a href="/" class="botao inline-flex justify-center">Ir para o início</a>
		{/if}
		{#if !naoEncontrada}
			<button
				type="button"
				class="botao-secundario cursor-pointer"
				onclick={() => location.reload()}>Tentar de novo</button
			>
		{/if}
	</div>
</div>
