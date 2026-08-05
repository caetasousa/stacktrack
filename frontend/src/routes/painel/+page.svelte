<script lang="ts">
	let { data } = $props();
</script>

<svelte:head><title>Painel · kanbanGo</title></svelte:head>

<h1 class="text-2xl font-semibold tracking-tight text-ink">Olá, {data.usuario.nome}</h1>
<p class="mt-2 text-sm text-mute">{data.usuario.email}</p>

<section class="mt-10 rounded-lg border border-hairline bg-surface p-6">
	<h2 class="text-xs font-semibold tracking-widest text-mute uppercase">Seus quadros</h2>
	<p class="mt-4 text-sm text-mute">
		Ainda não há quadros — eles chegam na fase 2. Esta página existe para provar que a sessão
		funciona: se você recarregar agora, continua aqui; se apagar o cookie, volta para o login.
	</p>
</section>
