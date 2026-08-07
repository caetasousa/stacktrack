// Renderizador de um subconjunto de Markdown para a descrição do card.
//
// Escrito à mão, sem biblioteca, e a razão é segurança: renderizar markdown
// significa gerar HTML a partir de texto que qualquer pessoa do quadro
// escreveu. Uma biblioteca completa exigiria um sanitizador junto (e acertar a
// configuração dele), e as duas coisas viram dependências que precisam ser
// acompanhadas por causa de CVE.
//
// Aqui a ordem é a garantia: TUDO é escapado primeiro, e só depois as marcas do
// subconjunto viram tags. Nada que veio do texto original consegue chegar ao
// HTML como marcação — nem `<script>`, nem `onerror=`, nem `javascript:`.
//
// O subconjunto é o que o template do Trello usa nas descrições: títulos,
// negrito, itálico, código, listas, citação e link.

const ESCAPES: Record<string, string> = {
	'&': '&amp;',
	'<': '&lt;',
	'>': '&gt;',
	'"': '&quot;',
	"'": '&#39;'
};

function escapar(texto: string): string {
	return texto.replace(/[&<>"']/g, (c) => ESCAPES[c]);
}

// urlSegura aceita só http(s) e caminhos internos. Sem isso,
// `[clique](javascript:alert(1))` viraria execução de script no clique.
function urlSegura(url: string): string | null {
	const limpa = url.trim();
	if (/^https?:\/\/[^\s]+$/i.test(limpa)) return limpa;
	if (/^\/[^/\s][^\s]*$/.test(limpa)) return limpa;
	return null;
}

// aplicarInline cuida do que vale dentro de uma linha. Recebe texto JÁ escapado.
function aplicarInline(escapado: string): string {
	return (
		escapado
			// `código` primeiro: o que estiver dentro dele não deve virar negrito
			.replace(/`([^`]+)`/g, '<code>$1</code>')
			// [texto](url) — a URL passa pela checagem de esquema
			.replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, (inteiro, rotulo: string, url: string) => {
				// A URL foi escapada junto com o resto; desfaz só o &amp; para
				// não quebrar query strings.
				const destino = urlSegura(url.replace(/&amp;/g, '&'));
				if (!destino) return inteiro;
				const externo = destino.startsWith('http');
				const atributos = externo ? ' target="_blank" rel="noopener noreferrer"' : '';
				return `<a href="${escapar(destino)}"${atributos}>${rotulo}</a>`;
			})
			.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
			.replace(/(^|[^*])\*([^*]+)\*/g, '$1<em>$2</em>')
			.replace(/~~([^~]+)~~/g, '<del>$1</del>')
	);
}

// renderizarMarkdown converte o texto em HTML seguro para usar com {@html}.
export function renderizarMarkdown(texto: string): string {
	if (!texto.trim()) return '';

	const linhas = escapar(texto).split('\n');
	const saida: string[] = [];
	let listaAberta: 'ul' | 'ol' | null = null;
	let blocoDeCodigo = false;

	const fecharLista = () => {
		if (listaAberta) {
			saida.push(`</${listaAberta}>`);
			listaAberta = null;
		}
	};

	for (const linha of linhas) {
		// bloco de código: nada dentro dele é interpretado
		if (linha.trim().startsWith('```')) {
			fecharLista();
			saida.push(blocoDeCodigo ? '</code></pre>' : '<pre><code>');
			blocoDeCodigo = !blocoDeCodigo;
			continue;
		}
		if (blocoDeCodigo) {
			saida.push(linha);
			continue;
		}

		if (!linha.trim()) {
			fecharLista();
			continue;
		}

		const titulo = /^(#{1,3})\s+(.*)$/.exec(linha);
		if (titulo) {
			fecharLista();
			const nivel = titulo[1].length + 2; // # vira h3, ## h4, ### h5
			saida.push(`<h${nivel}>${aplicarInline(titulo[2])}</h${nivel}>`);
			continue;
		}

		const citacao = /^&gt;\s?(.*)$/.exec(linha);
		if (citacao) {
			fecharLista();
			saida.push(`<blockquote>${aplicarInline(citacao[1])}</blockquote>`);
			continue;
		}

		const itemNumerado = /^\s*\d+\.\s+(.*)$/.exec(linha);
		const itemSimples = /^\s*[-*]\s+(.*)$/.exec(linha);
		if (itemNumerado || itemSimples) {
			const tipo = itemNumerado ? 'ol' : 'ul';
			if (listaAberta !== tipo) {
				fecharLista();
				saida.push(`<${tipo}>`);
				listaAberta = tipo;
			}
			saida.push(`<li>${aplicarInline((itemNumerado ?? itemSimples)![1])}</li>`);
			continue;
		}

		fecharLista();
		saida.push(`<p>${aplicarInline(linha)}</p>`);
	}

	fecharLista();
	if (blocoDeCodigo) saida.push('</code></pre>');

	return saida.join('\n');
}
