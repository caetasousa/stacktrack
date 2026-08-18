<script lang="ts">
	// A auditoria do quadro: quem mexeu no quê, do mais recente para o mais
	// antigo.
	//
	// O problema que ela resolve não é técnico. A informação SEMPRE esteve no log
	// de eventos — o histórico de cada card já a mostrava —, mas num quadro com
	// cinquenta cards ela só era alcançável abrindo card por card, que é o mesmo
	// que não estar. Esta tela não guarda nada de novo: junta o que já existia.
	import { auditoriaDoQuadro } from '$lib/api/extras';
	import { ApiError } from '$lib/api/client';
	import { fraseNoQuadro, quando, type Atividade } from '$lib/atividade';
	import { iniciais } from '$lib/iniciais';

	let { data } = $props();

	let linhas = $state<Atividade[]>([]);
	// Começa MOSTRANDO TUDO. A tela nasceu respondendo "quem bagunçou a ordem",
	// e por isso abria só com movimentações de card; mas quem chega aqui está
	// investigando, e um recorte ligado por padrão esconde justamente o que a
	// pessoa ainda não sabe que procura. O recorte estreito continua a um clique.
	let soMovimentacoes = $state(false);
	let autorId = $state('');
	let carregando = $state(false);
	let acabou = $state(false);
	let erro = $state('');

	// Quem aparece no seletor: quem participa do quadro HOJE, mais quem aparece
	// no log. A segunda parte importa — auditar costuma ser sobre alguém que já
	// saiu, e um filtro que só lista os membros atuais esconderia exatamente a
	// pessoa que se está procurando.
	//
	// O rótulo leva o email junto, e não só o nome: é uma lista onde se ESCOLHE
	// uma pessoa, e escolher entre dois "Ana Silva" idênticos é impossível.
	const pessoas = $derived.by(() => {
		const porID = new Map<string, { nome: string; email: string }>();
		for (const m of data.membros) porID.set(m.usuarioId, { nome: m.nome, email: m.email });
		for (const l of linhas) {
			if (l.autorId && !porID.has(l.autorId)) {
				porID.set(l.autorId, { nome: l.autorNome || 'conta removida', email: l.autorEmail });
			}
		}
		return [...porID].map(([usuarioId, p]) => ({
			usuarioId,
			rotulo: p.email ? `${p.nome} — ${p.email}` : p.nome
		}));
	});

	// Só as linhas que a tela sabe descrever. Um tipo de evento novo aparece como
	// silêncio, e não como uma frase pela metade no meio da auditoria.
	const descritas = $derived(
		linhas.map((a) => ({ a, texto: fraseNoQuadro(a) })).filter((l) => l.texto !== '')
	);

	// Recarrega do zero quando o recorte muda. Não é `$effect` sobre os filtros:
	// o primeiro lote veio do load(), e um efeito dispararia uma segunda busca
	// idêntica assim que a página montasse.
	async function aplicarFiltro() {
		carregando = true;
		erro = '';
		try {
			const resposta = await auditoriaDoQuadro(data.quadro.id, { soMovimentacoes, autorId });
			linhas = resposta.atividade;
			acabou = !resposta.temMais;
		} catch (e) {
			erro = e instanceof ApiError ? e.message : 'não foi possível ler as movimentações';
		} finally {
			carregando = false;
		}
	}

	// A página seguinte parte do MENOR seq já recebido, e não de um número de
	// página: o quadro continua sendo mexido enquanto se audita, e paginar por
	// deslocamento pularia em silêncio as linhas que entrassem no meio.
	async function carregarMais() {
		if (carregando || acabou || linhas.length === 0) return;
		carregando = true;
		erro = '';
		try {
			const resposta = await auditoriaDoQuadro(data.quadro.id, {
				soMovimentacoes,
				autorId,
				antesDe: linhas[linhas.length - 1].seq
			});
			linhas = [...linhas, ...resposta.atividade];
			acabou = !resposta.temMais;
		} catch (e) {
			erro = e instanceof ApiError ? e.message : 'não foi possível carregar mais';
		} finally {
			carregando = false;
		}
	}

	// O primeiro lote veio junto com a página, no load().
	$effect(() => {
		linhas = data.atividade;
		acabou = !data.temMais;
	});
