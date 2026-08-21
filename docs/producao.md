# 🚀 Produção

O stacktrack roda num VPS que **já hospedava o agendaGo**. Não há proxy novo,
não há IP novo: o Caddy que já estava lá passa a atender os dois domínios, e as
duas aplicações convivem sem se enxergar.

Este documento é o roteiro do servidor. A esteira que leva o commit até aqui
está em [entrega-continua.md](entrega-continua.md).

---

## 🏗️ Arquitetura no ar

```
                     ┌──────────────── VPS ────────────────┐
  :80/:443 ─────────►│ Caddy  (na stack do agendaGo)       │
                     │   agenda.dominio        ─┐          │
                     │   stacktrack.duckdns.org ┼─┐        │
                     └──────────────────────────┼─┼────────┘
                        rede do agendaGo        │ │  rede `borda`
                     ┌──────────────────┐       │ │  ┌────────────────────────┐
                     │ agendago api/web │◄──────┘ └─►│ stacktrack-api   :8080 │
                     │ agendago postgres│            │ stacktrack-web   :3000 │
                     └──────────────────┘            │ stacktrack postgres    │
                                                     └────────────────────────┘
```

Dentro de cada domínio, front e API compartilham a origem: o `handle_path` do
Caddy manda `/api/*` para a API **removendo o prefixo**, e todo o resto para o
frontend.

Nenhum serviço do stacktrack publica porta no host. Quem fala com o mundo é o
Caddy; o Postgres não é alcançável nem pelo vizinho.

Do lado do agendaGo isso já está pronto: o Caddyfile dele importa
`/etc/caddy/sites/*.caddy`, o container participa da rede `borda`, e o CI dele
cria essa rede de forma idempotente a cada deploy. O stacktrack só precisa
depositar o próprio bloco em `/home/deploy/caddy/sites` — o que a esteira faz,
recarregando o proxy **apenas quando o arquivo muda**.

### Por que subdomínio, e não um caminho

`stacktrack.duckdns.org`, e não `dominio-do-agendago/stacktrack`.

O cookie de sessão tem `Path=/`, e o navegador o envia em **toda** requisição
daquele host. Servindo os dois no mesmo domínio, o agendaGo passaria a receber o
token de sessão de quem entra no stacktrack. Reduzir o `Path` não resolve: o
prefixo `__Host-` **exige** `Path=/`, e abrir mão dele custaria a garantia de
que o cookie pertence àquele host exato.

### Por que uma origem só, dentro do domínio

Três coisas dependem disso, e nenhuma é estética:

- o cookie `__Host-` exige `Secure`, `Path=/` e **nenhum** `Domain`;
- a CSP de produção é `connect-src 'self'` — API noutra origem exigiria abrir
  exceção;
- o `wss://` do quadro fica coberto por `self`, sem abrir uma origem adicional.

---

## 📦 O host não recebe o código-fonte

O servidor guarda **três arquivos** e nada além:

| Arquivo | Vem de | Contém |
|---|---|---|
| `~/stacktrack/docker-compose.prod.yml` | repositório, via CI | a topologia |
| `~/stacktrack/.env` | **só existe no host**, escrito pelo Ansible | segredos |
| `~/stacktrack/scripts/backup.sh` | repositório, via CI | operação |

Mais o bloco de roteamento em `~/caddy/sites/stacktrack.caddy`, que pertence ao
Caddy.

As migrations não são exceção a isso: os `.sql` viajam **dentro** da imagem
`stacktrack-migrations` (ver `backend/Dockerfile.migrations`). Sem ela, o job de
migration dependeria de um bind mount de `backend/migrations`, obrigando a
manter um clone do repositório no VPS — e a mantê-lo em sincronia com a imagem
que está rodando.

```
/home/deploy/
├── agendago/        Caddyfile (dono das portas 80/443), compose, .env, scripts
├── stacktrack/      compose, .env, scripts
├── caddy/sites/     um arquivo .caddy por vizinho
└── backups/         COMUM, separado pelo prefixo do nome do arquivo
```

---

# 🧭 Roteiro do primeiro deploy

## 1️⃣ DNS

Um registro apontando `stacktrack.duckdns.org` para o IP do VPS — o mesmo IP do
agendaGo. O Caddy separa por `server_name`.

