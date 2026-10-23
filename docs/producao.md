# 🚀 Produção

O stacktrack roda na distro **`ubuntu-server` do WSL desta máquina**, publicada
por um **túnel da Cloudflare**. O servidor foi montado pelo `site.yml` do
studygo, que divide a máquina com ele; cada projeto tem o seu usuário e o seu
Docker rootless. O `cloudflared` do servidor termina o TLS lá fora e entrega
`stacktrack.d4ffy.com` em `http://localhost:8580` — a borda nginx que mora
**dentro** do `docker-compose.prod.yml`.

O servidor só fica no ar com o PC ligado e a distro de pé.

Este documento é o roteiro do servidor. Publicar as imagens e implantar (tudo
manual — não há esteira) está em [entrega-continua.md](entrega-continua.md).

---

## 🏗️ Arquitetura no ar

```
 internet ── HTTPS ──► Cloudflare ── túnel (conexão de SAÍDA) ──┐
                                                               │
 ┌────────────── ubuntu-server (WSL) ──────────────────────────┼──────┐
 │  cloudflared (do servidor)  stacktrack.d4ffy.com ──► localhost:8580 │
 │                                                             │      │
 │  Docker rootless do usuário `stacktrack`                    │      │
 │  ┌──────────────────────────────────────────────────────────▼───┐  │
 │  │ borda   nginx :8080 ── rede `borda` (10.89.0.0/24) ──┐        │  │
 │  │                         /api/* ─► api :8080  ◄───────┤        │  │
 │  │                         /*     ─► web :3000  ◄───────┘        │  │
 │  │ api ── rede default ──► postgres                              │  │
 │  └───────────────────────────────────────────────────────────────┘  │
 │  (o studygo tem o seu usuário, o seu Docker e a sua porta, 8480)    │
 └─────────────────────────────────────────────────────────────────────┘
```

Front e API compartilham a origem: a borda manda `/api/*` para a API
**removendo o prefixo**, e todo o resto para o frontend.

Só a `borda` publica porta, e só em `127.0.0.1:8580`. O WSL em modo NAT não
expõe a porta na rede local, e o túnel só abre conexões de saída: não há porta
aberta para fora nem firewall a manter. O Postgres não é alcançável de fora do
compose.

### A borda: o que o túnel alcança

O roteamento está **no serviço `borda` do `docker-compose.prod.yml`**, com a
config do nginx embutida no próprio arquivo (`configs.borda_nginx.content`) — não
num arquivo no servidor, e não neste repo em nenhum outro lugar:

```
location /api/   rewrite ^/api(/.*)$ $1  ->  api:8080   (Upgrade, timeout 1h)
location /                                ->  web:3000   (cabeçalhos de segurança)
```

`/api/boards` chega na API como `/boards`. É a origem única que faz o cookie
`__Host-` valer e a CSP em `connect-src 'self'` — o frontend chama `/api`
relativo (`PUBLIC_API_URL=/api` assado no build). O `/api/ws` passa pela mesma
location, que repassa o `Upgrade` do WebSocket e segura a conexão por até 1h.

A resposta do front leva HSTS, `X-Content-Type-Options`, `X-Frame-Options DENY`,
`Referrer-Policy`, `Permissions-Policy` e `Cross-Origin-Opener-Policy` (a CSP
**não** — quem a emite é o SvelteKit). O corpo tem teto de 12 MiB, um pouco
acima dos 11 da API, para que quem responda o 413 seja ela.

Três detalhes que não são óbvios:

- **O IP do visitante vem do `CF-Connecting-IP`.** Atrás do túnel, o peer da
  conexão é o `cloudflared`; o nginx aceita o cabeçalho de quem entra pela porta
  publicada (no rootless, chega pelo gateway `10.89.0.1`) e o repassa à API em
  `X-Real-IP`. A API confia nele só vindo da sub-rede da `borda`
  (`PROXIES_CONFIAVEIS: 10.89.0.0/24`). É por isso que a borda é nginx, e não
  Traefik: o Traefik preencheria o `X-Real-IP` com o IP do `cloudflared`, e todo
  mundo cairia num balde só do rate limit.
