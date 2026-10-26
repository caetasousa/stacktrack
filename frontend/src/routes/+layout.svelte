<script lang="ts">
	import './layout.css';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { sessao } from '$lib/stores/session.svelte';
	import { sino } from '$lib/stores/notificacoes.svelte';
	import { tema } from '$lib/stores/tema.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import SeletorTema from '$lib/components/SeletorTema.svelte';
	import SinoDeNotificacoes from '$lib/components/SinoDeNotificacoes.svelte';
	import DialogoDeConfirmacao from '$lib/components/DialogoDeConfirmacao.svelte';
	import { PRODUTO } from '$lib/instituicao';

	let { children } = $props();

	onMount(() => {
		// O tema já está pintado (static/tema.js); aqui a store só passa a
		// saber qual é, para o botão mostrar o ícone certo.
		tema.sincronizar();
		sessao.carregar();
	});

	// O sino acompanha a sessão: liga quando alguém entra, troca de conexão se
	// outra conta entrar na mesma aba, e desliga ao sair — sem isso, a aba
	// continuaria ouvindo as notificações de quem já saiu.
	$effect(() => {
		const usuario = sessao.usuario;
		if (usuario) sino.iniciar(usuario.id);
		else sino.parar();
	});

	async function encerrar() {
		await sessao.encerrar();
		goto('/login');
	}
</script>

<div class="flex min-h-screen flex-col bg-canvas font-sans text-body">
	<!-- Para quem navega pelo teclado ou por leitor de tela: sem isto, cada página
	     começa atravessando o cabeçalho inteiro antes de chegar ao que importa. -->
	<a
		href="#conteudo"
		class="sr-only rounded-sm bg-surface px-3 py-2 text-sm font-semibold text-ink focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-50"
	>
		Pular para o conteúdo
	</a>
	<header class="border-b border-hairline bg-surface">
		<div class="mx-auto flex max-w-6xl items-center justify-between gap-4 px-6 py-3">
			<a href="/" aria-label="{PRODUTO} — início" class="flex items-center">
				<Logo comNome />
			</a>

			<nav class="flex items-center gap-2 text-sm">
				{#if sessao.carregando}
					<span class="px-2 text-xs text-mute">…</span>
				{:else if sessao.usuario}
					<a href="/painel" class="px-2 py-1 text-mute hover:text-ink">Painel</a>
					<span class="hidden px-2 text-xs text-mute sm:inline">{sessao.usuario.nome}</span>
					<SinoDeNotificacoes />
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

	<main id="conteudo" class="mx-auto flex w-full max-w-6xl flex-1 flex-col px-6 py-12">
		{@render children()}
	</main>

	<!-- Um só diálogo de confirmação para o aplicativo inteiro. Ver
	     $lib/confirmar.svelte.ts. -->
	<DialogoDeConfirmacao />

	<footer class="border-t border-hairline">
		<div
			class="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-3 px-6 py-6 text-xs text-mute"
		>
			<span>{PRODUTO} — quadro colaborativo em tempo real.</span>
			<nav aria-label="Informações legais" class="flex flex-wrap gap-4">
				<a href="/termos" class="hover:text-ink">Termos de uso</a>
				<a href="/privacidade" class="hover:text-ink">Política de privacidade</a>
			</nav>
		</div>
	</footer>
</div>
