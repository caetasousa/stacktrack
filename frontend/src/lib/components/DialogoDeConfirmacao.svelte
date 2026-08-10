<script lang="ts">
	// O diálogo de confirmação, montado UMA vez no layout raiz.
	//
	// Um só para o aplicativo inteiro, alimentado por `$lib/confirmar`: cada tela
	// que apaga alguma coisa continua sendo uma linha de guarda, sem hospedar
	// estado de interface próprio. Ver o cabeçalho daquele arquivo para o porquê
	// de ter substituído o `confirm()` do navegador.
	import { pedidoAberto, responder } from '$lib/confirmar.svelte';

	const pedido = $derived(pedidoAberto());

	// Foco no botão que CANCELA, não no que confirma.
	//
	// É a escolha segura: um Enter perdido no teclado (ou o segundo toque de
	// quem apertou duas vezes) cancela em vez de apagar. Quem quer confirmar
	// alcança o outro botão com um Tab ou com o dedo.
	let botaoCancelar = $state<HTMLButtonElement | null>(null);
	$effect(() => {
		if (pedido) botaoCancelar?.focus();
	});

	// Ver o fundo do ModalDoCard: fechar exige que o APERTO tenha começado no
	// fundo. No celular, o clique de compatibilidade que o toque gera cairia
	// aqui e cancelaria sozinho o diálogo que acabou de abrir.
	let apertouNoFundo = $state(false);
</script>

<svelte:window
	onkeydown={(e) => {
		if (pedido && e.key === 'Escape') responder(false);
	}}
/>

{#if pedido}
	<div
		class="fixed inset-0 z-[60] flex items-center justify-center bg-black/60 p-4"
		role="presentation"
		onpointerdown={(e) => (apertouNoFundo = e.target === e.currentTarget)}
		onclick={(e) => {
			if (apertouNoFundo && e.target === e.currentTarget) responder(false);
			apertouNoFundo = false;
		}}
	>
		<div
			class="w-full max-w-sm rounded-lg border border-hairline bg-surface p-5 shadow-flutuante"
			role="alertdialog"
			aria-modal="true"
			aria-labelledby="confirmacao-titulo"
			aria-describedby={pedido.detalhe ? 'confirmacao-detalhe' : undefined}
		>
			<h2 id="confirmacao-titulo" class="text-base font-semibold text-ink">{pedido.titulo}</h2>
			{#if pedido.detalhe}
				<p id="confirmacao-detalhe" class="mt-2 text-sm text-mute">{pedido.detalhe}</p>
			{/if}

			<!-- Cancelar à esquerda e a ação à direita, e os dois do mesmo tamanho:
			     a posição é a pista de qual é qual, não o tamanho. -->
			<div class="mt-5 flex gap-2">
				<button bind:this={botaoCancelar} class="botao-secundario" onclick={() => responder(false)}>
					Cancelar
				</button>
				<button
					class={pedido.destrutivo === false ? 'botao' : 'botao-perigo'}
					onclick={() => responder(true)}
				>
					{pedido.acao}
				</button>
			</div>
		</div>
	</div>
{/if}