- **O upstream é resolvido a cada 10 s** (`resolver 127.0.0.11` + variável no
  `proxy_pass`). Sem isso o nginx guardaria o IP do boot, e todo `release` —
  que recria `api` e `web` — deixaria a borda apontando para containers mortos.
- **O Compose interpola `$`** na config embutida: as variáveis do nginx vão
  escritas como `$$`. E a `borda` é o único serviço sem `read_only` — o Compose
  recusa `configs` com `content` num serviço read_only. Ela compensa rodando
  como usuário sem privilégio (imagem `nginxinc/nginx-unprivileged`), sem
  capability nenhuma.

**Se a location `/api/` sumir**, o `/api` cai no SvelteKit, o login não completa
e o WebSocket não conecta.

**Regressão consciente:** o bloco Caddy de antigamente redigia do *access log do
proxy* os tokens de convite e de link público; o access log do nginx da borda não
redige. O log da **aplicação** (`internal/pkg/logging`) continua sanitizando.

### Por que subdomínio, e não um caminho

`stacktrack.d4ffy.com`, e não `d4ffy.com/stacktrack`.

O cookie de sessão tem `Path=/`, e o navegador o envia em **toda** requisição
daquele host. Servindo o stacktrack sob um caminho de outro domínio, o vizinho
passaria a receber o token de sessão de quem entra aqui. Reduzir o `Path` não
resolve: o prefixo `__Host-` **exige** `Path=/`, e abrir mão dele custaria a
garantia de que o cookie pertence àquele host exato.

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
| `~/docker-compose.prod.yml` | repositório, via Ansible | a topologia e a borda |
| `~/.env` | **só existe no host**, escrito pelo Ansible | segredos |
| `~/scripts/backup.sh` | repositório, via Ansible | operação |

O roteamento não é um quarto arquivo: é a config da `borda` dentro do
`docker-compose.prod.yml` (ver a seção anterior).

As migrations não são exceção a isso: os `.sql` viajam **dentro** da imagem
`stacktrack-migrations` (ver `backend/Dockerfile.migrations`). Sem ela, o job de
migration dependeria de um bind mount de `backend/migrations`, obrigando a
manter um clone do repositório no servidor — e a mantê-lo em sincronia com a
imagem que está rodando.

```
/home/stacktrack/             a aplicação (o home do usuário é a stack)
├── .env                      escrito pelo Ansible a partir do vault
├── docker-compose.prod.yml   inclui a borda nginx
├── scripts/backup.sh
├── backups/
└── .local/share/docker/      o Docker rootless: imagens e volumes do projeto

/usr/local/bin/stacktrack-release   o wrapper da chave de deploy
```

---

# 🧭 Roteiro do primeiro deploy

## 1️⃣ A rota do túnel

No painel da Cloudflare (Zero Trust → Networks → Tunnels), no túnel com token
que o servidor já roda, uma rota nova (**Routes** → *Published application*):

| Campo | Valor |
|---|---|
| Hostname | `stacktrack.d4ffy.com` |
| Service | `HTTP` → `localhost:8580` |

O painel cria sozinho o CNAME para `<id-do-túnel>.cfargotunnel.com`. Não há
certificado a emitir nem a renovar: o HTTPS é da Cloudflare. A rota pode existir
antes da stack — até ela subir, a Cloudflare responde 502.

O hostname precisa bater **exatamente** com `dominio` em
`group_vars/producao/vars.yml`. O `APP_ENV=production` liga o `Secure` no
cookie; com outro host, o navegador o descarta, e o sintoma é "entra e volta
para a tela de login", sem erro visível.

## 2️⃣ A chave e o wrapper de deploy

Uma chave dedicada ao deploy do stacktrack — não a de outro projeto do servidor. Uma
chave que abre dois projetos faz o comprometimento de um ser o comprometimento
do outro:

