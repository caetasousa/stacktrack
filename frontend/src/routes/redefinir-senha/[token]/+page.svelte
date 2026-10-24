<script lang="ts">
	// A tela que o link do email abre: a senha nova, e a entrada na conta.
	import { goto } from '$app/navigation';
	import { redefinirSenha } from '$lib/api/auth';
	import { ApiError } from '$lib/api/client';
	import { sessao } from '$lib/stores/session.svelte';

	let { data } = $props();

	let senha = $state('');
	let erro = $state('');
	let enviando = $state(false);
	// O link pode vencer entre abrir a tela e salvar: aí a tela vira a do link
	// inválido, em vez de mostrar um erro solto embaixo do formulário.
	let venceu = $state(false);

	const valeAte = $derived(
		data.recuperacao
			? new Date(data.recuperacao.expiraEm).toLocaleTimeString('pt-BR', {
					hour: '2-digit',
					minute: '2-digit'
				})
			: ''
	);

	async function enviar(evento: SubmitEvent) {
		evento.preventDefault();
		erro = '';
		enviando = true;
		try {
			sessao.definir(await redefinirSenha(data.token, senha));
			await goto('/painel');
		} catch (e) {
			if (e instanceof ApiError && e.status === 404) {
				venceu = true;
			} else {
				erro = e instanceof ApiError ? e.message : 'não foi possível salvar a senha';
			}
		} finally {
			enviando = false;
		}
	}
</script>

<svelte:head>
	<title>Senha nova · stacktrack</title>
	<!-- O token está na URL: nada desta página vai para buscador nem para o
	     Referer de outro site. -->
	<meta name="robots" content="noindex, nofollow" />
	<meta name="referrer" content="no-referrer" />
</svelte:head>

<div
	class="mx-auto my-auto w-full max-w-sm rounded-lg border border-hairline bg-surface p-7 shadow-flutuante"
>
	{#if data.recuperacao && !venceu}
		<h1 class="text-xl font-semibold tracking-tight text-ink">Criar uma senha nova</h1>
		<p class="mt-1.5 text-sm text-mute">
			Para a conta <b class="text-ink">{data.recuperacao.emailMascarado}</b>. O link vale até as {valeAte}.
		</p>

		<form onsubmit={enviar} class="mt-6 space-y-4">
			<div>
				<label class="rotulo" for="senha">Senha nova</label>
				<input
					id="senha"
					type="password"
					class="campo"
					bind:value={senha}
					required
					autocomplete="new-password"
				/>
				<p class="mt-1.5 text-xs text-mute">
					15 caracteres ou mais — uma frase que só você diria funciona melhor que símbolos.
				</p>
			</div>

			{#if erro}
				<p class="erro-form">{erro}</p>
			{/if}

			<button type="submit" class="botao" disabled={enviando}>
				{enviando ? 'Salvando…' : 'Salvar a senha nova'}
			</button>
		</form>

		<p class="mt-6 text-xs text-mute">
			Ao salvar, a conta sai de todos os aparelhos em que estava aberta, e você entra com a senha
			nova.
		</p>
	{:else}
		<h1 class="text-xl font-semibold tracking-tight text-ink">Este link é inválido ou já venceu</h1>
		<p class="mt-1.5 text-sm text-mute">
			O link de senha nova vale por uma hora e funciona uma vez só. Peça outro e use o email mais
			recente.
		</p>
		<a href="/recuperar-senha" class="botao mt-6 inline-flex justify-center">Pedir outro link</a>
	{/if}
</div>
