// Iniciais para avatar.
//
// Vive num módulo próprio porque três telas desenham avatar da mesma pessoa —
// a presença no cabeçalho do quadro, os responsáveis no card e os do modal —
// e duas letras diferentes para o mesmo nome em lugares diferentes pareceriam
// duas pessoas.

/**
 * iniciais devolve até duas letras para caber no círculo do avatar.
 *
 * Um nome só vira as duas primeiras letras ("Ana" → "AN"); vários viram a
 * primeira do primeiro e a primeira do último ("Ana Maria Souza" → "AS"), que
 * é como as pessoas se identificam por escrito.
 */
export function iniciais(nome: string): string {
	const partes = nome.trim().split(/\s+/).filter(Boolean);
	if (partes.length === 0) return '?';
	if (partes.length === 1) return partes[0].slice(0, 2).toUpperCase();
	return (partes[0][0] + partes[partes.length - 1][0]).toUpperCase();
}
