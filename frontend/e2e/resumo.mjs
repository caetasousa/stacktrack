// Gera e2e/relatorio/resumo.md a partir do resultados.json do Playwright.
//
// O resumo é o artefato que se confere sem rodar nada: cada id do CENARIOS.md
// com o resultado do teste que o cobre e o print do estado final, mais o
// commit contra o qual a suíte rodou — é o que torna a execução repetível. Um
// id sem teste aparece como lacuna; um teste sem id no catálogo, também.
//
// Não chama git: o commit chega por E2E_COMMIT, que o Makefile calcula.

import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const pasta = dirname(fileURLToPath(import.meta.url));
const relatorio = join(pasta, 'relatorio');
const arquivoDeResultados = join(relatorio, 'resultados.json');

if (!existsSync(arquivoDeResultados)) {
	console.error('resumo: e2e/relatorio/resultados.json não existe — a suíte rodou?');
	process.exit(1);
}
const resultados = JSON.parse(readFileSync(arquivoDeResultados, 'utf8'));

// Os cenários, na ordem do catálogo: | E4 | como quebra | o que o usuário vê |
const cenarios = [];
for (const linha of readFileSync(join(pasta, 'CENARIOS.md'), 'utf8').split('\n')) {
	const m = /^\| ([A-Z]\d+) \| (.+?) \| (.+?) \|$/.exec(linha);
	if (m) cenarios.push({ id: m[1], quebra: m[2] });
}

// A primeira linha do erro e, se houver, o seletor que não achou o que queria.
function resumirErro(mensagem) {
	const linhas = mensagem
		.replace(/\u001b\[\d+m/g, '')
		.split('\n')
		.map((l) => l.trim());
	const alvo = linhas.find((l) => /^(Locator|Expected|Received):/.test(l));
	return [linhas[0], alvo].filter(Boolean).join(' — ');
}

// Os testes, achatados: título, ids, estado, duração e o print final. Um teste
// pode cobrir mais de um cenário — `[F2][F3] …` —, quando o mesmo percurso pela
// tela é o que prova todos eles.
const testes = [];
function percorrer(suite) {
	for (const spec of suite.specs ?? []) {
		for (const t of spec.tests ?? []) {
			const r = t.results.at(-1) ?? {};
			const print = (r.attachments ?? []).find((a) => a.name === 'screenshot' && a.path);
			testes.push({
				ids: [...spec.title.matchAll(/\[([A-Z]\d+)\]/g)].map((m) => m[1]),
				titulo: spec.title.replace(/^(\[[A-Z]\d+\])+\s*/, ''),
				arquivo: spec.file,
				estado: t.status === 'expected' ? 'passou' : t.status === 'skipped' ? 'pulado' : 'FALHOU',
				erro: resumirErro(r.error?.message ?? ''),
				print: print ? relative(relatorio, print.path) : ''
			});
		}
	}
	for (const s of suite.suites ?? []) percorrer(s);
}
for (const s of resultados.suites) percorrer(s);

const passou = testes.filter((t) => t.estado === 'passou').length;
const falhou = testes.filter((t) => t.estado === 'FALHOU');
const pulado = testes.filter((t) => t.estado === 'pulado');
const semTeste = cenarios.filter((c) => !testes.some((t) => t.ids.includes(c.id)));
const semId = testes.filter((t) => !t.ids.some((id) => cenarios.some((c) => c.id === id)));

const linhas = [
	'# Resultado da suíte E2E',
	'',
	`- quando: ${new Date(resultados.stats.startTime).toLocaleString('pt-BR', { timeZone: 'America/Sao_Paulo' })}`,
	`- commit: ${process.env.E2E_COMMIT || '?'}`,
	`- duração: ${(resultados.stats.duration / 1000).toFixed(1)} s`,
	`- **${passou} de ${testes.length} passaram**` +
		(falhou.length ? `, ${falhou.length} falharam` : '') +
		(pulado.length ? `, ${pulado.length} pulados` : ''),
	'',
	'Refazer: `make run` noutro terminal e `make test-e2e`.',
	'',
	'## Por cenário',
	'',
	'| id | como quebra | resultado | print |',
	'|---|---|---|---|',
	...cenarios.map((c) => {
		const t = testes.find((x) => x.ids.includes(c.id));
		if (!t) return `| ${c.id} | ${c.quebra} | sem teste | |`;
		return `| ${c.id} | ${c.quebra} | ${t.estado} | ${t.print ? `[print](${t.print})` : ''} |`;
	}),
	''
];

if (falhou.length) {
	linhas.push('## Falhas', '');
	for (const t of falhou) {
		linhas.push(
			`- **${t.ids.map((id) => `[${id}]`).join('')} ${t.titulo}** (${t.arquivo}): ${t.erro}`
		);
	}
	linhas.push('', 'O relatório HTML (`html/index.html`) tem o trace de cada falha.', '');
}
if (semTeste.length) {
	linhas.push('## Lacunas', '', ...semTeste.map((c) => `- ${c.id}: ${c.quebra}`), '');
}
if (semId.length) {
	linhas.push('## Testes sem cenário no catálogo', '', ...semId.map((t) => `- ${t.titulo}`), '');
}

writeFileSync(join(relatorio, 'resumo.md'), linhas.join('\n'));
console.log(
	`resumo: ${passou}/${testes.length} passaram` +
		(falhou.length ? ` — falharam: ${falhou.flatMap((t) => t.ids).join(', ')}` : '') +
		' · e2e/relatorio/resumo.md'
);
