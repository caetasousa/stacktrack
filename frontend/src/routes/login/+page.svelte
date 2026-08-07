<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { entrar } from '$lib/api/auth';
	import { ApiError } from '$lib/api/client';
	import { sessao } from '$lib/stores/session.svelte';
	import { destinoDeVolta } from '$lib/navegacao';

	let email = $state('');
	let senha = $state('');
	let erro = $state('');
	let enviando = $state(false);

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

<svelte:head><title>Entrar · kanbanGo</title></svelte:head>

<div
	class="mx-auto w-full max-w-sm rounded-lg border border-hairline bg-surface p-7 shadow-flutuante"
>
	<h1 class="text-xl font-semibold tracking-tight text-ink">Entrar</h1>
	<p class="mt-1.5 text-sm text-mute">Bem-vindo de volta.</p>

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
			<label class="rotulo" for="senha">Senha</label>
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
		<a href="/cadastro" class="font-semibold text-accent-texto hover:underline">Criar conta</a>
	</p>
</div>
