<script lang="ts">
	import { goto } from '$app/navigation';
	import { entrar } from '$lib/api/auth';
	import { ApiError } from '$lib/api/client';
	import { sessao } from '$lib/stores/session.svelte';

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
			await goto('/painel');
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

<div class="mx-auto w-full max-w-sm">
	<h1 class="text-2xl font-semibold tracking-tight text-ink">Entrar</h1>

	<form onsubmit={enviar} class="mt-8 space-y-4">
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
		Não tem conta? <a href="/cadastro" class="text-ink hover:underline">Criar conta</a>
	</p>
</div>
