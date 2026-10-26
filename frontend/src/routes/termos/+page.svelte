<script lang="ts">
	// Os Termos de Uso.
	//
	// ⚠️ Texto-base para revisão jurídica: a identificação de quem responde pelo
	// serviço (RESPONSAVEL), o canal de contato e o foro dependem de quem o
	// oferece. Os limites citados são os do domínio — mudou lá, muda aqui.
	import PaginaLegal from '$lib/components/PaginaLegal.svelte';
	import { EMAIL_DE_CONTATO, PRODUTO, RESPONSAVEL } from '$lib/instituicao';
</script>

<PaginaLegal titulo="Termos de Uso">
	<p>
		Estes termos regem o uso do {PRODUTO}, um quadro Kanban colaborativo aberto a pessoas e empresas{#if RESPONSAVEL},
			oferecido por {RESPONSAVEL}{/if}. Ao criar uma conta ou usar o sistema, você concorda com eles
		e com a <a href="/privacidade">Política de Privacidade</a>.
	</p>

	<h2>A sua conta</h2>
	<ul>
		<li>
			Cada conta é de uma pessoa. Use o seu próprio nome e um email que seja seu — quem usa o
			{PRODUTO} pela empresa deve usar o email da empresa.
		</li>
		<li>
			A senha é pessoal e não deve ser compartilhada. Você responde pelo que for feito com a sua
			conta; se suspeitar que alguém a usou, troque a senha — isso encerra a sessão em todos os
			aparelhos.
		</li>
	</ul>

	<h2>Uso por empresas</h2>
	<ul>
		<li>
			Uma empresa adere ao {PRODUTO} quando as pessoas dela criam as próprias contas com o email da empresa.
			Não há conta compartilhada nem conta da empresa: cada quadro pertence a quem é dono dele.
		</li>
		<li>
			O dono de um quadro pode abri-lo ao domínio do próprio email (por exemplo,
			@suaempresa.com.br): quem tiver email confirmado desse domínio passa a encontrá-lo e pode
			entrar sem convite. Ao abrir, o dono declara que tem autorização da empresa para isso.
			Domínios de email pessoal (Gmail, Outlook e semelhantes) não podem ser abertos.
		</li>
		<li>
			Quando alguém deixa a empresa, cabe à empresa tirar essa pessoa dos quadros — e, se ela for a
			única dona de algum, promover outra pessoa a dona antes. Quem é removido de um quadro só volta
			por um convite novo.
		</li>
	</ul>

	<h2>O que você publica</h2>
	<ul>
		<li>
			O conteúdo dos quadros pertence a quem o criou e à equipe que o usa. Você só deve publicar o
			que tem direito de compartilhar com quem participa do quadro.
		</li>
		<li>
			Quem é dono de um quadro decide quem participa, com que papel, se ele fica aberto ao domínio
			da empresa e se terá um link público de acompanhamento. Ao abrir ou publicar, o dono assume a
			responsabilidade pelo que fica visível.
		</li>
		<li>
			Não use o sistema para conteúdo ilegal, ofensivo ou que viole direitos de terceiros, nem para
			enviar convites ou mensagens não solicitados.
		</li>
	</ul>

	<h2>Plano gratuito e limites</h2>
	<p>
		O {PRODUTO} é oferecido sem custo, com limites: até 100 quadros de que uma conta é dona, até 100 participantes
		por quadro e até 1 GiB de anexos por quadro, além dos limites de tamanho e de uso descritos no próprio
		sistema. Planos com outros limites podem vir a ser oferecidos; quando isso acontecer, estes termos
		serão atualizados antes.
	</p>

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

	{#if EMAIL_DE_CONTATO}
		<h2>Contato</h2>
		<p>Dúvidas sobre estes termos: <a href="mailto:{EMAIL_DE_CONTATO}">{EMAIL_DE_CONTATO}</a>.</p>
	{/if}
</PaginaLegal>