```bash
ssh-keygen -t ed25519 -C deploy-stacktrack -f ~/.ssh/stacktrack_deploy -N ""
```

A **pública** vai para `esteira_chave_publica` em
`deploy/ansible/group_vars/producao/vars.yml`, e o `make infra-preparar` a
instala no servidor com `restrict` e comando forçado apontando para o wrapper.
Guarde a **privada** onde só você a alcança.

### O que essa chave consegue fazer

Nada além de quatro verbos. Ela está no `authorized_keys` do usuário
`stacktrack` com `restrict` e `command="/usr/local/bin/stacktrack-release"`:
não abre shell, não faz `scp`, não encaminha porta nem agente.

A restrição é **da chave**, não da conta: a mesma conta aceita a sua chave de
operador sem limite algum e esta de deploy limitada a quatro verbos.

| Verbo | O que faz |
|---|---|
| `sync` | recebe o `docker-compose.prod.yml` pelo stdin, valida contra a allowlist (imagem do projeto, Postgres ou o nginx `unprivileged` da borda; sem `privileged`, namespace do host, bind mount ou capability nova) e instala. É o **único** que escreve arquivo |
| `release <sha>` | pré-voo, backup, `pull`, para web e api, `up -d`, limpa imagens órfãs |
| `backup` | um backup pontual |
| `estado` | `compose ps`, o sha256 do compose e do `backup.sh`, fuso e cron |

O que limita quem tem essa chave é o comando forçado, e só ele: a conta em que
ela mora tem Docker e sudo, porque é a conta que roda a aplicação.
`scripts/testa-wrapper-de-release.sh` prova as recusas (shell, encadeamento,
substituição de comando, SHA malformado, allowlist do `sync`) e roda em
`make infra-validar`.

O `DOMINIO` que a aplicação usa vem do `.env` (escrito pelo Ansible a partir de
`dominio` em `group_vars/producao/vars.yml`).

## 3️⃣ Provisionar o servidor

**Este passo deixou de ser manual.** O `.env`, os diretórios, o compose, o
`backup.sh`, o cron e os papéis do banco saem todos de um playbook Ansible:

```bash
make infra-segredos    # UMA vez: gera e cifra a senha do banco
make infra-check       # mostra o que mudaria, sem tocar em nada
make infra-apply       # aplica
```

Antes disso o servidor precisa existir — a distro `ubuntu-server`, o sshd na
2222, os pacotes do Docker rootless e o `cloudflared` são do `site.yml` do
studygo. E `make infra-preparar`, que cria o usuário `stacktrack`, o Docker
rootless dele, o `sudoers` restrito, as chaves e o wrapper `stacktrack-release`.
Na primeira vez, sem a conta, ele entra pela do studygo:

```bash
make infra-preparar ARGS="-e ansible_user=studygo -e ansible_ssh_private_key_file=~/.ssh/studygo_ci"
```

O procedimento completo, os segredos e como recomeçar do zero estão em
[infraestrutura.md](infraestrutura.md).

O que continua valendo, e que o playbook não dispensa você de saber:

**A senha do Postgres vale para o `initdb` do primeiro boot.** Trocá-la depois
exige `ALTER ROLE` dentro do container — não basta mudar o vault. A role tem um
`assert` que recusa a divergência em vez de deixar a API falhar a conexão no
deploy seguinte.

**O `.env` mora só no host.** Quem o escreve é o Ansible, a partir do vault
cifrado, e não um heredoc copiado à mão. O modelo comentado, com a explicação de
cada variável, é o [`.env.prod.example`](../.env.prod.example).

**Arquivo que começa com ponto não aparece no `ls`.** Para conferir o resultado,
`ls -la`:

```bash
ls -la /home/stacktrack/         # o .env aparece aqui
stat -c '%a %U' /home/stacktrack/.env   # 600 stacktrack
```

