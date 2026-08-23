<script lang="ts">
	// O quadro visto de fora: sem sessão, sem edição, sem arraste.
	//
	// A tela é escrita AQUI, e não reaproveitando ColunaDoQuadro/CardDoQuadro
	// com `podeEditar={false}`. A tentação é grande — o visual é o mesmo — mas
	// aqueles componentes existem para editar: importam apagarCard, o diálogo de
	// confirmação e a biblioteca de arraste, e recebem callbacks que chamam a
	// API autenticada. Passar uma bandeira `false` faria a página pública
	// carregar tudo isso e depender de um `{#if}` para não oferecê-lo. Uma tela
	// que não pode escrever é mais segura quando não tem como.
	import { renderizarMarkdown } from '$lib/markdown';
	import type { CardPublico } from '$lib/api/publicacao';

	let { data } = $props();

	// Qual card está com a descrição aberta. O card não abre modal — não há o que
	// abrir: comentários, anexos e histórico não vêm por esta rota.
	let descricaoAberta = $state<string | null>(null);

	// A chave é o caminho do card na estrutura, porque a resposta pública não
	// traz id nenhum. Serve: esta lista não reordena, ela só é substituída
	// inteira quando a página recarrega.
	const chave = (coluna: number, card: number) => `${coluna}:${card}`;

	const totalDeCards = $derived(
		data.quadro.colunas.reduce((total, coluna) => total + coluna.cards.length, 0)
	);

	const atualizadoEm = $derived(
		new Date(data.quadro.atualizadoEm).toLocaleString('pt-BR', {
			day: '2-digit',
			month: 'short',
			hour: '2-digit',
			minute: '2-digit'
		})
	);

	function prazoCurto(card: CardPublico): string {
		return card.prazo
			? new Date(card.prazo).toLocaleDateString('pt-BR', { day: '2-digit', month: 'short' })
			: '';
	}
</script>

<svelte:head>
	<title>{data.quadro.titulo} · stacktrack</title>
	<!-- O link é para quem o dono mandou, não para quem procurar no buscador.
	     A API pede o mesmo por cabeçalho (X-Robots-Tag): quem indexa a página e
	     quem indexa a resposta da API são rastreadores diferentes. -->
	<meta name="robots" content="noindex, nofollow" />
	<!-- O token está na URL desta página. Sem isto, clicar num link escrito na
	     descrição de um card mandaria o endereço inteiro — token incluído — no
	     Referer para o site de destino, e o segredo vazaria para um terceiro que
	     ninguém escolheu. -->
	<meta name="referrer" content="no-referrer" />
</svelte:head>

<div class="flex flex-wrap items-end justify-between gap-4">
	<div>
		<!-- Dizer que é uma vista pública, e dizer primeiro. Quem abre um quadro
		     sem ter feito login precisa entender por que está vendo isto. -->
		<p class="flex items-center gap-1.5 text-xs font-semibold tracking-widest text-mute uppercase">
			<svg
				class="size-3.5"
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="2"
				stroke-linecap="round"
				stroke-linejoin="round"
				aria-hidden="true"
			>
				<circle cx="12" cy="12" r="9" />
				<path d="M3 12h18M12 3c2.5 2.7 2.5 15.3 0 18M12 3c-2.5 2.7-2.5 15.3 0 18" />
			</svg>
			Acompanhamento público
		</p>
		<h1 class="mt-1.5 text-2xl font-semibold tracking-tight text-ink">{data.quadro.titulo}</h1>
		<p class="mt-0.5 text-sm text-mute">
			{data.quadro.colunas.length}
			{data.quadro.colunas.length === 1 ? 'coluna' : 'colunas'} ·
			{totalDeCards}
			{totalDeCards === 1 ? 'card' : 'cards'} · atualizado em {atualizadoEm}
		</p>
	</div>

	<!-- Somente leitura dito com todas as letras, e não deduzido da ausência de
	     botões: quem nunca viu o produto não sabe o que está faltando. -->
	<span class="chip chip-neutro">
		<i class="size-1.5 rounded-full bg-current"></i>somente leitura
	</span>
</div>

