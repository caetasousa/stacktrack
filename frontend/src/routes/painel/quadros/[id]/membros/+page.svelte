<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { ApiError } from '$lib/api/client';
	import type { Papel } from '$lib/api/boards';
	import {
		alterarPapel,
		convidar,
		removerMembro,
		revogarConvite,
		type ConviteCriado
	} from '$lib/api/membros';

	let { data } = $props();

	let email = $state('');
	let papel = $state<Papel>('editor');
	let erro = $state('');
	let enviando = $state(false);
	// O link do convite aparece uma vez só: a API devolve o token na resposta
	// que o criou e o banco guarda apenas o hash. Quem fechar esta tela sem
	// copiar precisa revogar e convidar de novo.
	let recemCriado = $state<ConviteCriado | null>(null);
	let copiado = $state(false);

	const souDono = $derived(data.quadro.papel === 'dono');

	const papeis: { valor: Papel; rotulo: string; explica: string }[] = [
		{ valor: 'dono', rotulo: 'Dono', explica: 'pode tudo, inclusive apagar o quadro' },
		{ valor: 'editor', rotulo: 'Editor', explica: 'mexe em colunas e cards' },
		{ valor: 'leitor', rotulo: 'Leitor', explica: 'só enxerga' }
	];

	async function recarregar() {
		erro = '';
		await invalidateAll();
	}

	function falhar(e: unknown, padrao: string) {
		erro = e instanceof ApiError ? e.message : padrao;
	}

	async function enviarConvite(evento: SubmitEvent) {
		evento.preventDefault();
		erro = '';
		enviando = true;
		copiado = false;
		try {
			const resultado = await convidar(data.quadro.id, email, papel);
			email = '';
			recemCriado = resultado.adicionado ? null : resultado;
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível convidar');
		} finally {
			enviando = false;
		}
	}

	async function copiarLink(link: string) {
		try {
			await navigator.clipboard.writeText(link);
			copiado = true;
		} catch {
			// Sem permissão de área de transferência: o link segue visível na
			// tela para copiar à mão, então não é erro que valha interromper.
			copiado = false;
		}
	}

	async function trocarPapel(usuarioId: string, novo: Papel) {
		try {
			await alterarPapel(data.quadro.id, usuarioId, novo);
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível trocar o papel');
			await recarregar();
		}
	}

	async function remover(usuarioId: string, nome: string) {
		if (!confirm(`Remover ${nome} do quadro?`)) return;
		try {
			await removerMembro(data.quadro.id, usuarioId);
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível remover');
		}
	}

	async function revogar(conviteId: string, alvo: string) {
		if (!confirm(`Revogar o convite de ${alvo}? O link deixa de funcionar.`)) return;
		try {
			await revogarConvite(data.quadro.id, conviteId);
			recemCriado = null;
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível revogar o convite');
		}
	}

	const formatarData = (iso: string) =>
		new Date(iso).toLocaleDateString('pt-BR', { day: '2-digit', month: 'short' });
</script>

<svelte:head><title>Membros · {data.quadro.titulo} · stacktrack</title></svelte:head>

<div class="flex flex-wrap items-baseline justify-between gap-3">
	<div>
		<h1 class="text-xl font-semibold tracking-tight text-ink">Membros</h1>
		<p class="mt-0.5 text-sm text-mute">{data.quadro.titulo}</p>
	</div>
	<a href="/painel/quadros/{data.quadro.id}" class="text-xs text-mute hover:text-ink">
		Voltar ao quadro
	</a>
</div>

