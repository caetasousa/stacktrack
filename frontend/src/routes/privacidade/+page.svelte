<script lang="ts">
	// A Política de Privacidade. Descreve SÓ o que o sistema faz de fato — os
	// prazos e os destinos abaixo são os do código e da infraestrutura (ver
	// docs/funcionalidades.md e docs/producao.md). Mudou um prazo lá, muda aqui.
	//
	// ⚠️ Texto-base para revisão jurídica: a identificação do controlador e o
	// canal do encarregado dependem da instituição que oferece o sistema.
	import PaginaLegal from '$lib/components/PaginaLegal.svelte';
	import { EMAIL_DE_CONTATO, PRODUTO } from '$lib/instituicao';
</script>

<PaginaLegal titulo="Política de Privacidade">
	<p>
		Esta política explica quais dados pessoais o {PRODUTO} trata, para quê, por quanto tempo e como você
		exerce os seus direitos previstos na Lei Geral de Proteção de Dados (Lei nº 13.709/2018). O controlador
		dos dados é a instituição que disponibiliza o {PRODUTO} a você.
	</p>

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
			Para isso, procure quem administra o acesso ao {PRODUTO} na sua instituição.
		{/if}
	</p>

	<h2>Mudanças nesta política</h2>
	<p>Quando esta política mudar, a data no topo da página muda junto.</p>
</PaginaLegal>
