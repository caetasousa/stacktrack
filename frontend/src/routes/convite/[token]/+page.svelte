<script lang="ts">
	import { goto } from '$app/navigation';
	import { aceitarConvite } from '$lib/api/membros';
	import { ApiError } from '$lib/api/client';
	import { mascararEmail } from '$lib/email';
	import { sessao } from '$lib/stores/session.svelte';

	let { data } = $props();

	let erro = $state('');
	let aceitando = $state(false);

	// Três situações diferentes, e cada uma pede uma ação diferente: sem conta,
	// logado com a conta certa, e logado com outra conta.
	const logado = $derived(data.usuario !== null);
	// A comparação é entre MÁSCARAS: a rota pública do convite não devolve mais
	// o endereço inteiro. É uma dica de tela, não uma autorização — duas contas
	// do mesmo domínio com a mesma inicial produzem a mesma máscara, e quem
	// decide de verdade é o servidor no aceite, que responde 403 quando o email
	// da sessão não é o do convite.
	const emailConfere = $derived(
		data.usuario ? mascararEmail(data.usuario.email) === data.convite.emailMascarado : false
	);

	// Para onde voltar depois de entrar ou criar conta, e para qual email o
	// convite é. O `para` viaja MASCARADO: é o mesmo que esta página pública já
	// mostra, e serve ao cadastro e ao login para dizer qual conta usar.
	const voltar = $derived(`/convite/${encodeURIComponent(data.token)}`);
	const consulta = $derived(
		`voltar=${encodeURIComponent(voltar)}&para=${encodeURIComponent(data.convite.emailMascarado)}`
	);

	const valeAte = $derived(
		new Date(data.convite.expiraEm).toLocaleString('pt-BR', {
			day: '2-digit',
			month: 'long',
			hour: '2-digit',
			minute: '2-digit'
		})
	);

	let saindo = $state(false);

	// Logado na conta errada: sair e voltar ao login JÁ apontando para este
	// convite. Antes a tela só oferecia "ir para o painel", e a pessoa precisava
	// descobrir sozinha como trocar de conta sem perder o link.
	async function trocarDeConta() {
		saindo = true;
		try {
			await sessao.encerrar();
			await goto(`/login?${consulta}`);
		} finally {
			saindo = false;
		}
	}

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

	<dl class="mt-5 space-y-2 rounded-md border border-hairline bg-surface-elevated p-4 text-xs">
		<div class="flex justify-between gap-3">
			<dt class="text-mute">Convite para</dt>
			<dd class="truncate font-medium text-ink">{data.convite.emailMascarado}</dd>
		</div>
		<div class="flex justify-between gap-3">
			<dt class="text-mute">Vale até</dt>
			<dd class="font-medium text-ink">{valeAte}</dd>
		</div>
	</dl>

	{#if erro}
		<p class="erro-form mt-4">{erro}</p>
	{/if}

	{#if !logado}
		<p class="mt-5 text-sm text-mute">
			Para aceitar, crie uma conta ou entre com <b class="text-ink">{data.convite.emailMascarado}</b
			>
			— o convite é desse email. Depois você volta para esta tela.
		</p>
		<div class="mt-4 flex flex-wrap gap-2">
			<a href="/cadastro?{consulta}" class="botao w-auto px-5">Criar conta</a>
			<a href="/login?{consulta}" class="botao-secundario w-auto px-5">Já tenho conta</a>
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
			<b class="text-ink">{data.convite.emailMascarado}</b>. Para aceitar, entre com a conta desse
			email.
		</p>
		<div class="mt-4 flex flex-wrap gap-2">
			<button class="botao w-auto px-5" onclick={trocarDeConta} disabled={saindo}>
				{saindo ? 'Saindo…' : 'Sair e entrar com outra conta'}
			</button>
			<a href="/painel" class="botao-secundario w-auto px-5">Ir para o painel</a>
		</div>
	{/if}
</div>
