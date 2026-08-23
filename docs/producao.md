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
/home/deploy/                 o vizinho, e o que é compartilhado
├── agendago/                 Caddyfile (dono das portas 80/443), compose, .env
├── caddy/sites/              um arquivo .caddy por vizinho — o nosso entra aqui
└── backups/                  os do agendaGo

/home/stacktrack/             a aplicação
├── .env                      escrito pelo Ansible a partir do vault
├── docker-compose.prod.yml
├── scripts/backup.sh
└── backups/

/home/stacktrack-esteira/     só .ssh/authorized_keys
```

Cada projeto na sua conta, com uma exceção: o bloco do Caddy continua em
`/home/deploy/caddy/sites`, porque é esse caminho que o container do proxy do
agendaGo monta. O usuário `stacktrack` entra no grupo `deploy` só para poder
escrever esse arquivo.

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

Primeiro, a chave — **exclusiva do stacktrack**, não a do agendaGo. Uma chave
que abre os dois projetos faz o comprometimento de um ser o comprometimento do
outro:

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
| Secret | `VPS_USER` | `stacktrack-esteira` |
| Secret | `VPS_SSH_KEY` | chave **privada** exclusiva da esteira |
| Secret | `VPS_KNOWN_HOSTS` | saída de `ssh-keyscan -H <ip>` |
| Variable | `DOMINIO` | `stacktrack.duckdns.org` (aba Variables, no repositório) |

E em **Settings → Branches**, proteja a `main` exigindo os checks da esteira
(`backend`, `frontend`, `e2e`, `infra`, `seguranca-*`).

Sem `VPS_HOST`, o job de deploy passa marcado como ignorado em vez de falhar —
o repositório funciona antes de o servidor existir. Sem `VPS_KNOWN_HOSTS`, ele
**falha de propósito**: a alternativa seria aceitar a identidade de quem
responder e entregar a chave de deploy junto.

### O que a chave da esteira consegue fazer

Nada além de três verbos. Ela está no `authorized_keys` do usuário
`stacktrack-esteira` com `restrict` e
`command="/usr/local/bin/stacktrack-release"`: não abre shell, não faz `scp`,
não encaminha porta nem agente.

| Verbo | O que faz |
|---|---|
| `release <sha>` | pré-voo, backup, `pull`, para web e api, `up -d`, limpa imagens órfãs |
| `backup` | um backup pontual |
| `estado` | `compose ps`, o sha256 dos arquivos de configuração, fuso e cron |

O usuário **não** está no grupo `docker` — acesso ao socket equivale a root
nesta máquina. Ele chega ao Docker por um `sudo` restrito ao wrapper, que valida
os argumentos antes de qualquer coisa. `scripts/testa-wrapper-de-release.sh`
prova as recusas (shell, encadeamento, substituição de comando, SHA
malformado) e roda no CI.

## 3️⃣ Provisionar o servidor

**Este passo deixou de ser manual.** O `.env`, os diretórios, o compose, o
`backup.sh`, o cron e o bloco do Caddy saem todos de um playbook Ansible:

```bash
make infra-segredos    # UMA vez: gera e cifra a senha do banco
make infra-check       # mostra o que mudaria, sem tocar em nada
make infra-apply       # aplica
```

Antes desses, o que exige privilégio no host: `make infra-preparar` — Docker,
os dois usuários, o wrapper da esteira e o hardening. Ele entra como `deploy` e
sobe por `sudo`, pedindo a senha na hora; o hardening pode ficar para depois com
`ARGS="--skip-tags hardening"`, já que é a única parte que alcança o host
inteiro, dividido com o agendaGo. O procedimento completo, os segredos e como
recomeçar do zero estão em [infraestrutura.md](infraestrutura.md).

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

A esteira testa, varre, publica as três imagens e implanta: sincroniza o
compose e o `backup.sh`, instala o bloco do Caddy e o recarrega, tira um backup,
garante a rede, sobe a stack fixando a tag no SHA do commit, confere o cron e
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

Automático pela esteira, que manda uma linha e nada mais:

```
ssh stacktrack-esteira@servidor "stacktrack-release release <sha>"
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

**A esteira não copia mais arquivo nenhum.** O compose, o `backup.sh` e o bloco
do Caddy são instalados pelo Ansible — havia duas fontes escrevendo os mesmos
caminhos, e ganhava a última que rodasse. Em troca, mudança nesses arquivos só
chega ao servidor com `make infra-apply`: o job de deploy compara o sha256 dos
três com os do commit e **falha** quando divergem, em vez de subir uma versão
nova do código com a configuração velha.

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

`scripts/backup.sh` roda por cron às 03:20 — deslocado do agendaGo (03:00)
porque dois `pg_dump` no mesmo minuto disputam CPU e disco sem necessidade. A
O agendamento é feito pelo Ansible (`make infra-apply`), com
`ansible.builtin.cron`, que gerencia só o bloco marcado e preserva a linha do
vizinho. A esteira apenas CONFERE que ele existe, e falha se não existir —
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
enche o disco do VPS e derruba tudo junto — **inclusive o agendaGo**.

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
- [ ] **console do provedor testado** — o hardening desliga a senha do SSH, e o
      console é o único caminho de volta se algo der errado
- [ ] chave exclusiva da esteira gerada, pública em `vars.yml` e privada no
      secret `VPS_SSH_KEY`
- [ ] environment `production` criado, com os segredos `VPS_*` dentro dele
- [ ] `main` protegida com os checks da esteira (incluindo `infra`)
- [ ] `make infra-preparar` verde (usuário da esteira, wrapper, firewall, SSH)
- [ ] variável `DOMINIO` no repositório
- [ ] `make infra-apply` verde (cria o `.env` com senha aleatória e `chmod 600`)
- [ ] imagens acessíveis ao servidor (públicas ou `docker login` feito)
- [ ] esteira verde até o job `implantar`
- [ ] certificado emitido (`https://` abre sem aviso)
- [ ] cadastro → login → criar quadro → F5
- [ ] **o agendaGo continua no ar** depois do reload do Caddy
- [ ] papel de runtime do banco criado (`stacktrack_app`) e a API conectando com ele
- [ ] cron do backup agendado (o `infra-apply` faz) e **um backup restaurado em ensaio**
- [ ] `free -h` com folga sobre a soma dos limites das duas stacks

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