E o `stacktrack-release release` **confere isto antes de qualquer coisa** (o
pré-voo do wrapper): sem o `.env` ou sem o Docker rootless respondendo, ele falha com a
instrução de rodar `make infra-apply`, em vez de subir a stack com todas as
variáveis vazias.

### 🔐 Dois papéis no banco: quem migra e quem serve

A API **não** se conecta mais com o dono do banco. Quem migra é o dono
(`POSTGRES_USER`, usado pelo Flyway); quem serve é `stacktrack_app`
(`DB_USER`), com `SELECT/INSERT/UPDATE/DELETE` e as sequências — sem CREATE,
ALTER, DROP, extensão, role ou database. Uma falha de execução remota na API não
alcança mais o schema.

Quem cria e mantém o papel é [`deploy/postgres/papeis.sql`](../deploy/postgres/papeis.sql),
aplicado pelo `make infra-apply` a cada provisionamento. É idempotente, e o
playbook só o executa quando a conferência mostra que falta algo — é o que
mantém o `changed=0` significando alguma coisa.

Dois detalhes que custam caro quando faltam:

- **`ALTER DEFAULT PRIVILEGES`** dá acesso às tabelas que ainda **não existem**.
  Sem ele, toda migration futura criaria tabela invisível para a aplicação, e a
  descoberta seria no primeiro `INSERT` depois do deploy seguinte.
- **A senha do papel de runtime é gerenciada pelo vault**, ao contrário de
  `POSTGRES_PASSWORD`, que o `initdb` gravou no volume. Trocar esta é editar o
  vault e reaplicar; trocar aquela exige `ALTER ROLE`.

`backend/test/repository/papeis_test.go` aplica o mesmo arquivo num Postgres de
verdade, conecta **com o papel da aplicação** e exige que o DDL falhe. É a
asserção negativa que um teste de leitura nunca faria.

## 4️⃣ Publicar e implantar

Os passos de build, push e `release` estão em
[entrega-continua.md](entrega-continua.md). Em resumo, da sua máquina:

```bash
sha=$(git rev-parse HEAD)
# build + push das 3 imagens para ghcr.io/caetasousa/stacktrack-* (:$sha e :latest)
make infra-apply                                   # se compose/backup.sh/papéis mudaram
ssh -i ~/.ssh/stacktrack_deploy -p 2222 stacktrack@127.0.0.1 "stacktrack-release release $sha"
```

As imagens nascem **privadas**. Ou você as torna públicas em cada pacote
(Packages → Package settings), ou o servidor autentica (o `make infra-apply` faz
o `docker login` se `vault_ghcr_token` estiver no vault). O token de push
precisa de `write:packages`; o de pull, só `read:packages`.

## 5️⃣ Conferir

O `release` já checa `/api/health` e `/` no fim. O que ele não checa, e você
deve:

```bash
# no servidor (ssh -p 2222 stacktrack@127.0.0.1)
cd ~
docker compose -f docker-compose.prod.yml ps      # api só fica healthy se enxergar o banco
docker compose -f docker-compose.prod.yml logs flyway   # quantas migrations aplicou
docker compose -f docker-compose.prod.yml logs borda | tail   # o IP real do visitante, não 10.89.0.1

# o túnel entrega no domínio certo, e /api vai para a API
curl -sI https://stacktrack.d4ffy.com | head -1
curl -fsS https://stacktrack.d4ffy.com/api/health

# o vizinho do servidor continua no ar
curl -sI https://studygo.d4ffy.com | head -1
```

E o teste que vale mais que todos: **criar uma conta, entrar, montar um quadro e
dar F5.** É ele que exercita o cookie `__Host-` de ponta a ponta — se o
domínio não batesse com o `dominio` do vars.yml, o login falharia aqui e em
nenhum dos `curl` acima.

---

# 🛠️ Operação

## Deploy do dia a dia

Uma linha, depois de publicar as imagens (ver
[entrega-continua.md](entrega-continua.md)):

```
ssh -i ~/.ssh/stacktrack_deploy stacktrack@<vps> "stacktrack-release release <sha>"
```

