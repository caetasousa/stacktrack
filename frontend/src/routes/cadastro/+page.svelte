<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { cadastrar } from '$lib/api/auth';
	import { ApiError } from '$lib/api/client';
	import { sessao } from '$lib/stores/session.svelte';
	import { destinoDeVolta } from '$lib/navegacao';

	let nome = $state('');
	let email = $state('');
	let senha = $state('');
	let erro = $state('');
	let enviando = $state(false);

	async function enviar(evento: SubmitEvent) {
		evento.preventDefault();
		erro = '';
		enviando = true;

		try {
			// O cadastro já devolve a conta autenticada (e o cookie de sessão),
			// então dá para popular a store sem uma segunda ida ao /auth/me.
			sessao.definir(await cadastrar({ nome, email, senha }));
			await goto(destinoDeVolta(page.url));
		} catch (e) {
			erro = e instanceof ApiError ? e.message : 'não foi possível criar a conta';
		} finally {
			enviando = false;
		}
	}
</script>

<svelte:head><title>Criar conta · stacktrack</title></svelte:head>

<div
	class="mx-auto my-auto w-full max-w-sm rounded-lg border border-hairline bg-surface p-7 shadow-flutuante"
>
	<h1 class="text-xl font-semibold tracking-tight text-ink">Criar conta</h1>
	<p class="mt-1.5 text-sm text-mute">Leva menos de um minuto.</p>

	<form onsubmit={enviar} class="mt-6 space-y-4">
		<div>
			<label class="rotulo" for="nome">Nome</label>
			<input id="nome" class="campo" bind:value={nome} required autocomplete="name" />
		</div>

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
				minlength="15"
				autocomplete="new-password"
			/>
			<p class="mt-1.5 text-xs text-mute">
				Ao menos 15 caracteres. Uma frase que só você diria vale mais que símbolos.
			</p>
		</div>

		{#if erro}
			<p class="erro-form">{erro}</p>
		{/if}

		<button type="submit" class="botao" disabled={enviando}>
			{enviando ? 'Criando…' : 'Criar conta'}
		</button>
	</form>

	<p class="mt-6 text-center text-sm text-mute">
		Já tem conta? <a href="/login" class="font-semibold text-accent-texto hover:underline">Entrar</a
		>
	</p>
</div>
