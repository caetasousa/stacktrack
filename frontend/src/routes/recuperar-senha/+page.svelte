<script lang="ts">
	// O "esqueci minha senha". A tela não sabe se o email tem conta — a API
	// responde igual nos dois casos —, e por isso diz "se existe uma conta".
	import { page } from '$app/state';
	import { pedirRecuperacao } from '$lib/api/auth';
	import { ApiError } from '$lib/api/client';

	// Vindo do login, o email que a pessoa já tinha digitado vem junto.
	let email = $state(page.url.searchParams.get('email') ?? '');
	let erro = $state('');
	let enviando = $state(false);
	let enviado = $state(false);

	async function enviar(evento: SubmitEvent) {
		evento.preventDefault();
		erro = '';
		enviando = true;
		try {
			await pedirRecuperacao(email);
			enviado = true;
		} catch (e) {
			erro = e instanceof ApiError ? e.message : 'não foi possível pedir o link';
		} finally {
			enviando = false;
		}
	}
</script>

<svelte:head><title>Recuperar a senha · stacktrack</title></svelte:head>

<div
	class="mx-auto my-auto w-full max-w-sm rounded-lg border border-hairline bg-surface p-7 shadow-flutuante"
>
	<h1 class="text-xl font-semibold tracking-tight text-ink">Recuperar a senha</h1>

	{#if enviado}
		<p class="mt-4 rounded-md border border-accent bg-accent-suave p-3 text-sm text-body">
			Se existe uma conta com esse email, mandamos um link para criar uma senha nova. Ele vale por
			uma hora e funciona uma vez só.
		</p>
		<p class="mt-4 text-sm text-mute">
			Não chegou? Confira a caixa de spam, ou peça de novo daqui a pouco.
		</p>
	{:else}
		<p class="mt-1.5 text-sm text-mute">
			Informe o email da conta. Mandamos um link para você criar uma senha nova.
		</p>

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

			{#if erro}
				<p class="erro-form">{erro}</p>
			{/if}

			<button type="submit" class="botao" disabled={enviando}>
				{enviando ? 'Enviando…' : 'Enviar link'}
			</button>
		</form>
	{/if}

	<p class="mt-6 text-center text-sm text-mute">
		Lembrou?
		<a href="/login" class="font-semibold text-accent-texto hover:underline">Entrar</a>
	</p>
</div>
