import { describe, expect, it } from 'vitest';
import { destinoDeVolta } from './navegacao';

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
