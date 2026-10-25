<script lang="ts">
	// O sino do cabeçalho: o número de avisos que pedem atenção e, ao abrir, a
	// lista deles. Quem mantém o número em dia é o stream de tempo real
	// ($lib/stores/notificacoes); este componente só mostra e reage ao clique.
	import { haQuanto } from '$lib/atividade';
	import { destinoDe, fraseDe, motivoDoEncerramento } from '$lib/notificacoes';
	import { agoraReativo } from '$lib/relogio.svelte';
	import { sino } from '$lib/stores/notificacoes.svelte';
	import type { Notificacao } from '$lib/api/notificacoes';

	let raiz = $state<HTMLElement | null>(null);
	let erro = $state('');

	const rotulo = $derived(
		sino.naoLidas === 0
			? 'Notificações'
			: `Notificações, ${sino.naoLidas} ${sino.naoLidas === 1 ? 'não lida' : 'não lidas'}`
	);

	async function alternar() {
		sino.aberto = !sino.aberto;
		if (sino.aberto && !sino.carregada) await recarregar();
	}

	async function recarregar() {
		erro = '';
		try {
			await sino.carregar();
		} catch {
			erro = 'não foi possível carregar as notificações';
		}
	}

	// O clique segue o link na hora; marcar como lido vai em paralelo, e uma
	// falha nisso não pode prender a pessoa no menu.
	function abrir(n: Notificacao) {
		sino.aberto = false;
		void sino.marcarLida(n).catch(() => {});
	}

	async function marcarTodas() {
		try {
			await sino.marcarTodas();
		} catch {
			erro = 'não foi possível marcar as notificações como lidas';
		}
	}

	async function carregarMais() {
		try {
			await sino.carregarMais();
		} catch {
			erro = 'não foi possível carregar mais notificações';
		}
	}

	// Fora do menu, ou Esc, fecha — como qualquer menu que abre por cima.
	function aoApertar(e: PointerEvent) {
		if (sino.aberto && raiz && !raiz.contains(e.target as Node)) sino.aberto = false;
	}
	function aoTeclar(e: KeyboardEvent) {
		if (sino.aberto && e.key === 'Escape') sino.aberto = false;
	}
</script>

<svelte:window onpointerdown={aoApertar} onkeydown={aoTeclar} />

<div class="relative" bind:this={raiz}>
	<button
		type="button"
		onclick={alternar}
		class="relative cursor-pointer rounded-sm p-1.5 text-mute hover:bg-surface-elevated hover:text-ink"
		aria-label={rotulo}
		title={rotulo}
		aria-haspopup="dialog"
		aria-expanded={sino.aberto}
	>
		<svg
			width="16"
			height="16"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			stroke-width="2"
			stroke-linecap="round"
			stroke-linejoin="round"
			aria-hidden="true"
		>
			<path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9" />
			<path d="M10.3 21a1.94 1.94 0 0 0 3.4 0" />
		</svg>
		{#if sino.naoLidas > 0}
			<!-- O laranja escuro do .botao, fixo nos dois temas: com texto branco é
			     o que passa em contraste AA — o acento da paleta não passa. -->
			<span
				class="absolute -top-0.5 -right-0.5 flex min-w-4 items-center justify-center rounded-full bg-[#c4410e] px-1 text-[10px] leading-4 font-semibold text-white tabular-nums"
				aria-hidden="true"
			>
				{sino.naoLidas > 9 ? '9+' : sino.naoLidas}
			</span>
		{/if}
	</button>

	{#if sino.aberto}
		<div
			role="dialog"
			aria-label="Notificações"
			class="absolute top-full right-0 z-30 mt-2 flex max-h-[70vh] w-[22rem] max-w-[calc(100vw-2rem)] flex-col rounded-lg border border-hairline bg-surface shadow-flutuante"
		>
			<div class="flex items-center justify-between gap-2 border-b border-hairline px-4 py-2.5">
				<h2 class="text-sm font-semibold text-ink">Notificações</h2>
				{#if sino.naoLidas > 0}
					<button
						type="button"
						class="cursor-pointer text-xs text-mute hover:text-ink"
						onclick={marcarTodas}
					>
						Marcar todas como lidas
					</button>
				{/if}
			</div>

			{#if erro}
				<p class="px-4 py-2 text-xs text-negativo">{erro}</p>
			{/if}

			<div class="overflow-y-auto">
				{#if !sino.carregada && sino.carregando}
					<p class="px-4 py-6 text-center text-xs text-mute">carregando…</p>
				{:else if sino.itens.length === 0}
					<p class="px-4 py-6 text-center text-xs text-mute">
						Nada por aqui. Convites, cards que passarem a ser seus, menções e prazos chegando
						aparecem aqui.
					</p>
				{:else}
					<ul class="divide-y divide-hairline">
						{#each sino.itens as n (n.id)}
							{@const frase = fraseDe(n, agoraReativo())}
							{@const motivo = motivoDoEncerramento(n)}
							<li>
								<a
									href={destinoDe(n)}
									onclick={() => abrir(n)}
									class="flex gap-2.5 px-4 py-3 text-sm transition-colors hover:bg-surface-elevated"
									class:opacity-60={motivo !== ''}
								>
									<span
										class="mt-1.5 size-2 shrink-0 rounded-full {!n.lida && !motivo
											? 'bg-accent'
											: 'bg-transparent'}"
										aria-hidden="true"
									></span>
									<span class="min-w-0 flex-1">
										<span class="block text-body">
											{#if frase.sujeito}<b class="font-semibold text-ink">{frase.sujeito}</b>
											{/if}{frase.texto}
										</span>
										{#if n.trecho}
											<span class="mt-0.5 block truncate text-xs text-mute">“{n.trecho}”</span>
										{/if}
										{#if motivo}
											<span class="mt-0.5 block text-xs text-mute">Não vale mais: {motivo}.</span>
										{/if}
										<span class="mt-0.5 block text-xs text-mute">
											{#if n.tipo !== 'convite'}{n.quadro} ·
											{/if}<span title={new Date(n.criadaEm).toLocaleString('pt-BR')}
												>{haQuanto(n.criadaEm, agoraReativo())}</span
											>
											{#if !n.lida && !motivo}<span class="sr-only"> · não lida</span>{/if}
										</span>
									</span>
								</a>
							</li>
						{/each}
					</ul>
					{#if sino.temMais}
						<button
							type="button"
							class="w-full cursor-pointer border-t border-hairline px-4 py-2.5 text-xs text-mute hover:text-ink"
							onclick={carregarMais}
						>
							Carregar mais
						</button>
					{/if}
				{/if}
			</div>
		</div>
	{/if}
</div>
