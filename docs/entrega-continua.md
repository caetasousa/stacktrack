# 🔄 Publicação, varredura e deploy — tudo manual

**Não há esteira.** O GitHub Actions foi removido. Testar, buildar, publicar as
imagens e implantar são passos que você roda à mão, na ordem abaixo. O servidor
continua nunca compilando nada — ele só puxa imagem pronta do GHCR.

O roteiro do servidor está em [producao.md](producao.md); o do host, em
[infraestrutura.md](infraestrutura.md).

---

## 1️⃣ Antes de publicar: testar

```bash
make test                 # backend rápido + frontend
cd frontend && npm run check   # tipos (svelte-check)
cd backend  && make test       # com -race, o detector de corrida
cd backend  && make test-integracao   # repositórios contra Postgres real (Docker)
make test-e2e             # Playwright sobre a stack (exige `make run` no ar)
```

Migration nova: além dos testes de integração, confira à mão que ela **não
escreve dado** (a regra do [CLAUDE.md](../CLAUDE.md#migrations-banco-de-dados)) —
não há mais automação para isso:

```bash
grep -rniE '^[[:space:]]*(INSERT|UPDATE|DELETE)[[:space:]]' backend/migrations/
# nada = ok. Achou algo → mova o backfill para um comando do domínio.
```

E valide o Ansible antes de aplicar:

```bash
make infra-validar        # ansible-lint + --syntax-check nos dois playbooks
```

---

## 2️⃣ Publicar as imagens no GHCR

Três imagens: `stacktrack-api`, `stacktrack-web`, `stacktrack-migrations`. A do
`web` leva `PUBLIC_API_URL=/api` embutido no build.

```bash
sha=$(git rev-parse HEAD)
echo "$GHCR_TOKEN" | docker login ghcr.io -u caetasousa --password-stdin

docker build -t ghcr.io/caetasousa/stacktrack-api:$sha        -t ghcr.io/caetasousa/stacktrack-api:latest        -f backend/Dockerfile.prod       backend
docker build -t ghcr.io/caetasousa/stacktrack-web:$sha        -t ghcr.io/caetasousa/stacktrack-web:latest        -f frontend/Dockerfile.prod --build-arg PUBLIC_API_URL=/api frontend
docker build -t ghcr.io/caetasousa/stacktrack-migrations:$sha -t ghcr.io/caetasousa/stacktrack-migrations:latest -f backend/Dockerfile.migrations backend

for img in stacktrack-api stacktrack-web stacktrack-migrations; do
  docker push --all-tags ghcr.io/caetasousa/$img
done
```

O token só precisa de `write:packages`. Os pacotes nascem privados — ou você os
torna públicos (Packages → Package settings), ou o servidor faz `docker login`
(o `make infra-apply` faz, se `vault_ghcr_token` estiver no vault).

**Fixe sempre a tag no SHA.** É o que garante que o servidor sobe exatamente o
que você testou, e é o que torna o rollback uma linha no `.env`.

---

## 3️⃣ Implantar

```bash
# config (compose, backup.sh, papéis do banco) — só quando esses mudam:
make infra-apply

# o deploy em si:
ssh -i ~/.ssh/stacktrack_deploy -p 2222 stacktrack@127.0.0.1 "stacktrack-release release $sha"
```

O wrapper (`stacktrack-release`, instalado pelo Ansible) faz, nessa ordem:
backup pré-release, `pull`, para `web` e `api` juntas, `up -d`, limpa imagens
órfãs. Depois, confira:

```bash
curl -fsS https://stacktrack.d4ffy.com/api/health
curl -sI  https://stacktrack.d4ffy.com | head -1
```

E o teste que vale mais que os `curl`: **cadastro → login → criar quadro → F5**
(exercita o cookie `__Host-` de ponta a ponta).

O wrapper é a fronteira do que uma chave de deploy alcança —
`scripts/testa-wrapper-de-release.sh` prova as recusas (shell, encadeamento,
substituição de comando, SHA malformado, allowlist do `sync`) e roda em
`make infra-validar`.

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

**Deploy que não responde** — a aplicação não respondeu no `curl` pós-release.
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
  a §4.
- **Análise estática de segurança** (CodeQL, gosec). Não configurada.

---

## 📚 Para estudar

- [govulncheck](https://go.dev/blog/govulncheck) — por que alcançabilidade muda o jogo
- [Trivy](https://trivy.dev/latest/docs/) — varredura de imagem
- [docker build / GHCR](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
