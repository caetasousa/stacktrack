// As frases do sino e para onde cada aviso leva — o que a pessoa lê e onde ela
// chega ao clicar.
//
// Como isto quebra:
//   - um tipo de aviso sem frase aparece como linha em branco no sino;
//   - o aviso que deixou de valer não diz por quê, e a pessoa abre um card que
//     já não é dela achando que é;
//   - o prazo diz "vence amanhã" no dia errado, por conta do fuso;
//   - o clique leva ao quadro sem abrir o card, ou o convite leva a lugar
//     nenhum.
import { describe, expect, it } from 'vitest';
import type { Notificacao } from '$lib/api/notificacoes';
import { destinoDe, fraseDe, motivoDoEncerramento, quandoVence } from './notificacoes';

function aviso(campos: Partial<Notificacao>): Notificacao {
	return {
		id: 'n1',
		tipo: 'atribuicao',
		quadroId: 'q1',
		quadro: 'Lançamento',
		cardId: 'c1',
		card: 'Migrar o banco',
		autor: 'Ana',
		criadaEm: '2026-10-25T12:00:00Z',
		lida: false,
		situacao: 'vigente',
		...campos
	};
}

describe('fraseDe', () => {
	it('diz quem fez e sobre o quê, para cada tipo', () => {
		expect(fraseDe(aviso({}))).toEqual({
			sujeito: 'Ana',
			texto: 'pôs você como responsável por "Migrar o banco"'
		});
		expect(fraseDe(aviso({ tipo: 'mencao' }))).toEqual({
			sujeito: 'Ana',
			texto: 'mencionou você em "Migrar o banco"'
		});
		expect(fraseDe(aviso({ tipo: 'convite', cardId: undefined, card: undefined }))).toEqual({
			sujeito: 'Ana',
			texto: 'convidou você para o quadro "Lançamento"'
		});
	});

	it('o prazo não tem autor: a frase começa pelo card', () => {
		const agora = new Date(2026, 9, 25, 15, 0);
		const prazo = new Date(2026, 9, 26, 12, 0).toISOString();
		expect(fraseDe(aviso({ tipo: 'prazo', autor: undefined, prazo }), agora)).toEqual({
			sujeito: '',
			texto: 'O prazo de "Migrar o banco" vence amanhã'
		});
	});

	it('sem o nome de quem fez (conta removida), diz "alguém" em vez de deixar o buraco', () => {
		expect(fraseDe(aviso({ autor: undefined })).sujeito).toBe('Alguém');
	});
});

describe('quandoVence', () => {
	const agora = new Date(2026, 9, 25, 9, 30);

	it('fala em dias do calendário, e não em horas', () => {
		expect(quandoVence(new Date(2026, 9, 25, 12).toISOString(), agora)).toBe('vence hoje');
		expect(quandoVence(new Date(2026, 9, 26, 12).toISOString(), agora)).toBe('vence amanhã');
		expect(quandoVence(new Date(2026, 9, 30, 12).toISOString(), agora)).toBe('vence em 30/10');
		expect(quandoVence(new Date(2026, 9, 24, 12).toISOString(), agora)).toBe('venceu em 24/10');
	});

	it('às 23h, o prazo do dia seguinte ainda é "amanhã"', () => {
		const tarde = new Date(2026, 9, 25, 23, 0);
		expect(quandoVence(new Date(2026, 9, 26, 12).toISOString(), tarde)).toBe('vence amanhã');
	});
});

describe('motivoDoEncerramento', () => {
	it('é vazio enquanto o aviso vale', () => {
		expect(motivoDoEncerramento(aviso({}))).toBe('');
	});

	it('diz por que deixou de valer, em palavras do que aconteceu', () => {
		expect(motivoDoEncerramento(aviso({ situacao: 'encerrada' }))).toBe(
			'você não responde mais por este card'
		);
		expect(motivoDoEncerramento(aviso({ tipo: 'prazo', situacao: 'encerrada' }))).toBe(
			'o prazo mudou, ou você não responde mais pelo card'
		);
		expect(motivoDoEncerramento(aviso({ tipo: 'convite', situacao: 'aceito' }))).toBe(
			'você aceitou'
		);
		expect(motivoDoEncerramento(aviso({ tipo: 'convite', situacao: 'recusado' }))).toBe(
			'você recusou'
		);
		expect(motivoDoEncerramento(aviso({ tipo: 'convite', situacao: 'revogado' }))).toBe(
			'o convite foi cancelado'
		);
		expect(motivoDoEncerramento(aviso({ tipo: 'convite', situacao: 'vencido' }))).toBe(
			'o convite venceu'
		);
	});
});

describe('destinoDe', () => {
	it('leva ao card aberto no quadro', () => {
		expect(destinoDe(aviso({}))).toBe('/painel/quadros/q1?card=c1');
	});

	it('o convite leva aos convites do painel', () => {
		expect(destinoDe(aviso({ tipo: 'convite', cardId: undefined }))).toBe('/painel#convites');
	});
});
