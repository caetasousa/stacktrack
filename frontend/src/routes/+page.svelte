<script lang="ts">
	import { onMount } from 'svelte';
	import { ApiError } from '$lib/api/client';
	import { verificarProcesso, verificarProntidao } from '$lib/api/saude';
	import { sessao } from '$lib/stores/session.svelte';

	type Estado = 'checando' | 'ok' | 'falha';

	let processo = $state<Estado>('checando');
	let prontidao = $state<Estado>('checando');
	let detalhe = $state('');

	// A checagem roda no navegador, não no SSR: dentro do container `web`,
	// localhost:8080 seria o próprio container, não a API.
	onMount(async () => {
		try {
			await verificarProcesso();
			processo = 'ok';
		} catch (e) {
			processo = 'falha';
			detalhe = e instanceof Error ? e.message : String(e);
			return;
		}

		try {
			await verificarProntidao();
			prontidao = 'ok';
		} catch (e) {
			prontidao = 'falha';
			detalhe = e instanceof ApiError ? e.message : 'não foi possível checar o banco';
		}
	});

	const rotulos: Record<Estado, string> = {
		checando: 'checando…',
		ok: 'no ar',
		falha: 'fora do ar'
	};

	function cor(estado: Estado) {
		if (estado === 'ok') return 'text-positivo';
		if (estado === 'falha') return 'text-negativo';
		return 'text-mute';
	}

	// O que já existe é afirmação; o que não existe leva o selo da fase em
	// que chega. Prometer na home o que o produto não faz é a forma mais
	// barata de perder a confiança de quem testa.
	const recursos = [
		{
			titulo: 'Quadros e colunas',
			texto: 'Monte o fluxo do jeito que a equipe trabalha, com as etapas que fizerem sentido.',
			fase: null
		},
		{
			titulo: 'Papéis por quadro',
			texto: 'Dono, editor e leitor — checados no servidor, em toda operação.',
			fase: null
		},
		{
			titulo: 'Sessão segura',
			texto: 'Cookie HttpOnly, senha em Argon2id e o token guardado só como hash.',
			fase: null
		},
		{
			titulo: 'Arrastar e soltar',
			texto: 'Ordenação fracionária: mover um card escreve uma linha, não renumera a coluna.',
			fase: 'fase 4'
		},
		{
			titulo: 'Tempo real',
			texto: 'WebSocket: você arrasta aqui e o card se move na tela de quem estiver junto.',
			fase: 'fase 5'
		},
		{
			titulo: 'Presença',
			texto: 'Quem está com o quadro aberto agora, e quem está editando qual card.',
			fase: 'fase 6'
		}
	];
</script>

<svelte:head><title>stacktrack</title></svelte:head>

<section class="flex flex-col items-center gap-5 py-10 text-center">
	<span
		class="inline-flex items-center gap-2 rounded-full border border-hairline-strong bg-surface py-1 pr-3 pl-1 text-xs"
	>
		<em class="rounded-full bg-accent-suave px-2 py-0.5 font-semibold text-accent-texto not-italic">
			Fase 2
		</em>
		quadro, colunas e cards
	</span>

	<h1 class="max-w-2xl text-4xl font-semibold tracking-tight text-balance text-ink sm:text-5xl">
		O quadro que todo mundo vê mudar na hora
	</h1>

	<p class="max-w-xl text-sm leading-relaxed text-mute">
		Kanban colaborativo escrito em Go e SvelteKit. Projeto de estudo, código aberto, sem conta paga
		— o tempo real chega na fase 5.
	</p>

	<div class="mt-1 flex flex-wrap justify-center gap-3">
		{#if sessao.usuario}
			<a href="/painel" class="botao w-auto px-5">Ir para o painel</a>
		{:else}
			<a href="/cadastro" class="botao w-auto px-5">Criar conta</a>
			<a href="/login" class="botao-secundario w-auto px-5">Entrar</a>
		{/if}
	</div>

	<p class="flex flex-wrap justify-center gap-2 text-xs text-mute">
		<span>Grátis para sempre</span><span aria-hidden="true">·</span><span>Código aberto</span><span
			aria-hidden="true">·</span
		><span>Sem cartão</span>
	</p>
</section>

<section
	class="grid gap-px overflow-hidden rounded-lg border border-hairline bg-hairline sm:grid-cols-3"
	aria-label="Estado da infraestrutura"
>
	<div class="bg-surface p-4">
		<b class="block text-lg font-semibold tracking-tight {cor(processo)}">{rotulos[processo]}</b>
		<span class="text-xs text-mute">API · /health</span>
	</div>
	<div class="bg-surface p-4">
		<b class="block text-lg font-semibold tracking-tight {cor(prontidao)}">{rotulos[prontidao]}</b>
		<span class="text-xs text-mute">Banco · /ready</span>
	</div>
	<div class="bg-surface p-4">
		<b class="block text-lg font-semibold tracking-tight text-ink">6</b>
		<span class="text-xs text-mute">migrations aplicadas</span>
	</div>
</section>

{#if detalhe}
	<p class="mt-4 text-xs text-negativo">{detalhe}</p>
{/if}

<section class="mt-14">
	<h2 class="text-xs font-semibold tracking-widest text-mute uppercase">O que já dá para fazer</h2>

	<ul class="mt-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
		{#each recursos as recurso (recurso.titulo)}
			<li class="rounded-lg border border-hairline bg-surface p-5">
				<span class="mb-3 block size-8 rounded-md bg-accent-suave"></span>
				<b class="flex flex-wrap items-center gap-2 text-sm font-semibold text-ink">
					{recurso.titulo}
					{#if recurso.fase}
						<span class="chip chip-neutro text-[0.6875rem]">{recurso.fase}</span>
					{/if}
				</b>
				<small class="mt-1.5 block text-xs leading-relaxed text-mute">{recurso.texto}</small>
			</li>
		{/each}
	</ul>
</section>
