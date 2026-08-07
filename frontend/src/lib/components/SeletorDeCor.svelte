<script lang="ts">
	// Escolha de cor para coluna e card. Um botão por cor da paleta, mais um
	// para "sem cor" — sem cor é um estado legítimo, e não a ausência de
	// escolha: card e coluna nascem assim.
	import { CORES, type Cor } from '$lib/api/boards';

	let {
		cor = $bindable(),
		rotulo = 'Cor',
		aoEscolher
	}: {
		cor: Cor | '';
		rotulo?: string;
		// Quem já tem o recurso salvo passa aoEscolher e grava no clique; quem
		// está montando um formulário usa bind:cor e grava no submit.
		aoEscolher?: (nova: Cor | '') => void;
	} = $props();

	function escolher(nova: Cor | '') {
		cor = nova;
		aoEscolher?.(nova);
	}
</script>

<div class="flex flex-wrap items-center gap-1" role="group" aria-label={rotulo}>
	<button
		type="button"
		onclick={() => escolher('')}
		class="size-5 rounded-full border border-dashed border-hairline-strong text-[9px] leading-none text-mute {cor ===
		''
			? 'ring-2 ring-accent ring-offset-1 ring-offset-surface'
			: ''}"
		aria-label="Sem cor"
		aria-pressed={cor === ''}
		title="Sem cor"
	>
		—
	</button>

	{#each CORES as opcao (opcao)}
		<button
			type="button"
			onclick={() => escolher(opcao)}
			class="size-5 rounded-full cor-{opcao} {cor === opcao
				? 'ring-2 ring-accent ring-offset-1 ring-offset-surface'
				: 'opacity-70 hover:opacity-100'}"
			style="background-color: var(--etq-texto)"
			aria-label={opcao}
			aria-pressed={cor === opcao}
			title={opcao}
		></button>
	{/each}
</div>
