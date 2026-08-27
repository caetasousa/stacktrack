# 🚀 Produção

O stacktrack roda num VPS cuja **borda é o projeto
[`loadbalancer`](https://github.com/caetasousa/loadbalancer)** (repo à parte):
um nginx em container recebe 80/443, termina o TLS (certbot) e roteia por
domínio. O stacktrack traz os próprios containers para a rede `borda` e é
alcançado por nome.

Este documento é o roteiro do servidor. A esteira que leva o commit até aqui
está em [entrega-continua.md](entrega-continua.md).

---

## 🏗️ Arquitetura no ar

```
                     ┌──────────────────── VPS ────────────────────┐
  :80/:443 ─────────►│ nginx  (projeto loadbalancer)  + certbot    │
                     │   stacktrack.caetasousa.tech ─┐             │
                     └──────────────────────────────┼─────────────┘
                                    rede `borda`    │
                                 ┌──────────────────┼──────────────────┐
                                 │  ┌───────────────▼────────────────┐  │
                                 │  │ stacktrack-web   :3000  (/)    │  │
                                 │  │ stacktrack-api   :8080  (/api) │  │
                                 │  │ stacktrack postgres           │  │
                                 │  └───────────────────────────────┘  │
                                 └─────────────────────────────────────┘
```

Front e API compartilham a origem: o bloco `:443` do `loadbalancer` manda
`/api/*` para a API **removendo o prefixo**, e todo o resto para o frontend.

Nenhum serviço do stacktrack publica porta no host. Quem fala com o mundo é o
nginx da borda; o Postgres não é alcançável de fora do compose.

A rede `borda` é criada pelo playbook do `loadbalancer`. O stacktrack só precisa
declará-la `external` no compose e subir nela — o nginx alcança
`stacktrack-web` e `stacktrack-api` por nome.

### O que o `loadbalancer` roteia para o stacktrack

O bloco `:443` de `stacktrack.caetasousa.tech` vive no `loadbalancer`
(`nginx/conf.d/stacktrack.conf`, no molde do `app.conf.exemplo`), **não neste
repositório**. Ele precisa fazer, como o Caddy fazia:

| Rota | Destino | Por quê |
|---|---|---|
| `/api/` (barra final remove o prefixo) | `http://stacktrack-api:8080/` | origem única — o frontend chama `/api` relativo (`PUBLIC_API_URL=/api` assado no build), o cookie é `__Host-` e a CSP é `connect-src 'self'` |
| `/api/ws` | idem, com `Upgrade`/`Connection` e `proxy_read_timeout 3600s` | o WebSocket do quadro é uma requisição que dura horas |
| `/` | `http://stacktrack-web:3000` | com `X-Forwarded-Proto`/`X-Forwarded-Host` — o `adapter-node` exige |

Mais `client_max_body_size 11m` (10 MiB de anexo + folga multipart; menor
devolve 413 antes de a API responder), os cabeçalhos de segurança (HSTS,
`X-Content-Type-Options`, `X-Frame-Options DENY`, `Referrer-Policy no-referrer`,
`Permissions-Policy`, `Cross-Origin-Opener-Policy`) e `X-Real-IP $remote_addr`
no `/api/` (é o que faz o `IPReal` e os tetos por IP valerem).

**Se o `loadbalancer` mandar tudo para `stacktrack-web:3000`**, o `/api` cai no
SvelteKit, o login não completa e o WebSocket não conecta.

**Regressão consciente:** o bloco Caddy redigia do *access log do proxy* os
tokens de convite e de link público (corridas de 24+ caracteres base64url) e
removia o `Referer`. O `log_format` do `loadbalancer` registra `$request`
inteiro. O log da **aplicação** (`internal/pkg/logging`) continua sanitizando; o
do proxy virou política da borda. O ensaio automatizado disso
(`test/repository/caddy_log_test.go`) foi removido junto com o bloco Caddy.

### Por que subdomínio, e não um caminho

`stacktrack.caetasousa.tech`, e não `caetasousa.tech/stacktrack`.

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
| `~/stacktrack/docker-compose.prod.yml` | repositório, via Ansible | a topologia |
| `~/stacktrack/.env` | **só existe no host**, escrito pelo Ansible | segredos |
| `~/stacktrack/scripts/backup.sh` | repositório, via Ansible | operação |

O roteamento não está aqui: o bloco `:443` é do `loadbalancer`
(ver a seção anterior).

As migrations não são exceção a isso: os `.sql` viajam **dentro** da imagem
`stacktrack-migrations` (ver `backend/Dockerfile.migrations`). Sem ela, o job de
migration dependeria de um bind mount de `backend/migrations`, obrigando a
manter um clone do repositório no VPS — e a mantê-lo em sincronia com a imagem
que está rodando.

```
/home/stacktrack/             a aplicação
├── .env                      escrito pelo Ansible a partir do vault
├── docker-compose.prod.yml
├── scripts/backup.sh
└── backups/

/opt/loadbalancer/            a borda (projeto à parte)
└── nginx/conf.d/stacktrack.conf   o bloco :443 do stacktrack
```

---

# 🧭 Roteiro do primeiro deploy

## 1️⃣ DNS e certificado

Um registro `A` apontando `stacktrack.caetasousa.tech` para `2.25.101.3` (o IP
do VPS). O nginx do `loadbalancer` separa por `server_name`.

**O DNS precisa resolver ANTES do certificado.** A emissão do primeiro
certificado de cada domínio é um passo manual do `loadbalancer`
(`ansible-playbook deploy.yml -e 'acme_domains=["stacktrack.caetasousa.tech"]'`);
sem certificado o `nginx -t` recusa o bloco `:443`, e sem HTTPS o login não
completa — `APP_ENV=production` liga o `Secure` no cookie, o navegador o
descarta, e o sintoma é "entra e volta para a tela de login", sem erro visível.

A renovação é automática (container `certbot` + cron da borda).

## 2️⃣ Configurar o deploy no GitHub

Primeiro, a chave — **exclusiva do stacktrack**, não a de outro projeto do VPS.
Uma chave que abre dois projetos faz o comprometimento de um ser o
comprometimento do outro:

```bash
ssh-keygen -t ed25519 -C github-actions-stacktrack -f ~/.ssh/stacktrack_deploy
```

A **pública** vai para `esteira_chave_publica` em
`deploy/ansible/group_vars/producao/vars.yml`, e é o `make infra-preparar` que a
instala no servidor — com `restrict` e comando forçado. A **privada** vira o
secret `VPS_SSH_KEY`.

Depois, em **Settings → Environments**, crie o environment `production` e
defina a proteção que quiser (aprovação manual, branches permitidas). Os
segredos abaixo ficam nele, e não no repositório: assim um job de pull request,
que não declara environment nenhum, não os alcança.

| Tipo | Nome | Valor |
|---|---|---|
| Secret | `VPS_HOST` | IP do servidor |
| Secret | `VPS_USER` | `stacktrack` |
| Secret | `VPS_SSH_KEY` | chave **privada** exclusiva da esteira |
| Secret | `VPS_KNOWN_HOSTS` | saída de `ssh-keyscan -H <ip>` |
| Variable | `DOMINIO` | `stacktrack.caetasousa.tech` (aba Variables, no repositório) |

E em **Settings → Branches**, proteja a `main` exigindo os checks da esteira
(`backend`, `frontend`, `e2e`, `infra`, `seguranca-*`).

Sem `VPS_HOST`, o job de deploy passa marcado como ignorado em vez de falhar —
o repositório funciona antes de o servidor existir. Sem `VPS_KNOWN_HOSTS`, ele
**falha de propósito**: a alternativa seria aceitar a identidade de quem
responder e entregar a chave de deploy junto.

### O que a chave da esteira consegue fazer

Nada além de três verbos. Ela está no `authorized_keys` do usuário
`stacktrack` com `restrict` e `command="/usr/local/bin/stacktrack-release"`:
não abre shell, não faz `scp`, não encaminha porta nem agente.

A restrição é **da chave**, não da conta: a mesma conta aceita a chave do
operador sem limite algum e a da esteira limitada a três verbos.

| Verbo | O que faz |
|---|---|
| `release <sha>` | pré-voo, backup, `pull`, para web e api, `up -d`, limpa imagens órfãs |
| `backup` | um backup pontual |
| `estado` | `compose ps`, o sha256 dos arquivos de configuração, fuso e cron |

O que limita a esteira é o comando forçado, e só ele: a conta em que a chave
mora tem Docker, porque é a conta que roda a aplicação. `scripts/testa-wrapper-de-release.sh`
prova as recusas (shell, encadeamento, substituição de comando, SHA malformado)
e roda no CI.

## 3️⃣ Provisionar o servidor

**Este passo deixou de ser manual.** O `.env`, os diretórios, o compose, o
`backup.sh`, o cron e os papéis do banco saem todos de um playbook Ansible:

```bash
make infra-segredos    # UMA vez: gera e cifra a senha do banco
make infra-check       # mostra o que mudaria, sem tocar em nada
make infra-apply       # aplica
```

Antes disso o host precisa existir — Docker, rede `borda`, firewall, hardening e
TLS são do projeto `loadbalancer` (rode o `site.yml` dele). E `make
infra-preparar`, que cria o usuário `stacktrack`, o `sudoers` restrito, as
chaves e o wrapper `stacktrack-release` (pede a senha do sudo). O procedimento
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
ls -la /home/stacktrack/         # o .env aparece aqui
grep -c . /home/stacktrack/.env  # 8 linhas
```

E o job `implantar` da esteira agora **confere isto antes de qualquer coisa**:
sem o `.env` ou sem a rede `borda`, ele falha com a instrução de rodar
`make infra-apply`, em vez de subir a stack com todas as variáveis vazias.

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

A esteira testa, varre, publica as três imagens e implanta: manda
`release <sha>` ao wrapper, que tira um backup, faz `pull`, para web e api, sobe
a stack fixando a tag no SHA do commit, e confere se `/api/health` e `/`
respondem. O compose e o `backup.sh` são do playbook — o passo `estado` do
deploy falha se o que está no servidor divergir do commit.

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

# o certificado é do domínio certo e não expirou, e /api vai para a API
curl -sI https://stacktrack.caetasousa.tech | head -1
curl -fsS https://stacktrack.caetasousa.tech/api/health

# os outros sites da borda continuam no ar (o nginx só recarregou)
curl -sI https://<outro-dominio-da-borda> | head -1
```

E o teste que vale mais que todos: **criar uma conta, entrar, montar um quadro e
dar F5.** É ele que exercita o cookie `__Host-` de ponta a ponta — se o
certificado não estivesse válido, o login falharia aqui e em nenhum dos `curl`
acima.

---

# 🛠️ Operação

## Deploy do dia a dia

Automático pela esteira, que manda uma linha e nada mais:

```
ssh stacktrack@servidor "stacktrack-release release <sha>"
```

A sequência mora no servidor, no wrapper que o Ansible instala — pré-voo,
backup, `pull`, parar web e api, `up -d`, limpar imagens órfãs. Manualmente, do
próprio servidor, é o mesmo comando ou os passos abertos:

```bash
stacktrack-release release <sha>     # ou `latest`

cd ~                                 # o que ele faz por dentro
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml stop web api
docker compose -f docker-compose.prod.yml up -d
```

**A esteira não copia arquivo nenhum.** O compose e o `backup.sh` são instalados
pelo Ansible; o roteamento é do `loadbalancer`. Em troca, mudança no compose ou
no `backup.sh` só chega ao servidor com `make infra-apply`: o job de deploy
compara o sha256 dos dois com os do commit e **falha** quando divergem, em vez
de subir uma versão nova do código com a configuração velha.

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
marcado. A esteira apenas CONFERE que ele existe, e falha se não existir —
escrever dos dois lados fazia os dois brigarem pelo mesmo arquivo.

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
enche o disco do VPS e derruba tudo junto — **inclusive a borda nginx e os
outros sites dela**.

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
cd ~/stacktrack
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

Os `mem_limit` desta stack somam ~1,15 GB:

| Serviço | Teto | Por quê |
|---|---|---|
| postgres | 512m | |
| api | 384m | Argon2id usa ~19 MiB por hash simultâneo |
| web | 256m | servidor Node do adapter-node |

A borda nginx e os outros sites do VPS têm os seus. `free -h` antes de subir. O
sintoma de estourar é a API morrendo por OOM em rajada de login.

## Contenção dos containers

Todos rodam com `no-new-privileges`, `cap_drop: ALL` e limite de processos.
`api` e `web` são `read_only` com `tmpfs` em `/tmp` — o upload de anexo precisa
de arquivo temporário, e é só isso que eles escrevem fora dos volumes. O
Postgres é a exceção: o entrypoint sobe como root e larga privilégio, então
mantém as cinco capabilities mínimas para essa troca.

---

## ✅ Checklist de go-live

- [ ] `A stacktrack.caetasousa.tech` resolvendo para `2.25.101.3`
- [ ] host provisionado pelo projeto `loadbalancer` (Docker, rede `borda`,
      firewall, borda nginx no ar)
- [ ] certificado de `stacktrack.caetasousa.tech` emitido pelo `loadbalancer`
      (`deploy.yml -e 'acme_domains=[...]'`)
- [ ] chave exclusiva da esteira gerada, pública em `vars.yml` e privada no
      secret `VPS_SSH_KEY`
- [ ] environment `production` criado, com os segredos `VPS_*` dentro dele
- [ ] `main` protegida com os checks da esteira (incluindo `infra`)
- [ ] `make infra-preparar` verde (usuário de deploy, wrapper, chaves)
- [ ] variável `DOMINIO` no repositório
- [ ] `make infra-apply` verde (cria o `.env` com senha aleatória e `chmod 600`)
- [ ] imagens acessíveis ao servidor (públicas ou `docker login` feito)
- [ ] esteira verde até o job `implantar`
- [ ] bloco `stacktrack.conf` no `loadbalancer` roteando `/api` → `stacktrack-api`
      e `/` → `stacktrack-web` (`https://` abre sem aviso e `/api/health` responde)
- [ ] cadastro → login → criar quadro → F5
- [ ] **os outros sites da borda continuam no ar** depois do reload do nginx
- [ ] papel de runtime do banco criado (`stacktrack_app`) e a API conectando com ele
- [ ] cron do backup agendado (o `infra-apply` faz) e **um backup restaurado em ensaio**
- [ ] `free -h` com folga sobre a soma dos limites das stacks do VPS

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

**TLS não é o último passo.** Sem HTTPS válido, o cookie `Secure` é descartado e
o login falha em silêncio.

**O `/api` tem que ir para a API.** O bloco `:443` do `loadbalancer` precisa
rotear `/api/*` para `stacktrack-api:8080` (prefixo removido) e o resto para
`stacktrack-web:3000`. Um bloco que mande tudo para o front deixa `/` no ar mas
quebra login e WebSocket. Ver "O que o `loadbalancer` roteia para o stacktrack".

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
