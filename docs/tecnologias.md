# 🧰 Tecnologias

Guia de estudo do stack: o que cada peça é, **por que está neste projeto** — com
referência a um arquivo real — e onde aprofundar.

---

## Backend

### Go 1.26

A linguagem escolhida pelo eixo do projeto: goroutines e canais são o assunto da
fase 5, e o resto do stack existe para chegar até lá.

O toolchain é fixado em `go.mod` (`go 1.26.5`). Não é decoração: seis das sete
vulnerabilidades que o `govulncheck` acusou antes do primeiro deploy eram da
biblioteca padrão, e a correção foi subir essa linha. Ver
[entrega-continua.md](entrega-continua.md), em "Casos reais deste projeto".

📚 [How to Write Go Code](https://go.dev/doc/code) · [Effective Go](https://go.dev/doc/effective_go)

### Arquitetura hexagonal

`domain` (regra pura, sem I/O) → `usecase` (orquestra, define as portas) →
`adapter` (HTTP, Postgres, disco).

A regra que segura o desenho: **`domain` e `usecase` não conhecem o mundo de
fora**. As interfaces de repositório são declaradas em
[`internal/usecase/board/repositorio.go`](../backend/internal/usecase/board/repositorio.go),
no pacote que as **consome**, e não no que as implementa. É isso que permite os
testes rodarem contra fakes em memória sem subir banco — e é a mesma costura que
vai receber a porta `Publicador` do WebSocket na fase 5, sem que nenhum usecase
saiba que WebSocket existe.

📚 [Ports & Adapters, de Alistair Cockburn](https://alistair.cockburn.us/hexagonal-architecture/)

### chi

Roteador HTTP fino, compatível com `net/http`. Escolhido por não ser framework:
o handler continua sendo `http.HandlerFunc`, e os middlewares são
`func(http.Handler) http.Handler`.

A ordem deles importa e está comentada em
[`config/server.go`](../backend/config/server.go): `RequestID` primeiro (para o
log e os handlers terem o id), `IPReal` antes do log (para registrar o cliente,
não o proxy), e o log de acesso **por fora** do `Recoverer`, para registrar
também os 500 que ele produz.

⚠️ O `Handler` do `http.Server` precisa ser informado explicitamente. Deixá-lo
`nil` faz cair no `http.DefaultServeMux` — que já vem com `/debug/pprof/*`
registrado, porque o `init()` de `net/http/pprof` entra no binário como
dependência transitiva do chi.

📚 [chi](https://github.com/go-chi/chi)

### pgx

Driver e pool do PostgreSQL. Usado direto, sem ORM: o SQL fica visível no
repositório, que é onde ele deve ser lido.

Os limites do pool são decididos pelo projeto em
[`config/database.go`](../backend/config/database.go), e não herdados do padrão
do pgx (`max(4, núcleos)`), que num VPS de 1 vCPU daria 4 conexões.

⚠️ **O SQL é a camada que os testes não alcançam.** Fakes em memória copiam a
struct inteira e passam mesmo quando a coluna não está no `INSERT`. Aconteceu
duas vezes — `cards.prazo` e `boards.fundo` existiam em todas as camadas e não
eram gravados. O guard que restou está em
[`test/repository/sql_cobre_as_colunas_test.go`](../backend/test/repository/sql_cobre_as_colunas_test.go).

📚 [pgx](https://github.com/jackc/pgx)

### log/slog

Log estruturado da biblioteca padrão. Cada requisição sai com `request_id`, que
é o que permite seguir uma chamada pelos logs em produção.

📚 [log/slog](https://pkg.go.dev/log/slog)

### Argon2id

Hash de senha, via [alexedwards/argon2id](https://github.com/alexedwards/argon2id).
Usa ~19 MiB por hash simultâneo — é essa conta que define o `mem_limit: 384m` da
API no compose de produção.

📚 [OWASP Password Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)

---

## Banco

### PostgreSQL 16 + Flyway

Migrations versionadas, aplicadas por um job que roda **antes** da API subir
(`depends_on: service_completed_successfully`), então o schema nunca fica atrás
do código.

Duas regras do projeto que o CI faz cumprir (ver [CLAUDE.md](../CLAUDE.md)):

- **migration não escreve dado** — todo backfill é decisão do domínio; em SQL
  ele vira uma segunda fonte da verdade, sem teste e sem conserto;
- **aperto de schema exige dois deploys** — o Flyway é forward-only, e um
  `SET NOT NULL` no mesmo deploy que estreia a coluna transforma o rollback num
  erro no primeiro `INSERT`.

⚠️ Migration aplicada é **imutável**: o Flyway guarda o checksum de cada `.sql` e
recusa o `validate` se o arquivo mudar. Por isso o nome antigo do projeto
sobrevive num comentário da `V1`.

Em produção os `.sql` viajam **dentro de uma imagem**
([`Dockerfile.migrations`](../backend/Dockerfile.migrations)), para o host não
precisar do código-fonte.

📚 [Flyway — naming](https://documentation.red-gate.com/fd/migrations-271585107.html) · [PostgreSQL — constraints](https://www.postgresql.org/docs/current/ddl-constraints.html)

---

## Frontend

### Svelte 5 + SvelteKit

Runes (`$state`, `$derived`, `$effect`, `$props`) em vez do sistema reativo
antigo. O modo runes é **obrigatório** no projeto, configurado em
[`vite.config.ts`](../frontend/vite.config.ts).

Toda rota que busca dados declara `ssr = false`: as chamadas à API acontecem no
navegador. É isso que permite `PUBLIC_API_URL=/api` — relativo — funcionar, já
que um caminho relativo não teria base num fetch rodando no servidor Node.

📚 [Runes](https://svelte.dev/docs/svelte/what-are-runes) · [SvelteKit](https://svelte.dev/docs/kit/introduction)

### adapter-node

Gera um servidor Node autônomo em `build/`. A imagem de produção
([`Dockerfile.prod`](../frontend/Dockerfile.prod)) copia **só** essa pasta — a
saída é autocontida — e **remove npm, npx, yarn e corepack**, que não são usados
em runtime e trazem as próprias dependências junto. Foi de dentro do npm que
saiu a única CVE CRITICAL que a esteira barrou.

Atrás do proxy ele precisa de `ORIGIN`, `PROTOCOL_HEADER` e `HOST_HEADER` — sem
isso só enxerga o IP do container e recusa requisições por origem inválida.

📚 [adapter-node](https://svelte.dev/docs/kit/adapter-node)

### Tailwind 4 e o design system

O sistema visual é o [Untitled UI](https://www.untitledui.com/), a base que o
[shelf.nu](https://www.shelf.nu/) publica no `tailwind.config.ts` deles: rampa de
cinza azulado de `#FCFCFD` a `#101828`, laranja `#EF6820` como marca, Inter,
raios de 6 e 8 px e sombras de 5% a 10%.

Tudo vive em tokens no topo de
[`src/routes/layout.css`](../frontend/src/routes/layout.css). Os componentes leem
os tokens por `var()` e pelas utilitárias que o `@theme inline` gera
(`bg-canvas`, `text-mute`, `border-hairline`) — trocar um tema é trocar valores,
e nenhum `.svelte` conhece cor.

**Escuro é o padrão; quem usa escolhe.** O seletor no cabeçalho grava a
preferência, e [`static/tema.js`](../frontend/static/tema.js) a aplica no `<html>`
**antes da primeira pintura** — sem ele a página apareceria escura e piscaria
para clara depois da hidratação. É script externo, e não inline, porque a CSP do
projeto é `script-src 'self'`.

⚠️ **Cascata:** no Tailwind 4 as utilitárias vivem em `@layer utilities`, e regra
**sem** layer vence regra **com** layer, independente de especificidade. As
classes de componente do projeto ficam dentro de `@layer components` por causa
disso — soltas, o `width: 100%` do `.botao` ganhava do `w-auto`.

📚 [Tailwind 4](https://tailwindcss.com/docs) · [MDN — @layer](https://developer.mozilla.org/en-US/docs/Web/CSS/@layer)

### svelte-dnd-action

Arrastar e soltar da fase 4. Zonas aninhadas: a lista de cards fica **dentro** do
item da lista de colunas, e é o `stopPropagation` do `mousedown` da zona interna
que faz pegar num card mover só o card.

⚠️ A biblioteca registra o `mousedown` do item **apenas quando `dragDisabled` já
é falso**. Um interruptor ligado no `pointerdown` chega tarde: o listener só
existe no gesto seguinte. Foi assim que uma "alça" de arrastar coluna nunca
funcionou, sem nenhum teste reclamar.

📚 [svelte-dnd-action](https://github.com/isaacHagoel/svelte-dnd-action)

---

## Ambiente e produção

### Docker Compose

Um arquivo para desenvolvimento (hot reload, bind mount, Mailpit) e outro para
produção (imagens prontas, sem porta publicada, contenção). Ver
[producao.md](producao.md).

### coder/websocket

O tempo real da fase 5. Escolhido por ter API sobre `context`, o que faz cada
envio e o desligamento gracioso caírem no mesmo mecanismo do resto do projeto.

O desenho está em [`internal/adapter/realtime/hub`](../backend/internal/adapter/realtime/hub/hub.go)
(salas por quadro, `RWMutex`, fan-out que não bloqueia) e em
[`internal/adapter/http/ws`](../backend/internal/adapter/http/ws/ws.go) (handshake,
ping/pong, uma goroutine de leitura e **uma** de escrita).

⚠️ **WebSocket não obedece CORS.** Sem `OriginPatterns`, qualquer site que a
vítima visitar abre uma conexão autenticada com o cookie dela e lê o quadro em
tempo real — o Cross-Site WebSocket Hijacking. O `SameSite=Lax` é a segunda
camada, não a primeira.

⚠️ **`WriteTimeout` do `http.Server` mata conexão longa.** Ele vale para a
conexão inteira, não por requisição. Os 15s que existiam derrubavam o quadro
sempre no mesmo tempo, sem erro no cliente e sem nada no log. Hoje é zero, e o
que protege está descrito em [`config/server.go`](../backend/config/server.go).

📚 [coder/websocket](https://pkg.go.dev/github.com/coder/websocket) · [RFC 6455](https://datatracker.ietf.org/doc/html/rfc6455) · [OWASP — WebSockets](https://cheatsheetseries.owasp.org/cheatsheets/HTML5_Security_Cheat_Sheet.html#websockets)

### Caddy

Proxy reverso com HTTPS automático. **Não é uma stack nossa**: o stacktrack
entra no Caddy que já servia o agendaGo no mesmo VPS, depositando um bloco de
site em `/home/deploy/caddy/sites`. Ver
[producao.md](producao.md), seção "Arquitetura no ar".

📚 [Caddy — reverse_proxy](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy)

---

## Armadilhas do ambiente

Coisas que já custaram tempo aqui.

### `EACCES` no `node_modules`

O container `web` monta `node_modules` e `.svelte-kit` em volumes próprios — a
instalação dele é Alpine/musl, a sua é glibc, e misturar quebra os binários
nativos do Vite. O Docker cria o ponto de montagem como **root** quando o
diretório ainda não existe, e depois disso todo `npm install` seu falha.

Por isso o `make run` instala no host **antes** do compose. Se já aconteceu:

```bash
docker run --rm -v "$PWD/frontend:/app" alpine \
  chown 1000:1000 /app/node_modules /app/.svelte-kit
```

### O `air` servindo binário velho

Quando a build falha, o `air` **mantém no ar o binário anterior**: a API responde
normalmente e só as rotas novas dão 404 — o que parece erro de código e é erro de
build. É o que acontece logo depois de um `go get`, porque o container ainda não
baixou o módulo:

```bash
go get <módulo>              # no host, atualiza go.mod/go.sum
docker compose restart api   # o container baixa e recompila
```

Na dúvida, `docker compose logs api` mostra a falha.

### Porta ocupada pelo agendaGo

As portas de desenvolvimento são as mesmas dos dois projetos. Rodando ambos, o
segundo `docker compose up` falha com "port is already allocated" — derrube o
outro antes.
