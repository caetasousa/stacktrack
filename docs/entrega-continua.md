# 🔄 Entrega contínua e varredura de vulnerabilidades

A esteira testa, varre, publica e implanta. Nada chega ao VPS sem ter passado
por tudo o que vem antes — e o servidor nunca compila nada.

O roteiro do servidor está em [producao.md](producao.md).

---

## 1️⃣ O caminho de um commit até o ar

```
push na main
     │
     ├─► backend            gofmt · go vet · go test -race · migrations não escrevem dado
     ├─► frontend           prettier · svelte-check · vitest
     ├─► e2e                stack completa · Playwright · celular · duas pessoas
     ├─► seguranca-codigo   govulncheck · npm audit
     ├─► infra              ansible-lint · syntax-check · wrapper — SEM a senha do vault
     ├─► imagens            (matriz: api, web, migrations)
     └─► seguranca-imagens  Trivy por imagem → SARIF → portão em CRITICAL
                    │
                    ▼  todos verdes
            publicar-imagens        GHCR: :latest e :<sha>
                    │
                    ▼
                implantar           SSH → `stacktrack-release release <sha>`
                    │                     (o wrapper faz backup, pull, troca)
                    │                     → confere /api/health e /
                    ▼
        https://stacktrack.caetasousa.tech
```

### O job `infra`, e por que ele não tem a senha do vault

A infraestrutura é código e passa pela mesma esteira: `--syntax-check` nos dois
playbooks e `ansible-lint` no diretório, com `publicar-imagens` dependendo dele.

O que o job **não** tem é a chave dos segredos de produção, e é por isso que ele
existe nesta forma. Enquanto o arquivo cifrado morava em `group_vars/producao/`,
o Ansible o decifrava ao montar as variáveis do grupo — validar o playbook aqui
exigiria dar a senha do vault ao CI, que é justamente o raio de explosão que se
está tentando reduzir. Hoje o vault entra por uma task `include_vars` no
`provisionar.yml`, que só abre quando a play roda de verdade.

O job confere que `.senha-vault` não existe no runner antes de validar: sem
isso, um dia em que a senha vazasse para o ambiente faria o critério passar por
acidente. Ver [infraestrutura.md](infraestrutura.md).

### O deploy manda três verbos, e não comandos

O job `implantar` não abre shell no servidor. A chave da esteira está no
`authorized_keys` com `restrict` e `command="/usr/local/bin/stacktrack-release"`:
o que o runner escreve chega em `SSH_ORIGINAL_COMMAND` e o wrapper decide se
aquilo é `release <sha>`, `backup` ou `estado` — ou nada.

Antes, o job montava linhas de comando no runner e as executava do outro lado.
Quem tivesse a chave tinha a máquina — que roda a borda Traefik e os outros sites
do VPS.

Duas consequências que valem conhecer:

- **A esteira não copia arquivo nenhum.** O compose e o `backup.sh` são do
  Ansible; o roteamento é do `loadbalancer`. Em troca, o passo `estado` compara
  o sha256 dos dois com os do commit e **falha** quando divergem — deploy de
  código com configuração velha é justamente o erro silencioso que a divisão de
  donos poderia ter criado. A instrução na mensagem é `make infra-apply`.
- **O job declara `environment: production`.** É lá que ficam os segredos do
  VPS, e é o que impede um job de pull request de alcançá-los: quem não declara
  o environment não enxerga os segredos dele.

### Por que o cancelamento automático não vale para a main

`concurrency.cancel-in-progress` é de **nível de workflow**: ele alcança todos os
jobs do run anterior, inclusive o `implantar`. Um segundo push durante o deploy
mataria o job no meio — entre o `pull` e o `up -d`, deixando a stack em versões
misturadas, ou no meio da reescrita do crontab, que é um
`crontab -l | ... | crontab -` e truncaria o agendamento do backup sem avisar.

Por isso o cancelamento é ligado só em pull request:

```yaml
cancel-in-progress: ${{ github.event_name == 'pull_request' }}
```

### Por que o VPS só faz `pull`, nunca `build`

Compilar Go e rodar `npm ci` no servidor consome a RAM que a aplicação — e a
borda Traefik e os outros sites do VPS — está usando. Além disso, build no
servidor significa que o artefato em produção **nunca foi testado**: ele é uma
segunda compilação, feita noutra máquina, em outro momento.

