<script lang="ts">
	import IconeRecurso from '$lib/components/IconeRecurso.svelte';
	import PreviaDoQuadro from '$lib/components/PreviaDoQuadro.svelte';
	import { sessao } from '$lib/stores/session.svelte';

	import { PRODUTO } from '$lib/instituicao';

	// Só o que o produto FAZ HOJE, e dito do ponto de vista de quem usa. A página
	// já trouxe selos de "fase 4" e nomes de algoritmo ("Argon2id", "cookie
	// HttpOnly"): o roteiro interno e a engenharia vazando para quem só quer
	// saber se o quadro resolve o trabalho dela. O detalhe técnico mora em docs/.
	const recursos = [
		{
			icone: 'tempo-real',
			titulo: 'Todo mundo vê na hora',
			texto:
				'Uma pessoa move um card e ele se move na tela das outras, sem recarregar. O quadro mostra quem está nele agora.'
		},
		{
			icone: 'notificacao',
			titulo: 'Avisos do que é seu',
			texto:
				'Card que passou a ser seu, menção num comentário, prazo chegando, convite para um quadro: no sino, na hora — e por email, se você não estava olhando.'
		},
		{
			icone: 'arrastar',
			titulo: 'Edição sem atropelo',
			texto:
				'Duas pessoas no mesmo quadro não desfazem o trabalho uma da outra. Quem salva por último é avisado, com o próprio texto preservado.'
		},
		{
			icone: 'conversa',
			titulo: 'Conversa e histórico',
			texto:
				'Comentários com formatação e @menções, e o registro de quem fez o quê e quando — no card e no quadro inteiro.'
		},
		{
			icone: 'organizacao',
			titulo: 'Etiquetas, prazos e responsáveis',
			texto:
				'Checklists, anexos, cores e data de entrega. O filtro responde "o que é meu?" sem sair do quadro.'
		},
		{
			icone: 'papeis',
			titulo: 'Acesso sob controle',
			texto:
				'Dono, editor e leitor. Só entra quem foi convidado — ou quem tem o email da empresa, se o dono abrir o quadro a ela —, e quem administra tira o acesso a qualquer momento.'
		}
	] as const;
</script>

<svelte:head>
	<title>{PRODUTO} — quadro Kanban colaborativo em tempo real</title>
	<meta
		name="description"
		content="Quadro Kanban para equipes e empresas: várias pessoas no mesmo quadro, cada mudança aparecendo na hora para todas, com convites, acesso pelo email da empresa, papéis e notificações."
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
			Kanban colaborativo para equipes e empresas: várias pessoas no mesmo quadro, cada movimento
			aparecendo na tela das outras no instante em que acontece.
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
			<span>Convite por email</span><span aria-hidden="true">·</span><span
				>Acesso pelo email da empresa</span
			><span aria-hidden="true">·</span><span>Funciona no celular</span>
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
     tipo de coisa que ninguém procura, mas que decide a adoção de quem procura —
     e por isso fala do que a pessoa ganha, e não do algoritmo que o entrega. -->
<section
	class="mt-4 flex flex-wrap items-center gap-x-8 gap-y-3 rounded-lg border border-hairline bg-surface px-6 py-5"
>
	<b class="text-xs font-semibold tracking-widest text-mute uppercase">Segurança e privacidade</b>
	<span class="text-[0.8125rem] text-body">Cada quadro só é visto por quem participa dele</span>
	<span class="text-[0.8125rem] text-body">Senhas guardadas de forma irreversível</span>
	<span class="text-[0.8125rem] text-body"
		>Trocar a senha encerra a sessão em todos os aparelhos</span
	>
	<a href="/privacidade" class="text-[0.8125rem] font-semibold text-accent-texto hover:underline"
		>Política de privacidade</a
	>
</section>

<!-- Para quem decide pela empresa: como a equipe inteira entra, e o que o plano
     gratuito comporta. Os números são os tetos do domínio (membro.MaximoPorQuadro,
     board.MaximoDeQuadrosPorConta, a cota de anexos) — mudou lá, muda aqui. -->
<section class="mt-14" aria-labelledby="titulo-empresas">
	<h2 id="titulo-empresas" class="text-xs font-semibold tracking-widest text-mute uppercase">
		Para empresas
	</h2>
	<div class="mt-6 grid gap-6 sm:grid-cols-3">
		<div>
			<b class="text-sm font-semibold text-ink">A equipe entra sem convite um a um</b>
			<p class="mt-1.5 text-[0.8125rem] leading-relaxed text-mute">
				Quem cria o quadro com o email da empresa pode abri-lo ao domínio: as colegas com email
				@suaempresa confirmado o encontram no painel e entram com um clique.
			</p>
		</div>
		<div>
			<b class="text-sm font-semibold text-ink">Cada pessoa com a própria conta</b>
			<p class="mt-1.5 text-[0.8125rem] leading-relaxed text-mute">
				Ninguém divide senha. O dono escolhe o papel de quem entra, e quem ele remove não volta
				sozinho — só por um convite novo.
			</p>
		</div>
		<div>
			<b class="text-sm font-semibold text-ink">Sem custo, com limites claros</b>
			<p class="mt-1.5 text-[0.8125rem] leading-relaxed text-mute">
				Até 100 quadros por conta, 100 pessoas por quadro e 1 GiB de anexos por quadro.
			</p>
		</div>
	</div>
</section>

{#if !sessao.usuario}
	<section
		class="mt-14 flex flex-col items-center gap-4 rounded-lg border border-hairline bg-surface px-6 py-12 text-center"
	>
		<h2 class="max-w-md text-2xl font-semibold tracking-tight text-balance text-ink">
			Crie um quadro e chame a equipe
		</h2>
		<p class="max-w-md text-sm leading-relaxed text-mute">
			A conta leva menos de um minuto. O convite chega por email: quem ainda não tem conta cria uma
			pelo próprio link, e quem já tem entra no quadro com a conta de sempre. Use o email da empresa
			para poder abrir os quadros à equipe inteira.
		</p>
		<a href="/cadastro" class="botao mt-1 w-auto px-6">Criar conta</a>
	</section>
{/if}
