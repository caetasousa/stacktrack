// Máscara de email, espelhando usuario.MascararEmail do backend.

/**
 * Devolve o email na forma `a***@exemplo.com`.
 *
 * Existe no cliente por um motivo só: a consulta pública do convite passou a
 * devolver o endereço MASCARADO, e a tela precisa decidir se a conta logada é
 * a convidada para escolher entre "aceitar" e "você está na conta errada".
 * Sem uma máscara igual à do servidor, não há o que comparar.
 *
 * A comparação por máscara é uma DICA, não uma autorização: `ana@x.com` e
 * `andre@x.com` produzem a mesma máscara. Quem decide de fato é o servidor, no
 * aceite, comparando os endereços inteiros — e é ele que responde 403 quando
 * não conferem. Aqui o erro possível é oferecer o botão a quem vai levar 403,
 * que é bem menos ruim que esconder o botão de quem tem direito a ele.
 */
export function mascararEmail(email: string): string {
	const normalizado = email.trim().toLowerCase();
	const arroba = normalizado.lastIndexOf('@');
	if (arroba <= 0) return '***';
	return `${normalizado.slice(0, 1)}***${normalizado.slice(arroba)}`;
}
