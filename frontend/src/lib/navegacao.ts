// Para onde levar a pessoa depois de entrar ou criar conta.

const PADRAO = '/painel';

// destinoDeVolta lê o parâmetro `voltar` da URL — é como a página de convite
// pede para o login devolver a pessoa ao convite em vez de despejá-la no painel.
//
// Só aceita caminho interno começando com uma única barra. Sem essa checagem, o
// parâmetro viraria redirecionamento aberto: bastava mandar
// /login?voltar=https://site-falso para a nossa tela de login jogar a pessoa
// autenticada num domínio de terceiro. `//outro.site` também é bloqueado — o
// navegador o entende como URL de protocolo relativo.
export function destinoDeVolta(url: URL): string {
	const voltar = url.searchParams.get('voltar');
	if (!voltar || !voltar.startsWith('/') || voltar.startsWith('//')) {
		return PADRAO;
	}
	return voltar;
}

// Uma máscara de email como o servidor a produz: uma letra, três asteriscos e
// o domínio (ver mascararEmail em $lib/email).
const MASCARA = /^[^\s@*]\*\*\*@[^\s@]+\.[^\s@]+$/;

/**
 * conviteEmAndamento diz se a tela de cadastro ou de login está no meio de um
 * convite — volta para /convite/… — e para qual email (mascarado) ele é.
 *
 * O `para` vem da URL, então só é aceito com a forma exata de uma máscara: sem
 * isso, um link forjado escreveria o que quisesse no aviso da tela.
 */
export function conviteEmAndamento(url: URL): { para: string } | null {
	const para = url.searchParams.get('para') ?? '';
	if (!destinoDeVolta(url).startsWith('/convite/') || !MASCARA.test(para)) {
		return null;
	}
	return { para };
}
