<script lang="ts">
	// A marca do stacktrack: monograma "s" recortado num quadrado de canto
	// arredondado, na laranja da paleta.
	//
	// O cabeçalho trazia um `<span>` quadrado com fundo de acento e nada dentro.
	// Num cabeçalho, um bloco de cor sólida sem forma lê-se como imagem que não
	// carregou — o mesmo problema do ícone de arquivar que virou retângulo vazio.
	//
	// DUAS TENTATIVAS FORAM DESCARTADAS, e por um motivo que vale registrar: o
	// tamanho que decide é o menor. Uma pilha de barras com opacidades diferentes
	// e um desenho de colunas com um card atravessando funcionavam a 64px e
	// viravam borrão a 16px — a opacidade some, e as formas finas se fundem. O
	// monograma foi o único que se manteve legível de 16px a 120px.
	//
	// A laranja é fixa nos dois temas (é a marca), e o "s" é recortado na cor do
	// canvas escuro: esse par tem contraste suficiente sobre fundo claro e
	// escuro, o que deixa a mesma arte servir ao favicon estático.
	let { class: classe = 'size-5', comNome = false }: { class?: string; comNome?: boolean } =
		$props();
</script>

{#snippet marca()}
	<svg class={classe} viewBox="0 0 24 24" role="img" aria-label="stacktrack">
		<rect width="24" height="24" rx="6.5" fill="#ef6820" />
		<path
			d="M15.6 8.8c-.9-1.1-2.1-1.6-3.6-1.6-2 0-3.4 1-3.4 2.4 0 1.5 1.4 2 3.4 2.4 2 .4 3.4 1 3.4 2.5 0 1.5-1.5 2.5-3.5 2.5-1.5 0-2.8-.6-3.6-1.7"
			fill="none"
			stroke="#0c111d"
			stroke-width="2.2"
			stroke-linecap="round"
		/>
	</svg>
{/snippet}

{#if comNome}
	<span class="flex items-center gap-2 text-sm font-semibold tracking-tight text-ink">
		{@render marca()}
		stacktrack
	</span>
{:else}
	{@render marca()}
{/if}
