<script lang="ts">
	// Um card na coluna. Mostra o RESUMO do que carrega — barras de etiqueta,
	// prazo, progresso de checklist e contagem de anexos — e abre o modal no
	// clique, que é onde tudo isso pode ser mexido.
	import { apagarCard, type Card, type Etiqueta } from '$lib/api/boards';
	import { ApiError } from '$lib/api/client';
	import { cliqueSemArraste } from '$lib/arrastar';

	let {
		card,
		etiquetasDoQuadro,
		podeEditar,
		aoAbrirCard,
		aoMudar,
		aoFalhar
	}: {
		card: Card;
		etiquetasDoQuadro: Etiqueta[];
		podeEditar: boolean;
		aoAbrirCard: (cardId: string) => void;
		aoMudar: () => Promise<void>;
		aoFalhar: (mensagem: string) => void;
	} = $props();

	// O card traz só os ids; os dados da etiqueta vêm uma vez, com o quadro.
	const etiquetas = $derived(
		card.etiquetas
			.map((id) => etiquetasDoQuadro.find((e) => e.id === id))
			.filter((e): e is Etiqueta => e !== undefined)
	);

	const temChecklist = $derived(card.checklist.total > 0);
	const checklistCompleta = $derived(
		temChecklist && card.checklist.concluidos === card.checklist.total
	);

	// Só dia e mês: o ano só interessa quando não é este, e isso é raro num
	// quadro de trabalho.
	const prazoCurto = $derived(
		card.prazo
			? new Date(card.prazo).toLocaleDateString('pt-BR', { day: '2-digit', month: 'short' })
			: ''
	);

	// O card é clicável E arrastável. Sem o limiar de distância, soltar depois
	// de arrastar abriria o modal em cima do movimento que a pessoa fez.
	const clique = cliqueSemArraste(() => aoAbrirCard(card.id));

	async function apagar(evento: MouseEvent) {
		evento.stopPropagation();
		if (!confirm(`Apagar o card "${card.titulo}"?`)) return;
		try {
			await apagarCard(card.id);
			await aoMudar();
		} catch (e) {
			aoFalhar(e instanceof ApiError ? e.message : 'não foi possível apagar o card');
		}
	}
</script>

<div
	class="w-full cursor-pointer rounded-sm border border-hairline bg-surface p-3 text-left shadow-ficha hover:border-hairline-strong"
	role="button"
	tabindex="0"
	{...clique}
	onkeydown={(e) =>
		(e.key === 'Enter' || e.key === ' ') && (e.preventDefault(), aoAbrirCard(card.id))}
>
	{#if etiquetas.length > 0}
		<div class="mb-2 flex flex-wrap gap-1">
			{#each etiquetas as etiqueta (etiqueta.id)}
				<span
					class="etiqueta-barra cor-{etiqueta.cor}"
					title={etiqueta.nome}
					aria-label="Etiqueta {etiqueta.nome}"
				></span>
			{/each}
		</div>
	{/if}

	<div class="flex items-start justify-between gap-2">
		<b class="flex-1 text-sm font-medium text-body">{card.titulo}</b>
		{#if podeEditar}
			<button
				onclick={apagar}
				class="cursor-pointer text-xs text-mute hover:text-negativo"
				aria-label="Apagar card"
			>
				×
			</button>
		{/if}
	</div>

	{#if card.prazo || temChecklist || card.qtdAnexos > 0 || card.descricao}
		<div class="mt-2 flex flex-wrap items-center gap-2 text-xs text-mute">
			{#if card.prazo}
				<!-- Vencido vem do servidor: o relógio do navegador pode estar
				     errado, e um card vermelho por engano confunde mais que ajuda. -->
				<span
					class="rounded-xs px-1.5 py-0.5 tabular-nums"
					class:bg-negativo={card.vencido}
					class:text-canvas={card.vencido}
					class:bg-surface-elevated={!card.vencido}
				>
					🕑 {prazoCurto}
				</span>
			{/if}
			{#if temChecklist}
				<span class="tabular-nums" class:text-positivo={checklistCompleta}>
					☑ {card.checklist.concluidos}/{card.checklist.total}
				</span>
			{/if}
			{#if card.qtdAnexos > 0}
				<span class="tabular-nums">📎 {card.qtdAnexos}</span>
			{/if}
			{#if card.descricao}
				<span title="Tem descrição">≡</span>
			{/if}
		</div>
	{/if}
</div>
