<script lang="ts">
	// O modal do card: descrição em markdown, etiquetas, prazo, checklists e
	// anexos. É onde cabe o que não cabe no card da coluna.
	import { ApiError } from '$lib/api/client';
	import { editarCard, type Cor, type Etiqueta } from '$lib/api/boards';
	import SeletorDeCor from './SeletorDeCor.svelte';
	import {
		anexarArquivo,
		anexarLink,
		aplicarEtiqueta,
		apagarAnexo,
		apagarChecklist,
		apagarItem,
		criarChecklist,
		criarItem,
		definirPrazo,
		detalharCard,
		editarItem,
		removerEtiqueta,
		urlDoAnexo,
		type CardDetalhado
	} from '$lib/api/extras';
	import { renderizarMarkdown } from '$lib/markdown';

	let {
		cardId,
		etiquetasDoQuadro,
		podeEditar,
		aoFechar,
		aoMudar
	}: {
		cardId: string;
		etiquetasDoQuadro: Etiqueta[];
		podeEditar: boolean;
		aoFechar: () => void;
		aoMudar: () => Promise<void>;
	} = $props();

	let card = $state<CardDetalhado | null>(null);
	let erro = $state('');
	let carregando = $state(true);

	let editandoTexto = $state(false);
	let titulo = $state('');
	let descricao = $state('');
	let tituloDaChecklist = $state('');
	let textoDoItem = $state<Record<string, string>>({});
	let urlDoLink = $state('');
	let nomeDoLink = $state('');
	let enviandoArquivo = $state(false);

	const idsAplicados = $derived(new Set(card?.etiquetasDoCard.map((e) => e.id) ?? []));

	async function carregar() {
		try {
			card = await detalharCard(cardId);
			erro = '';
		} catch (e) {
			erro = e instanceof ApiError ? e.message : 'não foi possível carregar o card';
		} finally {
			carregando = false;
		}
	}

	// Recarrega o card E avisa a página, para os selos da coluna acompanharem.
	async function recarregar() {
		await carregar();
		await aoMudar();
	}

	function falhar(e: unknown, padrao: string) {
		erro = e instanceof ApiError ? e.message : padrao;
	}

	$effect(() => {
		cardId;
		carregando = true;
		carregar();
	});

	function abrirEdicaoDeTexto() {
		if (!podeEditar || !card) return;
		titulo = card.titulo;
		descricao = card.descricao;
		editandoTexto = true;
	}

	async function salvarTexto(evento: SubmitEvent) {
		evento.preventDefault();
		try {
			// A cor vai junto porque o PATCH grava o card inteiro: omiti-la aqui
			// apagaria a cor do card a cada edição de texto.
			await editarCard(cardId, titulo, descricao, card?.cor ?? '');
			editandoTexto = false;
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível salvar o card');
		}
	}

	async function mudarCor(nova: Cor | '') {
		if (!card) return;
		try {
			await editarCard(cardId, card.titulo, card.descricao, nova);
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível mudar a cor do card');
		}
	}

	async function alternarEtiqueta(etiqueta: Etiqueta) {
		try {
			if (idsAplicados.has(etiqueta.id)) {
				await removerEtiqueta(cardId, etiqueta.id);
			} else {
				await aplicarEtiqueta(cardId, etiqueta.id);
			}
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível mudar a etiqueta');
		}
	}

	// O input date entrega 'AAAA-MM-DD'; a API espera data-hora. Meio-dia
	// evita que o fuso empurre o prazo para o dia anterior ou seguinte.
	async function mudarPrazo(valor: string) {
		try {
			await definirPrazo(cardId, valor ? new Date(`${valor}T12:00:00`).toISOString() : null);
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível mudar o prazo');
		}
	}

	async function adicionarChecklist(evento: SubmitEvent) {
		evento.preventDefault();
		try {
			await criarChecklist(cardId, tituloDaChecklist);
			tituloDaChecklist = '';
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível criar a checklist');
		}
	}

	async function adicionarItem(evento: SubmitEvent, checklistId: string) {
		evento.preventDefault();
		const texto = textoDoItem[checklistId]?.trim();
		if (!texto) return;
		try {
			await criarItem(checklistId, texto);
			textoDoItem[checklistId] = '';
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível criar o item');
		}
	}

	async function marcar(itemId: string, concluido: boolean) {
		try {
			await editarItem(itemId, { concluido });
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível marcar o item');
		}
	}

	async function removerItem(itemId: string) {
		try {
			await apagarItem(itemId);
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível apagar o item');
		}
	}

	async function removerChecklist(checklistId: string, titulo: string) {
		if (!confirm(`Apagar a checklist "${titulo}"? Os itens vão junto.`)) return;
		try {
			await apagarChecklist(checklistId);
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível apagar a checklist');
		}
	}

	async function adicionarLink(evento: SubmitEvent) {
		evento.preventDefault();
		try {
			await anexarLink(cardId, urlDoLink, nomeDoLink);
			urlDoLink = '';
			nomeDoLink = '';
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível anexar o link');
		}
	}

	async function enviarArquivo(evento: Event) {
		const entrada = evento.currentTarget as HTMLInputElement;
		const arquivo = entrada.files?.[0];
		if (!arquivo) return;
		enviandoArquivo = true;
		try {
			await anexarArquivo(cardId, arquivo);
			entrada.value = '';
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível enviar o arquivo');
		} finally {
			enviandoArquivo = false;
		}
	}

	async function removerAnexo(anexoId: string, nome: string) {
		if (!confirm(`Apagar o anexo "${nome}"?`)) return;
		try {
			await apagarAnexo(anexoId);
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível apagar o anexo');
		}
	}

	const formatarTamanho = (bytes?: number) =>
		!bytes
			? ''
			: bytes < 1024 * 1024
				? `${Math.round(bytes / 1024)} KB`
				: `${(bytes / 1024 / 1024).toFixed(1)} MB`;

	// O input date precisa de AAAA-MM-DD; a API devolve ISO completo.
	const prazoParaInput = (iso: string | null) => (iso ? iso.slice(0, 10) : '');
