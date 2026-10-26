<script lang="ts">
	// A moldura dos textos legais (Termos de Uso e Política de Privacidade): o
	// título, a data da versão e a largura de leitura. O texto em si fica em cada
	// página.
	import type { Snippet } from 'svelte';
	import { TEXTOS_LEGAIS_ATUALIZADOS_EM } from '$lib/instituicao';

	let { titulo, children }: { titulo: string; children: Snippet } = $props();
</script>

<svelte:head>
	<title>{titulo} · stacktrack</title>
</svelte:head>

<article class="mx-auto w-full max-w-2xl">
	<h1 class="text-2xl font-semibold tracking-tight text-ink">{titulo}</h1>
	<p class="mt-1 text-xs text-mute">Versão de {TEXTOS_LEGAIS_ATUALIZADOS_EM}</p>
	<div
		class="markdown mt-8 text-sm leading-relaxed text-body [&_h2]:mt-8 [&_h2]:text-base [&_h2]:font-semibold [&_h2]:text-ink"
	>
		{@render children()}
	</div>
</article>
