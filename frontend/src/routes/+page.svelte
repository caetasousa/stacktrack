<script lang="ts">
	import IconeRecurso from '$lib/components/IconeRecurso.svelte';
	import PreviaDoQuadro from '$lib/components/PreviaDoQuadro.svelte';
	import { sessao } from '$lib/stores/session.svelte';

	// Só o que o produto FAZ HOJE. A versão anterior desta lista trazia selos de
	// "fase 4", "fase 5" e "fase 6" em recursos que já estavam no ar havia muito
	// tempo — o roteiro interno vazando para a página e vendendo o produto por
	// menos do que ele é. Roteiro mora no PLANO.md.
	const recursos = [
		{
			icone: 'tempo-real',
			titulo: 'Todo mundo vê na hora',
			texto:
				'Uma pessoa move um card e ele se move na tela das outras, sem recarregar. Quem cai e volta recebe o que perdeu enquanto esteve fora.'
		},
		{
			icone: 'presenca',
			titulo: 'Quem está junto, agora',
			texto:
				'O avatar de cada pessoa aparece no quadro que ela abriu, e some quando ela sai. Dá para saber com quem se está trabalhando.'
		},
		{
			icone: 'arrastar',
			titulo: 'Arrastar sem atropelar',
			texto:
				'Mover um card reescreve uma linha, não a coluna inteira. Duas pessoas arrastando ao mesmo tempo não desfazem o trabalho uma da outra.'
		},
		{
			icone: 'conversa',
			titulo: 'Conversa e histórico no card',
			texto:
				'Comentários em markdown, e o registro de quem moveu de onde para onde, quem renomeou o quê e quando.'
		},
		{
			icone: 'organizacao',
			titulo: 'Etiquetas, prazos e responsáveis',
			texto:
				'Checklists, anexos, cores e data de entrega. O filtro responde "o que é meu?" sem sair do quadro.'
		},
		{
			icone: 'papeis',
			titulo: 'Papéis conferidos no servidor',
			texto:
				'Dono, editor e leitor. Convite por link associado ao email, acesso revogável — e a permissão checada a cada operação, não só na tela.'
		}
	] as const;
</script>

<svelte:head>
	<title>stacktrack — quadro Kanban colaborativo em tempo real</title>
	<meta
		name="description"
		content="Quadro Kanban em que várias pessoas movem cards ao mesmo tempo e todas veem na hora, sem recarregar a página."
	/>
</svelte:head>

<!-- Duas colunas a partir de lg: o texto conduz, a prévia mostra. Em telas
     estreitas a prévia vai para baixo do CTA, porque a decisão de criar conta
     não deve depender de rolar. -->
<section
	class="grid items-center gap-10 py-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.05fr)] lg:py-8"
>
	<div class="flex flex-col items-start gap-5">
		<h1 class="max-w-xl text-4xl font-semibold tracking-tight text-balance text-ink sm:text-5xl">
			O quadro que todo mundo vê mudar na hora
		</h1>

		<p class="max-w-lg text-base leading-relaxed text-body">
			Kanban colaborativo de verdade: várias pessoas no mesmo quadro, cada movimento aparecendo na
			tela das outras no instante em que acontece.
		</p>

		<div class="mt-1 flex flex-wrap gap-3">
			{#if sessao.usuario}
				<a href="/painel" class="botao w-auto px-5">Ir para o painel</a>
			{:else}
				<a href="/cadastro" class="botao w-auto px-5">Criar conta</a>
				<a href="/login" class="botao-secundario w-auto px-5">Entrar</a>
			{/if}
		</div>

		<p class="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-mute">
			<span>Grátis</span><span aria-hidden="true">·</span><span>Sem cartão</span><span
				aria-hidden="true">·</span
			><span>Código aberto</span>
		</p>
	</div>

	<PreviaDoQuadro />
</section>

<section class="mt-14">
	<h2 class="text-xs font-semibold tracking-widest text-mute uppercase">O que o quadro faz</h2>

	<ul
		class="mt-6 grid gap-px overflow-hidden rounded-lg border border-hairline bg-hairline sm:grid-cols-2 lg:grid-cols-3"
	>
		{#each recursos as recurso (recurso.titulo)}
			<li class="flex flex-col gap-2.5 bg-surface p-6">
				<span
					class="flex size-9 items-center justify-center rounded-md bg-accent-suave text-accent-texto"
				>
					<IconeRecurso nome={recurso.icone} />
				</span>
				<b class="text-sm font-semibold text-ink">{recurso.titulo}</b>
				<small class="text-[0.8125rem] leading-relaxed text-mute">{recurso.texto}</small>
			</li>
		{/each}
	</ul>
</section>

<!-- A faixa de segurança fica ao lado dos recursos, e não misturada a eles: é o
     tipo de coisa que ninguém procura, mas que decide a adoção de quem procura. -->
<section
	class="mt-4 flex flex-wrap items-center gap-x-8 gap-y-3 rounded-lg border border-hairline bg-surface px-6 py-5"
>
	<b class="text-xs font-semibold tracking-widest text-mute uppercase">Por baixo</b>
	<span class="text-[0.8125rem] text-body">Senhas em Argon2id</span>
	<span class="text-[0.8125rem] text-body">Sessão em cookie HttpOnly</span>
	<span class="text-[0.8125rem] text-body">Token guardado só como hash</span>
	<span class="text-[0.8125rem] text-body">Edição concorrente sem sobrescrita</span>
</section>

{#if !sessao.usuario}
	<section
		class="mt-14 flex flex-col items-center gap-4 rounded-lg border border-hairline bg-surface px-6 py-12 text-center"
	>
		<h2 class="max-w-md text-2xl font-semibold tracking-tight text-balance text-ink">
			Crie um quadro e chame a equipe
		</h2>
		<p class="max-w-md text-sm leading-relaxed text-mute">
			A conta leva menos de um minuto. Quem já tem conta entra direto; para quem ainda não tem, você
			envia o link do convite por onde preferir.
		</p>
		<a href="/cadastro" class="botao mt-1 w-auto px-6">Criar conta</a>
	</section>
{/if}