### Por que a tag é o SHA do commit

O deploy roda `IMAGE_TAG=<sha> docker compose pull`. Fixar no SHA garante que o
servidor sobe **exatamente** o artefato que passou nos testes — `latest` pode ter
mudado entre a publicação e o pull. É também o que torna o rollback trivial: uma
linha no `.env`.

### Por que o cron não implanta

A varredura semanal (`schedule`) roda sem commit novo. Reimplantar produção
sozinho toda segunda-feira não é o objetivo — ela existe para descobrir CVE
publicada numa semana sem push. Por isso `publicar-imagens` e `implantar` exigem
`github.event_name == 'push' || workflow_dispatch'`.

### Por que push só de documentação não roda nada

`paths-ignore` com `**.md` e `docs/**`. São minutos de runner para compilar e
varrer exatamente o mesmo código de antes. É seguro porque nenhum markdown é
entrada de build.

Consequência: push só de documentação também não implanta — e está certo, já que
o deploy só manda `release <sha>` ao wrapper. Para reimplantar sem commit novo,
existe o `workflow_dispatch`.

---

## 2️⃣ Varredura de vulnerabilidades

### O modelo mental: a aplicação tem três camadas

Cada ferramenta enxerga uma, e **nenhuma enxerga a do lado**:

| Camada | O que é | Ferramenta |
|---|---|---|
| Dependências Go | `pgx`, `chi`, `x/text`, stdlib | `govulncheck` |
| Dependências npm | `svelte`, `vite`, `adapter-node` | `npm audit` |
| Sistema da imagem | Alpine, glibc, OpenSSL, JRE do Flyway | `Trivy` |

Rodar só uma dá a sensação de cobertura sem a cobertura.

### 🔷 govulncheck — o diferencial é a *alcançabilidade*

Ele não lista CVE que existe no `go.mod`: lista a que o **seu código de fato
alcança**, com o rastro de chamadas. Isso separa "temos essa dependência" de
"temos esse risco".

A versão é **fixa** (`@v1.6.0`), e não `@latest`: com latest, uma release nova da
ferramenta reprova um commit que não mexeu em nada, e o build de ontem deixa de
ser reproduzível hoje. A varredura semanal já garante que CVEs novas apareçam.

### 🟠 npm audit — por que roda duas vezes

```yaml
- run: npm audit --omit=dev --audit-level=high   # trava o CI
- run: npm audit || true                          # relatório, não trava
```

Dependência de **desenvolvimento** não vai para produção: uma CVE no Vite não é
risco do que está no ar. Travar nisso transformaria o portão em ruído que se
aprende a ignorar. Mas o relatório completo continua aparecendo no log, porque
saber é diferente de barrar.

### 🐳 Trivy — o que os outros dois não veem

O binário Go pode estar limpo e a imagem carregar um OpenSSL com CVE crítica. O
Trivy varre as **três** imagens que vão para produção, inclusive a de migrations
— ela roda no VPS com acesso ao banco.

Três passos, nessa ordem:

1. **relatório** (HIGH + CRITICAL, não trava) — HIGH em imagem base costuma não
   ter correção disponível;
2. **SARIF** para a aba Security — no log o achado se perde no scroll e some no
   run seguinte; ali ele ganha histórico e data de aparecimento. `category`
   distinta por imagem, senão a última perna da matriz sobrescreve as anteriores;
3. **portão** (só CRITICAL, trava). Por último, para que relatório e SARIF subam
   mesmo quando ele reprova — é justamente na execução que falha que o registro
   importa mais.

---

## 3️⃣ Casos reais deste projeto

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

A tentação era marcar o passo como `continue-on-error` para a esteira ficar
verde. Isso teria publicado as sete em produção — que é exatamente o que o passo
existe para impedir.

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
confirma — achado de módulo não reprova a esteira.

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
Postgres de verdade. Naquela etapa, o ensaio aplicou as 13 migrations que
existiam com os drivers removidos.

### Caso 3 — o bug que só apareceu porque o script rodou