**O DNS precisa resolver ANTES de subir a stack.** O Caddy só consegue o
certificado depois disso, e sem certificado o login não completa:
`APP_ENV=production` liga o `Secure` no cookie, o navegador o descarta, e o
sintoma é "entra e volta para a tela de login", sem erro visível em lugar nenhum.

## 2️⃣ Configurar o deploy no GitHub

Em **Settings → Secrets and variables → Actions**, do repositório do stacktrack:

| Tipo | Nome | Valor |
|---|---|---|
| Secret | `VPS_HOST` | IP do servidor |
| Secret | `VPS_USER` | `deploy` |
| Secret | `VPS_SSH_KEY` | chave **privada** de deploy |
| Secret | `VPS_KNOWN_HOSTS` | saída de `ssh-keyscan -H <ip>` |
| Variable | `DOMINIO` | `stacktrack.duckdns.org` |

A chave pública correspondente vai no `~/.ssh/authorized_keys` do usuário
`deploy`. Dá para reaproveitar a mesma chave que o agendaGo usa.

Sem `VPS_HOST`, o job de deploy passa marcado como ignorado em vez de falhar —
o repositório funciona antes de o servidor existir. Sem `VPS_KNOWN_HOSTS`, ele
**falha de propósito**: a alternativa seria aceitar a identidade de quem
responder e entregar a chave de deploy junto.

## 3️⃣ Provisionar o servidor

**Este passo deixou de ser manual.** O `.env`, os diretórios, o compose, o
`backup.sh`, o cron e o bloco do Caddy saem todos de um playbook Ansible:

```bash
make infra-segredos    # UMA vez: gera e cifra a senha do banco
make infra-check       # mostra o que mudaria, sem tocar em nada
make infra-apply       # aplica
```

Numa máquina nova, antes disso: `make infra-preparar`, que instala o Docker e
cria o usuário `deploy` (é a única parte que exige root). O procedimento
completo, os segredos e como recomeçar do zero estão em
[infraestrutura.md](infraestrutura.md).

O que continua valendo, e que o playbook não dispensa você de saber:

**A senha do Postgres vale para o `initdb` do primeiro boot.** Trocá-la depois
exige `ALTER ROLE` dentro do container — não basta mudar o vault. A role tem um
`assert` que recusa a divergência em vez de deixar a API falhar a conexão no
deploy seguinte.

**O `.env` não é enviado pela esteira.** Ele mora só no host; a diferença é que
agora quem o escreve é o Ansible, a partir do vault cifrado, e não um heredoc
copiado à mão. O modelo comentado, com a explicação de cada variável, continua
sendo o [`.env.prod.example`](../.env.prod.example).

**Arquivo que começa com ponto não aparece no `ls`.** Para conferir o resultado,
`ls -la`:

```bash
ls -la /home/deploy/stacktrack/         # o .env aparece aqui
grep -c . /home/deploy/stacktrack/.env  # 6 linhas
```

E o job `implantar` da esteira agora **confere isto antes de qualquer coisa**:
sem o `.env` ou sem a rede `borda`, ele falha com a instrução de rodar
`make infra-apply`, em vez de subir a stack com todas as variáveis vazias.

### 🔐 Usuário de banco sem poder de DDL — ainda pendente

Hoje a API se conecta com o **dono** do banco, que pode criar, alterar e
derrubar qualquer tabela. Uma falha de execução remota na API herdaria esse
poder.

O compose já lê `DB_USER`/`DB_PASSWORD` com fallback para o dono, e o
`.env.prod.example` já tem as linhas comentadas. Falta criar o papel com os
`GRANT` certos — o agendaGo tem um `scripts/criar-usuario-app.sh` que serve de
modelo. Quem aplica migration continua sendo o dono, pelo Flyway.

## 4️⃣ Liberar as imagens no GHCR

As imagens nascem **privadas**. Ou você as torna públicas em cada pacote
(Packages → Package settings → Change visibility), ou o servidor autentica:

```bash
echo "$GHCR_TOKEN" | docker login ghcr.io -u caetasousa --password-stdin
```

O token precisa apenas de `read:packages`.

Isso só funciona depois que a esteira tiver publicado as imagens ao menos uma
vez — antes disso os pacotes não existem.

## 5️⃣ Publicar

```bash
# na sua máquina, no repositório do stacktrack
git push
```

