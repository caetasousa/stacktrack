# Deploy

O VPS já roda o **agendaGo** atrás de um Caddy que ocupa as portas 80 e 443. O
stacktrack entra ao lado dele: mesmo proxy, mesmo host, domínios separados.

As imagens não são construídas no servidor. O GitHub Actions testa, constrói e
publica no GHCR; o VPS só puxa o que passou.

```
push na main
     │
     ▼
GitHub Actions ── go test -race, govulncheck ─┐
                  svelte-check, vitest        ├─► falhou? nada é publicado
                  migrations não escrevem dado┘
     │ passou
     ▼
GHCR   stacktrack-api        (+ :sha-<commit>)
       stacktrack-web
       stacktrack-migrations  ← os .sql viajam DENTRO da imagem
     │
     ▼ (você, no VPS)
docker compose pull && up -d


                    ┌──────────── VPS ────────────┐
  :80/:443 ────────►│ Caddy (stack do agendaGo)   │
                    │   agenda.dominio  ─┐        │
                    │   kanban.dominio  ─┼─┐      │
                    └────────────────────┼─┼──────┘
                       rede default      │ │  rede `borda`
                    ┌─────────────────┐  │ │  ┌─────────────────────┐
                    │ agendago api/web│◄─┘ └─►│ stacktrack-api  :8080 │
                    │ agendago postgres│       │ stacktrack-web  :3000 │
                    └─────────────────┘       │ stacktrack postgres   │
                                              └─────────────────────┘
```

Só o Caddy participa das duas redes. Os dois Postgres continuam isolados, cada
um na rede interna do seu compose — um projeto não alcança o banco do outro.

## Por que subdomínio, e não um caminho

`stacktrack.duckdns.org`, e **não** `seudominio.com.br/kanban`.

O cookie de sessão tem `Path=/` e o navegador o envia em toda requisição
daquele host — servindo os dois no mesmo domínio, o agendaGo passaria a receber
o token de sessão de quem entrou no stacktrack. Reduzir o `Path` também não
resolve: o prefixo `__Host-` **exige** `Path=/`, e abrir mão dele custaria a
garantia de que o cookie pertence àquele host exato.

Subdomínio resolve os dois de uma vez, e o Caddy pega um certificado para ele
sozinho.

## Por que uma origem só (dentro de cada domínio)

O front chama a API por caminho relativo (`/api/...`) e o `handle_path` do Caddy
remove o prefixo antes de repassar. Três coisas dependem disso:

- o cookie `__Host-` exige `Secure`, `Path=/` e **nenhum** `Domain`;
- a CSP de produção é `connect-src 'self'`: API noutra origem exigiria exceção;
- na fase 5 o `wss://` entra coberto por `self`, sem tocar em nada.

É a mesma decisão que o agendaGo já tinha tomado.

## Primeira subida

```bash
# 1. a rede compartilhada (uma vez para o VPS inteiro)
docker network create borda

# 2. religar o Caddy do agendaGo — ele ganhou a rede `borda` e passou a
#    importar /etc/caddy/sites/*.caddy
sudo mkdir -p /opt/caddy/sites
cd ~/agendaGo && git pull
docker compose -f docker-compose.prod.yml up -d caddy

# 3. o bloco de site do stacktrack
sudo cp /caminho/do/stacktrack/deploy/caddy/stacktrack.caddy /opt/caddy/sites/
sudo $EDITOR /opt/caddy/sites/stacktrack.caddy        # troque o domínio
docker compose -f ~/agendaGo/docker-compose.prod.yml exec caddy \
  caddy reload --config /etc/caddy/Caddyfile

# 4. a stack do stacktrack — só dois arquivos, sem código-fonte
sudo mkdir -p /opt/stacktrack && cd /opt/stacktrack
# copie para cá: docker-compose.prod.yml e .env.prod.example
cp .env.prod.example .env.prod
chmod 600 .env.prod
openssl rand -hex 24        # cole em POSTGRES_PASSWORD
$EDITOR .env.prod           # ajuste DOMINIO

echo "$GHCR_TOKEN" | docker login ghcr.io -u caetasousa --password-stdin
docker compose -f docker-compose.prod.yml --env-file .env.prod pull
docker compose -f docker-compose.prod.yml --env-file .env.prod up -d

# 5. backup no cron
sudo crontab -e
# 17 3 * * * /opt/stacktrack/scripts/backup.sh >> /var/log/stacktrack-backup.log 2>&1
```

