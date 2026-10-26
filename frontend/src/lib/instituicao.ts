// Os dados de quem oferece o stacktrack, num lugar só: o nome que aparece nas
// telas, quem responde pelo serviço, o canal de contato e a data dos textos
// legais. Trocar qualquer um deles é editar este arquivo — e não procurar o
// texto tela a tela.

export const PRODUTO = 'stacktrack';

/**
 * Quem responde pelo serviço: o nome (ou a razão social) e o CPF ou CNPJ, como
 * devem aparecer nos Termos e na Política de Privacidade. Vazio enquanto não for
 * definido: os textos então falam só em nome do produto.
 */
export const RESPONSAVEL = '';

/**
 * O email de suporte e de pedidos sobre dados pessoais (LGPD). Vazio enquanto
 * não houver um: as telas não mostram um endereço inventado.
 */
export const EMAIL_DE_CONTATO = '';

/** A data da versão vigente dos Termos de Uso e da Política de Privacidade. */
export const TEXTOS_LEGAIS_ATUALIZADOS_EM = '26 de outubro de 2026';
