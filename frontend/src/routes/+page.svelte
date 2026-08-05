<script lang="ts">
	import { onMount } from 'svelte';
	import { ApiError } from '$lib/api/client';
	import { verificarProcesso, verificarProntidao } from '$lib/api/saude';

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
</script>

<svelte:head><title>kanbanGo</title></svelte:head>

<h1 class="text-3xl font-semibold tracking-tight text-ink">kanbanGo</h1>
<p class="mt-3 max-w-xl text-sm leading-relaxed text-mute">
	Quadro Kanban colaborativo em tempo real. A fundação está de pé: Postgres, migrations, API em Go e
	este frontend. O quadro em si começa na fase 2 — o tempo real, na 5.
</p>

<section class="mt-10 rounded-lg border border-hairline bg-surface p-6">
	<h2 class="text-xs font-semibold tracking-widest text-mute uppercase">Infraestrutura</h2>

	<dl class="mt-4 space-y-3 text-sm">
		<div class="flex items-center justify-between border-b border-hairline pb-3">
			<dt>
				API <span class="text-mute">(/health)</span>
			</dt>
			<dd class={cor(processo)}>{rotulos[processo]}</dd>
		</div>
		<div class="flex items-center justify-between">
			<dt>
				Banco <span class="text-mute">(/ready)</span>
			</dt>
			<dd class={cor(prontidao)}>{rotulos[prontidao]}</dd>
		</div>
	</dl>

	{#if detalhe}
		<p class="mt-4 text-xs text-negativo">{detalhe}</p>
	{/if}
</section>
