<script lang="ts">
	import { confirmar } from '$lib/confirmar.svelte';
	// O modal do card: descrição em markdown, etiquetas, prazo, checklists e
	// anexos. É onde cabe o que não cabe no card da coluna.
	import { ApiError } from '$lib/api/client';
	import { editarCard, type Cor, type Etiqueta } from '$lib/api/boards';
	import SeletorDeCor from './SeletorDeCor.svelte';
	import {
		anexarArquivo,
		anexarLink,
		aplicarEtiqueta,
		atribuir,
		desatribuir,
		apagarAnexo,
		apagarChecklist,
		apagarComentario,
		atividadeDoCard,
		comentar,
		editarComentario,
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
	import { listarParticipacao, type Membro } from '$lib/api/membros';
	import { iniciais } from '$lib/iniciais';
	import { renderizarMarkdown } from '$lib/markdown';
	import { sessao } from '$lib/stores/session.svelte';
	import { descritas, quando as quandoAconteceu, type Atividade } from '$lib/atividade';
	import type { RecarregarProjecaoAtiva } from '$lib/realtime/reconciliacao';

	let {
		cardId,
		etiquetasDoQuadro,
		podeEditar,
		podeAdministrar = false,
		aoFechar,
		aoMudar,
		registrarRecarga
	}: {
		cardId: string;
		etiquetasDoQuadro: Etiqueta[];
		podeEditar: boolean;
		// Só o dono apaga comentário alheio — a mesma regra do servidor.
		podeAdministrar?: boolean;
		aoFechar: () => void;
		aoMudar: () => Promise<void>;
		/** Registra a projeção que a página precisa aguardar antes do ack. */
		registrarRecarga: (recarregar: RecarregarProjecaoAtiva | null) => void;
	} = $props();

	let card = $state<CardDetalhado | null>(null);
	let erro = $state('');
	// conflito é o 409 do bloqueio otimista: outra pessoa gravou primeiro.
	let conflito = $state(false);
	let carregando = $state(true);

	// Título e descrição têm editores SEPARADOS. Antes havia um só, aberto por
	// um link no cabeçalho, e a seção "Descrição" era só leitura — quem via
	// "Sem descrição." não tinha pista nenhuma de como escrever uma.
	let editandoTitulo = $state(false);
	let editandoDescricao = $state(false);
	// A versão que estava na tela quando o editor foi aberto.
	//
	// Não é `card.version` na hora de gravar: com o modal recebendo mudanças ao
	// vivo, o card se atualiza embaixo de quem está escrevendo, e mandar a versão
	// FRESCA faria o servidor aceitar a gravação — apagando em silêncio o que a
	// outra pessoa acabou de escrever. É justamente o que o bloqueio otimista
	// existe para impedir.
	let versaoEmEdicao = $state(0);
	let titulo = $state('');
	let descricao = $state('');
	let tituloDaChecklist = $state('');
	let textoDoItem = $state<Record<string, string>>({});
	let urlDoLink = $state('');
	let nomeDoLink = $state('');
	let enviandoArquivo = $state(false);

	// --- conversa -------------------------------------------------------------
	let textoDoComentario = $state('');
	// Qual comentário está aberto para edição — null é nenhum.
	let comentarioEmEdicao = $state<string | null>(null);
	let textoEmEdicao = $state('');

	async function enviarComentario(evento: SubmitEvent) {
		evento.preventDefault();
		const texto = textoDoComentario.trim();
		if (!texto) return;
		try {
			await comentar(cardId, texto);
			textoDoComentario = '';
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível comentar');
		}
	}

	function abrirEdicaoDeComentario(id: string, texto: string) {
		comentarioEmEdicao = id;
		textoEmEdicao = texto;
	}

	async function salvarComentario(evento: SubmitEvent, id: string) {
		evento.preventDefault();
		try {
			await editarComentario(id, textoEmEdicao);
			comentarioEmEdicao = null;
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível editar o comentário');
		}
	}

	async function removerComentario(id: string) {
		const ok = await confirmar({ titulo: 'Apagar este comentário?', acao: 'Apagar' });
		if (!ok) return;
		try {
			await apagarComentario(id);
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível apagar o comentário');
		}
	}

	// Quem pode mexer em cada comentário. As duas regras são diferentes, e o
	// servidor aplica as mesmas: EDITAR é só do autor — ninguém põe palavras na
	// boca de outra pessoa —, e APAGAR o autor pode no próprio e quem administra
	// o quadro pode em qualquer um.
	const souAutor = (autorId: string) => sessao.usuario?.id === autorId;
	const podeApagarComentario = (autorId: string) => souAutor(autorId) || podeAdministrar;

	// --- histórico ------------------------------------------------------------
	//
	// Carregado sob demanda, e não junto do card: quase toda abertura de card é
	// para ler ou mexer, não para auditar. Trazer o histórico sempre pagaria uma
	// consulta a mais em todas elas.
	let atividade = $state<Atividade[]>([]);
	let historicoAberto = $state(false);
	let carregandoHistorico = $state(false);
	let proximaGeracaoDoHistorico = 0;
	let ultimaGeracaoDoHistorico = 0;

	const linhasDoHistorico = $derived(descritas(atividade));

	async function alternarHistorico() {
		historicoAberto = !historicoAberto;
		if (!historicoAberto || atividade.length > 0) return;
		await recarregarHistorico();
	}

	async function recarregarHistorico(propagar = false) {
		const id = cardId;
		const geracao = ++proximaGeracaoDoHistorico;
		carregandoHistorico = true;
		try {
			const novaAtividade = (await atividadeDoCard(id)).atividade;
			if (cardId === id && geracao >= ultimaGeracaoDoHistorico) {
				atividade = novaAtividade;
				ultimaGeracaoDoHistorico = geracao;
			}
		} catch (e) {
			if (cardId === id && geracao >= ultimaGeracaoDoHistorico) {
				ultimaGeracaoDoHistorico = geracao;
				falhar(e, 'não foi possível carregar o histórico');
			}
			if (propagar) throw e;
		} finally {
			if (cardId === id) carregandoHistorico = false;
		}
	}

	// Data curta: numa conversa o que importa é "quando", não a precisão.
	function quando(iso: string): string {
		return new Date(iso).toLocaleString('pt-BR', {
			day: '2-digit',
			month: 'short',
			hour: '2-digit',
			minute: '2-digit'
		});
	}

	const idsAplicados = $derived(new Set(card?.etiquetasDoCard.map((e) => e.id) ?? []));

	// Quem participa do quadro — é a lista de quem PODE ser responsável, e a
	// mesma regra vale no servidor. Buscada aqui, e não junto com o quadro,
	// porque só o modal precisa dela: a tela do quadro desenha os avatares a
	// partir do que já vem em cada card.
	let membros = $state<Membro[]>([]);
	const responsaveisAtuais = $derived(new Set(card?.responsaveis.map((r) => r.usuarioId) ?? []));

	async function buscarMembros(boardId: string, propagar: boolean): Promise<Membro[]> {
		try {
			return (await listarParticipacao(boardId)).membros;
		} catch (e) {
			if (propagar) throw e;
			// Falhar aqui não estraga o modal: sem a lista, o card continua
			// mostrando quem já é responsável — só não dá para mudar.
			return [];
		}
	}

	async function alternarResponsavel(usuarioId: string) {
		try {
			if (responsaveisAtuais.has(usuarioId)) {
				await desatribuir(cardId, usuarioId);
			} else {
				await atribuir(cardId, usuarioId);
			}
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível mudar o responsável');
		}
	}

	// Respostas podem terminar fora de ordem: a abertura do modal pode ainda
	// estar em voo quando chega a recarga do tempo real. Só a geração mais nova
	// pode escrever na tela, evitando que a resposta antiga desfaça o snapshot
	// que acabou de ser confirmado.
	let proximaGeracao = 0;
	let ultimaGeracaoDoCard = 0;

	async function carregar(
		id: string,
		propagar = false,
		incluirHistorico = cardId === id && historicoAberto
	): Promise<number | undefined> {
		const geracao = ++proximaGeracao;
		const geracaoDoHistorico = incluirHistorico ? ++proximaGeracaoDoHistorico : undefined;
		try {
			const novoCard = await detalharCard(id);
			// Membros vêm depois do snapshot versionado do card. Na mesma origem de
			// dados, essa leitura é no mínimo tão nova quanto `novoCard.revisao`.
			const novosMembros = await buscarMembros(novoCard.boardId, propagar);
			const novaAtividade = incluirHistorico ? (await atividadeDoCard(id)).atividade : undefined;

			if (cardId === id && geracao >= ultimaGeracaoDoCard) {
				card = novoCard;
				membros = novosMembros;
				if (
					novaAtividade &&
					geracaoDoHistorico !== undefined &&
					geracaoDoHistorico >= ultimaGeracaoDoHistorico
				) {
					atividade = novaAtividade;
					ultimaGeracaoDoHistorico = geracaoDoHistorico;
				}
				erro = '';
				ultimaGeracaoDoCard = geracao;
			}
			return novoCard.revisao;
		} catch (e) {
			if (cardId === id && geracao >= ultimaGeracaoDoCard) {
				ultimaGeracaoDoCard = geracao;
				if (geracaoDoHistorico !== undefined) {
					ultimaGeracaoDoHistorico = Math.max(ultimaGeracaoDoHistorico, geracaoDoHistorico);
				}
				erro = e instanceof ApiError ? e.message : 'não foi possível carregar o card';
			}
			if (propagar) throw e;
			return undefined;
		} finally {
			if (cardId === id) carregando = false;
		}
	}

	// A página coordena a ordem: quadro primeiro, esta projeção depois, cursor
	// por último. Recarregar o card aqui em paralelo abriria uma corrida entre
	// duas respostas e permitiria confirmar antes de uma delas falhar.
	async function recarregar() {
		await aoMudar();
	}

	function falhar(e: unknown, padrao: string) {
		erro = e instanceof ApiError ? e.message : padrao;
	}

	// Este efeito é do CARD, e só dele: ele reinicia o modal quando o modal passa
	// a mostrar outro card.
	//
	// ⚠️ Os três argumentos vão EXPLÍCITOS, e isso não é verbosidade. Os valores
	// padrão de `carregar` são `propagar = false` e
	// `incluirHistorico = cardId === id && historicoAberto` — expressões avaliadas
	// na chamada, ou seja, DENTRO deste efeito. Deixar que rodem faz o efeito
	// passar a depender de `historicoAberto`, e aí abrir o histórico o reinicia:
	// o efeito roda de novo, executa o `historicoAberto = false` abaixo e o painel
	// fecha sozinho no mesmo quadro de animação. Na prática o botão de histórico
	// simplesmente não funcionava.
	//
	// `false` é o valor certo dos dois: o card acabou de mudar, o histórico está
	// sendo fechado aqui mesmo, e não há o que incluir.
	$effect(() => {
		const id = cardId;
		carregando = true;
		// O histórico é de OUTRO card agora: mantê-lo mostraria a história errada
		// até a próxima abertura.
		atividade = [];
		historicoAberto = false;
		void carregar(id, false, false);
	});

	// O registro muda de identidade quando muda o card. Assim, se a troca ocorrer
	// durante uma reconciliação, a página percebe o interleaving e relê a nova
	// projeção antes de confirmar.
	$effect(() => {
		const id = cardId;
		const incluirHistorico = historicoAberto;
		const recarregarProjecao: RecarregarProjecaoAtiva = () => carregar(id, true, incluirHistorico);
		registrarRecarga(recarregarProjecao);
		return () => registrarRecarga(null);
	});

	function abrirEdicaoDeTitulo() {
		if (!podeEditar || !card) return;
		titulo = card.titulo;
		versaoEmEdicao = card.version;
		editandoTitulo = true;
	}

	function abrirEdicaoDeDescricao() {
		if (!podeEditar || !card) return;
		descricao = card.descricao;
		versaoEmEdicao = card.version;
		editandoDescricao = true;
	}

	async function salvarTitulo(evento: SubmitEvent) {
		evento.preventDefault();
		if (!card) return;
		await gravarTexto(titulo, card.descricao, () => (editandoTitulo = false));
	}

	async function salvarDescricao(evento: SubmitEvent) {
		evento.preventDefault();
		if (!card) return;
		await gravarTexto(card.titulo, descricao, () => (editandoDescricao = false));
	}

	// gravarTexto é o PATCH compartilhado pelos dois editores.
	//
	// A rota grava o card INTEIRO, então cada editor manda o campo que não está
	// editando com o valor atual — omiti-lo apagaria o outro. A cor vai junto
	// pela mesma razão.
	//
	// A versão é a que estava na tela quando o campo foi aberto: se outra pessoa
	// gravou nesse meio-tempo, o servidor recusa com 409 em vez de deixar este
	// texto apagar o dela.
	async function gravarTexto(novoTitulo: string, novaDescricao: string, aoConcluir: () => void) {
		try {
			await editarCard(cardId, novoTitulo, novaDescricao, card?.cor ?? '', versaoEmEdicao);
			aoConcluir();
			conflito = false;
			await recarregar();
		} catch (e) {
			if (e instanceof ApiError && e.status === 409) {
				// Não fecha o formulário: o texto digitado continua na tela para
				// ser copiado. Fechar aqui perderia o trabalho de quem escreveu —
				// exatamente o que o bloqueio existe para impedir.
				conflito = true;
				return;
			}
			falhar(e, 'não foi possível salvar');
		}
	}

	// recarregarDoServidor traz a versão de quem gravou primeiro e desarma o
	// aviso. Chamado pelo botão do conflito.
	async function trazerVersaoNova() {
		conflito = false;
		editandoTitulo = false;
		editandoDescricao = false;
		await recarregar();
	}

	async function mudarCor(nova: Cor | '') {
		if (!card) return;
		try {
			await editarCard(cardId, card.titulo, card.descricao, nova, card.version);
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
		const ok = await confirmar({
			titulo: `Apagar a checklist "${titulo}"?`,
			detalhe: 'Os itens dela vão junto.',
			acao: 'Apagar a checklist'
		});
		if (!ok) return;
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
		const ok = await confirmar({ titulo: `Apagar o anexo "${nome}"?`, acao: 'Apagar o anexo' });
		if (!ok) return;
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

	// Ver o comentário do fundo, abaixo: fechar depende de onde o gesto COMEÇOU.
	let apertouNoFundo = $state(false);
</script>

<!-- Escurece o fundo e fecha no clique fora ou no Esc. -->
<svelte:window onkeydown={(e) => e.key === 'Escape' && aoFechar()} />

<!-- Fechar exige que o APERTO tenha começado no fundo, e não só que o clique
     tenha terminado nele. Duas coisas dependem disso:

     1. no celular, um toque no card gera DOIS cliques (o sintetizado a partir
        do toque e o de compatibilidade). O primeiro abria este modal e o
        segundo, já com o fundo por cima do card, fechava no mesmo gesto —
        tocar num card não abria nada;
     2. no desktop, selecionar texto DENTRO do modal e soltar o mouse fora
        fechava o modal e perdia a edição em andamento. -->
<div
	class="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/60 p-4 sm:p-8"
	role="presentation"
	onpointerdown={(e) => (apertouNoFundo = e.target === e.currentTarget)}
	onclick={(e) => {
		if (apertouNoFundo && e.target === e.currentTarget) aoFechar();
		apertouNoFundo = false;
	}}
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
					{#if editandoTitulo}
						<form onsubmit={salvarTitulo} class="space-y-3">
							<input
								class="campo"
								bind:value={titulo}
								required
								maxlength="200"
								aria-label="Título"
							/>
							<div class="flex gap-2">
								<button type="submit" class="botao w-auto px-4 py-1.5 text-xs">Salvar</button>
								<button
									type="button"
									class="cursor-pointer px-2 text-xs text-mute hover:text-ink"
									onclick={() => (editandoTitulo = false)}>Cancelar</button
								>
							</div>
						</form>
					{:else}
						<h2 class="text-lg font-semibold tracking-tight text-ink">{card.titulo}</h2>
						{#if podeEditar}
							<button
								class="mt-1 cursor-pointer text-xs text-mute hover:text-ink"
								onclick={abrirEdicaoDeTitulo}>Editar título</button
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

			{#if conflito}
				<!-- O 409 do bloqueio otimista. O texto digitado continua na tela
				     de propósito: fechar o formulário aqui perderia o trabalho de
				     quem escreveu, que é justamente o que o bloqueio impede. -->
				<div class="erro-form m-5">
					<p><strong>Alguém alterou este card enquanto você escrevia.</strong></p>
					<p class="mt-1 text-xs">
						Seu texto continua aí — copie o que precisar antes de trazer a versão nova.
					</p>
					<button
						type="button"
						class="botao-secundario mt-3 w-auto px-3 py-1 text-xs"
						onclick={trazerVersaoNova}
					>
						Trazer a versão atual
					</button>
				</div>
			{/if}
			{#if erro}
				<p class="erro-form m-5">{erro}</p>
			{/if}

			<div class="space-y-6 p-5">
				<!-- responsáveis: quem responde por este card.
				     A lista oferecida é a de MEMBROS do quadro, e não a de contas
				     do sistema — a mesma regra que o servidor aplica. -->
				{#if membros.length > 0}
					<section>
						<h3 class="text-xs font-semibold tracking-widest text-mute uppercase">Responsáveis</h3>
						<div class="mt-2 flex flex-wrap gap-1.5">
							{#each membros as pessoa (pessoa.usuarioId)}
								<button
									class="flex items-center gap-1.5 rounded-full border py-0.5 pr-2.5 pl-0.5 text-xs {responsaveisAtuais.has(
										pessoa.usuarioId
									)
										? 'border-accent bg-accent-suave text-accent-texto'
										: 'border-hairline text-mute'} {podeEditar
										? 'cursor-pointer'
										: 'cursor-default'}"
									disabled={!podeEditar}
									onclick={() => alternarResponsavel(pessoa.usuarioId)}
									aria-pressed={responsaveisAtuais.has(pessoa.usuarioId)}
									title={pessoa.email}
								>
									<span
										class="flex size-5 items-center justify-center rounded-full bg-accent-suave text-[9px] font-semibold text-accent-texto"
									>
										{iniciais(pessoa.nome)}
									</span>
									{pessoa.nome}
								</button>
							{/each}
						</div>
					</section>
				{/if}

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
				     grava no clique, sem passar pelo formulário de editar texto.
				     Sem escolha aqui, o card veste a cor da coluna (ver $lib/paleta):
				     escolher uma é dizer "este card é exceção na etapa". -->
				<section>
					<h3 class="text-xs font-semibold tracking-widest text-mute uppercase">Cor</h3>
					<div class="mt-2">
						{#if podeEditar}
							<SeletorDeCor cor={card.cor} aoEscolher={mudarCor} rotulo="Cor do card" />
						{:else if card.cor}
							<span class="etiqueta cor-{card.cor}">{card.cor}</span>
						{:else}
							<span class="text-xs text-mute">segue a cor da coluna</span>
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

				<!-- descrição: editável AQUI, e não por um link no cabeçalho.
				     Antes esta seção era só leitura, e a única porta de entrada era
				     "Editar título e descrição" lá em cima — longe do campo e fácil
				     de não ver. Quem lia "Sem descrição." não tinha pista nenhuma de
				     que dava para escrever uma. -->
				<section>
					<h3 class="text-xs font-semibold tracking-widest text-mute uppercase">Descrição</h3>

					{#if editandoDescricao}
						<form onsubmit={salvarDescricao} class="mt-2 space-y-2">
							<textarea
								class="campo font-mono text-xs"
								bind:value={descricao}
								rows="8"
								maxlength="5000"
								placeholder="Aceita markdown: **negrito**, listas, [links](https://…)"
								aria-label="Descrição"></textarea>
							<div class="flex gap-2">
								<button type="submit" class="botao w-auto px-4 py-1.5 text-xs">Salvar</button>
								<button
									type="button"
									class="cursor-pointer px-2 text-xs text-mute hover:text-ink"
									onclick={() => (editandoDescricao = false)}>Cancelar</button
								>
							</div>
						</form>
					{:else if card.descricao.trim()}
						<!-- O HTML vem do renderizador próprio, que escapa tudo antes de
						     aplicar as marcas — ver lib/markdown.ts -->
						<div class="markdown mt-2">{@html renderizarMarkdown(card.descricao)}</div>
						{#if podeEditar}
							<button
								class="mt-1 cursor-pointer text-xs text-mute hover:text-ink"
								onclick={abrirEdicaoDeDescricao}>Editar descrição</button
							>
						{/if}
					{:else if podeEditar}
						<!-- O convite é a própria área: um retângulo tracejado do tamanho
						     do que vai ser escrito diz "escreva aqui" sem precisar de
						     rótulo. -->
						<button
							class="mt-2 w-full cursor-pointer rounded-sm border border-dashed border-hairline-strong p-3 text-left text-sm text-mute hover:border-accent hover:text-body"
							onclick={abrirEdicaoDeDescricao}
						>
							+ adicionar uma descrição
						</button>
					{:else}
						<p class="mt-2 text-sm text-mute">Sem descrição.</p>
					{/if}
				</section>

				<!-- conversa: o primeiro fluxo append-only do quadro. Não tem
				     posição nem ordenação — um comentário acontece e fica, e a
				     ordem é a do tempo. -->
				<section>
					<h3 class="text-xs font-semibold tracking-widest text-mute uppercase">
						Comentários{#if card.comentarios.length > 0}
							<span class="ml-1 tabular-nums normal-case">({card.comentarios.length})</span>
						{/if}
					</h3>

					<div class="mt-2 space-y-3">
						{#each card.comentarios as comentario (comentario.id)}
							<div class="flex gap-2">
								<span
									class="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full bg-accent-suave text-[10px] font-semibold text-accent-texto"
									title={comentario.autorNome}
								>
									{iniciais(comentario.autorNome)}
								</span>
								<div class="min-w-0 flex-1">
									<p class="text-xs text-mute">
										<b class="font-semibold text-body">{comentario.autorNome}</b>
										· {quando(comentario.criadoEm)}
										{#if comentario.editadoEm}
											· <span title={quando(comentario.editadoEm)}>editado</span>
										{/if}
									</p>

									{#if comentarioEmEdicao === comentario.id}
										<form
											onsubmit={(e) => salvarComentario(e, comentario.id)}
											class="mt-1 space-y-2"
										>
											<textarea
												class="campo text-sm"
												bind:value={textoEmEdicao}
												rows="3"
												maxlength="2000"
												aria-label="Editar comentário"></textarea>
											<div class="flex gap-2">
												<button type="submit" class="botao w-auto px-3 py-1 text-xs">Salvar</button>
												<button
													type="button"
													class="cursor-pointer px-2 text-xs text-mute hover:text-ink"
													onclick={() => (comentarioEmEdicao = null)}>Cancelar</button
												>
											</div>
										</form>
									{:else}
										<div class="markdown mt-0.5 text-sm">
											{@html renderizarMarkdown(comentario.texto)}
										</div>
										<div class="mt-0.5 flex gap-3 text-xs text-mute">
											{#if souAutor(comentario.autorId)}
												<button
													class="cursor-pointer hover:text-ink"
													onclick={() => abrirEdicaoDeComentario(comentario.id, comentario.texto)}
													>editar</button
												>
											{/if}
											{#if podeApagarComentario(comentario.autorId)}
												<button
													class="cursor-pointer hover:text-negativo"
													onclick={() => removerComentario(comentario.id)}>apagar</button
												>
											{/if}
										</div>
									{/if}
								</div>
							</div>
						{/each}
					</div>

					<!-- Comentar exige só PARTICIPAÇÃO, não papel de edição: acompanhar
					     e responder é ver, não mexer. Por isso não há podeEditar aqui. -->
					<form onsubmit={enviarComentario} class="mt-3 space-y-2">
						<textarea
							class="campo text-sm"
							bind:value={textoDoComentario}
							rows="2"
							maxlength="2000"
							placeholder="Escrever um comentário — aceita markdown"
							aria-label="Novo comentário"></textarea>
						{#if textoDoComentario.trim()}
							<button type="submit" class="botao w-auto px-4 py-1.5 text-xs">Comentar</button>
						{/if}
					</form>
				</section>

				<!-- histórico: um read model sobre o log de eventos, sem tabela
				     própria. Fica recolhido porque é consulta de auditoria — quem
				     abre o card quase sempre quer ler ou mexer, não investigar. -->
				<section>
					<button
						class="cursor-pointer text-xs font-semibold tracking-widest text-mute uppercase hover:text-body"
						onclick={alternarHistorico}
						aria-expanded={historicoAberto}
					>
						Histórico {historicoAberto ? '▾' : '▸'}
					</button>

					{#if historicoAberto}
						{#if carregandoHistorico}
							<p class="mt-2 text-sm text-mute">carregando…</p>
						{:else if linhasDoHistorico.length === 0}
							<p class="mt-2 text-sm text-mute">Nada registrado ainda.</p>
						{:else}
							<ul class="mt-2 space-y-1.5">
								{#each linhasDoHistorico as linha (linha.atividade.seq)}
									<li class="flex gap-2 text-xs text-mute">
										<span
											class="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full bg-surface-elevated text-[9px] font-semibold"
											title={linha.atividade.autorEmail ||
												linha.atividade.autorNome ||
												'conta removida'}
										>
											{iniciais(linha.atividade.autorNome || '?')}
										</span>
										<span>
											<!-- O email no `title` do nome, e não ao lado dele: aqui as linhas são
											     curtas e quase sempre da mesma pessoa, e um email repetido em cada
											     uma afogaria o que aconteceu. No histórico do QUADRO, onde se
											     compara gente, ele aparece por extenso. -->
											<b
												class="font-semibold text-body"
												title={linha.atividade.autorEmail || undefined}
											>
												{linha.atividade.autorNome || 'alguém'}
											</b>
											{linha.texto} · {quandoAconteceu(linha.atividade.ocorridoEm)}
										</span>
									</li>
								{/each}
							</ul>
						{/if}
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
