<script lang="ts">
	import { confirmar } from '$lib/confirmar.svelte';
	import { invalidateAll } from '$app/navigation';
	import { ApiError } from '$lib/api/client';
	import type { Papel } from '$lib/api/boards';
	import {
		abrirDominio,
		alterarPapel,
		convidar,
		fecharDominio,
		reenviarConvite,
		removerMembro,
		revogarConvite,
		type ConviteCriado,
		type ConvitePendente
	} from '$lib/api/membros';

	let { data } = $props();

	let email = $state('');
	let papel = $state<Papel>('editor');
	let erro = $state('');
	let enviando = $state(false);
	// O resultado do último convite (ou reenvio) feito nesta tela. O link
	// aparece só aqui: a API devolve o token nesta resposta e no email, e o
	// banco guarda apenas o hash — quem perdê-lo reenvia, e ganha outro.
	let recemCriado = $state<ConviteCriado | null>(null);
	let foiReenvio = $state(false);
	let copiado = $state(false);
	// Qual convite está sendo reenviado, para travar só o botão dele.
	let reenviando = $state('');

	const souDono = $derived(data.quadro.papel === 'dono');

	const papeis: { valor: Papel; rotulo: string; explica: string }[] = [
		{ valor: 'dono', rotulo: 'Dono', explica: 'pode tudo, inclusive apagar o quadro' },
		{ valor: 'editor', rotulo: 'Editor', explica: 'mexe em colunas e cards' },
		{ valor: 'leitor', rotulo: 'Leitor', explica: 'só enxerga' }
	];

	async function recarregar() {
		erro = '';
		await invalidateAll();
	}

	// O email sai DEPOIS da resposta do convite: quem manda é o worker do
	// outbox, segundos após o commit. A lista que a tela recarregou logo em
	// seguida ainda diz "na fila" — e ficaria dizendo até alguém dar F5,
	// fazendo o dono achar que o email nunca saiu. Enquanto houver email na
	// fila, a lista se relê sozinha, com prazo: o worker tenta de novo por até
	// uma hora, e a tela não precisa acompanhar isso inteiro.
	const ESPERA_ENTRE_LEITURAS_MS = 3000;
	const LEITURAS_MAXIMAS = 20;
	let leiturasDaFila = 0;
	$effect(() => {
		// Lê a LISTA, e não um booleano derivado dela: cada releitura traz uma
		// lista nova, e é isso que agenda a próxima — um "ainda há email na fila"
		// que continua verdadeiro não faria o efeito rodar de novo.
		const naFila = data.participacao.convites.some((c) => c.entrega?.estado === 'pendente');
		if (!naFila || leiturasDaFila >= LEITURAS_MAXIMAS) return;
		const t = setTimeout(() => {
			leiturasDaFila++;
			void invalidateAll();
		}, ESPERA_ENTRE_LEITURAS_MS);
		return () => clearTimeout(t);
	});

	function falhar(e: unknown, padrao: string) {
		erro = e instanceof ApiError ? e.message : padrao;
	}

	async function enviarConvite(evento: SubmitEvent) {
		evento.preventDefault();
		erro = '';
		enviando = true;
		copiado = false;
		try {
			const resultado = await convidar(data.quadro.id, email, papel);
			email = '';
			recemCriado = resultado;
			foiReenvio = false;
			leiturasDaFila = 0;
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível convidar');
		} finally {
			enviando = false;
		}
	}

	async function reenviar(convite: ConvitePendente) {
		erro = '';
		copiado = false;
		reenviando = convite.id;
		try {
			recemCriado = await reenviarConvite(data.quadro.id, convite.id);
			foiReenvio = true;
			leiturasDaFila = 0;
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível reenviar o convite');
		} finally {
			reenviando = '';
		}
	}

	async function copiarLink(link: string) {
		try {
			await navigator.clipboard.writeText(link);
			copiado = true;
		} catch {
			// Sem permissão de área de transferência: o link segue visível na
			// tela para copiar à mão, então não é erro que valha interromper.
			copiado = false;
		}
	}

	async function trocarPapel(usuarioId: string, novo: Papel) {
		try {
			await alterarPapel(data.quadro.id, usuarioId, novo);
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível trocar o papel');
			await recarregar();
		}
	}

	async function remover(usuarioId: string, nome: string) {
		const ok = await confirmar({
			titulo: `Remover ${nome} do quadro?`,
			detalhe: 'A pessoa perde o acesso na hora. O que ela escreveu fica.',
			acao: 'Remover'
		});
		if (!ok) return;
		try {
			await removerMembro(data.quadro.id, usuarioId);
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível remover');
		}
	}

	// --- a porta do domínio -----------------------------------------------------
	//
	// O dono abre o quadro a quem tem email confirmado do domínio dele: a colega
	// encontra o quadro no painel e entra com um clique. O domínio não é
	// digitado — é o do email do dono, e é isso que prova que a empresa é dele.
	let papelDoDominio = $state<'editor' | 'leitor'>('editor');
	let mexendoNaPorta = $state(false);

	async function abrirPorta() {
		erro = '';
		mexendoNaPorta = true;
		try {
			await abrirDominio(data.quadro.id, papelDoDominio);
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível abrir o quadro à empresa');
		} finally {
			mexendoNaPorta = false;
		}
	}

	async function fecharPorta(dominio: string) {
		const ok = await confirmar({
			titulo: `Fechar o quadro a @${dominio}?`,
			detalhe:
				'Quem ainda não entrou deixa de encontrá-lo. Quem já entrou continua — para tirar alguém, use Remover.',
			acao: 'Fechar'
		});
		if (!ok) return;
		erro = '';
		mexendoNaPorta = true;
		try {
			await fecharDominio(data.quadro.id);
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível fechar o quadro à empresa');
		} finally {
			mexendoNaPorta = false;
		}
	}

	async function revogar(conviteId: string, alvo: string) {
		const ok = await confirmar({
			titulo: `Revogar o convite de ${alvo}?`,
			detalhe: 'O link enviado deixa de funcionar.',
			acao: 'Revogar'
		});
		if (!ok) return;
		try {
			await revogarConvite(data.quadro.id, conviteId);
			recemCriado = null;
			await recarregar();
		} catch (e) {
			falhar(e, 'não foi possível revogar o convite');
		}
	}

	const formatarData = (iso: string) =>
		new Date(iso).toLocaleDateString('pt-BR', { day: '2-digit', month: 'short' });
	const formatarDataHora = (iso: string) =>
		new Date(iso).toLocaleString('pt-BR', {
			day: '2-digit',
			month: 'short',
			hour: '2-digit',
			minute: '2-digit'
		});

	// O que a lista diz sobre o email de cada convite. O dono precisa saber se
	// a pessoa já recebeu, ou se ele mesmo precisa mandar o link por outro meio.
	function situacaoDoEmail(c: ConvitePendente): { texto: string; ruim: boolean } {
		switch (c.entrega?.estado) {
			case 'enviado':
				return {
					texto: c.entrega.enviadoEm
						? `email enviado em ${formatarDataHora(c.entrega.enviadoEm)}`
						: 'email enviado',
					ruim: false
				};
			case 'pendente':
				return { texto: 'email na fila', ruim: false };
			case 'falhou':
				return { texto: 'o email não saiu — confira o endereço e reenvie', ruim: true };
			default:
				return { texto: 'sem email — entregue o link', ruim: false };
		}
	}
