<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { entrar } from '$lib/api/auth';
	import { ApiError } from '$lib/api/client';
	import { sessao } from '$lib/stores/session.svelte';
	import { conviteEmAndamento, destinoDeVolta } from '$lib/navegacao';

	let email = $state('');
	let senha = $state('');
	let erro = $state('');
	let enviando = $state(false);

	// Vindo de um convite: diz com qual conta entrar, e leva o contexto ao
	// "Criar conta" para quem perceber que ainda não tem uma.
	const convite = $derived(conviteEmAndamento(page.url));

	async function enviar(evento: SubmitEvent) {
		evento.preventDefault();
		erro = '';
		enviando = true;

		try {
			sessao.definir(await entrar({ email, senha }));
			await goto(destinoDeVolta(page.url));
		} catch (e) {
			// A mensagem da API é genérica de propósito ("credenciais
			// inválidas"): dizer se foi o email ou a senha revelaria quais
			// emails têm conta aqui.
			erro = e instanceof ApiError ? e.message : 'não foi possível entrar';
		} finally {
			enviando = false;
		}
	}
</script>

<svelte:head><title>Entrar · stacktrack</title></svelte:head>

<div
	class="mx-auto my-auto w-full max-w-sm rounded-lg border border-hairline bg-surface p-7 shadow-flutuante"
>
	<h1 class="text-xl font-semibold tracking-tight text-ink">Entrar</h1>
	<p class="mt-1.5 text-sm text-mute">Bem-vindo de volta.</p>

	{#if convite}
		<p class="mt-4 rounded-md border border-accent bg-accent-suave p-3 text-xs text-body">
			Entre com <b class="text-ink">{convite.para}</b> para aceitar o convite. Depois você volta para
			ele.
		</p>
	{/if}

	<form onsubmit={enviar} class="mt-6 space-y-4">
		<div>
			<label class="rotulo" for="email">Email</label>
			<input
				id="email"
				type="email"
				class="campo"
				bind:value={email}
				required
				autocomplete="email"
			/>
		</div>

		<div>
			<div class="flex items-baseline justify-between">
				<label class="rotulo" for="senha">Senha</label>
				<a
					href={email ? `/recuperar-senha?email=${encodeURIComponent(email)}` : '/recuperar-senha'}
					class="text-xs text-accent-texto hover:underline">Esqueci minha senha</a
				>
			</div>
			<input
				id="senha"
				type="password"
				class="campo"
				bind:value={senha}
				required
				autocomplete="current-password"
			/>
		</div>

		{#if erro}
			<p class="erro-form">{erro}</p>
		{/if}

		<button type="submit" class="botao" disabled={enviando}>
			{enviando ? 'Entrando…' : 'Entrar'}
		</button>
	</form>

	<p class="mt-6 text-center text-sm text-mute">
		Não tem conta?
		<a
			href={convite ? `/cadastro${page.url.search}` : '/cadastro'}
			class="font-semibold text-accent-texto hover:underline">Criar conta</a
		>
	</p>
</div>
