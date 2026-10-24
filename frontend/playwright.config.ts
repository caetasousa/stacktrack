// Testes de ponta a ponta: navegador de verdade contra a stack de
// desenvolvimento inteira (Postgres, API, frontend e Mailpit).
//
// A stack NÃO é subida por aqui. `webServer` do Playwright sabe subir um
// processo, e o que estes testes precisam são cinco — banco, migrations, API,
// frontend e Mailpit —, que o `make run` já orquestra com as dependências na
// ordem certa. Duplicar isso na configuração criaria uma segunda forma de subir
// o projeto, que divergiria da primeira no dia em que alguém mexesse só numa.
//
//   make run          # noutro terminal
//   make test-e2e     # roda a suíte e gera e2e/relatorio/resumo.md
import { defineConfig, devices } from '@playwright/test';

const BASE = process.env.E2E_BASE_URL ?? 'http://localhost:5173';

export default defineConfig({
	testDir: './e2e',
	outputDir: './e2e/relatorio/artefatos',
	// Um teste de tempo real espera evento chegar pela rede; o padrão de 30s do
	// Playwright é curto quando a stack acabou de subir e o Vite ainda compila
	// a página sob demanda.
	timeout: 60_000,
	expect: { timeout: 15_000 },

	// Sem paralelismo entre arquivos: os testes criam contas, e cadastro e login
	// têm teto por IP. Rodando junto, a suíte derrubaria a si mesma com 429 —
	// foi exatamente o que aconteceu com o teste de duas abas em Go.
	workers: 1,
	fullyParallel: false,

	// Sem nova tentativa: um teste que passa na segunda vez esconde exatamente a
	// intermitência que o E2E existe para mostrar.
	retries: 0,
	// Um `test.only` esquecido faria a suíte rodar um teste e sair verde.
	forbidOnly: true,

	// O relatório é o artefato da execução: o HTML guarda o trace de cada falha,
	// o JSON alimenta o e2e/resumo.mjs, que monta o resumo por cenário.
	reporter: [
		['list'],
		['html', { outputFolder: './e2e/relatorio/html', open: 'never' }],
		['json', { outputFile: './e2e/relatorio/resultados.json' }]
	],

	use: {
		baseURL: BASE,
		locale: 'pt-BR',
		timezoneId: 'America/Sao_Paulo',
		trace: 'retain-on-failure',
		// Todo teste termina com um print: é o que deixa o relatório conferível
		// por quem não rodou a suíte. Os testes que abrem contextos próprios
		// anexam o deles por `printFinal` (e2e/apoio.ts).
		screenshot: 'on',
		video: 'retain-on-failure'
	},

	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }]
});
