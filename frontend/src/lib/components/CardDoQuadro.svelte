<script lang="ts">
	import { confirmar } from '$lib/confirmar.svelte';
	// Um card na coluna. Mostra o RESUMO do que carrega — barras de etiqueta,
	// prazo, progresso de checklist e contagem de anexos — e abre o modal no
	// clique, que é onde tudo isso pode ser mexido.
	import { apagarCard, type Card, type Etiqueta } from '$lib/api/boards';
	import { ApiError } from '$lib/api/client';
	import { cliqueSemArraste } from '$lib/arrastar';
	import { haQuanto, quando } from '$lib/atividade';
	import { iniciais } from '$lib/iniciais';

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

	// A frase inteira fica no title: no card cabe "Ana · há 2 h", mas quem está
	// auditando precisa do trajeto e da data exata sem sair da tela. O histórico
	// completo continua no modal, e o do quadro na tela de movimentações.
	const detalheDaMovimentacao = $derived.by(() => {
		const m = card.ultimaMovimentacao;
		if (!m) return '';
		const quem = m.autorNome || 'alguém';
		const trajeto =
			m.de && m.para && m.de !== m.para
				? `moveu de ${m.de} para ${m.para}`
				: m.para
					? `reordenou em ${m.para}`
					: 'moveu este card';
		return `${quem} ${trajeto} — ${quando(m.ocorridoEm)}`;
	});

	// O card é clicável E arrastável. Sem o limiar de distância, soltar depois
	// de arrastar abriria o modal em cima do movimento que a pessoa fez.
	const clique = cliqueSemArraste(() => aoAbrirCard(card.id));

	async function apagar(evento: MouseEvent) {
		evento.stopPropagation();
		const ok = await confirmar({
			titulo: `Apagar o card "${card.titulo}"?`,
			detalhe: 'Comentários, checklists e anexos vão junto.',
			acao: 'Apagar o card'
		});
		if (!ok) return;
		try {
			await apagarCard(card.id);
			await aoMudar();
		} catch (e) {
			aoFalhar(e instanceof ApiError ? e.message : 'não foi possível apagar o card');
		}
	}
</script>

<!-- A cor do card pinta o card inteiro e engrossa a borda esquerda. O fundo é
     opaco (a cor é misturada na superfície, não sobreposta a ela) para o card
     continuar destacado da coluna, que também pode estar colorida. -->
<div
	class="w-full cursor-pointer rounded-sm border p-3 text-left shadow-ficha {card.cor
		? `cor-${card.cor} border-l-4`
		: 'border-hairline bg-surface hover:border-hairline-strong'}"
	style={card.cor
		? 'background-color: color-mix(in srgb, var(--etq-texto) 14%, var(--surface));' +
			'border-color: color-mix(in srgb, var(--etq-texto) 34%, transparent);' +
			'border-left-color: var(--etq-texto)'
		: ''}
	role="button"
	tabindex="0"
	{...clique}
	onkeydown={(e) =>
		(e.key === 'Enter' || e.key === ' ') && (e.preventDefault(), aoAbrirCard(card.id))}
>
	<!-- A etiqueta mostra a COR e o NOME. Só a cor exigia decorar a convenção
	     do quadro para significar alguma coisa — e não significava nada para
	     quem não distingue as cores. -->
	{#if etiquetas.length > 0}
		<div class="mb-2 flex flex-wrap gap-1">
			{#each etiquetas as etiqueta (etiqueta.id)}
				<span class="etiqueta-selo cor-{etiqueta.cor}" title={etiqueta.nome}>
					{etiqueta.nome}
				</span>
			{/each}
		</div>
	{/if}

	<div class="flex items-start justify-between gap-2">
		<b class="flex-1 text-sm font-medium text-body">{card.titulo}</b>
		<!-- Os avatares ficam à direita do título, e não no rodapé com os selos:
		     "de quem é isto" se lê junto com "o que é isto". -->
		{#if card.responsaveis.length > 0}
			<div class="flex shrink-0 -space-x-1.5">
				{#each card.responsaveis as pessoa (pessoa.usuarioId)}
					<span
						data-responsavel={pessoa.usuarioId}
						class="flex size-5 items-center justify-center rounded-full border border-surface bg-accent-suave text-[9px] font-semibold text-accent-texto"
						title="Responsável: {pessoa.nome}"
					>
						{iniciais(pessoa.nome)}
					</span>
				{/each}
			</div>
		{/if}
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

	{#if card.prazo || temChecklist || card.qtdAnexos > 0 || card.qtdComentarios > 0 || card.descricao}
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
			{#if card.qtdComentarios > 0}
				<span class="tabular-nums" title="{card.qtdComentarios} comentário(s)">
					💬 {card.qtdComentarios}
				</span>
			{/if}
			{#if card.descricao}
				<span title="Tem descrição">≡</span>
			{/if}
		</div>
	{/if}

	<!-- Quem mexeu neste card por último.

	     Fica numa LINHA PRÓPRIA, separada por um filete, e não junto dos selos
	     acima: aqueles descrevem o conteúdo do card (prazo, checklist, anexos),
	     e este descreve quem o tocou. Misturá-los faria a auditoria parecer mais
	     um atributo da tarefa.

	     Só aparece em card que já foi movido. Card recém-criado não ganha linha
	     nenhuma — ausência é a informação correta ali. -->
	{#if card.ultimaMovimentacao}
		<div
			class="mt-2 flex items-center gap-1.5 border-t border-hairline pt-2 text-[0.6875rem] text-mute"
			title={detalheDaMovimentacao}
		>
			<svg
				class="size-3 shrink-0"
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="2.2"
				stroke-linecap="round"
				stroke-linejoin="round"
				aria-hidden="true"
			>
				<path d="M4 7h11M11 3l4 4-4 4M20 17H9M13 13l-4 4 4 4" />
			</svg>
			<span class="truncate">{card.ultimaMovimentacao.autorNome || 'alguém'}</span>
			<span class="shrink-0" aria-hidden="true">·</span>
			<span class="shrink-0 tabular-nums">{haQuanto(card.ultimaMovimentacao.ocorridoEm)}</span>
		</div>
	{/if}
</div>