A sequência mora no servidor, no wrapper que o Ansible instala — pré-voo,
backup, `pull`, parar web e api, `up -d`, limpar imagens órfãs. Do próprio
servidor, é o mesmo comando ou os passos abertos:

```bash
stacktrack-release release <sha>     # ou `latest`

cd ~                                 # o que ele faz por dentro
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml stop web api
docker compose -f docker-compose.prod.yml up -d
```

**O `docker-compose.prod.yml`, o `backup.sh`, o `.env` e os papéis do banco são
do Ansible** — mudou algum deles, `make infra-apply` antes do `release`. (O
wrapper tem um verbo `sync` que instala só o compose, validando-o contra uma
allowlist; é útil para um ajuste pontual sem uma execução completa do playbook.)

Migration nova vai junto: o `depends_on: service_completed_successfully` garante
que o Flyway termina antes de a API subir. API e web são interrompidas juntas
antes disso para que backend antigo e cliente novo nunca convivam durante uma
troca de protocolo. O perfil atual aceita essa janela curta de manutenção em
troca de um rollout semanticamente demonstrável com uma única instância.

## Voltar atrás

O rollback usa somente uma imagem compatível com o schema e com o protocolo já
ativado. Depois que a primeira mutação com revisão for aceita, **não** se volta
ao writer anterior à V16: ele gravaria eventos sem revisão e abriria uma lacuna
que o cursor novo não consegue representar. Nesse ponto, falha de aplicação é
corrigida com roll-forward a partir da release revisionada. Voltar à versão
anterior só é seguro durante a janela de manutenção, antes de reabrir tráfego e
antes de qualquer escrita nova.

```bash
$EDITOR .env       # IMAGE_TAG=<sha do commit bom>
docker compose -f docker-compose.prod.yml stop web api
docker compose -f docker-compose.prod.yml up -d
```

**O banco não volta junto.** O Flyway é forward-only: a migration aplicada
continua aplicada, e a imagem antiga encontra um schema mais novo do que ela
conhece. É por isso que aperto de schema exige dois deploys — um `SET NOT NULL`
no mesmo deploy que estreia a coluna transforma o rollback num erro no primeiro
`INSERT`. Ver `CLAUDE.md`, "Migration que aperta exige dois deploys".

## Backup

`scripts/backup.sh` roda por cron às 03:20. O agendamento é feito pelo Ansible
(`make infra-apply`), com `ansible.builtin.cron`, que gerencia só o bloco
marcado. O `stacktrack-release estado` mostra se a linha do cron está lá.

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
cd ~
gunzip -c ~/backups/stacktrack-<data>.sql.gz | \
  docker compose -f docker-compose.prod.yml exec -T postgres \
  psql -q -U stacktrack -d ensaio
```

(criando antes o banco `ensaio` e derrubando depois).

### Quem avisa que o backup parou

Ninguém, hoje. O script aceita `URL_HEARTBEAT` e pinga ao terminar bem — o
alerta nasce da **ausência** do ping, porque um cron desativado, um PC
desligado e um dump que falhou produzem o mesmo silêncio.

### O que ainda falta

A cópia sai do servidor? **Não.** `~/backups` cobre erro de aplicação e
migration ruim; não cobre perder a máquina — e aqui o servidor e o PC de
desenvolvimento são o mesmo disco. O Supabase Storage fala S3 e resolveria com
uma linha no fim do script — com uma chave que só possa **escrever** no bucket,
senão quem invadir o servidor apaga os backups junto.

## Diagnóstico

```bash
cd ~
alias dc='docker compose -f docker-compose.prod.yml'

dc ps                  # api fica healthy só se enxergar o banco
dc logs -f api         # log estruturado (slog), com request id
dc logs flyway         # o que a última migration fez
dc exec postgres psql -U stacktrack -d stacktrack -c '\dt'