</script>

<svelte:head><title>Membros · {data.quadro.titulo} · stacktrack</title></svelte:head>

<div class="flex flex-wrap items-baseline justify-between gap-3">
	<div>
		<h1 class="text-xl font-semibold tracking-tight text-ink">Membros</h1>
		<p class="mt-0.5 text-sm text-mute">{data.quadro.titulo}</p>
	</div>
	<a href="/painel/quadros/{data.quadro.id}" class="text-xs text-mute hover:text-ink">
		Voltar ao quadro
	</a>
</div>

{#if erro}
	<p class="erro-form mt-5">{erro}</p>
{/if}

{#if souDono}
	<section class="mt-6 rounded-lg border border-hairline bg-surface p-5 shadow-ficha">
		<h2 class="text-sm font-semibold text-ink">Convidar por email</h2>
		<p class="mt-1 text-xs text-mute">
			A pessoa recebe um email com o link do convite. Se ainda não tiver conta, ela cria uma pelo
			próprio link, com esse mesmo email; se já tiver, é só entrar. O quadro aparece para ela em
			"Compartilhados com você".
		</p>

		<form onsubmit={enviarConvite} class="mt-4 flex flex-wrap gap-2">
			<input
				type="email"
				class="campo min-w-0 flex-1"
				bind:value={email}
				placeholder="email@exemplo.com"
				required
				maxlength="255"
				aria-label="Email de quem você quer convidar"
			/>
			<select class="campo w-auto shrink-0" bind:value={papel} aria-label="Papel">
				{#each papeis as opcao (opcao.valor)}
					<option value={opcao.valor}>{opcao.rotulo}</option>
				{/each}
			</select>
			<button type="submit" class="botao w-auto shrink-0 px-5" disabled={enviando}>
				{enviando ? 'Enviando…' : 'Enviar convite'}
			</button>
		</form>

		<p class="mt-2 text-xs text-mute">
			{papeis.find((p) => p.valor === papel)?.explica}
		</p>

		{#if recemCriado}
			<div class="mt-4 rounded-md border border-accent bg-accent-suave p-4" role="status">
				<p class="text-sm font-semibold text-accent-texto">
					{#if recemCriado.entrega === 'email'}
						{foiReenvio ? 'Convite reenviado' : 'Convite enviado'} para {recemCriado.convite.email}
					{:else}
						Convite {foiReenvio ? 'renovado' : 'criado'} para {recemCriado.convite.email}
					{/if}
				</p>
				<p class="mt-1 text-xs text-body">
					{#if recemCriado.entrega === 'email'}
						O email sai em instantes. O convite vale até {formatarData(
							recemCriado.convite.expiraEm
						)}.
					{:else if recemCriado.entrega === 'cota'}
						O limite de emails de convite foi atingido por hoje. Copie o link abaixo e mande você
						mesmo.
					{:else}
						O envio de email não está ligado neste ambiente. Copie o link abaixo e mande por onde
						preferir.
					{/if}
					{#if foiReenvio}
						O link anterior deixou de valer.
					{/if}
				</p>
				<p class="mt-3 text-xs text-mute">
					{recemCriado.entrega === 'email'
						? 'Prefere mandar você mesmo? O link só aparece agora:'
						: 'O link só aparece agora — se perder, reenvie e ganhe outro:'}
				</p>
				<div class="mt-1.5 flex flex-wrap items-center gap-2">
					<code
						class="min-w-0 flex-1 overflow-x-auto rounded-sm border border-hairline-strong bg-surface px-2 py-1.5 text-xs whitespace-nowrap text-body"
					>
						{recemCriado.link}
					</code>
					<button
						type="button"
						class="botao-secundario w-auto shrink-0 px-4 py-1.5 text-xs"
						onclick={() => copiarLink(recemCriado!.link)}
					>
						{copiado ? 'Copiado' : 'Copiar'}
					</button>
				</div>
			</div>
		{/if}
	</section>
{/if}

{#if souDono && data.porta}
	{@const porta = data.porta}
	<section
		class="mt-6 rounded-lg border border-hairline bg-surface p-5 shadow-ficha"
		aria-labelledby="titulo-empresa"
	>
		<h2 id="titulo-empresa" class="text-sm font-semibold text-ink">Aberto à empresa</h2>
		{#if porta.dominio}
			<p class="mt-1 text-xs text-mute">
				Quem tem email <b class="text-ink">@{porta.dominio}</b> confirmado encontra este quadro no
				painel e entra sem convite, como <b class="text-ink">{porta.papel}</b>.
			</p>
			<button
				type="button"
				class="botao-secundario mt-4 w-auto px-4 py-1.5 text-xs"
				disabled={mexendoNaPorta}
				onclick={() => fecharPorta(porta.dominio)}
			>
				Fechar à empresa
			</button>
		{:else if porta.disponivel}
			<p class="mt-1 text-xs text-mute">
				Abra o quadro a quem tem email <b class="text-ink">@{porta.disponivel}</b> confirmado: a pessoa
				encontra o quadro no painel e entra sem precisar de convite. Quem você remover não volta sozinho.
			</p>
			<div class="mt-4 flex flex-wrap items-center gap-2">
				<select
					class="campo w-auto shrink-0"
					bind:value={papelDoDominio}
					aria-label="Papel de quem entra pela empresa"
				>
					<option value="editor">Editor</option>
					<option value="leitor">Leitor</option>
				</select>
				<button
					type="button"
					class="botao w-auto shrink-0 px-5"
					disabled={mexendoNaPorta}
					onclick={abrirPorta}
				>
					Abrir para @{porta.disponivel}
				</button>
			</div>
		{:else if porta.impedimento === 'email_nao_confirmado'}
			<p class="mt-1 text-xs text-mute">
				Confirme o seu email para abrir este quadro a quem tem o mesmo domínio que você — é a
				confirmação que prova que o domínio é seu.
			</p>
		{:else}
			<p class="mt-1 text-xs text-mute">
				Para abrir o quadro às pessoas da sua empresa, use uma conta com o email da empresa. Emails
				pessoais (Gmail, Outlook…) não podem ser abertos: o quadro ficaria aberto a qualquer pessoa
				com conta lá.
			</p>
		{/if}
	</section>
{/if}

<section class="mt-8">
	<h2 class="text-xs font-semibold tracking-widest text-mute uppercase">
		Quem participa ({data.participacao.membros.length})
	</h2>

	<ul
		class="mt-3 divide-y divide-hairline overflow-hidden rounded-lg border border-hairline bg-surface"
	>
		{#each data.participacao.membros as membro (membro.usuarioId)}
			<li class="flex flex-wrap items-center justify-between gap-3 p-4">
				<div class="min-w-0">
					<b class="block text-sm font-medium text-ink">
						{membro.nome}
						{#if membro.usuarioId === data.usuarioId}
							<span class="text-xs font-normal text-mute">(você)</span>
						{/if}
					</b>
					<span class="block truncate text-xs text-mute">
						{membro.email} · desde {formatarData(membro.desdeEm)}
					</span>
				</div>

				<div class="flex shrink-0 items-center gap-3">
					{#if souDono}
						<select
							class="campo w-auto py-1 text-xs"
							value={membro.papel}
							aria-label="Papel de {membro.nome}"
							onchange={(e) => trocarPapel(membro.usuarioId, e.currentTarget.value as Papel)}
						>
							{#each papeis as opcao (opcao.valor)}
								<option value={opcao.valor}>{opcao.rotulo}</option>
							{/each}
						</select>
						<button
							class="cursor-pointer text-xs text-mute hover:text-negativo"
							onclick={() => remover(membro.usuarioId, membro.nome)}
						>
							Remover
						</button>
					{:else}
						<span class="chip" class:chip-neutro={membro.papel !== 'dono'}>{membro.papel}</span>
					{/if}
				</div>
			</li>
		{/each}
	</ul>
</section>

{#if souDono && data.participacao.convites.length > 0}
	<section class="mt-8">
		<h2 class="text-xs font-semibold tracking-widest text-mute uppercase">
			Convites pendentes ({data.participacao.convites.length})
		</h2>

		<ul
			class="mt-3 divide-y divide-hairline overflow-hidden rounded-lg border border-hairline bg-surface"
		>
			{#each data.participacao.convites as convite (convite.id)}
				{@const situacao = situacaoDoEmail(convite)}
				<li class="flex flex-wrap items-center justify-between gap-3 p-4">
					<div class="min-w-0">
						<b class="block truncate text-sm font-medium text-ink">{convite.email}</b>
						<span class="block text-xs text-mute">
							{convite.papel} ·
							{#if convite.expirado}
								<span class="text-negativo">expirou — reenvie para renovar</span>
							{:else}
								vale até {formatarData(convite.expiraEm)}
							{/if}
						</span>
						<span
							class="block text-xs"
							class:text-negativo={situacao.ruim}
							class:text-mute={!situacao.ruim}
						>
							{situacao.texto}
						</span>
					</div>
					<div class="flex shrink-0 items-center gap-3">
						<button
							class="cursor-pointer text-xs text-mute hover:text-ink disabled:cursor-default disabled:opacity-60"
							onclick={() => reenviar(convite)}
							disabled={reenviando === convite.id}
							aria-label="Reenviar o convite de {convite.email}"
						>
							{reenviando === convite.id ? 'Reenviando…' : 'Reenviar'}
						</button>
						<button
							class="cursor-pointer text-xs text-mute hover:text-negativo"
							onclick={() => revogar(convite.id, convite.email)}
							aria-label="Revogar o convite de {convite.email}"
						>
							Revogar
						</button>
					</div>
				</li>
			{/each}
		</ul>
	</section>
{/if}
