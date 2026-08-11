<script lang="ts">
	// O ícone de arquivar, em SVG inline.
	//
	// Era o emoji 🗄 (U+1F5C4, "card file box"), e emoji é uma aposta na FONTE de
	// quem está do outro lado: onde ela não cobre aquele ponto de código, o
	// navegador pinta um retângulo vazio. O botão continua lá, clicável e com o
	// aria-label certo — invisível só para quem olha. Foi assim que a ação de
	// arquivar nasceu impossível de achar.
	//
	// E os testes não pegam: eles selecionam por aria-label, que existe e está
	// correto. O seletor enxerga o que o olho não vê.
	//
	// SVG não depende de fonte nenhuma. `currentColor` deixa o ícone herdar a cor
	// do botão, então o hover continua sendo uma regra de CSS só.
	let { class: classe = 'size-4' }: { class?: string } = $props();
</script>

<svg
	class={classe}
	viewBox="0 0 24 24"
	fill="none"
	stroke="currentColor"
	stroke-width="2"
	stroke-linecap="round"
	stroke-linejoin="round"
	aria-hidden="true"
>
	<!-- A tampa da caixa, e o corpo dela: a metáfora de guardar sem jogar fora. -->
	<rect x="3" y="4" width="18" height="4" rx="1" />
	<path d="M5 8v11a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V8" />
	<path d="M10 12h4" />
</svg>
