<script lang="ts">
	// O campo de comentário com o autocompletar de menções: digitar "@" abre a
	// lista de quem participa do quadro, e escolher alguém escreve o nome
	// completo — a forma que o servidor sempre reconhece como menção, mesmo com
	// dois "Ana" no quadro. Quem escreve o nome à mão também menciona; a lista só
	// ajuda a acertar. A regra está em $lib/mencoes, onde é testada.
	import { tick } from 'svelte';
	import {
		aplicarMencao,
		consultaNoCursor,
		sugestoes,
		type ConsultaDeMencao,
		type Mencionavel
	} from '$lib/mencoes';

	let {
		valor = $bindable(''),
		pessoas,
		rotulo,
		placeholder = '',
		linhas = 2,
		maximo = 2000
	}: {
		valor?: string;
		pessoas: Mencionavel[];
		rotulo: string;
		placeholder?: string;
		linhas?: number;
		maximo?: number;
	} = $props();

	let campo = $state<HTMLTextAreaElement | null>(null);
	let consulta = $state<ConsultaDeMencao | null>(null);
	let destacada = $state(0);
	const opcoes = $derived(consulta ? sugestoes(pessoas, consulta.termo) : []);

	function atualizar() {
		if (!campo || campo.selectionStart !== campo.selectionEnd) {
			consulta = null;
			return;
		}
		consulta = consultaNoCursor(valor, campo.selectionStart);
		destacada = 0;
	}

	async function escolher(p: Mencionavel) {
		if (!campo || !consulta) return;
		const resultado = aplicarMencao(valor, consulta, campo.selectionStart, p.nome);
		valor = resultado.texto;
		consulta = null;
		await tick();
		campo.focus();
		campo.setSelectionRange(resultado.cursor, resultado.cursor);
	}

	// Com a lista aberta, as setas andam por ela e Enter (ou Tab) escolhe; com
	// ela fechada, o campo se comporta como sempre — Enter é quebra de linha.
	function aoTeclar(e: KeyboardEvent) {
		if (!consulta || opcoes.length === 0) return;
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			destacada = (destacada + 1) % opcoes.length;
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			destacada = (destacada - 1 + opcoes.length) % opcoes.length;
		} else if (e.key === 'Enter' || e.key === 'Tab') {
			e.preventDefault();
			void escolher(opcoes[destacada]);
		} else if (e.key === 'Escape') {
			// Esc fecha a lista, e não o modal do card por baixo.
			e.preventDefault();
			e.stopPropagation();
			consulta = null;
		}
	}

	function aoSoltarTecla(e: KeyboardEvent) {
		if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) atualizar();
	}
</script>

<div class="relative">
	<textarea
		bind:this={campo}
		bind:value={valor}
		class="campo text-sm"
		rows={linhas}
		maxlength={maximo}
		{placeholder}
		aria-label={rotulo}
		oninput={atualizar}
		onclick={atualizar}
		onkeydown={aoTeclar}
		onkeyup={aoSoltarTecla}
		onblur={() => (consulta = null)}></textarea>
	{#if consulta && opcoes.length > 0}
		<ul
			role="listbox"
			aria-label="Pessoas para mencionar"
			class="absolute left-0 z-20 mt-1 max-h-48 w-64 max-w-full overflow-y-auto rounded-md border border-hairline bg-surface py-1 shadow-flutuante"
		>
			{#each opcoes as pessoa, i (pessoa.usuarioId)}
				<!-- pointerdown com preventDefault: o campo não perde o foco, e a
				     escolha acontece antes de o blur fechar a lista. -->
				<li
					role="option"
					aria-selected={i === destacada}
					class="cursor-pointer px-3 py-1.5 text-sm {i === destacada
						? 'bg-accent-suave text-accent-texto'
						: 'text-body hover:bg-surface-elevated'}"
					onpointerdown={(e) => {
						e.preventDefault();
						void escolher(pessoa);
					}}
				>
					{pessoa.nome}
				</li>
			{/each}
		</ul>
	{/if}
</div>
