<script lang="ts">
	// Os ícones da página inicial, em SVG inline.
	//
	// Antes eram `<span>` quadrados com fundo de acento — um placeholder que
	// nunca virou ícone e que, numa página que apresenta o produto, lia-se como
	// carregamento travado. SVG também não depende de fonte, ao contrário do
	// emoji que já deixou um botão invisível neste projeto (ver IconeArquivar).
	//
	// Traço, e não preenchimento: pesa menos ao lado do texto e herda a cor do
	// contêiner por `currentColor`, então o tema troca sozinho.
	type Nome = 'tempo-real' | 'presenca' | 'papeis' | 'arrastar' | 'conversa' | 'organizacao';

	let { nome, class: classe = 'size-5' }: { nome: Nome; class?: string } = $props();

	// Cada caminho é descrito pelo que representa, não pelo desenho: quem
	// mexer aqui precisa saber qual recurso o ícone acompanha.
	const caminhos: Record<Nome, string> = {
		// Duas telas em sincronia.
		'tempo-real': 'M3 5h9v8H3zM12 11h9v8h-9zM8 17h4M16 7h4',
		// Pessoas juntas no mesmo lugar.
		presenca:
			'M9 11a3 3 0 1 0 0-6 3 3 0 0 0 0 6M3 20a6 6 0 0 1 12 0M16 6a3 3 0 0 1 0 6M17 20a6 6 0 0 0-2-4',
		// Escudo: o que o servidor confere.
		papeis: 'M12 3l7 3v6c0 4-3 7-7 8-4-1-7-4-7-8V6z M9.5 12l2 2 3.5-4',
		// Card saindo de uma coluna para outra.
		arrastar: 'M4 5h6v14H4zM14 5h6v14h-6M10 12h4M12.5 10l2 2-2 2',
		// Balão de conversa com linhas.
		conversa: 'M4 6a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H9l-5 4zM8 8h8M8 12h5',
		// Etiqueta com o furo do cordão.
		organizacao: 'M3 12V5a2 2 0 0 1 2-2h7l9 9-9 9zM7.5 7.5h.01'
	};
</script>

<svg
	class={classe}
	viewBox="0 0 24 24"
	fill="none"
	stroke="currentColor"
	stroke-width="1.6"
	stroke-linecap="round"
	stroke-linejoin="round"
	aria-hidden="true"
>
	<path d={caminhos[nome]} />
</svg>
