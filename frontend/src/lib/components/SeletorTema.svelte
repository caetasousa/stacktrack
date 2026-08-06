<script lang="ts">
	// Botão de troca de tema. Mostra o ícone do tema que será aplicado no
	// clique (sol quando está escuro), que é a convenção que as pessoas
	// leem como "ir para" e não como "estado atual".
	import { tema } from '$lib/stores/tema.svelte';

	const proximo = $derived(tema.atual === 'dark' ? 'claro' : 'escuro');
</script>

<button
	type="button"
	onclick={() => tema.alternar()}
	class="cursor-pointer rounded-sm p-1.5 text-mute hover:bg-surface-elevated hover:text-ink"
	aria-label="Mudar para o tema {proximo}"
	title="Mudar para o tema {proximo}"
>
	{#if tema.atual === 'dark'}
		<!-- sol -->
		<svg
			width="16"
			height="16"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			stroke-width="2"
			stroke-linecap="round"
			aria-hidden="true"
		>
			<circle cx="12" cy="12" r="4" />
			<path
				d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41"
			/>
		</svg>
	{:else}
		<!-- lua -->
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
			<path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
		</svg>
	{/if}
</button>
