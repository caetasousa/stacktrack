# 🔄 Publicação, varredura e deploy

> **Em uma frase:** como uma versão sai desta máquina e entra no ar — um comando,
> `make deploy` — e como varrer as vulnerabilidades do que está publicado.

Quem chega sem contexto começa por [como-funciona.md](como-funciona.md). O
servidor em si está em [producao.md](producao.md); o Ansible, em
[infraestrutura.md](infraestrutura.md).

**Não há esteira.** O GitHub Actions foi removido: nada roda sozinho a cada
push. O servidor continua nunca compilando nada — ele só puxa imagem pronta do
GHCR —, e quem constrói e publica essa imagem é a sua máquina.

---

## 1️⃣ Antes: testar

```bash
make verificar            # o que a esteira conferia, sem Docker (o deploy roda de novo)
make test-integracao      # casos de uso e servidor HTTP contra Postgres real (Docker)
make test-e2e             # Playwright sobre a stack (exige `make run` no ar)
```

O `make verificar` reúne o que a esteira fazia sozinha: `gofmt`, `go vet`, a
regra de que **migration não escreve dado** (o grep do
[CLAUDE.md](../CLAUDE.md#migrations-banco-de-dados)), os testes com `-race`, o
Prettier, o `svelte-check` e os testes de unidade do frontend. A integração e o
E2E ficam de fora do deploy porque precisam de Docker e da stack local de pé —
rode-os antes. O `make test-e2e` termina gerando
`frontend/e2e/relatorio/resumo.md`, com o resultado e o print de cada cenário.

---

## 2️⃣ Implantar: `make deploy`

Com o commit feito **e já no `origin/main`**:

```bash
make deploy                       # verifica, publica e implanta o commit atual
PULAR_VERIFICAR=1 make deploy     # o mesmo, quando o verificar acabou de passar
```

O [`scripts/deploy.sh`](../scripts/deploy.sh) faz, nesta ordem, e para no
primeiro erro:

| Passo | O que confere ou faz |
|---|---|
| pré-voo | árvore limpa, o commit no `origin/main`, a chave de release e a senha do vault presentes |
| `make verificar` | a mesma checagem da seção anterior |
| login no GHCR | só se ainda não houver; usa o `GHCR_TOKEN` do ambiente ou, na falta dele, o token que o git guarda para o push (precisa de `write:packages`) — e sai no fim |
| imagens | build de `stacktrack-api`, `stacktrack-migrations` e `stacktrack-web` (esta com `PUBLIC_API_URL=/api`), com a tag no SHA e `latest`, e push |
| `make infra-apply` | compose, `backup.sh`, `.env` e papéis do banco; sem mudança, não mexe em nada |
| release | `stacktrack-release release <sha>` pela chave de deploy: backup, `pull`, para web e api, `up -d` |
| conferência | a API no ar é a do SHA, e `https://<dominio>/api/health` responde |

Ele **não faz `git push`**: produção nunca roda um commit que não está no
GitHub, e publicar o código é decisão sua, antes.

**A tag é sempre o SHA.** É o que garante que o servidor sobe exatamente o que
você testou, e é o que torna o rollback o mesmo release, com o SHA anterior:

```bash
make rollback SHA=<sha do commit bom>   # as imagens dele já estão no GHCR
make estado                             # containers, versão no ar, cron do backup
```

O banco não volta junto — ver [producao.md](producao.md#voltar-atrás).

Depois do deploy, o teste que vale mais que o `/api/health`: **cadastro → login
→ criar quadro → F5** (exercita o cookie `__Host-` de ponta a ponta).

### Por que os pacotes do GHCR são públicos

O servidor puxa as imagens sem login, e por isso não há token de leitura para
guardar no vault nem para trocar quando vencer. O que elas contêm é o mesmo que
o repositório: nenhum segredo vai na imagem — o `.env` mora só no servidor.

### A chave de deploy

O release entra com `~/.ssh/stacktrack_release`, presa no servidor ao wrapper
`stacktrack-release`, que só entende três verbos (`release`, `backup`,
`estado`) e nenhum que escreva arquivo. Por isso compose e `backup.sh` mudam
pelo `make infra-apply`, que entra com a chave do operador.
`scripts/testa-wrapper-de-release.sh` prova as recusas (shell, encadeamento,
substituição de comando, SHA malformado) e roda em `make infra-validar`.

---

## 4️⃣ Varredura de vulnerabilidades

Rode periodicamente (uma dependência não fica insegura só quando você commita).

### O modelo mental: a aplicação tem três camadas

Cada ferramenta enxerga uma, e **nenhuma enxerga a do lado**:

| Camada | O que é | Ferramenta |
|---|---|---|
| Dependências Go | `pgx`, `chi`, `x/text`, stdlib | `govulncheck` |
| Dependências npm | `svelte`, `vite`, `adapter-node` | `npm audit` |
| Sistema da imagem | Alpine, glibc, OpenSSL, JRE do Flyway | `Trivy` |

Rodar só uma dá a sensação de cobertura sem a cobertura.

### 🔷 govulncheck — o diferencial é a *alcançabilidade*

```bash
cd backend
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

Ele não lista CVE que existe no `go.mod`: lista a que o **seu código de fato
alcança**, com o rastro de chamadas. Isso separa "temos essa dependência" de
"temos esse risco". Achado de *module* (não alcançável) não é problema.

### 🟠 npm audit

```bash
cd frontend
npm audit --omit=dev --audit-level=high   # o que vai para produção
npm audit                                  # relatório completo
```

Dependência de **desenvolvimento** não vai para produção: uma CVE no Vite não é
risco do que está no ar. Mas o relatório completo vale a leitura — saber é
diferente de barrar.

### 🐳 Trivy — o que os outros dois não veem

```bash
for img in stacktrack-api stacktrack-web stacktrack-migrations; do
  trivy image --severity HIGH,CRITICAL --ignorefile .trivyignore ghcr.io/caetasousa/$img:latest
done
```

O binário Go pode estar limpo e a imagem carregar um OpenSSL com CVE crítica. O
Trivy varre as três — inclusive a de migrations, que roda no servidor com acesso ao
banco. `.trivyignore` guarda os achados aceitos conscientemente (HIGH em imagem
base costuma não ter correção).

---

## 5️⃣ Casos reais deste projeto

### Caso 1 — sete CVEs da stdlib do Go, achadas antes do primeiro deploy

Ao adicionar o `govulncheck`, ele reprovou de cara: **sete vulnerabilidades
alcançáveis**, seis delas da biblioteca padrão (`crypto/tls`, `crypto/x509`,
`net`, `net/mail`, `net/textproto`) e uma no `golang.org/x/text`.

A causa não era uma dependência ruim: era o **toolchain** parado em 1.26.2.

```
go get go@1.26.5
go get golang.org/x/text@v0.39.0
→ "Your code is affected by 0 vulnerabilities."
```

A tentação era ignorar o passo. Isso teria publicado as sete em produção.

### Caso 1b — as mesmas sete viraram oito, e a correção foi a mesma linha

Meses depois o `govulncheck` reprovou de novo: **nove achados**, com o toolchain
em 1.26.5. O rastro assustava — `crypto/tls` a partir do `ListenAndServe`,
`encoding/xml` a partir de um `Scan` do pgx, `encoding/asn1` a partir do
`Close` do pool.

Oito dos nove eram a mesma causa de sempre: a stdlib da 1.26.5, corrigida na
1.26.6. `go 1.26.6` no `go.mod` zerou os oito.

O nono não tem conserto por atualização, e é o mais interessante:

```
GO-2026-5932  golang.org/x/crypto  →  Fixed in: N/A
```

É o aviso de que `golang.org/x/crypto/openpgp` não tem manutenção. O `x/crypto`
está no nosso grafo pelo **argon2** (hash de senha), `blake2b` e `sha3` — o
`openpgp` não entra no binário, e por isso o govulncheck o classifica como
*module*, não como alcançável. Não há versão que resolva: a correção é não
importar aquele subpacote, o que já é o caso. O `exit 0` do govulncheck
confirma — achado de módulo não reprova.

A lição que se repete: **o rastro de chamadas assusta mais que a causa**. Ler
"seu ListenAndServe alcança um bug de TLS" sugere um problema de arquitetura;
era o número numa linha do `go.mod`.

### Caso 2 — Flyway: a correção é *remover*, não atualizar

A imagem oficial do Flyway embarca centenas de MB de drivers para vinte e tantos
bancos. O stacktrack fala com PostgreSQL e nada mais. Couchbase, Databricks e
SQL Server trazem junto netty, jackson e mssql-jdbc — e são a origem da maioria
dos alertas.

Não existe versão corrigida de uma dependência que não deveria estar ali. A
correção é `rm -rf` no Dockerfile.

**Cuidado:** o Flyway resolve os plugins por `ServiceLoader` e os carrega
avidamente — remover o driver errado derruba o boot com `ClassNotFoundException`
mesmo sem uso nenhum daquele banco. O teste que vale é `flyway migrate` contra um
Postgres de verdade.

### Caso 3 — o bug que só apareceu porque o script rodou

Não é CVE, mas é o mesmo tipo de lição. O `backup.sh` empacotava os anexos com
`tar --sort=name` numa imagem `alpine` — que traz o **tar do BusyBox**, sem essa
opção. E usava o `-z` do tar, que chama o gzip **sem** `-n` e carimba a data no
cabeçalho.

O primeiro quebrava em voz alta. O segundo não quebraria nada: o backup
funcionaria, mas cada execução geraria bytes diferentes para o mesmo conteúdo, a
deduplicação nunca acertaria, e a descoberta viria meses depois na conta do
armazenamento.

Nenhuma revisão de código pegaria os dois. Rodar pegou.

---

## 6️⃣ Achou algo. E agora?

**`govulncheck`** — leia o rastro de chamadas. Se a função vulnerável for
alcançável, atualize a dependência (ou o toolchain, quando for stdlib). Se não
for, ele não teria reclamado: alcançabilidade é o critério dele.

**`npm audit --omit=dev`** — é dependência de produção. `npm audit fix`, e se
exigir major, avalie a mudança. Se a correção não existir ainda, a decisão de
seguir é consciente e merece registro.

**Trivy em CRITICAL** — normalmente é a imagem base. Suba a versão do Alpine, do
Node ou do Postgres. Se não houver correção publicada, a saída é trocar de base
ou remover o pacote (caso 2), e registrar no `.trivyignore` o que ficou.

**Deploy que não responde** — o `make deploy` parou na conferência.
`docker compose -f docker-compose.prod.yml logs api` no servidor. Costuma ser
migration que falhou ou `.env` incompleto.

---

## 7️⃣ O que fica de fora, e você precisa saber

- **Regressão automática.** Sem CI, nada roda os testes por você antes de um
  merge — a disciplina é sua.
- **Recuperação de desastre.** O `release` tira um backup, mas ninguém restaura
  um dump para provar que banco + anexos formam um conjunto recuperável. O
  ensaio de restauração é manual, em `producao.md`.
- **Dependabot / atualização de dependência.** Manual; quem avisa é você rodando
  a varredura abaixo.
- **Análise estática de segurança** (CodeQL, gosec). Não configurada.

---

## 📚 Para estudar

- [govulncheck](https://go.dev/blog/govulncheck) — por que alcançabilidade muda o jogo
- [Trivy](https://trivy.dev/latest/docs/) — varredura de imagem
- [docker build / GHCR](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
