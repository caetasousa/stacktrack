import tailwindcss from '@tailwindcss/vite';
import adapter from '@sveltejs/adapter-node';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';

export default defineConfig(({ mode }) => {
	const dev = mode !== 'production';

	return {
		// Sem isto o Vite só expõe variáveis com prefixo VITE_ (o SvelteKit não
		// configura o prefixo por conta própria), e `import.meta.env.PUBLIC_API_URL`
		// ficaria undefined no build de produção — a imagem cairia calada no
		// fallback localhost:8080 e o frontend publicado não acharia a API.
		envPrefix: 'PUBLIC_',
		plugins: [
			tailwindcss(),
			sveltekit({
				compilerOptions: {
					// Modo runes obrigatório no projeto, exceto em bibliotecas.
					// Pode sair no Svelte 6, quando runes vira o padrão.
					runes: ({ filename }) =>
						filename.split(/[/\\]/).includes('node_modules') ? undefined : true
				},

				// adapter-node gera o build de produção como servidor Node autônomo
				// (build/index.js) — o modo dev (npm run dev) não muda.
				adapter: adapter(),

				// Content-Security-Policy: a rede de proteção para o dia em que um XSS
				// escapar. `mode: 'hash'` deixa o SvelteKit hashear os scripts que ele
				// mesmo gera, o que permite `script-src 'self'` sem 'unsafe-inline'.
				//
				// Em desenvolvimento a API vive noutra origem (localhost:8080) e o Vite
				// abre um WebSocket para o HMR — daí as duas exceções em connect-src.
				// Na fase 5 o WebSocket do quadro passa por aqui: em dev ele já está
				// coberto por `ws:`; em produção, front e API ficam na mesma origem
				// atrás do proxy, e `self` cobre o wss:// também.
				csp: {
					mode: 'hash',
					directives: {
						'default-src': ['self'],
						'script-src': ['self'],
						// Tailwind e o SvelteKit injetam <style> inline no SSR; hash não
						// cobre atributo style, que os componentes usam.
						'style-src': ['self', 'unsafe-inline'],
						'img-src': ['self', 'data:'],
						'font-src': ['self'],
						// Em produção não há exceção a abrir: front e API compartilham a
						// origem, e `self` já cobre o wss:// do quadro — foi para isso
						// que o /api ficou no mesmo domínio.
						'connect-src': dev ? ['self', 'http://localhost:8080', 'ws:'] : ['self'],
						'object-src': ['none'],
						'base-uri': ['self'],
						'form-action': ['self'],
						'frame-ancestors': ['none']
					}
				}
			})
		],
		// Sem a condição `browser`, o Vite entrega a build de SERVIDOR do Svelte
		// para os testes, e `mount()` — que monta o componente de verdade para
		// conferir o que ele pinta — falha com "not available on the server".
		resolve: mode === 'test' ? { conditions: ['browser'] } : undefined,
		test: {
			include: ['src/**/*.test.ts'],
			// Node é o padrão porque quase todo teste aqui é de função pura; quem
			// precisa de DOM pede jsdom no cabeçalho do próprio arquivo.
			environment: 'node'
		}
	};
});
