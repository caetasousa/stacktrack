<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { cadastrar } from '$lib/api/auth';
	import { ApiError } from '$lib/api/client';
	import { sessao } from '$lib/stores/session.svelte';
	import { conviteEmAndamento, destinoDeVolta } from '$lib/navegacao';
	import { mascararEmail } from '$lib/email';

	let nome = $state('');
	let email = $state('');
	let senha = $state('');
	let erro = $state('');
	let enviando = $state(false);

	// Vindo de um convite, a tela sabe para qual email ele é (mascarado) e leva
	// o mesmo contexto ao "Já tem conta? Entrar" — senão quem trocasse de ideia
	// no meio do caminho perderia o convite.
	const convite = $derived(conviteEmAndamento(page.url));

	async function enviar(evento: SubmitEvent) {
		evento.preventDefault();
		erro = '';
		// O convite é amarrado ao email: uma conta com outro endereço nasceria e
		// não conseguiria aceitá-lo. A máscara é uma dica — quem decide é o
		// servidor no aceite —, mas basta para não deixar a pessoa criar a conta
		// errada sem saber.
		if (convite && mascararEmail(email) !== convite.para) {
			erro = `O convite é para ${convite.para}. Use esse email para criar a conta.`;
			return;
		}
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

	{#if convite}
		<p class="mt-4 rounded-md border border-accent bg-accent-suave p-3 text-xs text-body">
			<b class="font-semibold text-accent-texto">Você está aceitando um convite.</b>
			Crie a conta com <b class="text-ink">{convite.para}</b> — o convite é desse email. Depois você volta
			para ele.
		</p>
	{/if}

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
		<p class="text-center text-xs text-mute">
			Ao criar a conta, você concorda com os
			<a href="/termos" class="underline hover:text-ink">Termos de Uso</a> e com a
			<a href="/privacidade" class="underline hover:text-ink">Política de Privacidade</a>.
		</p>
	</form>

	<p class="mt-6 text-center text-sm text-mute">
		Já tem conta?
		<a
			href={convite ? `/login${page.url.search}` : '/login'}
			class="font-semibold text-accent-texto hover:underline">Entrar</a
		>
	</p>
</div>