</script>

<svelte:head><title>Histórico · {data.quadro.titulo} · stacktrack</title></svelte:head>

<div>
	<a
		href="/painel/quadros/{data.quadro.id}"
		class="inline-flex items-center gap-1 text-xs text-mute transition-colors hover:text-ink"
	>
		<svg
			class="size-3"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			stroke-width="2.5"
			stroke-linecap="round"
			stroke-linejoin="round"
			aria-hidden="true"
		>
			<path d="M15 5l-7 7 7 7" />
		</svg>
		{data.quadro.titulo}
	</a>
	<h1 class="mt-1 text-2xl font-semibold tracking-tight text-ink">Histórico do quadro</h1>
	<p class="mt-0.5 text-sm text-mute">
		Tudo o que aconteceu aqui — cards, colunas, etiquetas, anexos, checklists e participação —, do
		mais recente para o mais antigo.
	</p>
</div>

<div class="mt-6 flex flex-wrap items-center gap-2 text-xs">
	<span class="font-semibold tracking-widest text-mute uppercase">Filtrar</span>

	<select
		bind:value={autorId}
		onchange={aplicarFiltro}
		class="campo w-auto py-1 text-xs"
		aria-label="Pessoa"
	>
		<option value="">Qualquer pessoa</option>
		{#each pessoas as pessoa (pessoa.usuarioId)}
			<option value={pessoa.usuarioId}>{pessoa.rotulo}</option>
		{/each}
	</select>

	<label class="flex cursor-pointer items-center gap-1.5 text-mute">
		<input
			type="checkbox"
			bind:checked={soMovimentacoes}
			onchange={aplicarFiltro}
			class="cursor-pointer"
		/>
		Só movimentações de card
	</label>

	{#if carregando}
		<span class="text-mute">carregando…</span>
	{/if}
</div>

{#if erro}
	<p class="erro-form mt-4">{erro}</p>
{/if}

<section class="mt-4 overflow-hidden rounded-lg border border-hairline bg-surface">
	{#if descritas.length === 0}
		<p class="px-5 py-10 text-center text-sm text-mute">
			{soMovimentacoes
				? 'Ninguém moveu card nenhum por aqui ainda.'
				: 'Nada aconteceu neste quadro ainda.'}
		</p>
	{:else}
		<ul class="divide-y divide-hairline">
			{#each descritas as linha (linha.a.seq)}
				<li class="flex items-start gap-3 px-5 py-3">
					<span
						class="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full bg-accent-suave text-[0.625rem] font-semibold text-accent-texto"
						aria-hidden="true"
					>
						{iniciais(linha.a.autorNome || '?')}
					</span>
					<div class="min-w-0 flex-1">
						<p class="text-sm text-body">
							<b class="font-semibold text-ink">{linha.a.autorNome || 'conta removida'}</b>
							{linha.texto}
						</p>
						<!-- O email SEMPRE, e não só quando há homônimo. Uma regra que
						     esconde a informação na maior parte do tempo obriga quem audita
						     a saber que ela existe — e a auditoria é usada justamente por
						     quem não sabe ainda o que está procurando. -->
						{#if linha.a.autorEmail}
							<p class="truncate text-xs text-mute">{linha.a.autorEmail}</p>
						{/if}
					</div>
					<!-- Data completa, e não relativa: no card cabe "há 2 h", mas quem
					     audita precisa comparar horários entre linhas. -->
					<time class="shrink-0 text-xs tabular-nums text-mute" datetime={linha.a.ocorridoEm}>
						{quando(linha.a.ocorridoEm)}
					</time>
				</li>
			{/each}
		</ul>
	{/if}
</section>

{#if descritas.length > 0 && !acabou}
	<button onclick={carregarMais} disabled={carregando} class="botao-secundario mt-4">
		{carregando ? 'Carregando…' : 'Carregar mais'}
	</button>
{/if}
