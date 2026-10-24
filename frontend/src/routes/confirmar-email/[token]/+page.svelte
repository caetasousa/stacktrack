<script lang="ts">
	// A tela que o link de confirmação abre. A confirmação já aconteceu no load.
	let { data } = $props();
</script>

<svelte:head>
	<title>Confirmação de email · stacktrack</title>
	<!-- O token está na URL: nada desta página vai para buscador nem para o
	     Referer de outro site. -->
	<meta name="robots" content="noindex, nofollow" />
	<meta name="referrer" content="no-referrer" />
</svelte:head>

<div
	class="mx-auto my-auto w-full max-w-sm rounded-lg border border-hairline bg-surface p-7 shadow-flutuante"
>
	{#if data.confirmado}
		<h1 class="text-xl font-semibold tracking-tight text-ink">Email confirmado</h1>
		<p class="mt-1.5 text-sm text-mute">
			Os convites para os quadros de outras pessoas passam a aparecer no seu painel.
		</p>
	{:else}
		<h1 class="text-xl font-semibold tracking-tight text-ink">Este link é inválido ou já venceu</h1>
		<p class="mt-1.5 text-sm text-mute">
			O link de confirmação vale por sete dias e funciona uma vez só. Se a conta ainda não estiver
			confirmada, peça outro no painel.
		</p>
	{/if}
	<a href="/painel" class="botao mt-6 inline-flex justify-center">Ir para o painel</a>
</div>