{#if erro}
	<p class="erro-form mt-5">{erro}</p>
{/if}

{#if souDono}
	<section class="mt-6 rounded-lg border border-hairline bg-surface p-5 shadow-ficha">
		<h2 class="text-sm font-semibold text-ink">Convidar alguém</h2>
		<p class="mt-1 text-xs text-mute">
			Quem já tem conta entra na hora. Quem não tem recebe um link para você enviar.
		</p>

		<form onsubmit={enviarConvite} class="mt-4 flex flex-wrap gap-2">
			<input
				type="email"
				class="campo min-w-0 flex-1"
				bind:value={email}
				placeholder="email@exemplo.com"
				required
				maxlength="255"
				aria-label="Email de quem você quer convidar"
			/>
			<select class="campo w-auto shrink-0" bind:value={papel} aria-label="Papel">
				{#each papeis as opcao (opcao.valor)}
					<option value={opcao.valor}>{opcao.rotulo}</option>
				{/each}
			</select>
			<button type="submit" class="botao w-auto shrink-0 px-5" disabled={enviando}>
				{enviando ? 'Convidando…' : 'Convidar'}
			</button>
		</form>

		<p class="mt-2 text-xs text-mute">
			{papeis.find((p) => p.valor === papel)?.explica}
		</p>

		{#if recemCriado?.link}
			<div class="mt-4 rounded-md border border-accent bg-accent-suave p-4">
				<p class="text-sm font-semibold text-accent-texto">
					Convite criado para {recemCriado.convite?.email}
				</p>
				<p class="mt-1 text-xs text-body">
					Copie o link agora — por segurança ele não é mostrado de novo. Se perder, revogue o
					convite e faça outro.
				</p>
				<div class="mt-3 flex flex-wrap items-center gap-2">
					<code
						class="min-w-0 flex-1 overflow-x-auto rounded-sm border border-hairline-strong bg-surface px-2 py-1.5 text-xs whitespace-nowrap text-body"
					>
						{recemCriado.link}
					</code>
					<button
						type="button"
						class="botao-secundario w-auto shrink-0 px-4 py-1.5 text-xs"
						onclick={() => copiarLink(recemCriado!.link!)}
					>
						{copiado ? 'Copiado' : 'Copiar'}
					</button>
				</div>
			</div>
		{/if}
	</section>
{/if}

<section class="mt-8">
	<h2 class="text-xs font-semibold tracking-widest text-mute uppercase">
		Quem participa ({data.participacao.membros.length})
	</h2>

	<ul
		class="mt-3 divide-y divide-hairline overflow-hidden rounded-lg border border-hairline bg-surface"
	>
		{#each data.participacao.membros as membro (membro.usuarioId)}
			<li class="flex flex-wrap items-center justify-between gap-3 p-4">
				<div class="min-w-0">
					<b class="block text-sm font-medium text-ink">
						{membro.nome}
						{#if membro.usuarioId === data.usuarioId}
							<span class="text-xs font-normal text-mute">(você)</span>
						{/if}
					</b>
					<span class="block truncate text-xs text-mute">
						{membro.email} · desde {formatarData(membro.desdeEm)}
					</span>
				</div>

				<div class="flex shrink-0 items-center gap-3">
					{#if souDono}
						<select
							class="campo w-auto py-1 text-xs"
							value={membro.papel}
							aria-label="Papel de {membro.nome}"
							onchange={(e) => trocarPapel(membro.usuarioId, e.currentTarget.value as Papel)}
						>
							{#each papeis as opcao (opcao.valor)}
								<option value={opcao.valor}>{opcao.rotulo}</option>
							{/each}
						</select>
						<button
							class="cursor-pointer text-xs text-mute hover:text-negativo"
							onclick={() => remover(membro.usuarioId, membro.nome)}
						>
							Remover
						</button>
					{:else}
						<span class="chip" class:chip-neutro={membro.papel !== 'dono'}>{membro.papel}</span>
					{/if}
				</div>
			</li>
		{/each}
	</ul>
</section>

{#if souDono && data.participacao.convites.length > 0}
	<section class="mt-8">
		<h2 class="text-xs font-semibold tracking-widest text-mute uppercase">
			Convites pendentes ({data.participacao.convites.length})
		</h2>

		<ul
			class="mt-3 divide-y divide-hairline overflow-hidden rounded-lg border border-hairline bg-surface"
		>
			{#each data.participacao.convites as convite (convite.id)}
				<li class="flex flex-wrap items-center justify-between gap-3 p-4">
					<div class="min-w-0">
						<b class="block truncate text-sm font-medium text-ink">{convite.email}</b>
						<span class="block text-xs text-mute">
							{convite.papel} ·
							{#if convite.expirado}
								<span class="text-negativo">expirou</span>
							{:else}
								vale até {formatarData(convite.expiraEm)}
							{/if}
						</span>
					</div>
					<button
						class="shrink-0 cursor-pointer text-xs text-mute hover:text-negativo"
						onclick={() => revogar(convite.id, convite.email)}
					>
						Revogar
					</button>
				</li>
			{/each}
		</ul>
	</section>
{/if}
