<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import {
		apagarBoard,
		criarColuna,
		renomearBoard,
		FUNDOS,
		type Cor,
		type Fundo
	} from '$lib/api/boards';
	import {
		apagarEtiqueta,
		criarEtiqueta,
		definirFundo,
		editarEtiqueta,
		moverCard,
		moverColuna
	} from '$lib/api/extras';
	import type { Card, Coluna } from '$lib/api/boards';
	import { ApiError } from '$lib/api/client';
	import { dndzone, type DndEvent } from 'svelte-dnd-action';
	import { flip } from 'svelte/animate';
	import {
		CLASSES_ALVO,
		DURACAO_MS,
		enfeitarArrastado,
		TIPO_COLUNA,
		vizinhosDe
	} from '$lib/arrastar';
	import {
		conectarAoQuadro,
		type EventoDoQuadro,
		type Presente
	} from '$lib/realtime/conexao.svelte';
	import { iniciais } from '$lib/iniciais';
	import {
		FILTRO_VAZIO,
		filtrando as estaFiltrando,
		passa,
		pessoasDoQuadro,
		type Filtro
	} from '$lib/filtro';
	import ColunaDoQuadro from '$lib/components/ColunaDoQuadro.svelte';
	import ModalDoCard from '$lib/components/ModalDoCard.svelte';
	import SeletorDeCor from '$lib/components/SeletorDeCor.svelte';

	let { data } = $props();

	// O arraste precisa de uma cópia LOCAL: a lista que vem do load() é o estado
	// do servidor, e mexer nela direto faria a tela discordar do que a API sabe.
	// Esta cópia é a "atualização otimista" — o card se move na hora, e se a API
	// recusar ela volta a ser o que o servidor diz.
	let colunas = $state<Coluna[]>([]);

	// Re-sincroniza sempre que o load() traz dados novos (criar, apagar,
	// recarregar). Depende só de data.quadro, então mexer em `colunas` durante o
	// arraste não dispara isto de volta.
	$effect(() => {
		colunas = data.quadro.colunas.map((c) => ({ ...c, cards: [...c.cards] }));
	});

	// --- tempo real ---------------------------------------------------------
	//
	// A conexão nasce e morre com a página do quadro: $effect devolve a função
	// de limpeza, e o SvelteKit a chama ao navegar para fora. Sem isso, trocar
	// de quadro deixaria a sala antiga aberta e a pessoa receberia eventos de
	// um quadro que não está mais olhando.
	//
	// A dependência é `data.quadro.id`: mudar de quadro fecha uma conexão e abre
	// a outra, sem passar pelo estado "duas abertas".
	let conexao = $state<{ readonly situacao: string; fechar: () => void } | null>(null);

	$effect(() => {
		const id = data.quadro.id;
		const c = conectarAoQuadro(id, aoReceberEvento);
		conexao = c;
		return () => {
			c.fechar();
			conexao = null;
			presentes = [];
			// Uma recarga agendada que dispare depois da saída invalidaria dados
			// de um quadro que já não está na tela.
			if (recargaAgendada) {
				clearTimeout(recargaAgendada);
				recargaAgendada = null;
			}
		};
	});

	// Toda mudança de outra pessoa recarrega o quadro.
	//
	// É deliberadamente grosseiro para a primeira versão: o servidor já manda o
	// tipo e o payload de cada evento, mas aplicar diferença aqui significaria
	// manter um caminho de aplicação por tipo — e qualquer um deles errado
	// deixaria a tela discordando do banco em silêncio, que é o pior defeito
	// possível num quadro colaborativo. Recarregar é sempre correto; o custo é
	// uma requisição.
	//
	// O eco do próprio autor já foi filtrado pelo servidor: tudo que chega aqui
	// foi feito por outra pessoa.
	function aoReceberEvento(e: EventoDoQuadro) {
		// Presença é estado efêmero: aplica direto, sem ir ao banco. Recarregar o
		// quadro só porque alguém abriu a aba seria uma requisição por entrada e
		// saída de cada pessoa.
		if (e.tipo === 'presenca.alterada') {
			presentes = (e.dados as Presente[]) ?? [];
			return;
		}
		// `sincronizado` só carrega a posição atual da história — a conexão
		// registra o seq e não há nada a mostrar. Recarregar aqui daria uma
		// requisição a cada conexão, sem nenhuma mudança para ver.
		if (e.tipo === 'sincronizado') return;

		// Tudo o mais — inclusive `recarregue.tudo`, quando o intervalo perdido
		// foi grande demais para repor — cai na recarga do quadro.
		agendarRecarga();
	}

	// Quanto tempo se espera para juntar eventos numa recarga só.
	//
	// Curto o bastante para continuar parecendo instantâneo, longo o bastante
	// para colapsar uma rajada. É o que separa "uma recarga por evento" de "uma
	// recarga por surto de atividade".
	const JANELA_DE_RECARGA = 150;
	let recargaAgendada: ReturnType<typeof setTimeout> | null = null;

	// Junta eventos próximos numa recarga só.
	//
	// Sem isto, cada evento vira um GET do quadro inteiro — e são dois os
	// momentos em que isso machuca. Na reconexão, o backlog pode trazer até 200
	// eventos de uma vez, o que dispararia 200 recargas em sequência. E ao vivo,
	// o teto de requisições por sessão passa a ser consumido pelo movimento dos
	// OUTROS: quem está apenas olhando um quadro movimentado esgotaria a própria
	// cota e começaria a levar 429 justamente enquanto não fez nada.
	function agendarRecarga() {
		if (recargaAgendada) return;
		recargaAgendada = setTimeout(() => {
			recargaAgendada = null;
			recarregar();
		}, JANELA_DE_RECARGA);
	}

	// Quem está com este quadro aberto agora, incluindo você.
	let presentes = $state<Presente[]>([]);

	// --- filtro ---------------------------------------------------------------
	//
	// Aplicado no cliente, sobre o quadro que já está carregado: ele veio inteiro
	// numa requisição só, então filtrar no servidor custaria uma ida e volta para
	// esconder o que a tela já tem na mão. A regra em si vive em $lib/filtro,
	// onde dá para testá-la — um predicado errado não quebra nada, só some com
	// cards, e quem olha conclui que foram apagados.
	let filtro = $state<Filtro>({ ...FILTRO_VAZIO });

	const filtrando = $derived(estaFiltrando(filtro));
	const pessoasNoQuadro = $derived(pessoasDoQuadro(colunas));

	// A lista filtrada é uma VISTA: `colunas` continua sendo o estado real, que é
	// o que o arraste manipula. Enquanto o filtro está ligado o arraste fica
	// travado, porque os vizinhos calculados a partir de uma lista incompleta
	// colocariam o card entre cards que não são os vizinhos de verdade.
	const colunasVisiveis = $derived(
		filtrando
			? colunas.map((c) => ({ ...c, cards: c.cards.filter((card) => passa(card, filtro)) }))
			: colunas
	);

	const cardsVisiveis = $derived(colunasVisiveis.reduce((total, c) => total + c.cards.length, 0));

	function limparFiltro() {
		filtro = { ...FILTRO_VAZIO };
	}

	let erro = $state('');
	let renomeando = $state(false);
	let titulo = $state('');
	let tituloDaColuna = $state('');
	let corDaColuna = $state<Cor | ''>('');
	let criandoColuna = $state(false);
	// Qual card está aberto no modal — null é modal fechado.
	let cardAberto = $state<string | null>(null);
	let painelDeAjustes = $state(false);
	let nomeDaEtiqueta = $state('');
	let corDaEtiqueta = $state<Cor>('azul');

	const podeEditar = $derived(data.quadro.papel === 'dono' || data.quadro.papel === 'editor');
	const podeAdministrar = $derived(data.quadro.papel === 'dono');

	const cores: Cor[] = ['cinza', 'vermelho', 'laranja', 'amarelo', 'verde', 'azul', 'roxo', 'rosa'];
	const nomeDoFundo: Record<Fundo, string> = {
		padrao: 'Padrão',
		ardosia: 'Ardósia',
		oceano: 'Oceano',
		floresta: 'Floresta',
		ameixa: 'Ameixa',
		brasa: 'Brasa'
	};

	// Recarrega o quadro inteiro. É o que roda depois das próprias ações e
	// também quando chega um evento de outra pessoa.
	async function recarregar() {
		erro = '';
		await invalidateAll();
	}

	// --- arrastar e soltar --------------------------------------------------

	// Prévia do arraste: só espelha o que a biblioteca calculou, sem tocar na API.
	function espelharCards(colunaId: string, cards: Card[]) {
		colunas = colunas.map((c) => (c.id === colunaId ? { ...c, cards } : c));
	}

	// Ao soltar: aplica na tela, manda para a API e — se ela recusar — desfaz.
	// O `antes` é tirado do estado do SERVIDOR, e não da cópia local, que a esta
	// altura já foi mexida pela biblioteca.
	async function soltarCard(colunaId: string, cards: Card[], cardMovidoId: string | null) {
		espelharCards(colunaId, cards);
		if (!cardMovidoId) return;

		// A biblioteca dispara o finalize nas DUAS colunas envolvidas: na de
		// origem, o card já não está na lista, e é a de destino que manda.
		if (!cards.some((c) => c.id === cardMovidoId)) return;

		try {
			await moverCard(cardMovidoId, { colunaId, ...vizinhosDe(cards, cardMovidoId) });
		} catch (e) {
			tratar(e, 'não foi possível mover o card');
			await recarregar();
		}
	}

	function espelharColunas(novas: Coluna[]) {
		colunas = novas;
	}

	async function soltarColuna(novas: Coluna[], colunaMovidaId: string | null) {
		espelharColunas(novas);
		if (!colunaMovidaId) return;

		try {
			await moverColuna(colunaMovidaId, vizinhosDe(novas, colunaMovidaId));
		} catch (e) {
			tratar(e, 'não foi possível mover a coluna');
			await recarregar();
		}
	}

	function falhar(mensagem: string) {
		erro = mensagem;
	}

	function tratar(e: unknown, padrao: string) {
		falhar(e instanceof ApiError ? e.message : padrao);
	}

	async function renomear(evento: SubmitEvent) {
		evento.preventDefault();
		try {
			await renomearBoard(data.quadro.id, titulo);
			renomeando = false;
			await recarregar();
		} catch (e) {
			tratar(e, 'não foi possível renomear o quadro');
		}
	}

	async function adicionarColuna(evento: SubmitEvent) {
		evento.preventDefault();
		try {
			await criarColuna(data.quadro.id, tituloDaColuna, corDaColuna);
			tituloDaColuna = '';
			corDaColuna = '';
			criandoColuna = false;
			await recarregar();
		} catch (e) {
			tratar(e, 'não foi possível criar a coluna');
		}
	}

	async function apagar() {
		if (!confirm(`Apagar "${data.quadro.titulo}"? As colunas e os cards vão junto.`)) return;
		try {
			await apagarBoard(data.quadro.id);
			await goto('/painel');
		} catch (e) {
			tratar(e, 'não foi possível apagar o quadro');
		}
	}

	async function trocarFundo(fundo: Fundo) {
		try {
			await definirFundo(data.quadro.id, fundo);
			await recarregar();
		} catch (e) {
			tratar(e, 'não foi possível trocar o fundo');
		}
	}

	async function adicionarEtiqueta(evento: SubmitEvent) {
		evento.preventDefault();
		try {
			await criarEtiqueta(data.quadro.id, nomeDaEtiqueta, corDaEtiqueta);
			nomeDaEtiqueta = '';
			await recarregar();
		} catch (e) {
			tratar(e, 'não foi possível criar a etiqueta');
		}
	}

	async function trocarCor(etiquetaId: string, nome: string, cor: Cor) {
		try {
			await editarEtiqueta(etiquetaId, nome, cor);
			await recarregar();
		} catch (e) {
			tratar(e, 'não foi possível mudar a etiqueta');
		}
	}

	async function removerEtiquetaDoQuadro(etiquetaId: string, nome: string) {
		if (!confirm(`Apagar a etiqueta "${nome}"? Ela some de todos os cards.`)) return;
		try {
			await apagarEtiqueta(etiquetaId);
			await recarregar();
		} catch (e) {
			tratar(e, 'não foi possível apagar a etiqueta');
		}
	}
