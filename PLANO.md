# stacktrack — quadro Kanban colaborativo em tempo real

## Contexto

`/home/caetasousa/projectX` está vazio. O objetivo é um **projeto de aprendizagem**: o
agendaGo já cobriu CRUD + auth + testes + deploy, então mais um CRUD ensinaria pouco. Este
projeto força um eixo novo — **concorrência de verdade em Go e sincronização de estado entre
clientes** — reaproveitando o esqueleto que já é domínio conhecido (hexagonal, chi, pgx,
Postgres, Flyway, SvelteKit, Tailwind).

O produto: um quadro estilo Trello onde várias pessoas movem cards e todos veem na hora, sem
recarregar a página.

```
┌─────────────┬─────────────┬─────────────┬─────────────┐
│  A fazer    │  Fazendo    │  Revisão    │   Pronto    │   👤👤 online
├─────────────┼─────────────┼─────────────┼─────────────┤
│ ┌─────────┐ │ ┌─────────┐ │ ┌─────────┐ │ ┌─────────┐ │
│ │Migração │ │ │Login CSS│ │ │Fix #204 │ │ │Deploy v1│ │
│ └─────────┘ │ │✏ ana    │ │ └─────────┘ │ └─────────┘ │
│ ┌─────────┐ │ └─────────┘ │             │             │
│ │Testes E2E│ │             │             │             │
│ └─────────┘ │             │             │             │
└─────────────┴─────────────┴─────────────┴─────────────┘
```

### Decisões já tomadas

| Decisão | Escolha |
|---|---|
| Autenticação | Completa, como no agendaGo (confirmação por email, recuperação de senha, rate limiting, convites) |
| Infra inicial | Postgres + Docker Compose + Flyway. Testcontainers e CI entram na fase 8 |
| Documentação da API | A tabela de rotas do `README.md`, e só ela. **Swagger ficou de fora em definitivo** — ver a fase 8 |
| Proxy e TLS | **Caddy** — o que já atende o agendaGo no mesmo VPS, roteando por domínio. O plano original dizia nginx; na fase 8 ficou claro que o VPS já rodava Caddy, e enfiar um segundo proxy em série só somaria timeouts para depurar. O stacktrack deposita um bloco de site em `deploy/caddy/` e o Caddy do vizinho o importa |
| WebSocket | `github.com/coder/websocket` (API sobre `context`, casa com o shutdown gracioso) |
| Nome / caminho | `stacktrack` em `/home/caetasousa/projectX` |

### Convenções herdadas do agendaGo

Valem aqui sem discussão (copiar o `CLAUDE.md` e adaptar):
comentários e doc comments em português · migration não escreve dado, sem `DEFAULT`/`CHECK` de
regra de negócio · aperto de schema em dois deploys (expand → código → contract) · Conventional
Commits em português, um contexto por commit · nunca commitar sem perguntar, nunca dar push ·
README enxuto com tabela de rotas, detalhe em `docs/`.

---

## Arquitetura alvo

```
backend/
  cmd/api/main.go              # wiring: pool, repos, usecases, hub, rotas, shutdown
  config/                      # env vars tipadas, pool pgx
  internal/
    domain/                    # regra pura, sem I/O
      usuario/ session/ signup/ passwordreset/
      board/ coluna/ card/ membro/ evento/
    usecase/                   # orquestra domínio + portas
      auth/ board/ coluna/ card/ realtime/
    adapter/
      http/handler/  http/dto/  http/middleware/
      http/ws/                 # handshake, auth por cookie, bombas de leitura/escrita
      realtime/hub/            # salas, fan-out, presença (implementa a porta Publicador)
      repository/              # Postgres via pgx
      security/                # argon2id
      email/                   # go-mail + templates
    pkg/                       # logging, token, paging
  migrations/                  # V1__..., V2__... (Flyway)
  test/                        # domain/ usecase/ handler/ repository/memoria (fakes)

frontend/src/
  lib/api/                     # cliente fetch fino (espelha os DTOs do Go)
  lib/realtime/                # store da conexão WebSocket, reconexão, aplicação de eventos
  lib/stores/                  # sessão, quadro
  lib/components/              # Quadro, Coluna, Card, Presenca
  routes/                      # login, cadastro, quadros, quadros/[id]
```

**A regra que segura o desenho todo:** `domain` e `usecase` não sabem que WebSocket existe. O
usecase depende de uma porta `Publicador` (uma interface com `Publicar(boardID, evento)`); quem
implementa é o hub, em `adapter/realtime`. Trocar WebSocket por SSE amanhã não toca em regra de
negócio — é a mesma lição do `notificador` de email do agendaGo, agora com um adaptador vivo.

**O caminho de um movimento de card:**

```
navegador A                 API Go                              navegador B
    │                          │                                     │
    │ arrasta o card ──────────┤                                     │
    │ (UI move na hora)        │                                     │
    │ PATCH /cards/{id}/mover  │                                     │
    ├─────────────────────────►│                                     │
    │                          │ usecase: valida + version check     │
    │                          │ TX: UPDATE card + INSERT evento     │
    │                          │ COMMIT                              │
    │                          │ hub.Publicar("board:7", evento) ────┼──► card se move
    │◄─────────────────────────┤ 200 OK                              │    (sem F5)
    │ confirma (ou desfaz)     │                                     │
```

---

## Fases

Cada fase termina com algo que funciona no navegador. Dá pra parar em qualquer uma sem ficar com
projeto pela metade. As fases 0–3 são terreno conhecido (é onde você constrói a base); o
aprendizado novo começa firme na 4 e explode na 5.

---

### Fase 0 — Fundação

**Conceito novo:** nenhum. É montar o mesmo terreno do agendaGo, de propósito, para o resto ser
sobre o que interessa.

**Entrega:**
- `docker-compose.yml`: `postgres` (16-alpine, healthcheck), `flyway` (migrate, depende do
  healthcheck), `api` (Go), `web` (node), `mailpit` (a auth completa precisa de SMTP em dev)
