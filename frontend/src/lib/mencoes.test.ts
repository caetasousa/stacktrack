// O autocompletar de menções no campo de comentário.
//
// Como isto quebra:
//   - digitar "@" no meio de um email abre a lista de pessoas;
//   - a lista não acha "José" para quem digita "jose", ou acha só pelo começo
//     do nome inteiro — e quem digita o sobrenome não encontra ninguém;
//   - escolher a pessoa apaga o resto do texto, ou deixa o cursor fora do
//     lugar;
//   - o nome inserido não é o nome completo, e o servidor não reconhece a
//     menção quando o primeiro nome é repetido no quadro.
import { describe, expect, it } from 'vitest';
import { aplicarMencao, consultaNoCursor, sugestoes } from './mencoes';

const pessoas = [
	{ usuarioId: 'b', nome: 'Bruno Costa' },
	{ usuarioId: 'c', nome: 'Carla Dias' },
	{ usuarioId: 'j', nome: 'José Álvaro' },
	{ usuarioId: 'a1', nome: 'Ana Silva' },
	{ usuarioId: 'a2', nome: 'Ana Souza' }
];

describe('consultaNoCursor', () => {
	it('reconhece o "@" seguido do que já foi digitado', () => {
		expect(consultaNoCursor('Oi @Br', 6)).toEqual({ inicio: 3, termo: 'Br' });
		expect(consultaNoCursor('@', 1)).toEqual({ inicio: 0, termo: '' });
	});

	it('aceita o espaço do sobrenome', () => {
		expect(consultaNoCursor('@Ana S', 6)).toEqual({ inicio: 0, termo: 'Ana S' });
	});

	it('ignora o "@" de um email', () => {
		expect(consultaNoCursor('bruno@exe', 9)).toBeNull();
	});

	it('não atravessa quebra de linha nem espaço duplo', () => {
		expect(consultaNoCursor('@Ana\nSi', 7)).toBeNull();
		expect(consultaNoCursor('@Ana  ', 6)).toBeNull();
	});

	it('só olha para trás do cursor', () => {
		expect(consultaNoCursor('@Bruno e mais', 6)).toEqual({ inicio: 0, termo: 'Bruno' });
	});
});

describe('sugestoes', () => {
	it('acha sem caixa e sem acento, por qualquer palavra do nome', () => {
		expect(sugestoes(pessoas, 'jose').map((p) => p.usuarioId)).toEqual(['j']);
		expect(sugestoes(pessoas, 'dias').map((p) => p.usuarioId)).toEqual(['c']);
		expect(sugestoes(pessoas, 'ana s').map((p) => p.usuarioId)).toEqual(['a1', 'a2']);
	});

	it('com o termo vazio, oferece todo mundo até o limite', () => {
		expect(sugestoes(pessoas, '', 3)).toHaveLength(3);
	});
});

describe('aplicarMencao', () => {
	it('troca o que foi digitado pelo nome completo e põe o cursor depois', () => {
		const texto = 'Oi @an, veja';
		const consulta = consultaNoCursor(texto, 6)!;
		expect(aplicarMencao(texto, consulta, 6, 'Ana Souza')).toEqual({
			texto: 'Oi @Ana Souza, veja',
			cursor: 13
		});
	});

	it('separa com um espaço quando o que vem depois é palavra, ou é o fim', () => {
		const consultaNoMeio = consultaNoCursor('@Br veja', 3)!;
		expect(aplicarMencao('@Br veja', consultaNoMeio, 3, 'Bruno Costa').texto).toBe(
			'@Bruno Costa veja'
		);
		const colado = consultaNoCursor('@Brveja', 3)!;
		expect(aplicarMencao('@Brveja', colado, 3, 'Bruno Costa').texto).toBe('@Bruno Costa veja');
	});

	it('no fim do texto também', () => {
		const consulta = consultaNoCursor('@Car', 4)!;
		expect(aplicarMencao('@Car', consulta, 4, 'Carla Dias')).toEqual({
			texto: '@Carla Dias ',
			cursor: 12
		});
	});
});