**O DNS precisa apontar antes.** O Caddy só consegue o certificado depois que
`stacktrack.duckdns.org` resolve para o IP do VPS. E sem certificado o login
não completa: `APP_ENV=production` liga o `Secure` no cookie, o navegador o
descarta, e o sintoma é "entra e volta para a tela de login", sem erro visível.

## Deploy do dia a dia

```bash
cd /opt/stacktrack
docker compose -f docker-compose.prod.yml --env-file .env.prod pull
docker compose -f docker-compose.prod.yml --env-file .env.prod up -d
```

Migration nova vai junto: os `.sql` estão dentro da imagem
`stacktrack-migrations`, e o `depends_on: service_completed_successfully` garante
que o Flyway termina antes de a API subir.

### Voltar atrás

```bash
$EDITOR .env.prod       # IMAGE_TAG=sha-abc1234
docker compose -f docker-compose.prod.yml --env-file .env.prod up -d
```

**O banco não volta junto.** O Flyway é forward-only: a migration aplicada
continua aplicada, e a imagem antiga encontra um schema mais novo do que ela
conhece. É por isso que aperto de schema exige dois deploys — um `SET NOT NULL`
no mesmo deploy que estreia a coluna transforma o rollback num erro no primeiro
`INSERT`. Ver `CLAUDE.md`, "Migration que aperta exige dois deploys".

## Antes da fase 5 (tempo real)

O Caddy é transparente para WebSocket e não impõe timeout de leitura à conexão
— desse lado não há nada a fazer. **O que derruba é o lado Go:**

| Onde | Valor | Sintoma se esquecido |
|---|---|---|
| `backend/config/server.go` — `WriteTimeout` | 15s | a conexão cai sozinha, sempre no mesmo tempo |

O comentário no próprio arquivo já marca o lugar.

## Diagnóstico

```bash
cd /opt/stacktrack
alias dc='docker compose -f docker-compose.prod.yml --env-file .env.prod'

dc ps                  # quem está de pé; `api` fica healthy só se enxergar o banco
dc logs -f api         # log estruturado (slog), com request id
dc logs flyway         # o que a última migration fez
dc exec postgres psql -U stacktrack -d stacktrack -c '\dt'

# a API não publica porta no host: para falar com ela, entre pela rede
docker run --rm --network borda alpine:3.21 \
  wget -qO- http://stacktrack-api:8080/ready
```

Os logs rotacionam em 10 MB × 3 arquivos por serviço. Sem isso, um log de acesso
enche o disco do VPS e derruba tudo junto — inclusive o agendaGo.

## Contenção dos containers

Todos rodam com `no-new-privileges`, `cap_drop: ALL` e limites de memória e de
processos. `api` e `web` são `read_only` com `tmpfs` em `/tmp` — o upload de
anexo precisa de arquivo temporário, e é só isso que eles escrevem fora dos
volumes. O Postgres é a exceção: o entrypoint dele sobe como root e larga
privilégio, então mantém as cinco capabilities mínimas para essa troca.

## O que ainda não está aqui

- **Usuário de banco só com DML.** Hoje a API se conecta com o dono do banco,
  que pode derrubar qualquer tabela. O `.env.prod` já tem `DB_USER`/`DB_PASSWORD`
  comentados e o compose já os lê; falta o script que cria o papel com os
  `GRANT` certos — o agendaGo tem um em `scripts/criar-usuario-app.sh` que serve
  de modelo.
- **Cópia do backup fora do VPS.** O `scripts/backup.sh` grava em
  `/var/backups/stacktrack` e rotaciona em 14 dias: cobre erro de aplicação, não
  cobre perder a máquina. O gancho para o Supabase Storage está no fim do script.
- **Varredura de imagem (Trivy) e E2E no CI**, que o agendaGo já tem.
- **Deploy automático.** O CI publica; quem manda o VPS puxar é você. Guardar
  chave de servidor no GitHub é decisão de segurança que vale tomar de propósito.
