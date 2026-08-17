<script lang="ts">
	// O painel de compartilhamento público de um quadro.
	//
	// Só o dono chega aqui, e a razão está na API, não nesta tela: o token é o
	// segredo do link, e quem o recebe pode repassá-lo a quem quiser. Esconder o
	// botão é conveniência — GET /boards/{id}/publicacao recusa um editor com 403
	// mesmo que alguém chame a rota na mão.
	import { confirmar } from '$lib/confirmar.svelte';
	import { ApiError } from '$lib/api/client';
	import { despublicar, obterPublicacao, publicar, type Publicacao } from '$lib/api/publicacao';

	let {
		boardId,
		aoFechar,
		aoMudar
	}: {
		boardId: string;
		aoFechar: () => void;
		// Chamado quando o estado muda, para o quadro atrás recarregar e o aviso
		// de "está público" acompanhar sem a pessoa precisar atualizar a página.
		aoMudar: () => Promise<void>;
	} = $props();

	let publicacao = $state<Publicacao | null>(null);
	let carregando = $state(true);
	let ocupado = $state(false);
	let erro = $state('');
	// Some sozinho: é confirmação de uma ação instantânea, não um estado.
	let copiado = $state(false);

	// O estado do link é buscado ao abrir, e não vem com o quadro: o token não
	// viaja no GET do quadro justamente para não sair para quem não é dono.
	$effect(() => {
		const id = boardId;
		let vivo = true;
		obterPublicacao(id)
			.then((p) => vivo && (publicacao = p))
			.catch((e) => vivo && (erro = mensagem(e, 'não foi possível ler o compartilhamento')))
			.finally(() => vivo && (carregando = false));
		return () => (vivo = false);
	});

	function mensagem(e: unknown, padrao: string): string {
		return e instanceof ApiError ? e.message : padrao;
	}

	async function ligar() {
		ocupado = true;
		erro = '';
		try {
			publicacao = await publicar(boardId);
			await aoMudar();
		} catch (e) {
			erro = mensagem(e, 'não foi possível publicar o quadro');
		} finally {
			ocupado = false;
		}
	}

	async function desligar() {
		// Confirmação com o efeito dito por inteiro: desligar não é uma chave que
		// se religa no mesmo lugar. Quem religar vai receber um endereço novo, e
		// quem estiver com o antigo fica de fora — inclusive as pessoas certas.
		const ok = await confirmar({
			titulo: 'Desativar o link público?',
			detalhe:
				'Quem tiver o endereço deixa de conseguir abrir na hora. Publicar de novo gera um link DIFERENTE — o atual não volta a funcionar.',
			acao: 'Desativar o link'
		});
		if (!ok) return;

		ocupado = true;
		erro = '';
		try {
			await despublicar(boardId);
			publicacao = { publicado: false };
			copiado = false;
			await aoMudar();
		} catch (e) {
			erro = mensagem(e, 'não foi possível desativar o link');
		} finally {
			ocupado = false;
		}
	}

	async function copiar() {
		if (!publicacao?.url) return;
		try {
			await navigator.clipboard.writeText(publicacao.url);
			copiado = true;
			setTimeout(() => (copiado = false), 2000);
		} catch {
			// Área de transferência negada (permissão, ou página fora de HTTPS): o
			// campo ao lado é selecionável, então há um caminho manual — dizer isso
			// é melhor do que um botão que não faz nada.
			erro = 'não foi possível copiar; selecione o endereço e copie à mão';
		}
	}

	// Mesmo tratamento do ModalDoCard: fechar exige que o APERTO tenha começado
	// no fundo. No celular, o clique de compatibilidade que o toque gera cairia
	// aqui e fecharia sozinho o painel que acabou de abrir.
	let apertouNoFundo = $state(false);
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && aoFechar()} />

<div
	class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
	role="presentation"
	onpointerdown={(e) => (apertouNoFundo = e.target === e.currentTarget)}
	onclick={(e) => {
		if (apertouNoFundo && e.target === e.currentTarget) aoFechar();
		apertouNoFundo = false;
	}}
