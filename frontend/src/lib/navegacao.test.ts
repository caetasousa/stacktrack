import { describe, expect, it } from 'vitest';
import { conviteEmAndamento, destinoDeVolta } from './navegacao';

const comVoltar = (valor: string) =>
	new URL(`http://localhost:5173/login?voltar=${encodeURIComponent(valor)}`);

describe('destinoDeVolta', () => {
	it('devolve o caminho interno pedido', () => {
		expect(destinoDeVolta(comVoltar('/convite/abc123'))).toBe('/convite/abc123');
	});

	it('cai no painel quando não há parâmetro', () => {
		expect(destinoDeVolta(new URL('http://localhost:5173/login'))).toBe('/painel');
	});

	// Sem esta checagem o parâmetro vira redirecionamento aberto: bastaria
	// mandar /login?voltar=https://site-falso para a nossa tela de login jogar
	// alguém recém-autenticado num domínio de terceiro.
	it('recusa destino externo', () => {
		const externos = [
			'https://site-falso.example',
			'http://site-falso.example',
			// protocolo relativo: o navegador entende como outro domínio
			'//site-falso.example',
			'javascript:alert(1)'
		];

		for (const destino of externos) {
			expect(destinoDeVolta(comVoltar(destino))).toBe('/painel');
		}
	});
});

// O aviso de "você está aceitando um convite" no cadastro e no login. Como
// quebra: some quando a pessoa alterna entre as duas telas (e ela cria a conta
// com outro email sem saber); ou aparece fora de um convite, com um texto que
// qualquer link pode plantar na tela.
describe('conviteEmAndamento', () => {
	const tela = (consulta: string) => new URL(`http://localhost:5173/cadastro?${consulta}`);

	it('reconhece a volta para um convite e o email dele', () => {
		const url = tela(
			`voltar=${encodeURIComponent('/convite/abc')}&para=${encodeURIComponent('b***@teste.dev')}`
		);
		expect(conviteEmAndamento(url)).toEqual({ para: 'b***@teste.dev' });
	});

	it('não vê convite quando a volta é para outro lugar', () => {
		const url = tela(
			`voltar=${encodeURIComponent('/painel')}&para=${encodeURIComponent('b***@x.com')}`
		);
		expect(conviteEmAndamento(url)).toBeNull();
	});

	it('não vê convite sem o email', () => {
		expect(conviteEmAndamento(tela(`voltar=${encodeURIComponent('/convite/abc')}`))).toBeNull();
	});

	// O `para` vem da URL: só serve se tiver a forma de uma máscara, e não de
	// um texto qualquer — senão um link forjado escreveria o que quisesse no aviso.
	it('recusa um "para" que não é uma máscara de email', () => {
		for (const para of ['ana@teste.dev', 'clique aqui para ganhar', '***@x.com', 'a***@']) {
			const url = tela(
				`voltar=${encodeURIComponent('/convite/abc')}&para=${encodeURIComponent(para)}`
			);
			expect(conviteEmAndamento(url)).toBeNull();
		}
	});
});