<div class="painel-fundo fundo-{data.quadro.fundo} mt-6 rounded-lg p-4">
	<!-- Mesma grade da tela do quadro: o link público mostra o mesmo quadro, e
	     duas disposições diferentes para a mesma coisa confundiriam quem recebe
	     o link depois de já ter visto o quadro por dentro. -->
	<div class="grid grid-cols-2 items-start gap-4 pb-2 lg:grid-cols-4">
		{#each data.quadro.colunas as coluna, i (i)}
			<section
				class="flex w-full min-w-0 flex-col rounded-lg border {coluna.cor
					? `cor-${coluna.cor}`
					: 'border-hairline bg-surface-elevated'}"
				style={coluna.cor
					? 'background-color: color-mix(in srgb, var(--etq-texto) 16%, var(--surface-elevated));' +
						'border-color: color-mix(in srgb, var(--etq-texto) 32%, transparent)'
					: ''}
			>
				<header
					class="flex items-center gap-2 border-b px-4 py-3 {coluna.cor ? '' : 'border-hairline'}"
					style={coluna.cor
						? 'background-color: color-mix(in srgb, var(--etq-texto) 26%, var(--surface-elevated));' +
							'border-bottom-color: color-mix(in srgb, var(--etq-texto) 30%, transparent);' +
							'border-top: 3px solid var(--etq-texto)'
						: ''}
				>
					<h2 class="flex-1 text-sm font-medium text-ink">{coluna.titulo}</h2>
					<span
						class="rounded-full border border-hairline bg-surface px-1.5 text-xs tabular-nums text-mute"
					>
						{coluna.cards.length}
					</span>
				</header>

				<ul class="min-h-12 flex-1 space-y-2 p-3">
					{#each coluna.cards as card, j (chave(i, j))}
						<li
							class="rounded-sm border p-3 shadow-ficha {card.cor
								? `cor-${card.cor} border-l-4`
								: 'border-hairline bg-surface'}"
							style={card.cor
								? 'background-color: color-mix(in srgb, var(--etq-texto) 14%, var(--surface));' +
									'border-color: color-mix(in srgb, var(--etq-texto) 34%, transparent);' +
									'border-left-color: var(--etq-texto)'
								: ''}
						>
							{#if card.etiquetas.length > 0}
								<div class="mb-2 flex flex-wrap gap-1">
									{#each card.etiquetas as etiqueta, k (k)}
										<span class="etiqueta-selo cor-{etiqueta.cor}">{etiqueta.nome}</span>
									{/each}
								</div>
							{/if}

							<b class="block text-sm font-medium text-body">{card.titulo}</b>

							{#if card.prazo || card.checklist.total > 0}
								<div class="mt-2 flex flex-wrap items-center gap-2 text-xs text-mute">
									{#if card.prazo}
										<!-- Vencido vem do servidor: o relógio de quem visita pode
										     estar errado, e um card vermelho por engano confunde. -->
										<span
											class="rounded-xs px-1.5 py-0.5 tabular-nums"
											class:bg-negativo={card.vencido}
											class:text-canvas={card.vencido}
											class:bg-surface-elevated={!card.vencido}
										>
											🕑 {prazoCurto(card)}
										</span>
									{/if}
									{#if card.checklist.total > 0}
										<span
											class="tabular-nums"
											class:text-positivo={card.checklist.concluidos === card.checklist.total}
										>
											☑ {card.checklist.concluidos}/{card.checklist.total}
										</span>
									{/if}
								</div>
							{/if}

							{#if card.descricao}
								<button
									class="mt-2 cursor-pointer text-xs text-mute underline-offset-2 hover:text-body hover:underline"
									onclick={() =>
										(descricaoAberta = descricaoAberta === chave(i, j) ? null : chave(i, j))}
									aria-expanded={descricaoAberta === chave(i, j)}
								>
									{descricaoAberta === chave(i, j) ? 'esconder detalhes' : 'ver detalhes'}
								</button>
								{#if descricaoAberta === chave(i, j)}
									<!-- O markdown é renderizado pelo mesmo escapador do resto do
									     produto: tudo é escapado ANTES de virar tag, então nada do
									     que alguém escreveu num card chega ao HTML como marcação.
									     Aqui importa mais que em qualquer outra tela — a plateia é
									     desconhecida. Ver $lib/markdown. -->
									<div class="markdown mt-2 border-t border-hairline pt-2 text-xs">
										{@html renderizarMarkdown(card.descricao)}
									</div>
								{/if}
							{/if}
						</li>
					{/each}

					{#if coluna.cards.length === 0}
						<li class="py-2 text-center text-xs text-mute">nada por aqui</li>
					{/if}
				</ul>
			</section>
		{/each}
	</div>

	{#if data.quadro.colunas.length === 0}
		<p class="py-8 text-center text-sm text-mute">Este quadro ainda não tem colunas.</p>
	{/if}
</div>

<p class="mt-6 text-xs text-mute">
	Esta é uma vista somente leitura, publicada por quem administra o quadro. Comentários, anexos,
	histórico e quem trabalha em cada card ficam fora dela.
</p>
