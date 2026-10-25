<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { destinoDe, fraseDe, motivoDoEncerramento } from '$lib/notificacoes';
	import { sino } from '$lib/stores/notificacoes.svelte';

	let { data } = $props();

	const aviso = $derived(data.notificacao);
	const motivo = $derived(aviso ? motivoDoEncerramento(aviso) : '');
	const frase = $derived(aviso ? fraseDe(aviso) : null);

	onMount(async () => {
		const n = data.notificacao;
		if (!n) return;
		// Abrir pelo email é ler: o sino das outras abas acompanha.
		await sino.marcarLida(n).catch(() => {});
		if (!motivoDoEncerramento(n)) await goto(destinoDe(n), { replaceState: true });
	});
</script>

<svelte:head>
	<title>Notificação · stacktrack</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div
	class="mx-auto my-auto w-full max-w-sm rounded-lg border border-hairline bg-surface p-7 shadow-flutuante"
>
	{#if !aviso}
		<h1 class="text-xl font-semibold tracking-tight text-ink">Este aviso não existe mais</h1>
		<p class="mt-1.5 text-sm text-mute">
			O card, o comentário ou o quadro de que ele falava foi apagado — ou você não participa mais do
			quadro.
		</p>
		<a href="/painel" class="botao mt-6 inline-flex justify-center">Ir para o painel</a>
	{:else if motivo}
		<h1 class="text-xl font-semibold tracking-tight text-ink">Este aviso não vale mais</h1>
		<p class="mt-1.5 text-sm text-body">
			{#if frase?.sujeito}<b class="font-semibold text-ink">{frase.sujeito}</b>
			{/if}{frase?.texto}.
		</p>
		<p class="mt-2 text-sm text-mute">Depois disso, {motivo}.</p>
		<div class="mt-6 flex flex-col gap-2">
			<a href={destinoDe(aviso)} class="botao-secundario inline-flex justify-center">
				{aviso.tipo === 'convite' ? 'Ver seus convites' : 'Abrir o card mesmo assim'}
			</a>
			<a href="/painel" class="botao inline-flex justify-center">Ir para o painel</a>
		</div>
	{:else}
		<p class="text-sm text-mute">Abrindo…</p>
	{/if}
</div>
