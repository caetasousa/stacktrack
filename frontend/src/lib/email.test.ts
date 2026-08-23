import { describe, expect, it } from 'vitest';
import { mascararEmail } from './email';

describe('mascararEmail', () => {
	// A máscara do cliente precisa bater com a do servidor
	// (usuario.MascararEmail): é ela que decide se a tela do convite oferece
	// "aceitar" ou avisa que a conta é outra.
	it('reduz a parte local à primeira letra e preserva o domínio', () => {
		expect(mascararEmail('novo@exemplo.com')).toBe('n***@exemplo.com');
		expect(mascararEmail('a@x.com')).toBe('a***@x.com');
	});

	it('normaliza caixa e espaços, como o backend', () => {
		expect(mascararEmail('  NOVO@Exemplo.COM ')).toBe('n***@exemplo.com');
	});

	it('não devolve a entrada crua quando não há parte local', () => {
		// Devolver o valor original aqui seria justamente o vazamento que a
		// função existe para impedir.
		expect(mascararEmail('@exemplo.com')).toBe('***');
		expect(mascararEmail('sem-arroba')).toBe('***');
		expect(mascararEmail('')).toBe('***');
	});
});
