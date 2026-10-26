<script lang="ts">
	// A Política de Privacidade. Descreve SÓ o que o sistema faz de fato — os
	// prazos e os destinos abaixo são os do código e da infraestrutura (ver
	// docs/funcionalidades.md e docs/producao.md). Mudou um prazo lá, muda aqui.
	//
	// ⚠️ Texto-base para revisão jurídica: a identificação do controlador
	// (RESPONSAVEL) e o canal do encarregado (EMAIL_DE_CONTATO) precisam estar
	// preenchidos em $lib/instituicao antes de o texto valer.
	import PaginaLegal from '$lib/components/PaginaLegal.svelte';
	import { EMAIL_DE_CONTATO, PRODUTO, RESPONSAVEL } from '$lib/instituicao';
</script>

<PaginaLegal titulo="Política de Privacidade">
	<p>
		Esta política explica quais dados pessoais o {PRODUTO} trata, para quê, por quanto tempo e como você
		exerce os seus direitos previstos na Lei Geral de Proteção de Dados (Lei nº 13.709/2018).
	</p>

	<h2>Quem é responsável pelos dados</h2>
	<ul>
		<li>
			<b>Dados da sua conta</b> (nome, email, senha, acessos): o {PRODUTO}{#if RESPONSAVEL},
				oferecido por {RESPONSAVEL}{/if}, é o controlador — decide como esses dados são tratados,
			nos termos desta política.
		</li>
		<li>
			<b>Conteúdo dos quadros de uma empresa:</b> quando uma empresa usa o {PRODUTO} para o trabalho da
			equipe, ela decide o que vai nos quadros e quem participa deles — é a controladora desse conteúdo,
			e o {PRODUTO} o trata em nome dela, só para prestar o serviço. Pedidos sobre esse conteúdo podem
			ser feitos também à própria empresa.
		</li>
	</ul>

	<h2>Que dados tratamos</h2>
	<ul>
		<li>
			<b>Conta:</b> nome, email e senha. A senha nunca é guardada em texto: guardamos apenas uma versão
			embaralhada dela, que não permite recuperá-la.
		</li>
		<li>
			<b>O que você escreve no sistema:</b> quadros, colunas, cards, descrições, comentários, checklists,
			etiquetas, prazos e arquivos anexados, além do registro de quem fez cada alteração e quando (o histórico
			do quadro).
		</li>
		<li>
			<b>Convites:</b> o email de quem é convidado para um quadro. No histórico, ele aparece mascarado
			(a***@exemplo.com).
		</li>
		<li>
			<b>Acesso pelo domínio da empresa:</b> se o dono de um quadro o abre ao domínio do email dele, o
			domínio fica registrado no quadro; e quem o dono remove de um quadro fica registrado, para não poder
			voltar sozinho.
		</li>
		<li>
			<b>Notificações:</b> os avisos gerados para você — convites, cards atribuídos, menções e prazos.
		</li>
		<li>
			<b>Dados técnicos de acesso:</b> endereço IP, data e hora, endereço acessado e o tipo de navegador,
			nos registros do servidor, e o cookie de sessão que mantém você conectado.
		</li>
	</ul>

	<h2>Para que usamos</h2>
	<ul>
		<li>Prestar o serviço que você usa: autenticar o acesso, mostrar e guardar os seus quadros.</li>
		<li>Avisar você, no sistema e por email, sobre convites, atribuições, menções e prazos.</li>
		<li>
			Enviar os emails necessários à conta: confirmação de email, recuperação de senha e convites.
		</li>
		<li>
			Proteger o serviço e as pessoas que o usam: limitar tentativas de acesso, investigar abusos e
			falhas.
		</li>
	</ul>
	<p>
		Não usamos os seus dados para publicidade, não os vendemos e não usamos ferramentas de
		rastreamento ou de análise de terceiros.
	</p>

	<h2>Quem vê o quê</h2>
	<ul>
		<li>
			O conteúdo de um quadro é visto apenas por quem participa dele, conforme o papel de cada um
			(dono, editor ou leitor). Quem participa vê o nome e o email das outras pessoas do quadro.
		</li>
		<li>
			Se o dono abrir um quadro ao domínio da empresa, quem tem email confirmado desse domínio vê,
			antes de entrar, o título do quadro, o nome do dono e quantas pessoas participam.
		</li>
		<li>
			O dono de um quadro pode gerar um link público de acompanhamento. Ele mostra colunas, cards,
			descrições, etiquetas, prazos e checklists — mas nunca nomes de pessoas, comentários, anexos
			ou o histórico.
		</li>
	</ul>

	<h2>Com quem compartilhamos</h2>
	<p>Apenas com os operadores necessários para o serviço funcionar:</p>
	<ul>
		<li>
			<b>Provedor de email</b> (Brevo), que entrega os emails do sistema e, para isso, recebe o endereço
			de destino e o texto da mensagem. Os emails levam apenas títulos de quadros e cards — nunca o texto
			de comentários ou descrições.
		</li>
		<li>
			<b>Cloudflare</b>, que protege e encaminha as conexões até o servidor e, para isso, trata o
			endereço IP e os dados técnicos da conexão.
		</li>
	</ul>
	<p>Também podemos compartilhar dados quando a lei ou uma ordem judicial exigir.</p>

	<h2>Por quanto tempo guardamos</h2>
	<ul>
		<li>
			Conta e conteúdo: enquanto a conta existir, ou até serem apagados por quem tem permissão.
		</li>
		<li>Sessão: até você sair, ou por no máximo 7 dias.</li>
		<li>Links de recuperação de senha: 1 hora. Links de confirmação de email: 7 dias.</li>
		<li>
			Notificações lidas: pelo prazo escolhido pelo dono de cada quadro (de 1 a 365 dias). Nenhuma
			notificação passa de um ano.
		</li>
		<li>
			Convites encerrados e o registro do envio de cada email: 90 dias. O texto do email é apagado
			assim que ele sai.
		</li>
		<li>
			O registro de quem foi removido de um quadro: até a pessoa aceitar um convite novo para ele,
			ou até o quadro ser apagado.
		</li>
		<li>Cópias de segurança do banco de dados: 7 dias.</li>
		<li>
			Arquivos anexados que foram apagados podem continuar armazenados até a conclusão da rotina de
			remoção definitiva, que só os elimina depois de confirmar que existe uma cópia de segurança
			posterior.
		</li>
	</ul>

	<h2>Cookies e armazenamento no navegador</h2>
	<p>
		Usamos um único cookie, essencial, que mantém a sua sessão. A escolha entre tema claro e escuro
		fica guardada no seu próprio navegador. Não há cookies de publicidade nem de análise.
	</p>

	<h2>Segurança</h2>
	<p>
		As conexões são cifradas, as senhas e os links de acesso são guardados de forma irreversível, os
		emails aguardando envio ficam cifrados, e trocar a senha encerra a sessão em todos os aparelhos.
		Nenhum sistema é imune a incidentes; se um incidente afetar os seus dados, você será informado
		conforme a lei.
	</p>

	<h2>Os seus direitos</h2>
	<p>
		Você pode pedir confirmação de que tratamos os seus dados, acesso, correção, anonimização ou
		eliminação, informação sobre com quem os compartilhamos, e revogar consentimentos, nos termos do
		art. 18 da LGPD.
		{#if EMAIL_DE_CONTATO}
			Para isso, escreva para <a href="mailto:{EMAIL_DE_CONTATO}">{EMAIL_DE_CONTATO}</a>.
		{:else}
			O canal para esses pedidos será publicado nesta página; sobre o conteúdo dos quadros de uma
			empresa, o pedido também pode ser feito à própria empresa.
		{/if}
	</p>

	<h2>Mudanças nesta política</h2>
	<p>Quando esta política mudar, a data no topo da página muda junto.</p>
</PaginaLegal>