</script>

<svelte:head><title>{data.quadro.titulo} · stacktrack</title></svelte:head>

<div class="flex flex-wrap items-baseline justify-between gap-4">
	{#if renomeando}
		<form onsubmit={renomear} class="flex flex-1 gap-2">
			<input
				class="campo"
				bind:value={titulo}
				required
				maxlength="120"
				aria-label="Título do quadro"
			/>
			<button type="submit" class="botao w-auto px-4">Salvar</button>
			<button
				type="button"
				onclick={() => (renomeando = false)}
				class="cursor-pointer px-2 text-sm text-mute hover:text-ink"
			>
				Cancelar
			</button>
		</form>
	{:else}
		<div>
			<h1 class="text-xl font-semibold tracking-tight text-ink">{data.quadro.titulo}</h1>
			<p class="mt-0.5 text-sm text-mute">
				{data.quadro.colunas.length}
				{data.quadro.colunas.length === 1 ? 'coluna' : 'colunas'}
			</p>
		</div>
		<div class="flex shrink-0 flex-wrap items-center gap-3 text-xs text-mute">
			<!-- Quem está no quadro agora. A lista vem do mapa de conexões do
			     hub, não do banco: é estado que só existe enquanto a aba está
			     aberta. -->
			{#if presentes.length > 0}
				<div class="flex items-center -space-x-1.5" aria-label="Quem está neste quadro agora">
					{#each presentes as pessoa (pessoa.id)}
						<span
							class="flex size-6 items-center justify-center rounded-full border border-surface bg-accent-suave text-[10px] font-semibold text-accent-texto"
							title={pessoa.nome}
						>
							{iniciais(pessoa.nome)}
						</span>
					{/each}
				</div>
			{/if}
			<span class="chip" class:chip-neutro={data.quadro.papel !== 'dono'}>
				<i class="size-1.5 rounded-full bg-current"></i>{data.quadro.papel}
			</span>
			<!-- A situação da conexão fica à vista: quando ela cai, quem está
			     olhando precisa saber que a tela parou de se atualizar sozinha —
			     senão vai confiar num quadro velho. Ao vivo não vira selo verde
			     de propósito: o normal não precisa de aviso. -->
			{#if conexao && conexao.situacao !== 'ao-vivo'}
				<span class="chip chip-neutro" title="A tela não está se atualizando sozinha">
					<i class="size-1.5 rounded-full bg-aviso"></i>
					{conexao.situacao === 'conectando' ? 'conectando' : 'reconectando'}
				</span>
			{/if}
			{#if podeEditar}
				<button
					onclick={() => (criandoColuna = !criandoColuna)}
					class="botao w-auto px-3 py-1 text-xs"
					aria-expanded={criandoColuna}
				>
					+ Nova coluna
				</button>
				<button
					onclick={() => (painelDeAjustes = !painelDeAjustes)}
					class="cursor-pointer hover:text-ink"
					aria-expanded={painelDeAjustes}
				>
					Ajustes
				</button>
			{/if}
			<a href="/painel/quadros/{data.quadro.id}/membros" class="hover:text-ink">Membros</a>
			{#if podeAdministrar}
				<button
					onclick={() => ((titulo = data.quadro.titulo), (renomeando = true))}
					class="cursor-pointer hover:text-ink">Renomear</button
				>
				<button onclick={apagar} class="cursor-pointer hover:text-negativo">Apagar</button>
			{/if}
			<a href="/painel" class="hover:text-ink">Voltar</a>
		</div>
	{/if}
</div>

{#if erro}
	<p class="erro-form mt-4">{erro}</p>
{/if}

<!-- O formulário mora aqui, no cabeçalho, e não no fim da faixa de colunas:
     lá ele saía da tela junto com a rolagem horizontal assim que o quadro
     ganhava colunas demais. -->
{#if criandoColuna && podeEditar}
	<section class="mt-4 rounded-lg border border-hairline bg-surface p-4 shadow-ficha">
		<form onsubmit={adicionarColuna} class="flex flex-wrap items-center gap-3">
			<input
				class="campo min-w-0 flex-1 text-sm"
				bind:value={tituloDaColuna}
				placeholder="Nome da coluna"
				required
				maxlength="120"
				aria-label="Título da nova coluna"
			/>
			<SeletorDeCor bind:cor={corDaColuna} rotulo="Cor da nova coluna" />
			<button type="submit" class="botao w-auto px-4 py-1 text-xs">Criar</button>
			<button
				type="button"
				class="cursor-pointer px-2 text-xs text-mute hover:text-ink"
				onclick={() => (criandoColuna = false)}>Cancelar</button
			>
		</form>
	</section>
{/if}

{#if painelDeAjustes && podeEditar}
	<section class="mt-5 space-y-6 rounded-lg border border-hairline bg-surface p-5 shadow-ficha">
		<div>
			<h2 class="text-sm font-semibold text-ink">Etiquetas do quadro</h2>
			<p class="mt-1 text-xs text-mute">
				Valem para todos os cards. Renomear ou trocar a cor muda em todos de uma vez.
			</p>

			<ul class="mt-4 space-y-2">
				{#each data.quadro.etiquetas as etiqueta (etiqueta.id)}
					<li class="flex flex-wrap items-center gap-2">
						<span class="etiqueta cor-{etiqueta.cor} min-w-24">{etiqueta.nome}</span>
						<div class="flex gap-1">
							{#each cores as cor (cor)}
								<button
									class="etiqueta-barra cor-{cor} cursor-pointer {etiqueta.cor === cor
										? 'ring-2 ring-accent ring-offset-1 ring-offset-surface'
										: 'opacity-50'}"
									onclick={() => trocarCor(etiqueta.id, etiqueta.nome, cor)}
									aria-label="Mudar {etiqueta.nome} para {cor}"
								></button>
							{/each}
						</div>
						<button
							class="ml-auto cursor-pointer text-xs text-mute hover:text-negativo"
							onclick={() => removerEtiquetaDoQuadro(etiqueta.id, etiqueta.nome)}
						>
							Apagar
						</button>
					</li>
				{/each}
				{#if data.quadro.etiquetas.length === 0}
					<li class="text-xs text-mute">Nenhuma etiqueta ainda.</li>
				{/if}
			</ul>

			<form onsubmit={adicionarEtiqueta} class="mt-4 flex flex-wrap gap-2">
				<input
					class="campo min-w-0 flex-1 py-1 text-xs"
					bind:value={nomeDaEtiqueta}
					placeholder="Nome da nova etiqueta"
					required
					maxlength="60"
					aria-label="Nome da nova etiqueta"
				/>
				<select
					class="campo py-1 text-xs font-semibold"
					style="color: var(--cor-{corDaEtiqueta})"
					bind:value={corDaEtiqueta}
					aria-label="Cor"
				>
					{#each cores as cor (cor)}
						<option value={cor} style="color: var(--cor-{cor})">{cor}</option>
					{/each}
				</select>
				<button type="submit" class="botao w-auto px-4 py-1 text-xs">Criar</button>
			</form>
		</div>

		{#if podeAdministrar}
			<div class="border-t border-hairline pt-5">
				<h2 class="text-sm font-semibold text-ink">Fundo do quadro</h2>
				<div class="mt-3 flex flex-wrap items-center gap-2 text-xs">
					{#each FUNDOS as fundo (fundo)}
						<button
							class="fundo-{fundo} flex cursor-pointer items-center gap-1.5 rounded-sm border px-2 py-1"
							style="color: var(--fundo-cor); border-color: color-mix(in srgb, var(--fundo-cor) {data
								.quadro.fundo === fundo
								? '70%'
								: '25%'}, transparent)"
							onclick={() => trocarFundo(fundo)}
							aria-pressed={data.quadro.fundo === fundo}
						>
							<i
								class="size-2.5 rounded-full"
								style="background-color: color-mix(in srgb, var(--fundo-cor) {data.quadro.fundo ===
								fundo
									? '100%'
									: '45%'}, transparent)"
							></i>
							{nomeDoFundo[fundo]}
						</button>
					{/each}
				</div>
			</div>
		{/if}
	</section>
{/if}

<!-- Filtro. Fica fora do painel do quadro de propósito: ele age SOBRE o quadro,
     e some junto com a barra quando não há nada para filtrar. -->
{#if pessoasNoQuadro.length > 0 || data.quadro.etiquetas.length > 0}
	<div class="mt-6 flex flex-wrap items-center gap-2 text-xs">
		<span class="font-semibold tracking-widest text-mute uppercase">Filtrar</span>

		{#if pessoasNoQuadro.length > 0}
			<select
				bind:value={filtro.responsavelId}
				class="campo w-auto py-1 text-xs"
				aria-label="Responsável"
			>
				<option value="">Qualquer responsável</option>
				{#each pessoasNoQuadro as pessoa (pessoa.usuarioId)}
					<option value={pessoa.usuarioId}>{pessoa.nome}</option>
				{/each}
			</select>
		{/if}

		{#if data.quadro.etiquetas.length > 0}
			<select
				bind:value={filtro.etiquetaId}
				class="campo w-auto py-1 text-xs"
				aria-label="Etiqueta"
			>
				<option value="">Qualquer etiqueta</option>
				{#each data.quadro.etiquetas as etiqueta (etiqueta.id)}
					<option value={etiqueta.id}>{etiqueta.nome}</option>
				{/each}
			</select>
		{/if}

		<label class="flex cursor-pointer items-center gap-1.5 text-mute">
			<input type="checkbox" bind:checked={filtro.soVencidos} class="cursor-pointer" />
			Só vencidos
		</label>

		{#if filtrando}
			<span class="text-mute tabular-nums">
				{cardsVisiveis}
				{cardsVisiveis === 1 ? 'card' : 'cards'}
			</span>
			<button onclick={limparFiltro} class="cursor-pointer underline hover:text-body">
				limpar
			</button>
			<!-- Dizer POR QUE o arraste parou: sem isto, a pessoa tenta mover um
			     card, nada acontece, e ela conclui que a tela travou. -->
			<span class="text-mute">· arraste desativado enquanto filtra</span>
		{/if}
	</div>
{/if}

<div class="painel-fundo fundo-{data.quadro.fundo} mt-3 rounded-lg p-4">
	<div class="flex items-start gap-4 overflow-x-auto pb-4">
		<!-- Zona das colunas. Ela ENVOLVE a zona dos cards, e é o aninhamento que
		     faz o arraste começar na zona certa: um card é filho do <ul> de
		     cards, não desta lista. -->
		<div
			class="flex items-start gap-4"
			use:dndzone={{
				items: colunasVisiveis,
				type: TIPO_COLUNA,
				flipDurationMs: DURACAO_MS,
				dragDisabled: !podeEditar || filtrando,
				dropTargetStyle: {},
				dropTargetClasses: CLASSES_ALVO,
				transformDraggedElement: enfeitarArrastado
			}}
			onconsider={(e: CustomEvent<DndEvent<Coluna>>) => espelharColunas(e.detail.items)}
			onfinalize={(e: CustomEvent<DndEvent<Coluna>>) =>
				soltarColuna(e.detail.items, e.detail.info.id)}
		>
			{#each colunasVisiveis as coluna (coluna.id)}
				<div animate:flip={{ duration: DURACAO_MS }}>
					<ColunaDoQuadro
						{coluna}
						etiquetasDoQuadro={data.quadro.etiquetas}
						{podeEditar}
						arrasteTravado={filtrando}
						aoAbrirCard={(id) => (cardAberto = id)}
						aoMudar={recarregar}
						aoFalhar={falhar}
						aoArrastarCards={espelharCards}
						aoSoltarCard={soltarCard}
					/>
				</div>
			{/each}
		</div>
	</div>

	{#if colunas.length === 0}
		<p class="py-8 text-center text-sm text-mute">
			{podeEditar
				? 'Este quadro ainda não tem colunas — comece por "+ Nova coluna", ali em cima.'
				: 'Este quadro ainda não tem colunas.'}
		</p>
	{/if}
</div>

{#if cardAberto}
	<ModalDoCard
		cardId={cardAberto}
		etiquetasDoQuadro={data.quadro.etiquetas}
		{podeEditar}
		aoFechar={() => (cardAberto = null)}
		aoMudar={recarregar}
	/>
{/if}
