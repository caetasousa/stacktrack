<script lang="ts">
	import './layout.css';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { sessao } from '$lib/stores/session.svelte';
	import { tema } from '$lib/stores/tema.svelte';
	import SeletorTema from '$lib/components/SeletorTema.svelte';

	let { children } = $props();

	onMount(() => {
		// O tema já está pintado (static/tema.js); aqui a store só passa a
		// saber qual é, para o botão mostrar o ícone certo.
		tema.sincronizar();
		sessao.carregar();
	});

	async function encerrar() {
		await sessao.encerrar();
		goto('/login');
	}
</script>

<div class="flex min-h-screen flex-col bg-canvas font-sans text-body">
	<header class="border-b border-hairline bg-surface">
		<div class="mx-auto flex max-w-6xl items-center justify-between gap-4 px-6 py-3">
			<a href="/" class="flex items-center gap-2 text-sm font-semibold tracking-tight text-ink">
				<span class="inline-block size-5 rounded-sm bg-accent"></span>
				kanbanGo
			</a>

			<nav class="flex items-center gap-2 text-sm">
				{#if sessao.carregando}
					<span class="px-2 text-xs text-mute">…</span>
				{:else if sessao.usuario}
					<a href="/painel" class="px-2 py-1 text-mute hover:text-ink">Painel</a>
					<span class="hidden px-2 text-xs text-mute sm:inline">{sessao.usuario.nome}</span>
					<button onclick={encerrar} class="cursor-pointer px-2 py-1 text-mute hover:text-ink">
						Sair
					</button>
				{:else}
					<a href="/login" class="px-2 py-1 text-mute hover:text-ink">Entrar</a>
					<a
						href="/cadastro"
						class="rounded-sm bg-accent-suave px-3 py-1.5 font-semibold text-accent-texto"
					>
						Criar conta
					</a>
				{/if}
				<SeletorTema />
			</nav>
		</div>
	</header>

	<main class="mx-auto flex w-full max-w-6xl flex-1 flex-col px-6 py-12">
		{@render children()}
	</main>

	<footer class="border-t border-hairline">
		<div
			class="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-3 px-6 py-6 text-xs text-mute"
		>
			<span>kanbanGo — quadro colaborativo em tempo real.</span>
			<span>Projeto de estudo · Go + SvelteKit</span>
		</div>
	</footer>
</div>
