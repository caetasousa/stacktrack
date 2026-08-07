import { describe, expect, it } from 'vitest';
import { renderizarMarkdown } from './markdown';

// TAGS_PERMITIDAS é tudo que o renderizador pode emitir. Qualquer outra tag na
// saída veio do texto de quem escreveu — ou seja, escapou.
const TAGS_PERMITIDAS = new Set([
	'p',
	'h3',
	'h4',
	'h5',
	'ul',
	'ol',
	'li',
	'strong',
	'em',
	'del',
	'code',
	'pre',
	'blockquote',
	'a'
]);

// tagsDe extrai os nomes das tags VIVAS da saída (o que foi escapado virou
// &lt; e não casa aqui).
function tagsDe(html: string): string[] {
	return [...html.matchAll(/<\/?([a-zA-Z][a-zA-Z0-9]*)/g)].map((m) => m[1].toLowerCase());
}

describe('escape', () => {
	// A descrição é texto que qualquer pessoa do quadro escreveu, e vai para a
	// tela com {@html}. Se algo aqui passar, é XSS.
	//
	// A checagem é por lista de permissão, e não por lista dos perigosos que eu
	// lembrei de escrever: assim uma tag nova que vaze no futuro reprova sozinha.
	it('nenhuma tag do texto vira tag viva', () => {
		const perigosos = [
			'<script>alert(1)</script>',
			'<img src=x onerror="alert(1)">',
			'<iframe src="https://evil.example"></iframe>',
			'<div onclick="roubar()">clique</div>',
			'<svg/onload=alert(1)>',
			'<a href="javascript:alert(1)">clique</a>',
			'<style>body{display:none}</style>'
		];

		for (const entrada of perigosos) {
			const html = renderizarMarkdown(entrada);
			for (const tag of tagsDe(html)) {
				expect(TAGS_PERMITIDAS, `entrada: ${entrada}`).toContain(tag);
			}
			// o texto continua visível, só que inerte
			expect(html).toContain('&lt;');
		}
	});

	// Atributo de evento só é perigoso dentro de uma tag viva. Como nenhuma tag
	// do texto sobrevive, "onerror" na saída é texto, e é isso que se confirma.
	it('atributo de evento sobra como texto, não como atributo', () => {
		const html = renderizarMarkdown('<img src=x onerror="alert(1)">');
		expect(html).toBe('<p>&lt;img src=x onerror=&quot;alert(1)&quot;&gt;</p>');
	});

	it('escapa aspas, que fechariam um atributo', () => {
		expect(renderizarMarkdown('texto " e \'')).toContain('&quot;');
	});
});

describe('links', () => {
	it('renderiza http e https, abrindo em outra aba com rel seguro', () => {
		const html = renderizarMarkdown('veja o [repo](https://github.com/org/repo)');

		expect(html).toContain('href="https://github.com/org/repo"');
		expect(html).toContain('rel="noopener noreferrer"');
		expect(html).toContain('>repo</a>');
	});

	it('aceita caminho interno sem abrir em outra aba', () => {
		const html = renderizarMarkdown('vá ao [painel](/painel)');
		expect(html).toContain('href="/painel"');
		expect(html).not.toContain('target="_blank"');
	});

	// Sem essa checagem, [clique](javascript:...) viraria execução de script.
	it('recusa esquema perigoso e deixa o texto cru', () => {
		const perigosos = [
			'[clique](javascript:alert(1))',
			'[clique](data:text/html,<script>alert(1)</script>)',
			'[clique](vbscript:msgbox)',
			'[clique](//evil.example)'
		];

		for (const entrada of perigosos) {
			const html = renderizarMarkdown(entrada);
			// O que importa é não virar link: o texto sobra visível e inerte
			// dentro do parágrafo, que é o comportamento certo — mostrar o que
			// a pessoa escreveu, sem executá-lo.
			expect(html, `entrada: ${entrada}`).not.toContain('<a ');
			expect(html).toContain('<p>');
		}
	});
});

describe('subconjunto suportado', () => {
	it('títulos viram h3 a h5, e não h1', () => {
		// A página já tem um h1; um h1 vindo da descrição quebraria a hierarquia
		// de cabeçalhos para quem navega por leitor de tela.
		const html = renderizarMarkdown('# Título\n## Subtítulo\n### Menor');
		expect(html).toContain('<h3>Título</h3>');
		expect(html).toContain('<h4>Subtítulo</h4>');
		expect(html).toContain('<h5>Menor</h5>');
		expect(html).not.toContain('<h1');
	});

	it('negrito, itálico, riscado e código', () => {
		const html = renderizarMarkdown('**forte** e *leve* e ~~fora~~ e `codigo`');
		expect(html).toContain('<strong>forte</strong>');
		expect(html).toContain('<em>leve</em>');
		expect(html).toContain('<del>fora</del>');
		expect(html).toContain('<code>codigo</code>');
	});

	it('listas com marcador e numeradas', () => {
		const comMarcador = renderizarMarkdown('- um\n- dois');
		expect(comMarcador).toContain('<ul>');
		expect(comMarcador).toContain('<li>um</li>');

		const numerada = renderizarMarkdown('1. um\n2. dois');
		expect(numerada).toContain('<ol>');
		expect(numerada).toContain('<li>dois</li>');
	});

	it('citação', () => {
		expect(renderizarMarkdown('> observação')).toContain('<blockquote>observação</blockquote>');
	});

	// Dentro de um bloco de código nada é interpretado — é o ponto dele.
	it('bloco de código não interpreta o conteúdo', () => {
		const html = renderizarMarkdown('```\n**não é negrito**\n<script>x</script>\n```');
		expect(html).toContain('<pre><code>');
		expect(html).not.toContain('<strong>');
		expect(html).not.toContain('<script>');
	});

	it('fecha o bloco de código não terminado', () => {
		const html = renderizarMarkdown('```\nsem fechar');
		expect(html).toContain('</code></pre>');
	});
});

describe('texto comum', () => {
	it('vira parágrafo', () => {
		expect(renderizarMarkdown('linha simples')).toBe('<p>linha simples</p>');
	});

	it('texto vazio não gera nada', () => {
		expect(renderizarMarkdown('')).toBe('');
		expect(renderizarMarkdown('   \n  ')).toBe('');
	});
});