A esteira testa, varre, publica as três imagens e implanta: sincroniza o
compose e o `backup.sh`, instala o bloco do Caddy e o recarrega, tira um backup,
garante a rede, sobe a stack fixando a tag no SHA do commit, agenda o cron e
confere se `/api/health` e `/` respondem.

**A ordem entre 4️⃣ e 5️⃣ tem um nó**: o `docker login` precisa das imagens
publicadas, e as imagens só existem depois do primeiro push. Se os pacotes
nascerem privados, o primeiro deploy falha no `pull` — faça o push, torne os
pacotes públicos (ou rode o `docker login`), e reexecute a esteira pelo
**workflow_dispatch**, sem precisar de commit novo.

Do segundo deploy em diante não há passo manual nenhum. Ver
[entrega-continua.md](entrega-continua.md).

## 6️⃣ Conferir

A esteira já checa `/api/health` e `/` no fim do deploy — se ela ficou verde,
os dois responderam. O que ela não checa, e você deve:

```bash
# no servidor
cd ~/stacktrack
docker compose -f docker-compose.prod.yml ps      # api só fica healthy se enxergar o banco
docker compose -f docker-compose.prod.yml logs flyway   # quantas migrations aplicou

# o certificado é do domínio certo e não expirou
curl -sI https://stacktrack.duckdns.org | head -1

# o agendaGo continua no ar (o Caddy dele foi recarregado)
curl -sI https://<dominio-do-agendago> | head -1
```

E o teste que vale mais que todos: **criar uma conta, entrar, montar um quadro e
dar F5.** É ele que exercita o cookie `__Host-` de ponta a ponta — se o
certificado não estivesse válido, o login falharia aqui e em nenhum dos `curl`
acima.

---

# 🛠️ Operação

## Deploy do dia a dia

Automático pela esteira. Manualmente:

```bash
cd ~/stacktrack
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d
```

Migration nova vai junto: o `depends_on: service_completed_successfully` garante
que o Flyway termina antes de a API subir.

## Voltar atrás

```bash
$EDITOR .env       # IMAGE_TAG=<sha do commit bom>
docker compose -f docker-compose.prod.yml up -d
```

**O banco não volta junto.** O Flyway é forward-only: a migration aplicada
continua aplicada, e a imagem antiga encontra um schema mais novo do que ela
conhece. É por isso que aperto de schema exige dois deploys — um `SET NOT NULL`
no mesmo deploy que estreia a coluna transforma o rollback num erro no primeiro
`INSERT`. Ver `CLAUDE.md`, "Migration que aperta exige dois deploys".

## Backup

`scripts/backup.sh` roda por cron às 03:20 — deslocado do agendaGo (03:00)
porque dois `pg_dump` no mesmo minuto disputam CPU e disco sem necessidade. A
esteira agenda o cron sozinha, de forma idempotente, preservando a linha do
vizinho.

Ele grava dois artefatos em `~/backups`:

```
stacktrack-2026-08-07-032000-schema<versao>.sql.gz   + .sha256 + .json
stacktrack-anexos-2026-08-07-032000.tar.gz     + .sha256
```

### Por que dois artefatos

Os anexos dos cards são **arquivos num volume**, não linhas no banco: o
`pg_dump` não os inclui. Restaurar só o banco devolveria cards apontando para
anexos que não existem mais.

### Dia sem alteração não gera backup novo

A deduplicação olha três coisas — hash do conteúdo, versão do schema e tag da
imagem. Dado idêntico sob um schema ou uma imagem **nova** merece ponto de
restauração próprio: é justamente o retrato pré-migration que o rollback vai
querer.

Quando deduplica, o script dá `touch` no arquivo anterior. Não é detalhe: sem
isso, num banco parado por mais tempo que a retenção, a rotação apagaria o único
backup existente.

### O carimbo de versão

O `.json` guarda `schema_version` e `image_tag` **da imagem em execução**, lida
do container e não do `.env` — um deploy passa `IMAGE_TAG` por variável de
ambiente sem tocar no arquivo, e o `.env` diria `latest` num servidor rodando um
SHA fixo.

Sem esse carimbo, descobrir a que versão do código um backup pertence exige
restaurá-lo e ler o `flyway_schema_history`. A hora dessa arqueologia costuma ser
a hora do incidente.

### Testar a restauração

Um dump que ninguém nunca restaurou é uma esperança, não um backup:

