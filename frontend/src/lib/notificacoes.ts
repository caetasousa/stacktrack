// O sino em frases: o que cada aviso diz, por que deixou de valer e para onde
// leva.
//
// A frase mora aqui, e não no servidor, pela mesma razão das frases do
// histórico ($lib/atividade): a API diz o que aconteceu, e a redação é
// apresentação. E mora fora do componente porque um tipo esquecido viraria uma
// linha em branco no sino sem quebrar nada — o defeito mais fácil de não notar.
import type { Notificacao } from '$lib/api/notificacoes';

/** A frase em duas partes: quem fez, em negrito, e o resto. */
export interface Frase {
	sujeito: string;
	texto: string;
}

/**
 * fraseDe descreve o aviso. O prazo não tem autor: a frase começa pelo card, e
 * o sujeito vem vazio.
 */
export function fraseDe(n: Notificacao, agora: Date = new Date()): Frase {
	const autor = n.autor || 'Alguém';
	const card = `"${n.card ?? 'um card'}"`;
	switch (n.tipo) {
		case 'atribuicao':
			return { sujeito: autor, texto: `pôs você como responsável por ${card}` };
		case 'mencao':
			return { sujeito: autor, texto: `mencionou você em ${card}` };
		case 'convite':
			return { sujeito: autor, texto: `convidou você para o quadro "${n.quadro}"` };
		case 'prazo':
			return {
				sujeito: '',
				texto: `O prazo de ${card} ${n.prazo ? quandoVence(n.prazo, agora) : 'está chegando'}`
			};
		default:
			// Um tipo que o servidor ganhou depois deste cliente: ainda diz alguma
			// coisa, em vez de uma linha vazia.
			return { sujeito: autor, texto: 'mexeu num quadro de que você participa' };
	}
}

/**
 * quandoVence diz o prazo em dias do CALENDÁRIO local — "hoje", "amanhã",
 * "em 30/10" —, e não em horas: o prazo é uma data, e "vence em 21 horas"
 * obrigaria quem lê a fazer a conta para saber se é hoje ou amanhã.
 */
export function quandoVence(prazoIso: string, agora: Date): string {
	const prazo = new Date(prazoIso);
	if (Number.isNaN(prazo.getTime())) return 'está chegando';
	const dias = Math.round((meiaNoite(prazo) - meiaNoite(agora)) / 86_400_000);
	const data = prazo.toLocaleDateString('pt-BR', { day: '2-digit', month: '2-digit' });
	if (dias < 0) return `venceu em ${data}`;
	if (dias === 0) return 'vence hoje';
	if (dias === 1) return 'vence amanhã';
	return `vence em ${data}`;
}

function meiaNoite(d: Date): number {
	return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
}

/**
 * motivoDoEncerramento diz por que o aviso deixou de valer, ou vazio quando ele
 * ainda vale. Sem isto, quem abre um aviso de atribuição desfeita chega ao card
 * achando que ele ainda é seu.
 */
export function motivoDoEncerramento(n: Notificacao): string {
	switch (n.situacao) {
		case 'vigente':
			return '';
		case 'aceito':
			return 'você aceitou';
		case 'recusado':
			return 'você recusou';
		case 'revogado':
			return 'o convite foi cancelado';
		case 'vencido':
			return 'o convite venceu';
		default:
			return n.tipo === 'prazo'
				? 'o prazo mudou, ou você não responde mais pelo card'
				: 'você não responde mais por este card';
	}
}

/**
 * destinoDe é para onde o clique leva: o card aberto no quadro, ou os convites
 * do painel.
 */
export function destinoDe(n: Notificacao): string {
	if (n.tipo === 'convite' || !n.cardId) return '/painel#convites';
	return `/painel/quadros/${n.quadroId}?card=${n.cardId}`;
}
