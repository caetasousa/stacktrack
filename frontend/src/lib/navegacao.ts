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