# a API não publica porta: para falar com ela, entre pela rede da stack
docker run --rm --network stacktrack_borda alpine:3.21 wget -qO- http://api:8080/ready
```

Os logs rotacionam em 10 MB × 3 arquivos por serviço. Sem isso, um log de acesso
enche o disco e derruba tudo junto — **inclusive o studygo e a distro de
desenvolvimento**, que estão no mesmo disco virtual do WSL.

---

## Manutenção do banco

A imagem de produção traz um segundo binário, `manutencao`, ao lado da API. Ele
existe para o que **não pode** ser feito por migration: a regra do
[CLAUDE.md](../CLAUDE.md#migrations-banco-de-dados) é que migration não escreve
dado, porque todo backfill decide com que valor as linhas antigas ficam — e essa
decisão é do domínio.

O entrypoint da imagem continua sendo a API; o comando é invocado
explicitamente, no mesmo container e com a mesma versão do domínio que está no
ar.

```bash
cd ~
alias dcm='docker compose -f docker-compose.prod.yml run --rm --entrypoint manutencao api'

dcm reparar-ordenacao --conferir   # só relata; sai 1 se houver trabalho
dcm reparar-ordenacao              # repara e reconfere; sai 0 só se ficou limpo
```

### `reparar-ordenacao`

Redistribui chaves de ordenação **duplicadas** — o estado que a versão anterior
da aplicação conseguia gravar numa rajada de inserções no mesmo ponto, e que o
código de hoje detecta e conserta sozinho no uso normal.

Rodar isto é a **pré-condição do contract** `UNIQUE (coluna_id, chave)` /
`UNIQUE (board_id, chave)`: o `CREATE UNIQUE INDEX` recusa criar enquanto houver
duplicidade herdada, e descobrir isso no meio da janela de manutenção é o pior
momento possível.

O que ele garante, e por que dá para rodar com a aplicação no ar:

- **um quadro por transação**, sob o lock daquele quadro. Não para a escrita dos
  outros quadros, e uma falha no meio não desfaz o que já foi reparado;
- **relê as chaves dentro da transação**. Entre a consulta que listou as
  duplicidades e o lock cabe qualquer mutação — inclusive um rebalanceamento
  disparado pelo próprio uso —, e aí não há mais o que reparar naquele contêiner;
- **idempotente**: rodar de novo termina o serviço. Uma segunda passada num
  banco limpo não abre transação nenhuma;
- **reconfere no fim**. O código de saída 0 significa que a consulta de
  pré-condição voltou vazia, não que o comando achou que deu certo.

O reparo é uma mutação como qualquer outra: avança a revisão do quadro e deixa
evento no log, com `autor_id` **vazio** e a origem no payload — não houve pessoa
por trás. Quem está com o quadro aberto reconcilia ao reconectar; a auditoria
mostra "manutenção redistribuiu a ordenação do quadro", que é a frase honesta
para explicar depois por que a ordem mudou numa madrugada.

---

## 📊 Dimensionamento

Os `mem_limit` desta stack somam ~1,2 GB:

| Serviço | Teto | Por quê |
|---|---|---|
| postgres | 512m | |
| api | 384m | Argon2id usa ~19 MiB por hash simultâneo |
| web | 256m | servidor Node do adapter-node |
| borda | 64m | nginx |

São tetos, não reservas. A VM do WSL tem ~4 GB e é dividida com o studygo, com a
distro de desenvolvimento e com o runner dela. `free -h` no servidor antes de
subir. O sintoma de estourar é a API morrendo por OOM em rajada de login.

## Contenção dos containers

Todos rodam com `no-new-privileges`, `cap_drop: ALL` e limite de processos.
`api` e `web` são `read_only` com `tmpfs` em `/tmp` — o upload de anexo precisa
de arquivo temporário, e é só isso que eles escrevem fora dos volumes. O
Postgres é uma exceção: o entrypoint sobe como root e larga privilégio, então
mantém as cinco capabilities mínimas para essa troca. A `borda` é a outra: sem
`read_only`, pelo motivo em "A borda: o que o túnel alcança".

E tudo isso roda dentro do Docker **rootless**: o root de um container é um uid
sem privilégio no servidor.

---

## ✅ Checklist de go-live

- [ ] rota `stacktrack.d4ffy.com` → `http://localhost:8580` no túnel da Cloudflare
- [ ] servidor provisionado pelo `site.yml` do studygo (sshd na 2222, Docker
      rootless, cloudflared no ar)
