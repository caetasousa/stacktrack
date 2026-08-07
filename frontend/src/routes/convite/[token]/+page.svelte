<script lang="ts">
	import { goto } from '$app/navigation';
	import { aceitarConvite } from '$lib/api/membros';
	import { ApiError } from '$lib/api/client';

	let { data } = $props();

	let erro = $state('');
	let aceitando = $state(false);

	// Três situações diferentes, e cada uma pede uma ação diferente: sem conta,
	// logado com a conta certa, e logado com outra conta.
	const logado = $derived(data.usuario !== null);
	const emailConfere = $derived(data.usuario?.email === data.convite.email);

	// Para onde voltar depois de entrar ou criar conta.
	const voltar = $derived(`/convite/${encodeURIComponent(data.token)}`);

	const explicacaoDoPapel: Record<string, string> = {
		dono: 'poder total no quadro, inclusive apagá-lo',
		editor: 'criar e mexer em colunas e cards',
		leitor: 'ver o quadro, sem alterar nada'
	};

	async function aceitar() {
		erro = '';
		aceitando = true;
		try {
			const quadro = await aceitarConvite(data.token);
			await goto(`/painel/quadros/${quadro.id}`);
		} catch (e) {
			erro = e instanceof ApiError ? e.message : 'não foi possível aceitar o convite';
		} finally {
			aceitando = false;
		}
	}
</script>

<svelte:head><title>Convite · stacktrack</title></svelte:head>

<div
	class="mx-auto w-full max-w-md rounded-lg border border-hairline bg-surface p-7 shadow-flutuante"
>
	<span class="chip">convite</span>

	<h1 class="mt-4 text-xl font-semibold tracking-tight text-ink">
		{data.convite.convidadoPor
			? `${data.convite.convidadoPor} convidou você`
			: 'Você foi convidado'}
	</h1>
	<p class="mt-2 text-sm text-mute">
		para participar do quadro <b class="font-semibold text-ink">{data.convite.quadro}</b> como
		<b class="font-semibold text-ink">{data.convite.papel}</b> — {explicacaoDoPapel[
			data.convite.papel
		]}.
	</p>

	<dl class="mt-5 rounded-md border border-hairline bg-surface-elevated p-4 text-xs">
		<div class="flex justify-between gap-3">
			<dt class="text-mute">Convite para</dt>
			<dd class="truncate font-medium text-ink">{data.convite.email}</dd>
		</div>
	</dl>

	{#if erro}
		<p class="erro-form mt-4">{erro}</p>
	{/if}

	{#if !logado}
		<p class="mt-5 text-sm text-mute">
			Entre com <b class="text-ink">{data.convite.email}</b> para aceitar. Se ainda não tem conta, crie
			uma com esse email.
		</p>
		<div class="mt-4 flex flex-wrap gap-2">
			<a href="/cadastro?voltar={encodeURIComponent(voltar)}" class="botao w-auto px-5">
				Criar conta
			</a>
			<a href="/login?voltar={encodeURIComponent(voltar)}" class="botao-secundario w-auto px-5">
				Entrar
			</a>
		</div>
	{:else if emailConfere}
		<button class="botao mt-5" onclick={aceitar} disabled={aceitando}>
			{aceitando ? 'Entrando no quadro…' : 'Aceitar convite'}
		</button>
	{:else}
		<!-- O convite é amarrado ao email: sem isso, um link encaminhado
		     colocaria qualquer pessoa dentro do quadro. -->
		<p class="mt-5 text-sm text-mute">
			Você está com a conta <b class="text-ink">{data.usuario?.email}</b>, e este convite é para
			<b class="text-ink">{data.convite.email}</b>. Saia e entre com a conta convidada para aceitar.
		</p>
		<a href="/painel" class="botao-secundario mt-4 inline-block w-auto px-5">Ir para o painel</a>
	{/if}
</div>