</script>

<!-- Escurece o fundo e fecha no clique fora ou no Esc. -->
<svelte:window onkeydown={(e) => e.key === 'Escape' && aoFechar()} />

<div
	class="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/60 p-4 sm:p-8"
	role="presentation"
	onclick={(e) => e.target === e.currentTarget && aoFechar()}
>
	<div
		class="w-full max-w-2xl rounded-lg border border-hairline bg-surface shadow-flutuante"
		role="dialog"
		aria-modal="true"
		aria-label="Detalhes do card"
	>
		{#if carregando}
			<p class="p-8 text-center text-sm text-mute">carregando…</p>
		{:else if !card}
			<div class="p-8 text-center">
				<p class="text-sm text-negativo">{erro || 'card não encontrado'}</p>
				<button class="botao-secundario mt-4 w-auto px-4" onclick={aoFechar}>Fechar</button>
			</div>
		{:else}
			<header class="flex items-start justify-between gap-4 border-b border-hairline p-5">
				<div class="min-w-0 flex-1">
					{#if editandoTexto}
						<form onsubmit={salvarTexto} class="space-y-3">
							<input
								class="campo"
								bind:value={titulo}
								required
								maxlength="200"
								aria-label="Título"
							/>
							<textarea
								class="campo font-mono text-xs"
								bind:value={descricao}
								rows="8"
								maxlength="5000"
								placeholder="Descrição — aceita markdown: **negrito**, listas, [links](https://…)"
								aria-label="Descrição"></textarea>
							<div class="flex gap-2">
								<button type="submit" class="botao w-auto px-4 py-1.5 text-xs">Salvar</button>
								<button
									type="button"
									class="cursor-pointer px-2 text-xs text-mute hover:text-ink"
									onclick={() => (editandoTexto = false)}>Cancelar</button
								>
							</div>
						</form>
					{:else}
						<h2 class="text-lg font-semibold tracking-tight text-ink">{card.titulo}</h2>
						{#if podeEditar}
							<button
								class="mt-1 cursor-pointer text-xs text-mute hover:text-ink"
								onclick={abrirEdicaoDeTexto}>Editar título e descrição</button
							>
						{/if}
					{/if}
				</div>
				<button
					class="shrink-0 cursor-pointer rounded-sm p-1 text-mute hover:text-ink"
					onclick={aoFechar}
					aria-label="Fechar">✕</button
				>
			</header>

			{#if erro}
				<p class="erro-form m-5">{erro}</p>
			{/if}

			<div class="space-y-6 p-5">
				<!-- etiquetas -->
				{#if etiquetasDoQuadro.length > 0}
					<section>
						<h3 class="text-xs font-semibold tracking-widest text-mute uppercase">Etiquetas</h3>
						<div class="mt-2 flex flex-wrap gap-1.5">
							{#each etiquetasDoQuadro as etiqueta (etiqueta.id)}
								<button
									class="etiqueta cor-{etiqueta.cor} {idsAplicados.has(etiqueta.id)
										? ''
										: 'opacity-40'} {podeEditar ? 'cursor-pointer' : 'cursor-default'}"
									disabled={!podeEditar}
									onclick={() => alternarEtiqueta(etiqueta)}
									aria-pressed={idsAplicados.has(etiqueta.id)}
								>
									{etiqueta.nome}
								</button>
							{/each}
						</div>
					</section>
				{/if}

				<!-- cor: propriedade do card, como etiqueta e prazo. Vale por si — a
				     cor é do card, não do momento em que ele foi criado — e por isso
				     grava no clique, sem passar pelo formulário de editar texto. -->
				<section>
					<h3 class="text-xs font-semibold tracking-widest text-mute uppercase">Cor</h3>
					<div class="mt-2">
						{#if podeEditar}
							<SeletorDeCor cor={card.cor} aoEscolher={mudarCor} rotulo="Cor do card" />
						{:else if card.cor}
							<span class="etiqueta cor-{card.cor}">{card.cor}</span>
						{:else}
							<span class="text-xs text-mute">sem cor</span>
						{/if}
					</div>
				</section>

				<!-- prazo -->
				<section>
					<h3 class="text-xs font-semibold tracking-widest text-mute uppercase">Prazo</h3>
					<div class="mt-2 flex flex-wrap items-center gap-2">
						<input
							type="date"
							class="campo w-auto py-1 text-xs"
							value={prazoParaInput(card.prazo)}
							disabled={!podeEditar}
							aria-label="Data de entrega"
							onchange={(e) => mudarPrazo(e.currentTarget.value)}
						/>
						{#if card.prazo}
							<span class="chip" class:chip-neutro={!card.vencido}>
								{card.vencido ? 'vencido' : 'no prazo'}
							</span>
						{:else}
							<span class="text-xs text-mute">sem data</span>
						{/if}
					</div>
				</section>

				<!-- descrição -->
				<section>
					<h3 class="text-xs font-semibold tracking-widest text-mute uppercase">Descrição</h3>
					{#if card.descricao.trim()}
						<!-- O HTML vem do renderizador próprio, que escapa tudo antes de
						     aplicar as marcas — ver lib/markdown.ts -->
						<div class="markdown mt-2">{@html renderizarMarkdown(card.descricao)}</div>
					{:else}
						<p class="mt-2 text-sm text-mute">Sem descrição.</p>
					{/if}
				</section>

				<!-- checklists -->
				<section>
					<h3 class="text-xs font-semibold tracking-widest text-mute uppercase">Checklists</h3>

					<div class="mt-2 space-y-4">
						{#each card.checklists as lista (lista.id)}
							{@const feitos = lista.itens.filter((i) => i.concluido).length}
							<div class="rounded-md border border-hairline bg-surface-elevated p-3">
								<div class="flex items-center justify-between gap-2">
									<b class="text-sm font-medium text-ink">{lista.titulo}</b>
									<div class="flex items-center gap-2">
										<span class="text-xs tabular-nums text-mute">{feitos}/{lista.itens.length}</span
										>
										{#if podeEditar}
											<button
												class="cursor-pointer text-xs text-mute hover:text-negativo"
												onclick={() => removerChecklist(lista.id, lista.titulo)}
												aria-label="Apagar checklist">✕</button
											>
										{/if}
									</div>
								</div>

								<ul class="mt-2 space-y-1">
									{#each lista.itens as item (item.id)}
										<li class="flex items-start gap-2 text-sm">
											<input
												type="checkbox"
												class="mt-0.5 accent-accent"
												checked={item.concluido}
												disabled={!podeEditar}
												aria-label={item.texto}
												onchange={(e) => marcar(item.id, e.currentTarget.checked)}
											/>
											<span
												class="flex-1 {item.concluido ? 'text-mute line-through' : 'text-body'}"
											>
												{item.texto}
											</span>
											{#if podeEditar}
												<button
													class="cursor-pointer text-xs text-mute hover:text-negativo"
													onclick={() => removerItem(item.id)}
													aria-label="Apagar item">✕</button
												>
											{/if}
										</li>
									{/each}
								</ul>

								{#if podeEditar}
									<form onsubmit={(e) => adicionarItem(e, lista.id)} class="mt-2">
										<input
											class="campo py-1 text-xs"
											bind:value={textoDoItem[lista.id]}
											placeholder="+ novo item"
											maxlength="500"
											aria-label="Novo item de {lista.titulo}"
										/>
									</form>
								{/if}
							</div>
						{/each}

						{#if podeEditar}
							<form onsubmit={adicionarChecklist}>
								<input
									class="campo text-xs"
									bind:value={tituloDaChecklist}
									placeholder="+ nova checklist"
									required
									maxlength="120"
									aria-label="Título da nova checklist"
								/>
							</form>
						{/if}
					</div>
				</section>

				<!-- anexos -->
				<section>
					<h3 class="text-xs font-semibold tracking-widest text-mute uppercase">Anexos</h3>

					<ul class="mt-2 space-y-1.5">
						{#each card.anexos as anexo (anexo.id)}
							<li
								class="flex items-center justify-between gap-2 rounded-md border border-hairline bg-surface-elevated px-3 py-2"
							>
								<a
									href={anexo.tipo === 'link' ? anexo.url : urlDoAnexo(anexo.id)}
									target="_blank"
									rel="noopener noreferrer"
									class="min-w-0 flex-1 truncate text-sm text-accent-texto hover:underline"
								>
									{anexo.tipo === 'link' ? '🔗' : '📎'}
									{anexo.nome}
								</a>
								<span class="shrink-0 text-xs text-mute">{formatarTamanho(anexo.tamanho)}</span>
								{#if podeEditar}
									<button
										class="shrink-0 cursor-pointer text-xs text-mute hover:text-negativo"
										onclick={() => removerAnexo(anexo.id, anexo.nome)}
										aria-label="Apagar anexo">✕</button
									>
								{/if}
							</li>
						{/each}
						{#if card.anexos.length === 0}
							<li class="text-sm text-mute">Nada anexado.</li>
						{/if}
					</ul>

					{#if podeEditar}
						<div class="mt-3 space-y-2">
							<form onsubmit={adicionarLink} class="flex flex-wrap gap-2">
								<input
									type="url"
									class="campo min-w-0 flex-1 py-1 text-xs"
									bind:value={urlDoLink}
									placeholder="https://…"
									required
									aria-label="Endereço do link"
								/>
								<input
									class="campo w-32 py-1 text-xs"
									bind:value={nomeDoLink}
									placeholder="rótulo"
									maxlength="255"
									aria-label="Rótulo do link"
								/>
								<button type="submit" class="botao-secundario w-auto px-3 py-1 text-xs">
									Anexar link
								</button>
							</form>

							<label class="block text-xs text-mute">
								<span class="mb-1 block">
									Ou envie um arquivo (até 10 MB — imagem, PDF, texto, CSV, JSON ou ZIP)
								</span>
								<input
									type="file"
									class="campo py-1 text-xs"
									disabled={enviandoArquivo}
									onchange={enviarArquivo}
								/>
							</label>
							{#if enviandoArquivo}
								<p class="text-xs text-mute">enviando…</p>
							{/if}
						</div>
					{/if}
				</section>
			</div>
		{/if}
	</div>
</div>