>
	<div
		class="w-full max-w-lg rounded-lg border border-hairline bg-surface p-6 shadow-flutuante"
		role="dialog"
		aria-modal="true"
		aria-labelledby="compartilhar-titulo"
	>
		<div class="flex items-start justify-between gap-4">
			<div>
				<h2 id="compartilhar-titulo" class="text-base font-semibold text-ink">
					Compartilhar o andamento
				</h2>
				<p class="mt-1 text-sm text-mute">
					Um endereço só de leitura, para quem precisa acompanhar sem entrar na equipe.
				</p>
			</div>
			<button onclick={aoFechar} class="cursor-pointer text-mute hover:text-ink" aria-label="Fechar"
				>×</button
			>
		</div>

		{#if carregando}
			<p class="mt-6 text-sm text-mute">carregando…</p>
		{:else if publicacao?.publicado}
			<div class="mt-5 rounded-md border border-hairline bg-canvas p-4">
				<div class="flex items-center gap-2">
					<i class="size-1.5 rounded-full bg-positivo"></i>
					<b class="text-sm font-semibold text-ink">O link está ativo</b>
				</div>

				<div class="mt-3 flex flex-wrap gap-2">
					<!-- readonly, e não disabled: o endereço precisa continuar
					     selecionável para quem não puder usar a área de transferência. -->
					<input
						class="campo min-w-0 flex-1 font-mono text-xs"
						value={publicacao.url}
						readonly
						onfocus={(e) => e.currentTarget.select()}
						aria-label="Endereço público do quadro"
					/>
					<button onclick={copiar} class="botao w-auto px-4 text-sm">
						{copiado ? 'Copiado' : 'Copiar'}
					</button>
				</div>

				<a
					href={publicacao.url}
					target="_blank"
					rel="noopener noreferrer"
					class="mt-3 inline-block text-xs text-mute underline-offset-2 hover:text-ink hover:underline"
				>
					Abrir como quem recebe o link
				</a>
			</div>

			<!-- O que sai e o que não sai, à vista antes de o link ser enviado. É a
			     parte que decide se a pessoa vai publicar: sem ela, o dono publica
			     sem saber se está expondo a conversa do card. -->
			<dl class="mt-4 space-y-1.5 text-xs">
				<div class="flex gap-2">
					<dt class="w-24 shrink-0 text-mute">Fica visível</dt>
					<dd class="text-body">colunas, cards, descrições, etiquetas, prazos e checklists</dd>
				</div>
				<div class="flex gap-2">
					<dt class="w-24 shrink-0 text-mute">Fica fora</dt>
					<dd class="text-body">comentários, anexos, histórico e quem trabalha em cada card</dd>
				</div>
				<div class="flex gap-2">
					<dt class="w-24 shrink-0 text-mute">Quem entra</dt>
					<dd class="text-body">qualquer pessoa com o endereço, sem conta e sem convite</dd>
				</div>
			</dl>

			<button
				onclick={desligar}
				disabled={ocupado}
				class="botao-secundario mt-5 hover:text-negativo"
			>
				Desativar o link público
			</button>
		{:else}
			<p class="mt-5 text-sm text-body">
				Este quadro é privado. Só quem participa dele consegue abri-lo.
			</p>
			<p class="mt-2 text-xs text-mute">
				Ao ativar, qualquer pessoa com o endereço vê as colunas, os cards e as descrições — sem
				conta e sem convite. Comentários, anexos, histórico e os nomes de quem trabalha nos cards
				não vão junto.
			</p>
			<button onclick={ligar} disabled={ocupado} class="botao mt-5">
				{ocupado ? 'Ativando…' : 'Ativar o link público'}
			</button>
		{/if}

		{#if erro}
			<p class="erro-form mt-4">{erro}</p>
		{/if}
	</div>
</div>