- [ ] chave de deploy gerada, pública em `esteira_chave_publica` no `vars.yml`,
      privada guardada
- [ ] `dominio: stacktrack.d4ffy.com` no `group_vars/producao/vars.yml`
- [ ] `make infra-preparar` verde (usuário de deploy, Docker rootless, wrapper,
      chaves) — e `changed=0` na segunda vez
- [ ] `make infra-apply` verde (cria o `.env` com senha aleatória e `chmod 600`,
      sobe a stack com a borda)
- [ ] `make infra-validar` verde (ansible-lint + syntax-check + wrapper)
- [ ] as 3 imagens publicadas no GHCR e acessíveis ao servidor (públicas ou
      `docker login`)
- [ ] `stacktrack-release release <sha>` verde
- [ ] `https://stacktrack.d4ffy.com` abre e `/api/health` responde 200 (prova
      que a location `/api/` vai para a API)
- [ ] o log da `borda` mostra o IP real de quem acessou, não `10.89.0.1`
- [ ] cadastro → login → criar quadro → F5
- [ ] **o studygo continua no ar** (`https://studygo.d4ffy.com`)
- [ ] papel de runtime do banco criado (`stacktrack_app`) e a API conectando com ele
- [ ] cron do backup agendado (o `infra-apply` faz) e **um backup restaurado em ensaio**
- [ ] `free -h` no servidor com folga sobre a soma dos limites das stacks

### Antes do próximo deploy: o contract da ordenação

O `V18` está no repositório e **é aplicado pelo Flyway no próximo deploy**. Ele
cria os índices únicos da chave de ordenação, e o `CREATE UNIQUE INDEX` falha se
o banco ainda tiver duplicidade herdada — falhando o Flyway, a stack não sobe.
Por isso os dois passos abaixo são pré-deploy, não pós:

- [ ] `manutencao reparar-ordenacao --conferir` sai **0** contra o banco de
      produção. Saindo 1, rodar `manutencao reparar-ordenacao` (sem `--conferir`)
      e conferir de novo — ele repara um quadro por transação, com a aplicação
      no ar.
- [ ] Estimativa de lock registrada numa cópia representativa: linhas, tamanho
      das tabelas e tempo de construção do índice. Não cabendo na janela, a
      saída é criação concorrente, não abrir mão da unicidade.

Os comandos e o SQL da conferência estão em
[backend/migrations/README.md](../backend/migrations/README.md).

### Passo agendado para depois

- [ ] **Ativar o GC de arquivos.** O outbox `arquivo_exclusoes` já acumula e o
      coletor já funciona, mas a porta de cobertura é `CoberturaNegada` e nada
      sai do disco. Ligar exige os manifests dos backups externos — é trabalho
      de A6, e só depois do primeiro restore drill aprovado.

---

## 🚨 Armadilhas conhecidas

**O domínio tem que bater nos três lugares.** A rota do túnel, o `dominio` do
`vars.yml` e o host que o navegador usa. Fora disso, o cookie `Secure`/`__Host-`
é descartado e o login falha em silêncio.

**O `/api` tem que ir para a API.** A location `/api/` da `borda` precisa
remover o prefixo e repassar o `Upgrade`. Sem ela, `/api/*` cai no SvelteKit e
login e WebSocket quebram. Ver "A borda: o que o túnel alcança".

**A 8580 é do stacktrack na máquina inteira.** As distros do WSL dividem a
rede: um `vite`, um `docker run -p 8580` ou qualquer coisa na distro de
desenvolvimento que ocupe a porta tira o stacktrack do ar.

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
