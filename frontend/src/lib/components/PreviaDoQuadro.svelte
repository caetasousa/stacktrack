<script lang="ts">
	// A prévia do quadro na página inicial: uma ilustração ESTÁTICA, montada com
	// os mesmos tokens da interface de verdade.
	//
	// Ela ocupa o lugar que era do painel de infraestrutura (`/health`, `/ready`,
	// "6 migrations aplicadas"). Aquilo era diagnóstico de quem opera o sistema,
	// não resposta à pergunta de quem chega na página: "o que é isto?". Mostrar o
	// produto responde melhor que descrevê-lo.
	//
	// Não é interativa, e não finge ser: `aria-hidden` a esconde de quem navega
	// por leitor de tela, porque o texto ao lado já diz tudo o que ela ilustra —
	// anunciar cards falsos seria ruído.
	const colunas = [
		{
			titulo: 'A fazer',
			cor: 'var(--mute)',
			cards: [
				{ titulo: 'Revisar contrato de API', etiqueta: null, selos: '' },
				{ titulo: 'Ajustar webhook de cobrança', etiqueta: 'urgente', selos: '2' }
			]
		},
		{
			titulo: 'Em andamento',
			cor: 'var(--aviso)',
			cards: [{ titulo: 'Migrar autenticação', etiqueta: 'backend', selos: '4' }]
		},
		{
			titulo: 'Pronto',
			cor: 'var(--positivo)',
			cards: [{ titulo: 'Padronizar erros da API', etiqueta: null, selos: '' }]
		}
	];
</script>

<div
	class="overflow-hidden rounded-lg border border-hairline bg-surface shadow-flutuante"
	aria-hidden="true"
>
	<!-- Barra do quadro: título à esquerda, quem está junto à direita. É a
	     presença que traduz "colaborativo" sem precisar da palavra. -->
	<div class="flex items-center justify-between gap-3 border-b border-hairline px-4 py-3">
		<div class="flex items-center gap-2">
			<span class="text-sm font-semibold text-ink">Migração do checkout</span>
			<span class="chip chip-neutro text-[0.6875rem]">3 colunas</span>
		</div>
		<div class="flex items-center gap-2">
			<span class="hidden text-[0.6875rem] text-mute sm:inline">3 pessoas agora</span>
			<div class="flex -space-x-1.5">
				{#each ['AM', 'BR', 'CS'] as iniciais (iniciais)}
					<span
						class="flex size-6 items-center justify-center rounded-full border border-surface bg-accent-suave text-[0.625rem] font-semibold text-accent-texto"
					>
						{iniciais}
					</span>
				{/each}
			</div>
		</div>
	</div>

	<div class="flex gap-3 overflow-hidden p-4">
		{#each colunas as coluna (coluna.titulo)}
			<div class="flex min-w-0 flex-1 flex-col gap-2 rounded-md bg-canvas p-2.5">
				<div class="flex items-center gap-2 px-0.5">
					<i class="size-1.5 rounded-full" style="background: {coluna.cor}"></i>
					<span class="truncate text-xs font-medium text-body">{coluna.titulo}</span>
					<span class="ml-auto text-[0.6875rem] tabular-nums text-mute">{coluna.cards.length}</span>
				</div>

				{#each coluna.cards as card (card.titulo)}
					<div class="rounded-sm border border-hairline bg-surface p-2.5 shadow-ficha">
						{#if card.etiqueta}
							<span
								class="mb-1.5 inline-block rounded-xs bg-accent-suave px-1.5 py-px text-[0.5625rem] font-medium text-accent-texto"
							>
								{card.etiqueta}
							</span>
						{/if}
						<p class="text-[0.6875rem] leading-snug text-body">{card.titulo}</p>
						{#if card.selos}
							<div class="mt-1.5 flex items-center gap-1 text-[0.5625rem] text-mute">
								<svg
									class="size-2.5"
									viewBox="0 0 24 24"
									fill="none"
									stroke="currentColor"
									stroke-width="2"
								>
									<path d="M4 6a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H9l-5 4z" />
								</svg>
								{card.selos}
							</div>
						{/if}
					</div>
				{/each}
			</div>
		{/each}
	</div>
</div>
