// Um relógio reativo, UM só para o aplicativo inteiro.
//
// Existe por um defeito concreto: o card mostrava "há 39 min" e continuava
// mostrando "há 39 min" meia hora depois. `haQuanto()` calcula a partir de
// `new Date()`, e `new Date()` não é estado reativo — o Svelte não tem como
// saber que o resultado envelheceu, então o texto só se corrigia quando alguma
// outra coisa forçava a tela a redesenhar.
//
// Tempo relativo parado é pior do que não ter tempo nenhum: ele não parece
// desatualizado, parece errado. Quem olha conclui que ninguém mexeu no card
// desde então.
//
// Um relógio compartilhado, e não um `setInterval` por componente: um quadro
// com cinquenta cards teria cinquenta temporizadores fazendo a mesma conta em
// momentos ligeiramente diferentes — e cinquenta vazamentos se algum
// componente esquecesse de limpar o seu.

// 30 segundos, e não 60: `haQuanto` arredonda por minuto, então um tique a cada
// minuto mostraria o valor certo com até um minuto de atraso. Com 30s o erro
// máximo é de meio minuto, que ninguém percebe.
const INTERVALO_MS = 30_000;

let agora = $state(new Date());
let temporizador: ReturnType<typeof setInterval> | null = null;

/**
 * agoraReativo devolve o instante atual como estado reativo: quem o lê num
 * `$derived` ou no template se redesenha sozinho a cada tique.
 *
 * O temporizador nasce na primeira leitura, e não na importação do módulo:
 * assim um teste de unidade que só importe `haQuanto` não deixa um intervalo
 * pendurado sem nunca ter pedido as horas.
 */
export function agoraReativo(): Date {
	if (temporizador === null && typeof setInterval === 'function') {
		temporizador = setInterval(() => (agora = new Date()), INTERVALO_MS);
	}
	return agora;
}

/**
 * pararRelogio existe para os testes. Em produção o relógio acompanha a aba e
 * morre com ela — parar e religar por navegação só criaria a chance de ele
 * ficar parado.
 */
export function pararRelogio(): void {
	if (temporizador !== null) {
		clearInterval(temporizador);
		temporizador = null;
	}
}
