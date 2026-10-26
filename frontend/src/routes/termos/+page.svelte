<script lang="ts">
	// Os Termos de Uso.
	//
	// ⚠️ Texto-base para revisão jurídica: as obrigações da instituição que
	// oferece o sistema, e o foro, dependem dela.
	import PaginaLegal from '$lib/components/PaginaLegal.svelte';
	import { EMAIL_DE_CONTATO, PRODUTO } from '$lib/instituicao';
</script>

<PaginaLegal titulo="Termos de Uso">
	<p>
		Estes termos regem o uso do {PRODUTO}, um quadro Kanban colaborativo oferecido pela instituição
		que o disponibiliza a você. Ao criar uma conta ou usar o sistema, você concorda com eles e com a
		<a href="/privacidade">Política de Privacidade</a>.
	</p>

	<h2>A sua conta</h2>
	<ul>
		<li>Use o seu próprio nome e um email que seja seu.</li>
		<li>
			A senha é pessoal. Você responde pelo que for feito com a sua conta; se suspeitar que alguém a
			usou, troque a senha — isso encerra a sessão em todos os aparelhos.
		</li>
	</ul>

	<h2>O que você publica</h2>
	<ul>
		<li>
			O conteúdo dos quadros pertence a quem o criou e à equipe que o usa. Você só deve publicar o
			que tem direito de compartilhar com quem participa do quadro.
		</li>
		<li>
			Quem é dono de um quadro decide quem participa, com que papel, e se ele terá um link público
			de acompanhamento. Ao publicar, o dono assume a responsabilidade pelo que fica visível.
		</li>
		<li>
			Não use o sistema para conteúdo ilegal, ofensivo ou que viole direitos de terceiros, nem para
			enviar convites ou mensagens não solicitados.
		</li>
	</ul>

	<h2>Uso aceitável</h2>
	<p>
		Não tente acessar quadros de que você não participa, contornar os limites de uso, interferir no
		funcionamento do serviço ou automatizar acessos de forma abusiva. O acesso de quem descumprir
		estes termos pode ser suspenso.
	</p>

	<h2>Disponibilidade</h2>
	<p>
		O serviço pode ficar indisponível em manutenções ou por falhas. Mantemos cópias de segurança
		periódicas, mas recomendamos não usar o sistema como único lugar de guarda de informações
		críticas.
	</p>

	<h2>Mudanças nestes termos</h2>
	<p>Quando estes termos mudarem, a data no topo da página muda junto.</p>

	<h2>Contato</h2>
	<p>
		{#if EMAIL_DE_CONTATO}
			Dúvidas sobre estes termos: <a href="mailto:{EMAIL_DE_CONTATO}">{EMAIL_DE_CONTATO}</a>.
		{:else}
			Dúvidas sobre estes termos: procure quem administra o acesso ao {PRODUTO} na sua instituição.
		{/if}
	</p>
</PaginaLegal>