- Esqueleto Go: `cmd/api/main.go` com chi, `slog` configurado, `config/` lendo env, `GET /health`,
  shutdown gracioso via `signal.NotifyContext`
- SvelteKit + Tailwind + TypeScript, `lib/api/client.ts` (o wrapper fino sobre `fetch` com
  `credentials: 'include'`)
- `Makefile` com `test-backend` / `test-frontend`, `.env.example`, `CLAUDE.md`, `README.md`

**Arquivos:** `docker-compose.yml`, `backend/go.mod`, `backend/cmd/api/main.go`,
`backend/config/*.go`, `frontend/` (scaffold), `Makefile`

**Pronto quando:** `docker compose up` sobe tudo, `curl localhost:8080/health` responde, a home
aparece em `:5173`, e `Ctrl+C` desliga sem erro no log.

**Estudo:**
- [How to Write Go Code](https://go.dev/doc/code) — módulos e layout
- [chi](https://github.com/go-chi/chi) — router e middlewares
- [log/slog](https://pkg.go.dev/log/slog) — logging estruturado
- [Docker Compose file reference](https://docs.docker.com/reference/compose-file/) e
  [Flyway — naming de migrations](https://documentation.red-gate.com/fd/migrations-271585107.html)
- [SvelteKit — project structure](https://svelte.dev/docs/kit/project-structure)

---

### Fase 1 — Contas e sessão

**Conceito novo:** nenhum grande, mas é a fase que fixa argon2id e cookie `__Host-` com você no
comando (no agendaGo isso já existia pronto).

**Entrega:** domínio `usuario`, `session`, `signup`, `passwordreset`; hash argon2id; cadastro com
confirmação por email (capturado no Mailpit); login/logout; `GET /auth/me`; recuperação de senha;
rate limiting com `httprate`. Frontend: telas de cadastro, confirmação, login, recuperar/redefinir
senha, e a store de sessão (`session.svelte.ts`).

**Migrations:** `V1__cria_tabela_usuarios.sql`, `V2__cria_tabela_sessions.sql`,
`V3__cria_tabela_cadastros_pendentes.sql`, `V4__cria_tabela_password_reset_tokens.sql`

**Pronto quando:** cadastro → email no Mailpit (`localhost:8025`) → confirma → login → `/auth/me`
devolve o usuário → logout invalida a sessão. Testes de domínio e usecase (fakes em memória)
passando.

**Estudo:**
- [alexedwards/argon2id](https://github.com/alexedwards/argon2id) e
  [OWASP Password Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)
- [MDN — Set-Cookie](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Set-Cookie)
  e [SameSite](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Set-Cookie/SameSite)
  — atenção especial ao prefixo `__Host-`; ele volta na fase 5
- [OWASP Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)

---

### Fase 2 — Quadro, colunas e cards (ainda sem tempo real)

**Conceito novo:** modelagem de hierarquia com autorização por recurso (quem pode ver/editar
*este* quadro), diferente do agendaGo onde o dono do dado era sempre o prestador logado.

**Entrega:** domínio `board`, `coluna`, `card`, `membro` (papéis dono/editor/leitor); CRUD REST
completo; autorização checada no usecase (nunca só no handler). Frontend: lista de quadros e a
tela do quadro renderizando colunas e cards em CSS grid, com criar/renomear/apagar — **com
recarga manual mesmo**, para a fase 5 ter contraste.

**Migrations:** `V5__cria_tabela_boards.sql`, `V6__cria_tabela_board_membros.sql`,
`V7__cria_tabela_colunas.sql`, `V8__cria_tabela_cards.sql`

Campos que já nascem pensando nas fases seguintes: `colunas.posicao`, `cards.posicao`
(`double precision`) e `cards.version` (`integer`).

**Pronto quando:** dá pra montar um quadro inteiro pela UI e ele sobrevive ao F5. Um usuário não
enxerga o quadro do outro (teste de autorização cobrindo isso).

**Estudo:**
- [Atlassian — o que é Kanban](https://www.atlassian.com/agile/kanban) (o conceito, já que você
  nunca usou o Trello) e [WIP limits](https://www.atlassian.com/agile/kanban/wip-limits)
- [PostgreSQL — chaves estrangeiras](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-FK)
- [pgx — CollectRows / RowTo](https://pkg.go.dev/github.com/jackc/pgx/v5) para montar as listas
  aninhadas sem N+1

---

### Fase 3 — Convites e colaboração

**Conceito novo:** o quadro deixa de ser de uma pessoa. É o pré-requisito humano do tempo real —
sem duas contas no mesmo quadro, não há o que sincronizar.

**Entrega:** convidar por email para o quadro, aceitar convite (com token), listar/remover membros,
papéis aplicados na autorização (leitor não move card). Frontend: painel de membros do quadro.

**Migrations:** `V9__cria_tabela_convites_board.sql`

**Pronto quando:** duas contas diferentes abrem o mesmo quadro em dois navegadores. (Ainda sem
sincronia — cada uma precisa dar F5 para ver o que a outra fez. Guarde essa sensação: é o problema
que a fase 5 resolve.)

**Estudo:**
- [RBAC — NIST](https://csrc.nist.gov/projects/role-based-access-control) (visão geral do modelo
  de papéis)
- [crypto/rand](https://pkg.go.dev/crypto/rand) — tokens de convite imprevisíveis

---

### Fase 4 — Arrastar e soltar + ordenação fracionária

**Conceito novo (o primeiro grande):** como ordenar itens quando várias pessoas inserem no meio ao
mesmo tempo.

Se a posição for `1, 2, 3, 4`, arrastar um card para o meio obriga a reescrever todas as linhas
abaixo — muitas linhas alteradas e corrida garantida com dois usuários. Com **fractional
indexing**, a posição é fracionária e inserir entre `2.0` e `3.0` é gravar `2.5`: **uma linha
alterada, nenhuma corrida.**

```
antes:   A(1.0)   B(2.0)   C(3.0)
                     ▲ solta aqui
depois:  A(1.0)   B(2.0)  X(2.5)  C(3.0)     ← só X foi escrito
```

**Entrega:** `PATCH /cards/{id}/mover` recebendo `{colunaDestino, posicao}`; o front calcula a
posição como a média dos vizinhos; `svelte-dnd-action` no arrastar; **atualização otimista** —
a UI move na hora e desfaz se a API recusar.

**Pronto quando:** arrastar entre colunas e dentro da coluna persiste; o F5 mantém a ordem; e o
log do Postgres mostra **um** `UPDATE` por movimento.

**A armadilha, de propósito:** ficar dividindo o mesmo intervalo (`2.5`, `2.75`, `2.875`…) esgota a
mantissa de 53 bits do `double precision` em ~50 inserções no mesmo ponto e dois cards colidem na
mesma posição. Deixe acontecer, escreva o teste que reproduz — a fase 9 conserta.

**Estudo:**
- [Figma — Realtime editing of ordered sequences](https://www.figma.com/blog/realtime-editing-of-ordered-sequences/)
  — leitura obrigatória desta fase, explica exatamente esse problema
- [Implementing Fractional Indexing](https://observablehq.com/@dgreensp/implementing-fractional-indexing)
  — a versão com chaves textuais (o destino da fase 9); o termo para pesquisar depois é *LexoRank*,
  a solução do Jira para o mesmo problema
- [svelte-dnd-action](https://github.com/isaacHagoel/svelte-dnd-action) — a lib de drag & drop
- [Svelte 5 — runes](https://svelte.dev/docs/svelte/what-are-runes) (`$state`, `$derived`) — a
  atualização otimista vive aqui
- [IEEE 754 / precisão de ponto flutuante](https://docs.python.org/3/tutorial/floatingpoint.html)
  — a explicação mais didática que existe do limite que você vai bater (é em Python, mas o
  problema é do padrão, não da linguagem)

---

### Fase 5 — Tempo real: WebSocket + hub

**Conceito novo (o coração do projeto):** o servidor falando primeiro, e uma goroutine por
conexão.

HTTP é pergunta-e-resposta: o servidor não tem como avisar ninguém. O WebSocket começa como um
`GET` comum e **troca de protocolo** no meio (`101 Switching Protocols`), deixando um cano aberto
nos dois sentidos.

```
navegador A ──WS──┐
navegador B ──WS──┼──► hub ──► sala "board:7" ──► broadcast para as outras conexões
navegador C ──WS──┘            (map[boardID]map[*conexao]struct{} + sync.RWMutex)
```

**Entrega:**
- `GET /ws?board={id}` em `adapter/http/ws`: autentica pelo **mesmo cookie de sessão** (o
  handshake é HTTP, o cookie viaja junto), valida que o usuário é membro do quadro, e faz o
  `websocket.Accept` com `OriginPatterns` restrito
- `adapter/realtime/hub`: registro/remoção de conexão, salas por quadro, `Publicar` sem bloquear
  (canal com buffer; conexão lenta é derrubada, não segura o servidor)
- Uma goroutine de leitura e **uma única** de escrita por conexão, com ping/pong para detectar
  cliente morto
- Porta `Publicador` no usecase; cada usecase de escrita publica **depois do commit**
- Frontend: `lib/realtime/conexao.svelte.ts` — abre o socket ao entrar no quadro, aplica os eventos
  recebidos no `$state`, fecha ao sair
- **O teste que define o projeto:** Playwright com dois `BrowserContext`, um arrasta e o outro vê

**Pronto quando:** `make test-e2e` passa com o teste de duas abas, e `go test -race ./...` está
limpo.

**Segurança que não pode passar batido:** WebSocket **não obedece CORS**. Sem checar `Origin`, um
site qualquer abre um socket autenticado com o cookie da vítima (*Cross-Site WebSocket
Hijacking*). O `OriginPatterns` do `coder/websocket` é a defesa — e o cookie `SameSite` da fase 1
é a segunda camada.

**Estudo:**
- [coder/websocket — godoc](https://pkg.go.dev/github.com/coder/websocket) e os
  [exemplos do repositório](https://github.com/coder/websocket/tree/master/internal/examples)
- [O exemplo de chat do gorilla/websocket](https://github.com/gorilla/websocket/tree/main/examples/chat)
  — mesmo usando outra lib, `hub.go` e `client.go` são a referência canônica do padrão hub em Go;
  leia os dois arquivos inteiros
- [RFC 6455](https://datatracker.ietf.org/doc/html/rfc6455) — o handshake e os frames (leia as
  seções 1 e 4; o resto é referência)
- [MDN — WebSocket API](https://developer.mozilla.org/en-US/docs/Web/API/WebSockets_API) para o lado
  do navegador
- [Go blog — Share memory by communicating](https://go.dev/blog/codelab-share) e
  [Go Concurrency Patterns (Rob Pike)](https://go.dev/talks/2012/concurrency.slide) — o modelo
  mental do hub
- [pkg.go.dev/sync](https://pkg.go.dev/sync) — `RWMutex` para o mapa de salas
- [Data Race Detector](https://go.dev/doc/articles/race_detector) — rode com `-race` desde o
  primeiro teste desta fase
- [Playwright — browser contexts](https://playwright.dev/docs/browser-contexts) — como abrir duas
  sessões independentes no mesmo teste
- [OWASP HTML5 Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/HTML5_Security_Cheat_Sheet.html#websockets)
  — a seção de WebSockets cobre o CSWSH

---

### Fase 6 — Presença e edição concorrente

**Conceito novo:** estado efêmero (que **não** vai para o banco) e resolução de conflito.

**Entrega:**
- Presença: avatares de quem está com o quadro aberto, derivados do próprio mapa de conexões do
  hub; entrada/saída viram eventos; heartbeat limpa quem sumiu sem fechar direito
- "Ana está editando este card": marca temporária com TTL, renovada enquanto o campo tem foco
- **Bloqueio otimista:** todo `UPDATE` de card leva `WHERE id = $1 AND version = $2` e incrementa
  a versão; zero linhas afetadas → `409 Conflict`. A UI mostra "alguém mudou este card" e recarrega
  o card em vez de sobrescrever

**Pronto quando:** dois navegadores abrem o quadro e cada um vê o avatar do outro; fechar uma aba
remove o avatar em segundos; e um teste de handler prova que o segundo `UPDATE` com versão velha
devolve 409.

**Estudo:**
- [Martin Fowler — Optimistic Offline Lock](https://martinfowler.com/eaaCatalog/optimisticOfflineLock.html)
  — o padrão exato do `version`
- [PostgreSQL — isolamento de transações](https://www.postgresql.org/docs/current/transaction-iso.html)
  — por que `READ COMMITTED` (o padrão) não te salva sozinho
- [MDN — Status 409 Conflict](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Status/409)
- [time.Ticker](https://pkg.go.dev/time#Ticker) — a varredura periódica de presença morta

---

### Fase 7 — Reconexão e replay de eventos

**Conceito novo:** entrega confiável sobre um canal que cai. É o mesmo raciocínio de *offset* do
Kafka, na versão caseira — e o momento em que "tempo real" vira "tempo real **correto**".

O wi-fi cai por 20 segundos e o cliente perde 3 eventos. Ao voltar, ele não pode fingir que nada
aconteceu.

**Entrega:**
- Tabela `board_events` (`seq BIGSERIAL`, `board_id`, `tipo`, `payload JSONB`, `autor_id`,
  `criado_em`), escrita **na mesma transação** da mudança do dado — é o *transactional outbox*
  em miniatura: ou o card move e o evento existe, ou nenhum dos dois
- O cliente guarda o último `seq` aplicado; ao reconectar, envia `?desde=41` e recebe o backlog
  antes de voltar ao vivo
- Reconexão com backoff exponencial + jitter no front, e indicador visual de "reconectando"
- Idempotência no cliente: aplicar o mesmo evento duas vezes não pode duplicar card (o `seq`
  resolve — descarta o que for `<=` ao último aplicado)
- Teste: derrubar a API com `docker compose stop api`, mexer no quadro pela outra aba, subir de
  volta e verificar a convergência

**Migrations:** `V10__cria_tabela_board_events.sql`

**Pronto quando:** matar a conexão, mexer no quadro pelo outro navegador e reconectar deixa as duas
telas idênticas — sem F5.

**Estudo:**
- [microservices.io — Transactional Outbox](https://microservices.io/patterns/data/transactional-outbox.html)
- [AWS — Exponential backoff and jitter](https://aws.amazon.com/builders-library/timeouts-retries-and-backoff-with-jitter/)
  — por que o jitter importa quando 50 clientes reconectam juntos
- [Idempotência — Stripe API](https://docs.stripe.com/api/idempotent_requests) — a explicação mais
  clara do conceito em API real
- [PostgreSQL — tipos JSON](https://www.postgresql.org/docs/current/datatype-json.html) para o
  payload do evento

---

### Fase 8 — Endurecer (produtização)

**Conceito novo:** nenhum — é trazer o rigor do agendaGo para um projeto que agora tem
concorrência de verdade para proteger.

**Entrega:** tabela de rotas no README, Testcontainers nos testes de
repositório, `compatibilidade_schema_test.go` (o guard de expand/contract), CI no GitHub Actions
rodando `-race`, `docker-compose.prod.yml` atrás do Caddy do VPS, e `docs/tecnologias.md` no formato de guia de
estudo do agendaGo.

**O Swagger saiu do plano.** Era entrega desta fase e não será feito: a tabela do README já
responde à mesma pergunta, e uma segunda descrição das rotas — gerada de anotações no código —
seria mais uma fonte da verdade para manter alinhada, ao custo de blocos `@Summary`/`@Router` em
cada handler. A conta só muda se a API passar a ser consumida por terceiros.

**Atenção específica deste projeto:** proxy reverso e conexão longa não se dão bem por padrão —
timeout de leitura, buffering e keep-alive precisam de atenção, e é aqui que o ping/pong da fase 5
prova seu valor.

**Estudo:**
- [Caddy — reverse_proxy](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy) — ele faz
  o upgrade do WebSocket sozinho e não impõe timeout de leitura à conexão, ao contrário do nginx,
  onde o `Upgrade` e o `Connection` precisam ser repassados à mão e o `proxy_read_timeout` padrão
  de 60s derruba conexão longa em silêncio
- [Testcontainers for Go](https://golang.testcontainers.org/)
- [GitHub Actions — workflow syntax](https://docs.github.com/en/actions/reference/workflow-syntax-for-github-actions)
- [net/http — Server timeouts](https://pkg.go.dev/net/http#Server) — `ReadTimeout` mata WebSocket se
  configurado sem pensar

---

### Fase 9 — Chaves de ordenação textuais ✅

**Conceito novo:** consertar em produção uma escolha de tipo, usando a doutrina expand/contract do
próprio agendaGo.

Trocar `cards.posicao` de `double precision` para `text` (chaves estilo LexoRank: entre `"a"` e
`"b"` cabe `"an"`, infinitamente). Em três passos: coluna nova anulável → código novo escrevendo
nas duas → `SET NOT NULL` e `DROP` da antiga no deploy seguinte. O backfill roda **por um comando
do domínio**, nunca por SQL na migration.

**Estudo:** [Implementing Fractional Indexing](https://observablehq.com/@dgreensp/implementing-fractional-indexing)
· o próprio `CLAUDE.md` do agendaGo, seção "Migration que aperta exige dois deploys"

> **Feita, nos dois deploys.**
>
> O limite era real e está medido: o float esgotava em **52 inserções** seguidas
> no mesmo ponto. A chave textual leva o teto para perto de **750** — e esse
> número só apareceu no contract, quando o domínio passou a conhecer o
> `VARCHAR(200)` da coluna. Até então os testes prometiam "mil inserções nunca
> esgotam", o que era falso: eles passavam porque ninguém perguntava ao banco se
> a chave cabia. Sem essa amarração, o estouro sairia como erro de driver — 500
> em vez do 409 previsto.
>
> No caminho apareceu um desperdício do algoritmo: entre `"bq"` e `"c"` ele
> estendia para três caracteres quando `"br".."bz"` cabiam com dois, gastando uma
> letra POR inserção. Nesse padrão o teto era **199**, não 750. Corrigido, os
> quatro padrões de reordenação convergem para perto de 750.
>
> O teto ainda existe, e é assim que fica: uma coluna mais larga só o adiaria. O
> conserto de verdade, no dia em que alguém alcançá-lo, é redistribuir as chaves
> da lista — a reescrita em massa que o esquema evita no caso comum.
>
> A invariante que sustenta o esquema: **nenhuma chave termina no menor
> caractere**. Sem ela, `"a"` seria um beco sem saída, porque não existe string
> entre `""` e `"a"` — e inserir no topo passaria a ser impossível. (O plano
> ilustrava com "entre a e b cabe an"; na implementação `"a"` não é chave
> válida, e o exemplo equivalente é entre `"b"` e `"c"`.)
>
> O ciclo completo, nesta ordem:
>
> 1. **expand** (`V18`): `chave` anulável em `cards` e `colunas`, com índice em
>    `COLLATE "C"`;
> 2. **código novo**: escreve as duas colunas, o `mover` calcula pela chave, e a
>    leitura ordena por ela com a posição como desempate (`NULLS FIRST`, para a
>    linha ainda não preenchida aparecer onde aparecia);
> 3. **backfill**: comando do domínio (`BackfillUseCase`), idempotente e
>    retomável, que rodava no start da aplicação. Preservava a ordem que a
>    posição ditava — um backfill que embaralhasse o quadro seria pior que
>    nenhum — e **não subia a `version`**, porque preencher a chave não é uma
>    edição feita por ninguém e subir a versão faria o bloqueio otimista recusar
>    a próxima gravação legítima;
> 4. **contract** (`V19`): `SET NOT NULL` na chave e `DROP` de `posicao`, só
>    depois de o passo 3 ter rodado em produção. Fazê-lo junto quebraria a
>    versão anterior da aplicação, que continua no ar durante o deploy e é para
>    onde um rollback volta. O `BackfillUseCase` saiu no mesmo commit: um
>    backfill que já rodou e cuja coluna de origem não existe mais é código
>    morto.
>
> O contract precisou **declarar-se ao guard**: `compatibilidade_schema_test.go`
> reprova coluna apertada e coluna removida por padrão, e agora lê uma linha
> `-- CONTRACT:` na migration listando exatamente o que está autorizado. Ele
> falha nos dois sentidos — se a migration mexer em algo fora da lista, e se
> sobrar declaração sem uso, porque autorização esquecida é a que passa
> despercebida no dia do acidente.
>
> ⚠️ **O erro que a fase 9 quase escondeu.** A implementação inicial estava
> verde e **não funcionava**: o cálculo do float vinha antes do da chave, e o
> `ErrSemEspaco` dele abortava o movimento — o mesmo 409 que a fase existia para
> matar, na 53ª inserção. O teste não pegou porque movia sempre entre os
> *mesmos* dois vizinhos, e aí o intervalo nunca apertava. A lição virou método:
> **teste verde não é prova**. O que passou a valer é quebrar o código de
> propósito e conferir que algum teste cai.
>
> Duas coisas mais só apareceram assim. O backfill **reordenava** colunas mistas
> — encadeava a partir de `UltimaChave` (o MAX), mas a leitura usava
> `NULLS FIRST`, então as linhas antigas pulavam para o fim; e o teste disso só
> checava a ordem, enquanto o defeito produzia chaves **duplicadas** que a
> `posicao` desempatava. E duas pessoas soltando um card no mesmo ponto ao mesmo
> tempo geravam a **mesma** chave, depois da qual não cabia mais nada entre
> elas: a chave nova passou a ser sorteada dentro da folga em vez de fixada no
> meio exato.
>
> ⚠️ A armadilha que se confirmou: **a ordenação de texto no Postgres depende da
> collation**. O `ORDER BY` e o índice usam `COLLATE "C"` — a ordem de bytes,
> que é a que o domínio assume. Com collations diferentes entre índice e
> consulta, o Postgres nem usaria o índice.

---

## Continuação — o produto

As fases de 0 a 8 entregaram um quadro que funciona e sincroniza. O que segue é o que falta para
ele ser usado por um time de verdade, na ordem em que cada peça passa a fazer falta.

A ordem não é arbitrária: a **10** faz o quadro responder "o que é meu?", a **11** cria o primeiro
fluxo em que recarregar tudo é obviamente desperdício, e a **12** cobra a fatura que a 11 gerou.
As três formam um arco. As demais são independentes entre si.

---

### Fase 10 — Responsável no card, e filtro ✅

> **Feita.** O filtro trava o arraste enquanto está ligado, e isso é decisão, não
> limitação: os vizinhos calculados a partir de uma lista incompleta colocariam o
> card entre cards que não são os vizinhos reais. A tela diz isso na barra, em
> vez de deixar o arraste falhar em silêncio.

**Conceito novo:** nenhum grande no backend — é um vínculo N:N reaproveitando a autorização que já
existe. O que muda é a pergunta que o quadro passa a responder: hoje ele mostra *o que existe*, e
não *o que é meu*.

**Entrega:**
- Tabela `card_responsaveis` (`card_id`, `usuario_id`, `criado_em`), chave primária composta — a
  mesma forma de `card_etiquetas`, e pelo mesmo motivo: ela já impede a atribuição repetida
- Regra de domínio: **só dá para atribuir quem participa do quadro**. A checagem é do usecase,
  como todas as outras; a chave estrangeira aponta para `usuarios`, não para `board_membros`
- Avatar no card, no mesmo padrão da presença (iniciais em círculo)
- Filtro na tela do quadro: por responsável, etiqueta e prazo — combináveis, aplicados no
  cliente sobre o quadro já carregado
- A leitura do quadro devolve os responsáveis de todos os cards **numa consulta só**, no mesmo
  molde de `EtiquetasDoBoardPorCard`

**Migrations:** `V15__cria_tabela_card_responsaveis.sql`

**A decisão que não dá para adiar:** o que acontece com a atribuição quando a pessoa é removida do
quadro. Manter gera uma lista de responsáveis que mente — nomes de quem não tem mais acesso.
Recomendo remover o vínculo junto, na mesma transação da remoção do membro, e é a
`UnidadeDeTrabalho` da fase 8 que torna isso barato.

**Pronto quando:** duas contas no mesmo quadro, uma atribui a outra, e o avatar aparece na tela do
outro navegador sem F5. O filtro "meus cards" esconde o resto e sobrevive ao recarregar.

**Estudo:**
- [pgx — CollectRows](https://pkg.go.dev/github.com/jackc/pgx/v5) para agregar sem N+1 (o
  `EtiquetasDoBoardPorCard` já é o exemplo pronto dentro do projeto)
- [Trello — atribuir membros a um cartão](https://support.atlassian.com/trello/docs/adding-members-to-a-card/)
  para ver o que a referência faz com o caso de quem sai do quadro

---

### Fase 11 — Comentários e histórico de atividade ✅

> **Feita, as duas metades.** Conversa em markdown no card, com autor, data e
> marca de edição; selo 💬 no card. As três permissões saíram como planejado —
> escrever exige só participação (o leitor comenta), editar é **só do autor**, e
> apagar o autor no próprio ou quem administra em qualquer um.
>
> **A armadilha do payload foi resolvida pelo caminho recomendado.** O evento
> passou a guardar o estado ANTERIOR onde ele importa: `card.movido` leva a
> coluna de origem, `card.alterado` leva o título anterior, `card.apagado` leva o
> nome (que some com o card). E guarda **nomes**, não só ids — um log registra o
> que era verdade no momento, e resolver o id na leitura mostraria o título de
> hoje numa frase sobre ontem.
>
> O histórico não tem tabela: é um read model sobre `board_events`, com uma
> coluna `card_id` (expand puro) para ele ser lido por índice em vez de varrer
> o payload de todos os eventos do quadro.
>
> Três defeitos apareceram só ao exercitar a API de verdade, todos da mesma
> família (a borda HTTP): um erro de domínio sem tradução virando **500**, e dois
> campos que o DTO declarava mas o conversor não copiava — um deles chegando
> como `null` e quebrando a tela em tempo de execução. Os três estão trancados
> por teste de handler.
>
> **A fase nasceu muda, e isso só apareceu ao medir.** O quadro se atualizava
> sozinho, mas o modal do card carregava os dados UMA vez, na abertura: duas
> pessoas no mesmo card não viam o comentário uma da outra, e o histórico aberto
> congelava. A recarga que o evento dispara atualiza o quadro ATRÁS do modal, e
> o modal tem dados próprios — comentários, histórico, anexos — que o `GET` do
> quadro nem traz.
>
> O conserto é um contador (`pulso`) que a página incrementa a cada rajada de
> mudanças alheias; o modal relê o card quando ele muda. Relê SEMPRE, sem olhar
> de que card era o evento: filtrar exigiria um caminho por tipo para saber onde
> procurar o id no payload, que é exatamente o que a fase 12 pesa antes de fazer.
>
> Duas armadilhas de Svelte 5 no caminho, ambas caçadas pelo teste:
>
> - ler `pulso` dentro do efeito do `cardId` torna aquele efeito dependente
>   dele, e cada mudança alheia reiniciava o modal inteiro — fechando o
>   histórico aberto. `untrack` resolve, e é o que declara a intenção;
> - a versão do bloqueio otimista passou a ser **congelada na abertura do
>   editor**. Antes era `card.version` na hora de gravar, o que só funcionava
>   por acidente: nada relia o card durante a edição. Com o modal ao vivo, a
>   versão fresca chegaria a tempo de o servidor ACEITAR a gravação, apagando em
>   silêncio o que a outra pessoa acabou de escrever — o defeito exato que o
>   bloqueio existe para impedir.

**Conceito novo:** o primeiro fluxo **append-only** do projeto, e o primeiro em que recarregar o
quadro inteiro por um evento fica evidentemente errado — um comentário chegando não deveria
provocar um `GET /boards/{id}`.

**Entrega:**
- Tabela `comentarios` (`id`, `card_id`, `autor_id`, `texto`, `criado_em`, `editado_em` anulável).
  O `autor_id` **sem** `ON DELETE CASCADE`: apagar uma conta não pode reescrever a conversa
- Renderização com o `lib/markdown.ts` que já existe
- Tipo de evento novo, `comentario.criado`, com payload útil — este é o primeiro evento cujo
  conteúdo a tela vai querer aplicar direto, e não usar como aviso
- **Histórico de atividade do card, sem tabela nova:** `board_events` já guarda tipo, autor,
  payload e data. O feed é um *read model* sobre o que a fase 7 já escreve

**Migrations:** `V16__cria_tabela_comentarios.sql`

**A armadilha, e ela é concreta:** o payload dos eventos estruturais **não basta para o feed**. O
`card.movido` guarda o card já movido, com a coluna de destino — a coluna de origem se perde, e
sem ela não dá para escrever "moveu de *A fazer* para *Fazendo*". O mesmo vale para renomear, que
não guarda o título anterior.

São dois caminhos, e é melhor escolher com os olhos abertos:

1. **Enriquecer o payload agora** — o evento passa a levar o antes e o depois. O feed fica bom, e
   o custo é a tabela crescer mais rápido;
2. **Aceitar um feed pobre** — "fulano moveu este card", sem de onde para onde.

Recomendo o **1**, e fazê-lo *nesta* fase: mudar o formato do payload depois não corrige o
histórico já gravado, e evento antigo não se reescreve.

**Pronto quando:** ana comenta e bruno vê aparecer sem recarregar; o card abre mostrando quem fez o
quê desde que ele nasceu.

**Estudo:**
- [PostgreSQL — tipos JSON](https://www.postgresql.org/docs/current/datatype-json.html), agora
  para versionar o formato do payload sem quebrar o que já está gravado
- [Event sourcing vs. event log](https://martinfowler.com/eaaDev/EventSourcing.html) — o suficiente
  para ver que aqui o log é *derivado*, não a fonte da verdade, e por que essa diferença importa

---

### Fase 12 — Aplicação incremental de eventos

**Conceito novo:** deixar de recarregar e passar a aplicar diferença — e assumir o risco que isso
traz, que é a tela poder divergir do banco **em silêncio**.

Hoje todo evento que não é presença cai numa recarga do quadro. A escolha foi deliberada e está
certa para uma primeira versão: recarregar é sempre correto. O preço é que o servidor manda o tipo
e o payload de cada evento, e o cliente joga os dois fora.

> ⚠️ **O custo foi MEDIDO, e ele ainda não justifica a fase.** O `GET` do quadro com um card são
> 554 bytes, e um comentário gera exatamente uma requisição — a janela de 150ms já junta rajadas.
> O que realmente doía era outra coisa, e foi consertado sem tocar nesta fase: o modal do card não
> escutava evento nenhum (ver o fecho da fase 11).
>
> O argumento que sobra para a 12 não é tráfego, é o teto de requisições por sessão ser consumido
> pelo movimento dos OUTROS num quadro grande. Vale refazer a medição com dezenas de cards e várias
> pessoas antes de aceitar o risco de a tela divergir do banco em silêncio.

**Entrega:**
- Um caminho de aplicação por tipo de evento, sobre o `$state`
- `recarregue.tudo` permanece como rede de segurança, e passa a ser usado também quando a
  aplicação incremental encontra um evento que não sabe tratar
- **Reconciliação:** ao reganhar o foco da aba, comparar o último `seq` aplicado com o do servidor
  e recarregar se divergirem. É o que impede um erro de aplicação de durar para sempre

**Pronto quando:** mover um card no navegador A **não gera requisição nenhuma** no navegador B —
confira na aba Network — e o teste de duas abas continua verde.

**A armadilha, que é a razão de esta fase vir depois da 11:** qualquer caminho de aplicação errado
deixa a tela discordando do banco sem ninguém perceber, que é o pior defeito possível num quadro
colaborativo. Faça só depois de ter um caso em que recarregar dói de verdade — senão você paga o
risco sem receber o benefício.

**Estudo:**
- [Svelte 5 — runes](https://svelte.dev/docs/svelte/what-are-runes), agora `$state.snapshot` e
  atualização granular de listas
- [Convergência e reconciliação](https://martin.kleppmann.com/2015/05/11/please-stop-calling-databases-cp-or-ap.html)
  — o vocabulário para pensar "a tela e o banco concordam?"

---

### Fase 13 — Arquivar, com desfazer ❌ retirada

> **Foi feita inteira e depois RETIRADA, por decisão de produto.** O texto
> abaixo é o plano original, mantido porque o assunto pode voltar.
>
> O que sobrou no repositório, e não é esquecimento:
>
> - **`V20` continua no `migrations/`.** O Flyway é forward-only e guarda o
>   checksum do que aplicou; apagar um arquivo já aplicado faz a validação
>   falhar no próximo start. A migration rodou em produção — o que já aconteceu
>   não se desfaz apagando o texto.
> - **As colunas `arquivado_em` continuam no banco**, sem ninguém que as leia ou
>   escreva. Elas são exatamente o que o guard de SQL caça, então passaram a ser
>   declaradas em `migrations/COLUNAS-ORFAS.md`, e o guard aprendeu a diferença
>   entre uma órfã deliberada e o defeito que ele existe para pegar. A
>   declaração não é permanente: ele reprova se a coluna sumir e a linha ficar, e
>   reprova também se o código voltar a usá-la sem a linha sair.
>
> ⚠️ **A declaração quase foi parar dentro da V20, e isso derrubaria o deploy.**
> O Flyway guarda o checksum de cada migration aplicada e valida na partida:
> acrescentar UM COMENTÁRIO a um arquivo já aplicado muda o checksum e o start
> falha com `Migration checksum mismatch`. Aconteceu aqui, na stack local, antes
> de virar commit. Migration aplicada é imutável inclusive nos comentários — daí
> a declaração morar num `.md`, que o Flyway não recolhe.
>
> ⏳ **Falta o contract — `V21`, e ele é do PRÓXIMO deploy.** Derrubar as colunas
> no mesmo deploy que retira o código quebraria a versão que está no ar durante
> a janela de publicação, que é também para onde um rollback volta: ela lê e
> escreve `arquivado_em` em todo SELECT e INSERT de card e coluna. A mesma
> doutrina da fase 9, agora na direção contrária — ali uma coluna nascia, aqui
> uma morre, e nos dois casos são dois deploys.
>
> **O que a fase deixou de bom, e fica:** a correção do vazamento de anexos no
> disco (o `ON DELETE CASCADE` limpava a tabela e não o volume), que apareceu ao
> levantar os motivos desta fase e não tem relação com arquivar.


**Conceito novo:** exclusão reversível, e o ciclo expand/contract numa coluna nova que **não tem
contract** — ela nasce anulável e assim fica.

Hoje `DELETE /cards/{id}` é definitivo e leva checklists e anexos por cascata. Não há desfazer, e
não há como recuperar. O Trello arquiva.

**Entrega:**
- `arquivado_em TIMESTAMPTZ` anulável em `cards` e `colunas`
- Toda leitura passa a filtrar `arquivado_em IS NULL`
- Tela de arquivados do quadro, com desarquivar
- Apagar de vez continua existindo, só do dono, e a partir da tela de arquivados

**Migrations:** `V17__adiciona_arquivado_em_em_cards_e_colunas.sql`

**A armadilha:** **todo** `SELECT` existente precisa passar a filtrar, e um esquecido faz o card
arquivado reaparecer no quadro. É exatamente o tipo de defeito que os fakes em memória não pegam —
e é onde os testes de repositório contra Postgres real da fase 8 pagam o investimento.

**Pronto quando:** arquivar um card o tira do quadro nos dois navegadores, ele aparece na tela de
arquivados, e desarquivar o devolve à mesma coluna e posição.

---

### Fase 14 — WIP limit por coluna

**Conceito novo:** a primeira regra que **recusa** uma operação por política do quadro, e não por
falta de permissão. E a primeira em que a contagem precisa acontecer dentro da transação.

**Entrega:**
- `colunas.wip_limite INTEGER` anulável (nulo = sem limite)
- O domínio recusa criar e mover para uma coluna cheia; o handler responde **409** com a mensagem
- A coluna mostra `3/5`, e muda de cor ao encher

**Migrations:** `V18__adiciona_wip_limite_em_colunas.sql`

**A armadilha, e ela é do eixo do projeto:** duas pessoas movem um card ao mesmo tempo para uma
coluna com **uma** vaga. Se a contagem for feita antes da transação, as duas passam e a coluna
estoura o próprio limite. Contar **dentro** da transação da escrita é o que resolve — e a
`UnidadeDeTrabalho` da fase 8 já entrega os repositórios ligados a ela.

**Pronto quando:** existe um teste que dispara dois movimentos concorrentes contra uma coluna com
uma vaga e prova que **exatamente um** passa.

**Estudo:**
- [Atlassian — WIP limits](https://www.atlassian.com/agile/kanban/wip-limits)
- [PostgreSQL — isolamento de transações](https://www.postgresql.org/docs/current/transaction-iso.html),
  desta vez para valer: é aqui que `READ COMMITTED` não basta sozinho

---

### Fase 15 (opcional) — Mais de uma instância da API

**Conceito novo:** o hub é um mapa em memória, e memória não é compartilhada entre processos.

Subir uma segunda réplica hoje quebra o tempo real **em silêncio**: quem está conectado na
instância A não recebe o que aconteceu na B. Nada falha, nada aparece no log — a tela só para de
atualizar para metade das pessoas.

**Entrega:** o hub continua sendo a sala local; entra uma ponte por
[LISTEN/NOTIFY](https://www.postgresql.org/docs/current/sql-notify.html) que avisa as outras
instâncias.

**O detalhe que faz o desenho ficar simples:** o `NOTIFY` tem teto de payload (8000 bytes), então
**não mande o evento pelo canal — mande só o `seq`**. Cada instância lê o evento de
`board_events`, que é a fonte da verdade e já está lá desde a fase 7. Some o problema de tamanho, e
some também o de formato.

**Pronto quando:** duas instâncias da API atrás do mesmo proxy, dois navegadores em instâncias
diferentes, e o card move nos dois.

**Estudo:**
- [PostgreSQL — NOTIFY](https://www.postgresql.org/docs/current/sql-notify.html) e o
  [suporte do pgx](https://pkg.go.dev/github.com/jackc/pgx/v5#Conn.WaitForNotification)
- [NATS](https://docs.nats.io/) ou [Redis Pub/Sub](https://redis.io/docs/latest/develop/interact/pubsub/)
  — a saída industrial, para saber o que se está trocando

---

### Depois disso, se der gosto

**Já foram feitos, fora de ordem:** etiquetas, prazo, checklist e anexos entraram junto com a
adaptação do template do Trello, entre as fases 3 e 4. A consequência a pagar está na fase 5:
cada uma dessas tabelas é uma fonte de eventos que o hub vai precisar propagar — etiqueta
aplicada, item marcado, anexo enviado —, e isso não estava no desenho original daquela fase.

Comentários, histórico de atividade, WIP limit e múltiplas instâncias saíram desta lista: viraram
as fases 11, 14 e 15 acima. O que segue realmente de fora:

- **Cursor de cada pessoa flutuando na tela.** Bonito e caro: exige um canal de altíssima
  frequência (dezenas de mensagens por segundo por pessoa) que não pode passar pelo mesmo caminho
  dos eventos do quadro, nem ser gravado. É estado efêmero levado ao extremo.
- **"Fulano está editando este card"**, que a fase 6 previa e não foi feito: marca temporária com
  TTL, renovada enquanto o campo tem foco. Depende de o cliente passar a **enviar** mensagens pelo
  socket, coisa que hoje ele não faz — `lerAteMorrer` existe só para processar os pongs.
- **Notificações** (por email ou push) e **quadros públicos por link**.
- **Edição concorrente de texto de verdade** (dois digitando na mesma descrição): é o território
  de [CRDTs](https://crdt.tech/) — [Yjs](https://docs.yjs.dev/) e
  [Automerge](https://automerge.org/) são as implementações de referência.

---

## Verificação

Ao fim de cada fase:

```bash
make test              # domínio + usecases + handlers (fakes) + Vitest
docker compose up -d   # postgres, flyway, api, web, mailpit
```

- **Fase 1:** cadastro → Mailpit em `localhost:8025` → confirma → login → `/auth/me` → logout
- **Fase 2–3:** montar um quadro pela UI; segunda conta convidada abre o mesmo quadro; leitor não
  consegue mover card (403)
- **Fase 4:** arrastar, dar F5, ordem preservada; um `UPDATE` por movimento
- **Fase 5 em diante — o teste que importa:** dois navegadores lado a lado no mesmo quadro. Arrastar
  em um move no outro em menos de um segundo, sem F5. Automatizado com dois `BrowserContext` no
  Playwright (`make test-e2e`)
- **Fase 5 em diante, sempre:** `go test -race ./...` limpo. Concorrência sem `-race` é fé, não
  engenharia
- **Fase 7:** `docker compose stop api`, mexer no quadro pela outra aba, `start api` — as duas telas
  convergem sozinhas

## Como estudar isto

A ordem que funciona: **ler a fonte principal da fase antes de escrever a primeira linha dela**, e
as demais durante. Se for ler só três coisas no projeto inteiro, leia o post da Figma (fase 4), o
`hub.go`+`client.go` do gorilla (fase 5) e o padrão Optimistic Offline Lock (fase 6) — são os três
que mudam como você pensa, o resto é ferramenta.

## Primeiro passo

Fase 0 inteira, num commit `chore:` por peça (compose, backend, frontend, Makefile).
