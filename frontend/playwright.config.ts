// Testes de ponta a ponta: navegador de verdade contra a stack de
// desenvolvimento inteira (Postgres, API, frontend).
//
// A stack NÃO é subida por aqui. `webServer` do Playwright sabe subir um
// processo, e o que estes testes precisam são quatro — banco, migrations, API e
// frontend —, que o `make run` já orquestra com as dependências na ordem certa.
// Duplicar isso na configuração criaria uma segunda forma de subir o projeto,
// que divergiria da primeira no dia em que alguém mexesse só numa.
//
//   make run          # noutro terminal
//   make test-e2e
import { defineConfig, devices } from '@playwright/test';

const BASE = process.env.E2E_BASE_URL ?? 'http://localhost:5173';

export default defineConfig({
	testDir: './e2e',
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

	// Na esteira, um teste que passa só na segunda tentativa é um teste que
	// esconde intermitência. Localmente, uma retentativa poupa o incômodo de
	// reexecutar à mão quando a stack ainda está esquentando.
	retries: process.env.CI ? 0 : 1,
	forbidOnly: !!process.env.CI,

	reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : [['list']],

	use: {
		baseURL: BASE,
		// O trace só é guardado quando o teste falha na retentativa: é o que
		// permite abrir depois e ver o que a tela mostrava, sem encher o disco
		// com o rastro de execuções que deram certo.
		trace: 'on-first-retry',
		screenshot: 'only-on-failure',
		video: 'retain-on-failure'
	},

	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }]
});
