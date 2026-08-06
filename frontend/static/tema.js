// Aplica o tema escolhido ANTES da primeira pintura.
//
// Se isto rodasse dentro da aplicação, a página apareceria com o tema
// padrão por um instante e trocaria depois da hidratação — o "flash"
// branco que denuncia tema mal implementado. Por isso é um script comum
// no <head>, síncrono, sem depender do bundle.
//
// Externo (e não inline) porque a CSP do projeto é `script-src 'self'`:
// script embutido no HTML seria bloqueado pelo navegador.
(function () {
	'use strict';

	var CHAVE = 'kanbango:tema';
	var PADRAO = 'dark';

	var escolhido = null;
	try {
		escolhido = localStorage.getItem(CHAVE);
	} catch (e) {
		// localStorage indisponível (navegação privada, cookies bloqueados):
		// segue com o padrão em vez de quebrar a página inteira.
	}

	document.documentElement.setAttribute(
		'data-theme',
		escolhido === 'light' || escolhido === 'dark' ? escolhido : PADRAO
	);
})();
