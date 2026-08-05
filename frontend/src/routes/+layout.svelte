<script lang="ts">
	import './layout.css';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { sessao } from '$lib/stores/session.svelte';

	let { children } = $props();

	onMount(() => {
		sessao.carregar();
	});

	async function encerrar() {
		await sessao.encerrar();
		goto('/login');
	}
</script>

<div class="flex min-h-screen flex-col bg-canvas font-sans text-body">
	<header class="border-b border-hairline">
		<div class="mx-auto flex max-w-5xl items-center justify-between px-6 py-4">
			<a href="/" class="text-sm font-semibold tracking-tight text-ink">kanbanGo</a>

			<nav class="flex items-center gap-4 text-sm">
				{#if sessao.carregando}
					<span class="text-xs text-mute">…</span>
				{:else if sessao.usuario}
					<a href="/painel" class="text-mute hover:text-ink">Painel</a>
					<span class="text-xs text-mute">{sessao.usuario.nome}</span>
					<button onclick={encerrar} class="cursor-pointer text-mute hover:text-ink">Sair</button>
				{:else}
					<a href="/login" class="text-mute hover:text-ink">Entrar</a>
					<a href="/cadastro" class="text-ink hover:underline">Criar conta</a>
				{/if}
			</nav>
		</div>
	</header>

	<main class="mx-auto flex w-full max-w-5xl flex-1 flex-col px-6 py-16">
		{@render children()}
	</main>

	<footer class="border-t border-hairline">
		<div class="mx-auto flex max-w-5xl items-center justify-between px-6 py-8 text-xs text-mute">
			<span>kanbanGo — quadro colaborativo em tempo real.</span>
			<span>Projeto de estudo · Go + SvelteKit</span>
		</div>
	</footer>
</div>