```bash
cd ~/stacktrack
gunzip -c ~/backups/stacktrack-<data>.sql.gz | \
  docker compose -f docker-compose.prod.yml exec -T postgres \
  psql -q -U stacktrack -d ensaio
```

(criando antes o banco `ensaio` e derrubando depois).

### Quem avisa que o backup parou

Ninguém, hoje. O script aceita `URL_HEARTBEAT` e pinga ao terminar bem — o
alerta nasce da **ausência** do ping, porque um cron desativado, um VPS
desligado e um dump que falhou produzem o mesmo silêncio.

### O que ainda falta

A cópia sai do VPS? **Não.** `~/backups` cobre erro de aplicação e migration
ruim; não cobre perder a máquina. O Supabase Storage fala S3 e resolveria com
uma linha no fim do script — com uma chave que só possa **escrever** no bucket,
senão quem invadir o VPS apaga os backups junto.

## Diagnóstico

```bash
cd ~/stacktrack
alias dc='docker compose -f docker-compose.prod.yml'

dc ps                  # api fica healthy só se enxergar o banco
dc logs -f api         # log estruturado (slog), com request id
dc logs flyway         # o que a última migration fez
dc exec postgres psql -U stacktrack -d stacktrack -c '\dt'

# a API não publica porta: para falar com ela, entre pela rede
docker run --rm --network borda alpine:3.21 wget -qO- http://stacktrack-api:8080/ready
```

Os logs rotacionam em 10 MB × 3 arquivos por serviço. Sem isso, um log de acesso
enche o disco do VPS e derruba tudo junto — **inclusive o agendaGo**.

---

## 📊 Dimensionamento

Os `mem_limit` desta stack somam ~1,15 GB:

| Serviço | Teto | Por quê |
|---|---|---|
| postgres | 512m | |
| api | 384m | Argon2id usa ~19 MiB por hash simultâneo |
| web | 256m | servidor Node do adapter-node |

E o agendaGo tem os seus, mais o Caddy. `free -h` antes de subir. O sintoma de
estourar é a API morrendo por OOM em rajada de login.

## Contenção dos containers

Todos rodam com `no-new-privileges`, `cap_drop: ALL` e limite de processos.
`api` e `web` são `read_only` com `tmpfs` em `/tmp` — o upload de anexo precisa
de arquivo temporário, e é só isso que eles escrevem fora dos volumes. O
Postgres é a exceção: o entrypoint sobe como root e larga privilégio, então
mantém as cinco capabilities mínimas para essa troca.

---

## ✅ Checklist de go-live

- [ ] DNS resolvendo para o IP do VPS
- [ ] agendaGo publicado com o Caddyfile que importa `sites/*.caddy`
- [ ] segredos `VPS_*` e variável `DOMINIO` no repositório
- [ ] `make infra-apply` verde (cria o `.env` com senha aleatória e `chmod 600`)
- [ ] imagens acessíveis ao servidor (públicas ou `docker login` feito)
- [ ] esteira verde até o job `implantar`
- [ ] certificado emitido (`https://` abre sem aviso)
- [ ] cadastro → login → criar quadro → F5
- [ ] **o agendaGo continua no ar** depois do reload do Caddy
- [ ] cron do backup agendado (a esteira faz) e **um backup restaurado em ensaio**
- [ ] `free -h` com folga sobre a soma dos limites das duas stacks

---

## 🚨 Armadilhas conhecidas

**TLS não é o último passo.** Sem HTTPS válido, o cookie `Secure` é descartado e
o login falha em silêncio.

**Rollback não desfaz migration.** Flyway é forward-only. Toda migration que
aperta (`SET NOT NULL`, `DROP COLUMN`, `RENAME`, `UNIQUE` novo) exige dois
deploys.

**`WriteTimeout` e `ReadTimeout` precisam continuar em zero.** Um timeout do
`http.Server` vale para a conexão inteira, e o WebSocket é uma requisição que
dura horas. Os dois já estão zerados deliberadamente em
`backend/config/server.go`; reintroduzi-los faria o quadro cair sempre no mesmo
intervalo. O ping/pong do próprio WebSocket é quem limita conexão morta.

**O nome antigo sobrevive num comentário da migration V1.** O Flyway guarda
checksum de cada `.sql` e recusa o `validate` se o arquivo mudar — por isso a
renomeação do projeto não a tocou.