Não é CVE, mas é o mesmo tipo de lição. O `backup.sh` empacotava os anexos com
`tar --sort=name` numa imagem `alpine` — que traz o **tar do BusyBox**, sem essa
opção. E usava o `-z` do tar, que chama o gzip **sem** `-n` e carimba a data no
cabeçalho.

O primeiro quebrava o passo em voz alta. O segundo não quebraria nada: o backup
funcionaria, mas cada execução geraria bytes diferentes para o mesmo conteúdo, a
deduplicação nunca acertaria, e a descoberta viria meses depois na conta do
armazenamento.

Nenhuma revisão de código pegaria os dois. Rodar pegou.

---

## 4️⃣ Ficou vermelho. E agora?

**`govulncheck`** — leia o rastro de chamadas que ele imprime. Se a função
vulnerável for alcançável, atualize a dependência (ou o toolchain, quando for
stdlib). Se não for, ele não teria reclamado: alcançabilidade é o critério dele.

**`npm audit` no passo que trava** — é dependência de produção. `npm audit fix`,
e se exigir major, avalie a mudança. Se a correção não existir ainda, a decisão
de seguir é consciente e merece registro.

**Trivy em CRITICAL** — normalmente é a imagem base. Suba a versão do Alpine, do
Node ou do Postgres. Se não houver correção publicada, a saída é trocar de base
ou remover o pacote (caso 2).

**Deploy vermelho no passo de verificação** — a aplicação não respondeu em 60s.
`docker compose logs api` no VPS. Costuma ser migration que falhou ou `.env`
incompleto.

---

## 5️⃣ As proteções chatas do workflow

**`timeout-minutes` em todos os jobs.** Um `pull` travado seguraria o runner até
o limite de 6 horas do GitHub.

**`permissions: contents: read` no topo.** Menor privilégio por padrão; só
`publicar-imagens` (packages: write) e `seguranca-imagens` (security-events:
write) elevam, e só o que precisam.

**`VPS_KNOWN_HOSTS` obrigatório.** A alternativa preguiçosa é um `ssh-keyscan` na
hora do deploy — que aceita a identidade de quem responder e entrega a chave de
deploy junto.

**As actions locais em `.github/actions/`.** O Docker Hub limita pull anônimo por
IP e os runners compartilham IP: quando a cota estoura, o "Booting builder" do
buildx morre sem explicação. Autenticar (quando há segredo) e pré-baixar com
retentativa resolve. Sem os segredos, as actions não fazem nada — o repositório
funciona para quem clonar sem configurar nada.

**O deploy sem segredos não falha, só não faz nada.** `VPS_HOST` vazio marca o
job como ignorado. O repositório precisa funcionar antes de o VPS existir.

**A esteira não toca no proxy.** O roteamento de `stacktrack.caetasousa.tech`
está nas labels do `docker-compose.prod.yml`; o Traefik do `loadbalancer` as lê
pelo Docker. O deploy daqui só sobe os containers na rede `borda`.

---

## ⚠️ O que esta esteira **não** cobre

**Arrastar com mouse no Playwright.** A suíte cobre a colaboração em duas
sessões, a reconexão, o modal ao vivo, o link público e os gestos de toque. O
caso que ainda falta é pegar um card com mouse, soltá-lo em outra coluna e
verificar a mudança na segunda sessão.

**Recuperação real de desastre.** O workflow executa o script de backup antes
do deploy e confere o cron, mas não restaura automaticamente um dump nem prova
que os anexos e o banco formam um conjunto recuperável. O ensaio de restauração
continua sendo operação manual documentada em `producao.md`.

**Dependabot.** Não configurado. Hoje a atualização de dependência é manual, e
quem avisa é a varredura semanal.

**Análise estática de segurança do código** (CodeQL, gosec). A aba Security
recebe só os achados do Trivy.

---

## 📚 Para estudar

- [govulncheck](https://go.dev/blog/govulncheck) — por que alcançabilidade muda o
  jogo
- [Trivy](https://trivy.dev/latest/docs/) — varredura de imagem
- [SARIF no GitHub](https://docs.github.com/en/code-security/code-scanning/integrating-with-code-scanning/sarif-support-for-code-scanning)
- [GitHub Actions — workflow syntax](https://docs.github.com/en/actions/reference/workflow-syntax-for-github-actions)
- [OpenSSF — Security Scorecard](https://securityscorecards.dev/)
