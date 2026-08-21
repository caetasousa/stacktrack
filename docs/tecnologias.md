# 🧰 Tecnologias

Guia de estudo do stack: o que cada peça é, **por que está neste projeto** — com
referência a um arquivo real — e onde aprofundar.

---

## Backend

### Go 1.26

A linguagem escolhida pelo eixo do projeto: goroutines e canais sustentam o hub
de tempo real, e o restante do stack existe para tornar essa concorrência útil e
segura no produto.

O toolchain é fixado em `go.mod` (`go 1.26.6`). Não é decoração, e já foi a
correção **duas vezes**: seis das sete vulnerabilidades que o `govulncheck`
acusou antes do primeiro deploy eram da biblioteca padrão, e depois outras oito
de uma vez. Nos dois casos nenhuma linha de código nosso mudou — o CI instala a
versão que esta linha diz (`go-version-file: backend/go.mod`), então ela é o
botão. Ver [entrega-continua.md](entrega-continua.md), em "Casos reais deste
projeto".

📚 [How to Write Go Code](https://go.dev/doc/code) — módulos e layout
📝 [Effective Go](https://go.dev/doc/effective_go) — o guia de estilo com exemplo em cada seção
📝 [Go by Example](https://gobyexample.com/) — cada conceito num programa curto que roda

### Arquitetura hexagonal

`domain` (regra pura, sem I/O) → `usecase` (orquestra, define as portas) →
`adapter` (HTTP, Postgres, disco).

A regra que segura o desenho: **`domain` e `usecase` não conhecem o mundo de
fora**. As interfaces de repositório são declaradas em
[`internal/usecase/board/repositorio.go`](../backend/internal/usecase/board/repositorio.go),
no pacote que as **consome**, e não no que as implementa. É isso que permite os
testes rodarem contra fakes em memória sem subir banco — e é a mesma costura que
recebe a porta `Publicador`: os usecases emitem eventos sem saber que o hub os
entrega por WebSocket.

📚 [Ports & Adapters, de Alistair Cockburn](https://alistair.cockburn.us/hexagonal-architecture/) — o texto original
📝 [Hexagonal architecture in Go, de Matthias Noback](https://matthiasnoback.nl/2017/08/hexagonal-architecture/) — com o passo a passo de como as portas nascem
📝 [Standard Package Layout, de Ben Johnson](https://www.gobeyond.dev/standard-package-layout/) — por que a interface mora no pacote que a consome, com código

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

📚 [chi](https://github.com/go-chi/chi) — o roteador
📝 [Middleware em Go, de Alex Edwards](https://www.alexedwards.net/blog/making-and-using-middleware) — a anatomia de `func(http.Handler) http.Handler`, com exemplos que compilam
📝 [net/http timeouts, de Filippo Valsorda](https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/) — o diagrama que explica por que WriteTimeout mata WebSocket

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

📚 [pgx](https://github.com/jackc/pgx) — driver e pool
📝 [Organising database access, de Alex Edwards](https://www.alexedwards.net/blog/organising-database-access) — as quatro formas de injetar o banco, com o código de cada uma
📝 [Optimistic Offline Lock, de Martin Fowler](https://martinfowler.com/eaaCatalog/optimisticOfflineLock.html) — o padrão do `WHERE version` que este projeto usa

### log/slog

Log estruturado da biblioteca padrão. Cada requisição sai com `request_id`, que
é o que permite seguir uma chamada pelos logs em produção.

📚 [log/slog](https://pkg.go.dev/log/slog) — a referência
📝 [Structured logging with slog, de Jonathan Amsterdam](https://go.dev/blog/slog) — do autor do pacote, com exemplos de handler próprio

### Argon2id

Hash de senha, via [alexedwards/argon2id](https://github.com/alexedwards/argon2id).
Usa ~19 MiB por hash simultâneo — é essa conta que define o `mem_limit: 384m` da
API no compose de produção.

📚 [OWASP Password Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html) — os parâmetros recomendados
📝 [How to Hash and Verify Passwords with Argon2, de Alex Edwards](https://www.alexedwards.net/blog/how-to-hash-and-verify-passwords-with-argon2-in-go) — o código exato que este projeto usa

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

📚 [Flyway — naming de migrations](https://documentation.red-gate.com/fd/migrations-271585107.html)
📝 [Expand/contract, de Danilo Sato (martinfowler.com)](https://martinfowler.com/bliki/ParallelChange.html) — o padrão dos dois deploys, com o exemplo passo a passo
📝 [Zero-downtime Postgres migrations, do time do GoCardless](https://gocardless.com/blog/zero-downtime-postgres-migrations-the-hard-parts/) — quais DDLs travam a tabela e por quanto tempo

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
📝 [Svelte 5 tutorial oficial](https://svelte.dev/tutorial/svelte/welcome-to-svelte) — interativo, cada rune num exercício que roda no navegador

### adapter-node

Gera um servidor Node autônomo em `build/`. A imagem de produção
([`Dockerfile.prod`](../frontend/Dockerfile.prod)) copia **só** essa pasta — a
saída é autocontida — e **remove npm, npx, yarn e corepack**, que não são usados
em runtime e trazem as próprias dependências junto. Foi de dentro do npm que
saiu a única CVE CRITICAL que a esteira barrou.

Atrás do proxy ele precisa de `ORIGIN`, `PROTOCOL_HEADER` e `HOST_HEADER` — sem
isso só enxerga o IP do container e recusa requisições por origem inválida.

📚 [adapter-node](https://svelte.dev/docs/kit/adapter-node) — as variáveis de ambiente atrás de proxy
📝 [Deploying SvelteKit with Docker](https://kit.svelte.dev/docs/adapter-node#deploying) — o Dockerfile mínimo, que é a base do nosso

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
📝 [A Complete Guide to CSS Cascade Layers (CSS-Tricks)](https://css-tricks.com/css-cascade-layers/) — com os exemplos que mostram regra sem layer vencendo regra com layer

### svelte-dnd-action

Arrastar e soltar da fase 4. Zonas aninhadas: a lista de cards fica **dentro** do
item da lista de colunas, e é o `stopPropagation` do `mousedown` da zona interna
que faz pegar num card mover só o card.

⚠️ A biblioteca registra o `mousedown` do item **apenas quando `dragDisabled` já
é falso**. Um interruptor ligado no `pointerdown` chega tarde: o listener só
existe no gesto seguinte. Foi assim que uma "alça" de arrastar coluna nunca
funcionou, sem nenhum teste reclamar.

⚠️ **`delayTouchStart` não é ajuste fino, é o que torna a biblioteca usável no
celular.** No padrão (`false`) o arraste começa no primeiro contato do dedo, e
aí não existe gesto de rolagem: arrastar para ver o resto da coluna levanta um
card, e o gesto lateral que percorre o quadro reordena as colunas. Com uma
espera, o toque curto continua sendo toque e o arraste vira toque-e-segure — o
gesto que todo aplicativo de celular usa. Ver `ESPERA_DE_TOQUE_MS` em
[`src/lib/arrastar.ts`](../frontend/src/lib/arrastar.ts).

⚠️ A biblioteca **se recusa a iniciar arraste quando o toque começa num elemento
com `value`** — e `<button>` tem, mesmo vazio. Vale saber nas duas direções: é
por isso que os botões do cabeçalho da coluna não disparam arraste sem querer, e
é a razão de um teste que mira o título da coluna nunca chegar perto do arraste.

📚 [svelte-dnd-action](https://github.com/isaacHagoel/svelte-dnd-action) — a API
📝 [Exemplos ao vivo da própria biblioteca](https://svelte-dnd-action-examples.vercel.app/) — inclusive as zonas aninhadas, que é o caso deste projeto

---

## Ambiente e produção

### Docker Compose

Um arquivo para desenvolvimento (hot reload, bind mount e Mailpit reservado
para um canal de email futuro) e outro para produção (imagens prontas, sem
porta publicada, contenção). Ver
[producao.md](producao.md).

### coder/websocket — o tempo real, por dentro

Esta seção é longa de propósito: é a peça com mais decisões por linha do
projeto, e quase todas existem por causa de um jeito específico de dar errado.

#### Por que WebSocket, e não recarregar de tempos em tempos

Um quadro Kanban colaborativo tem uma exigência incômoda: a mudança precisa
aparecer **na tela da outra pessoa**, e não na próxima vez que ela recarregar.

As três saídas possíveis:

| Como                                                        | O que custa                                                                                                                                                                |
| ----------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Polling** — o cliente pergunta "mudou?" a cada N segundos | N segundos de atraso, e uma requisição por cliente por N segundos mesmo quando nada acontece. Dez pessoas num quadro parado = 10 consultas a cada N segundos, para sempre. |
| **SSE** (Server-Sent Events)                                | Resolve o servidor→cliente com HTTP comum. Mas é **mão única**: o "estou editando esta coluna" precisaria de outro canal.                                                  |
| **WebSocket**                                               | Mão dupla sobre uma conexão só, mantida aberta. O custo é que ela é _sua_ para cuidar: nada expira sozinho, e conexão morta não avisa.                                     |

Escolhemos WebSocket porque o produto já pede o caminho de volta (quem está
editando o quê) e porque o resto desta seção — a parte cara — teria de existir
quase igual com SSE.

O `coder/websocket` entra por ter API sobre `context`: cada envio e o
desligamento gracioso caem no mesmo mecanismo do resto do projeto.

#### O handshake é um GET que troca de protocolo no meio

É o detalhe que resolve a autenticação de graça, e vale entender bem.

```
CLIENTE                                          SERVIDOR
   |                                                |
   |  GET /ws?board=abc  HTTP/1.1                   |
   |  Upgrade: websocket                            |
   |  Cookie: __Host-stacktrack_session=...   ──────>   ← é HTTP: o cookie vai
   |                                                |
   |                    <────  HTTP/101 Switching Protocols
   |                                                |
   |  ===== daqui pra frente não é mais HTTP =====  |
   |  <────────────  frames JSON  ────────────────> |
```

O handshake é uma requisição HTTP normal — por isso o cookie de sessão viaja
junto, e por isso o middleware de autenticação do resto da API vale aqui sem
nenhuma adaptação. **Não existe token na query string**, e isso não é economia:
query string vai para log de acesso, para histórico do navegador e para o
cabeçalho `Referer`.

Depois do `101`, acabou o HTTP. Não há mais requisição, não há mais cabeçalho,
não há mais cookie — só frames. Guarde isso: metade das decisões abaixo existe
porque **a conexão sobrevive à requisição que a criou**.

#### Uma conexão, duas goroutines, e por que exatamente duas

Ver [`ws.go`](../backend/internal/adapter/http/ws/ws.go), em `Acompanhar`:

```
              ┌─────────── uma conexão ───────────┐
              │                                   │
   hub ──► canal do assinante ──► escreverAteMorrer ──► frames ──► cliente
                                                   │
                       lerAteMorrer ◄── frames ◄───┘
```

**Uma de escrita, e só uma.** A biblioteca **não** serializa escritas
concorrentes: dois `Write` ao mesmo tempo intercalam bytes e corrompem o frame.
O sintoma é cruel — funciona nos testes, e a conexão morre com erro de protocolo
sob carga. Por isso todo envio sai de um lugar só, e a única fonte é o canal do
assinante.

**Uma de leitura, mesmo que o cliente quase não fale.** Ela existiria mesmo se o
cliente fosse mudo, por dois motivos que não são óbvios:

1. **os pongs chegam por ali.** A biblioteca entrega a resposta do ping pelo
   mesmo caminho das mensagens; sem alguém lendo, o pong nunca é processado e a
   conexão morre sozinha em 30 segundos;
2. **é o que detecta o cliente fechando.** Sem leitura, a queda do outro lado só
   apareceria no próximo envio — que pode nunca vir, num quadro parado.

As duas morrem juntas: um `context.WithCancel` por conexão, e quem terminar
primeiro cancela o outro.

#### O hub: salas por quadro, e o fan-out que não pode bloquear

O [`hub`](../backend/internal/adapter/realtime/hub/hub.go) é um mapa de salas —
uma por quadro — e cada sala é um conjunto de assinantes. Quem não participa do
quadro nunca é registrado, então **nunca recebe nada**: o isolamento é
estrutural, não um filtro que alguém pode esquecer.

A decisão central está em `Publicar`:

```go
select {
case a.Eventos <- e:   // coube na fila: entregue
default:               // fila cheia: este cliente é derrubado
    lentos = append(lentos, a)
}
```

O `default` é o que separa este desenho de um que trava. **Quem publica não é um
processo de fundo: é a requisição HTTP de outra pessoa.** Se o envio esperasse
por um cliente lento, mover um card ficaria pendurado até um notebook do outro
lado do mundo resolver ler. Com fila cheia, a conexão lenta é fechada e a
requisição segue.

A fila é de **32** eventos: nem zero (que derrubaria qualquer um numa rajada
legítima, como arrastar rápido) nem grande (que guardaria memória por conexão
morta e ainda entregaria um passado longo quando ela voltasse).

Duas sutilezas que só aparecem quando dá errado:

- **derrubar alguém muda quem está no quadro**, então a sala é avisada logo em
  seguida. Sem isso o avatar de quem caiu por lentidão ficava preso na tela dos
  outros — o `defer Cancelar` do handler roda depois e encontra o assinante já
  fora da sala, então não anuncia nada, e ninguém mais o faria;
- **o anúncio sai FORA do lock.** `anunciarPresenca` chama `Publicar`, e fazer
  isso com o mutex na mão travaria o processo em si mesmo.

#### Reconexão sem buraco e sem duplicata

É a parte mais sutil, e o que a torna possível é o **`seq`**: um `BIGSERIAL` do
banco, atribuído no log de eventos. Ordem total — nada de UUID (que não ordena)
nem de timestamp (que empata).

O cliente guarda o maior `seq` que aplicou. Ao reconectar, manda `?desde=N`:

```
1ª conexão   ── sem ?desde ──►  servidor responde `sincronizado` com o seq atual
                                (nada de história: só "você está aqui")

reconexão    ── ?desde=41 ──►  servidor entrega 42, 43, 44… e então o ao vivo

intervalo    ── ?desde=7  ──►  mais de 200 eventos perdidos: em vez de
grande demais                   reproduzir, manda `recarregue.tudo`
```

**A ordem importa mais do que parece.** O assinante entra na sala **antes** de o
histórico ser reposto. Parece errado — vai receber evento ao vivo no meio da
reposição — e é exatamente o ponto:

```
  assina  ────────────────────────────────────►  eventos ao vivo vão para a fila
             repõe 42,43,44 ──►
                                ▲
                    aqui pode chegar o 45 ao vivo; ele espera na fila
```

Se a assinatura viesse **depois** da reposição, um evento que acontecesse no
meio cairia no vão entre os dois e **se perderia para sempre** — e ninguém
perceberia, porque o cliente não tem como saber o que não recebeu.

A escolha é deliberada: **preferir repetir a arriscar buraco**. E a repetição é
inofensiva porque o cliente descarta evento com `seq` menor ou igual ao último
aplicado — ver `onmessage` em
[`conexao.svelte.ts`](../frontend/src/lib/realtime/conexao.svelte.ts).

#### Conexão morta não avisa

Um notebook que fecha a tampa deixa o socket aberto do lado do servidor **para
sempre**. Nada no TCP avisa. O hub continuaria entregando eventos para ninguém.

O ping de 30s transforma isso em erro: sem pong em 10s, a conexão cai. É a única
forma de descobrir — não existe "está vivo?" no protocolo além dela.

#### Autorizar no handshake não basta

O handshake pergunta uma vez; a conexão dura horas. Nesse meio-tempo **duas
coisas mudam sem que a conexão saiba**:

- a pessoa faz **logout** — a sessão morre no banco, e o socket continua
  transmitindo o quadro;
- o dono a **remove do quadro** — e ela segue vendo tudo até fechar a aba.

Por isso um segundo `ticker`, de 30s, reconfere as duas. É separado do ping de
propósito: detectar cliente morto e reconferir permissão são perguntas
diferentes, e uma não deve reger a cadência da outra.

#### O caminho de volta: o cliente fala

Até a funcionalidade de "quem está editando", o canal era mão única. Hoje o
cliente manda **uma** coisa:

```json
{ "tipo": "foco", "colunaId": "..." }
```

Sendo a primeira entrada vinda de fora por esse canal, ela veio com o que
entrada de fora exige: formato fechado (um `tipo`, e não um mapa aberto), teto
de **512 bytes** por mensagem no lugar dos 32 KB padrão da biblioteca, e limite
no tamanho do id — que é **redistribuído para toda a sala**, então um valor de
dez mil caracteres seria multiplicado por cada pessoa conectada.

Mensagem malformada é **ignorada**, não derruba a conexão: o canal carrega o
quadro ao vivo, e perder isso por um JSON torto seria trocar o essencial pelo
acessório.

#### As três armadilhas que já morderam este projeto

⚠️ **WebSocket não obedece CORS.** Nenhum navegador aplica a política de mesma
origem a um handshake de WebSocket. Sem `OriginPatterns`, qualquer site que a
vítima visitar abre uma conexão **autenticada com o cookie dela** e lê o quadro
inteiro em tempo real — é o _Cross-Site WebSocket Hijacking_. O `SameSite=Lax` é
a segunda camada, não a primeira.

⚠️ **`WriteTimeout` do `http.Server` mata conexão longa.** Ele vale para a
conexão **inteira**, não por requisição — e uma conexão de tempo real é uma
requisição só que dura horas. Os 15s que existiam derrubavam o quadro sempre no
mesmo tempo, sem erro no cliente e sem nada no log. Hoje é zero, e o que protege
está em [`config/server.go`](../backend/config/server.go).

⚠️ **Escrever de dois lugares corrompe o frame.** Já explicado acima; está aqui
de novo porque é o erro mais fácil de cometer ao acrescentar um envio novo.

#### Como estudar isso na prática

```bash
make run                        # noutro terminal
cd backend && make test-tempo-real    # duas conexões de verdade, sem navegador
cd backend && go test -race ./test/realtime/   # o hub sob concorrência
```

Os testes do hub rodam com `-race` de propósito: a peça existe para ser usada
por muitas goroutines ao mesmo tempo, e um teste sequencial provaria pouco sobre
ela. O de duas abas fica atrás da build tag `tempo_real` porque exige a API no
ar — ele exercita o handshake de verdade, com cookie real e checagem de origem.

Para ver os frames: DevTools → Network → filtro **WS** → a conexão → aba
**Messages**. É a forma mais rápida de entender o `seq` na prática — feche a aba,
mexa no quadro noutra, reabra e observe o `?desde=` no handshake.

📚 [coder/websocket](https://pkg.go.dev/github.com/coder/websocket) · [RFC 6455](https://datatracker.ietf.org/doc/html/rfc6455) — leia as seções 1 (visão geral) e 4 (handshake); o resto é referência
📝 [O exemplo de chat do gorilla/websocket](https://github.com/gorilla/websocket/tree/main/examples/chat) — `hub.go` e `client.go` são a referência canônica do padrão hub em Go, e o desenho daqui vem deles
📝 [Go Concurrency Patterns, de Rob Pike](https://go.dev/talks/2012/concurrency.slide) — o modelo mental de canais que o hub usa
📝 [OWASP — WebSockets](https://cheatsheetseries.owasp.org/cheatsheets/HTML5_Security_Cheat_Sheet.html#websockets) — a seção do Cross-Site WebSocket Hijacking
📝 [MDN — WebSocket API](https://developer.mozilla.org/en-US/docs/Web/API/WebSockets_API) — o lado do navegador, que é o que `conexao.svelte.ts` usa

### Log de eventos (o outbox)

A tabela `board_events` guarda o que aconteceu em cada quadro, com um `seq`
crescente. É o que permite a quem reconecta perguntar "o que houve desde o 41?"
em vez de fingir que nada aconteceu.

`BIGSERIAL`, e não UUID: o cliente precisa comparar "já apliquei até aqui", e
isso exige **ordem total**. Identificador aleatório não ordena, e timestamp
empata — dois eventos no mesmo microssegundo ficariam sem sucessor definido.

**A mudança e o evento caem no mesmo commit.** Quem faz isso é
`repository.UnidadeDeTrabalho`: ela abre a transação, entrega à operação os
repositórios ligados a ela, grava o evento e comita. Ou o card move e o evento
existe, ou nenhum dos dois.

A peça que torna isso barato é a interface `consultante` — o que `*pgxpool.Pool`
e `pgx.Tx` têm em comum. Os repositórios dependem dela, e não do pool concreto,
então o mesmo SQL serve para uma consulta solta e para uma dentro de transação,
sem uma linha duplicada.

A publicação ao vivo fica **fora** da transação, de propósito: anunciar antes do
commit avisaria de uma mudança que o rollback ainda pode desfazer, e quem
recebesse o evento recarregaria o quadro para encontrar o estado anterior.

⚠️ **Nem todo evento passa por ali, e isso é decisão.** Etiqueta, checklist e
anexo publicam pelo caminho simples, sem transação comum: o evento deles é um
aviso de "recarregue o quadro", e perder um não deixa buraco perceptível —
qualquer evento seguinte, ou a própria reconexão, manda a tela buscar tudo de
novo. O que exige atomicidade são as mudanças estruturais, onde o buraco é
invisível para quem reconecta.

📚 [Transactional outbox (microservices.io)](https://microservices.io/patterns/data/transactional-outbox.html)
📝 [Idempotência na API do Stripe](https://docs.stripe.com/api/idempotent_requests) — a explicação mais clara do conceito em API real
📝 [Exponential backoff and jitter (AWS Builders' Library)](https://aws.amazon.com/builders-library/timeouts-retries-and-backoff-with-jitter/) — por que o jitter importa quando cinquenta clientes reconectam juntos

### Confirmação de ações destrutivas

Não há `confirm()` no projeto. Toda pergunta do tipo "tem certeza?" passa por
[`src/lib/confirmar.svelte.ts`](../frontend/src/lib/confirmar.svelte.ts), que
devolve uma **promessa** e alimenta um diálogo único montado no layout raiz.

O `confirm()` do navegador funcionava; saiu por três razões. Ele **bloqueia a
thread** — nada na tela se atualiza enquanto a caixa está aberta, num quadro que
existe para se atualizar sozinho. A aparência é do sistema operacional, e o
texto não pode distinguir o que apaga uma coluna inteira do que apaga um item.
E no celular ele aparece colado ao topo do navegador, longe do polegar.

A forma de promessa é o que manteve os pontos de chamada com uma linha só —
`if (!(await confirmar({ ... }))) return;`. Hospedar um diálogo por tela
quebraria cada ação em duas metades e espalharia estado de interface por todo
componente que apaga alguma coisa.

Duas decisões dentro do diálogo:

- **o foco vai para Cancelar**, não para a ação. Um Enter perdido, ou o segundo
  toque de quem apertou duas vezes, cancela em vez de apagar;
- **o vermelho do botão não é o `--negativo` da paleta.** Com texto branco por
  cima ele dá 3,76:1 e reprova em AA — o mesmo problema que o laranja da marca
  já tinha. `.botao-perigo` usa um tom mais escuro, 6,57:1.

📚 [WAI-ARIA — alertdialog](https://www.w3.org/WAI/ARIA/apg/patterns/alertdialog/) — o papel que o diálogo usa, e por quê

### Paginar por cursor, e não por deslocamento

A auditoria do quadro pagina com `antesDe={seq}` — o seq da última linha
recebida —, e não com `OFFSET`/número de página.

O motivo não é desempenho. O log de eventos recebe escrita o tempo todo,
inclusive **enquanto alguém audita**: com deslocamento, um evento novo entre a
primeira página e a segunda empurra uma linha para fora da janela, e ela nunca é
lida. Numa auditoria, uma linha pulada em silêncio é o pior defeito possível —
ela some justamente da tela feita para não deixar nada passar.

O `seq` é `BIGSERIAL`, ordem total, então o cursor não escorrega. `WHERE seq < $n
ORDER BY seq DESC` devolve sempre o mesmo intervalo, quantas escritas caiam no
meio. O teste `TestAuditoriaPaginaPorCursorSemPularLinha` grava um evento novo
entre as duas páginas de propósito.

O par disso é o `temMais`: o servidor pede **uma linha a mais** do que devolve, e
é assim que ele responde "há próxima página?" sem uma segunda consulta e sem
contar a tabela. Sem ele, a tela ofereceria um "carregar mais" que devolve lista
vazia — um botão mentindo.

📚 [Use the Index, Luke — paginação por keyset](https://use-the-index-luke.com/no-offset) — por que OFFSET é errado antes de ser lento

### A rota pública: uma projeção, não uma permissão

O link de acompanhamento é a única rota da aplicação que serve conteúdo de
quadro **sem sessão**. Duas decisões de arquitetura seguram isso, e as duas são
estruturais: elas não dependem de ninguém lembrar da regra depois.

**Um usecase separado, com dependências de menos.** O caminho público é
[`PublicacaoUseCase`](../backend/internal/usecase/board/publicacao.go), e não um
método a mais no `QuadroUseCase`. Ele não recebe repositório de comentário, de
anexo, de responsável nem de evento — então uma alteração futura não consegue,
por descuido, pendurar ali uma escrita ou um dado de pessoa. A garantia é a
lista de dependências do construtor, não a disciplina de quem mexer.

**Uma projeção própria, sem ids.** `QuadroPublico` é um tipo à parte, e não o
`Detalhado` com campos zerados na saída. Reaproveitar o tipo da tela autenticada
deixaria cada campo novo dele — e campo novo é sempre pensado para a tela
autenticada — escapar por esta rota até alguém reparar.

Os testes seguem o mesmo raciocínio invertido: em vez de conferirem campo a
campo o que **devia** sair, eles procuram no JSON serializado o que **não devia**
— nome, email, id de usuário, texto de comentário, ids de quadro, coluna e card.
Um campo acrescentado amanhã cai nessa rede sem ninguém precisar atualizar o
teste. Ver `TestOQuadroPublicoNaoVazaPessoas` e `TestOCorpoPublicoNaoLevaPessoaNemID`.

Três detalhes de borda que não são decoração:

- **`Cache-Control: no-store`** na resposta. Revogar precisa valer na hora, e uma
  cópia guardada por um intermediário continuaria sendo servida depois de o dono
  desligar o link — sem ele ter como saber.
- **`meta name="referrer" content="no-referrer"`** na página. O token está na
  URL; sem isso, clicar num link escrito na descrição de um card mandaria o
  endereço inteiro no `Referer` para o site de destino.
- **`credentials: 'omit'`** na chamada, a única do projeto. A rota não olha
  sessão, então mandar o cookie de quem por acaso está logado só exporia a
  credencial numa requisição que não precisa dela.

📚 [OWASP — Insecure Direct Object Reference](https://cheatsheetseries.owasp.org/cheatsheets/Insecure_Direct_Object_Reference_Prevention_Cheat_Sheet.html) — por que o id do quadro não vira endereço público
📝 [MDN — Referrer-Policy](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Referrer-Policy) — como um segredo na URL vaza para terceiros

### O toque não é um clique pequeno

Três defeitos que só existiam no celular vieram do mesmo lugar: o navegador
SINTETIZA eventos de mouse a partir do toque, e a síntese não é fiel.

- o `click` sintetizado chega **sem `clientX`/`clientY`**. Qualquer conta feita
  com eles dá `NaN`, e comparação com `NaN` é sempre falsa — um guarda de
  "isto foi arraste?" que medisse pelo clique reprovaria todo toque;
- um toque gera **dois** cliques, o sintetizado e o de compatibilidade. Se o
  primeiro abre algo que cobre a tela, o segundo cai no que abriu;
- "fechar ao clicar fora" precisa exigir que o **aperto** tenha começado fora, e
  não só que o clique termine fora. Vale além do celular: sem isso, selecionar
  texto dentro de um modal e soltar o mouse fora fecha o modal.

Ver [`src/lib/arrastar.ts`](../frontend/src/lib/arrastar.ts) e o fundo do
[`ModalDoCard.svelte`](../frontend/src/lib/components/ModalDoCard.svelte).

📚 [MDN — eventos de toque e compatibilidade com mouse](https://developer.mozilla.org/en-US/docs/Web/API/Touch_events/Supporting_both_TouchEvent_and_MouseEvent)
📝 [Pointer events (MDN)](https://developer.mozilla.org/en-US/docs/Web/API/Pointer_events) — a API que unifica os dois, e a saída para não depender da síntese

### Chave de ordenação textual

A ordem de cards e colunas é uma **chave de texto**, não um número. A posição
fracionária em `double precision` funcionava até a mantissa acabar — o
esgotamento foi medido em **52 inserções** seguidas no mesmo ponto —, e a partir
daí o movimento respondia 409 com um erro sem saída pela interface.

Com chave textual o limite fica muito mais longe: entre `"b"` e `"c"` cabe
`"bn"`, e a chave só cresce quando o espaço no comprimento atual acaba. O teto
passa a ser o da coluna (`VARCHAR(200)`), o que dá perto de **750** reordenações
seguidas no mesmo ponto — e o domínio conhece esse número, para o estouro sair
como 409 e não como erro de driver. Ver
[`internal/domain/ordem/chave.go`](../backend/internal/domain/ordem/chave.go).
A posição em float continua viva para **etiqueta e checklist**, que só
acrescentam no fim: sem inserção no meio, não há divisão repetida do mesmo
intervalo e o limite não é alcançável.

A chave nova é **sorteada dentro da folga**, não fixada no meio exato. Duas
pessoas soltando um card no mesmo ponto ao mesmo tempo calculariam a mesma
chave determinística, e depois disso não caberia mais nada entre elas — o
sorteio troca um empate garantido por um improvável.

⚠️ **`COLLATE "C"` não é detalhe.** A ordenação de texto no Postgres depende da
collation do banco, e uma que ignore caixa ou trate acentos ordenaria diferente
do que o domínio assume ao gerar a chave. `"C"` é a ordem de BYTES — a mesma que
`ChaveEntre` usa para decidir o que vem antes. E o índice precisa da mesma
collation da consulta: com collations diferentes o Postgres nem o usa, e a
ordenação vira sort em memória a cada leitura do quadro.

📚 [Implementing Fractional Indexing](https://observablehq.com/@dgreensp/implementing-fractional-indexing) — o algoritmo, e o termo para pesquisar depois é _LexoRank_
📚 [PostgreSQL — collation support](https://www.postgresql.org/docs/current/collation.html)

### Testcontainers

Sobe um PostgreSQL de verdade para os testes de repositório e o derruba no fim.
É a camada que os fakes não alcançam — eles copiam a struct inteira, então um
campo que o SQL não grava passa por eles sem reclamar.

⚠️ A espera é por **duas** ocorrências de "ready to accept connections": o
entrypoint do Postgres sobe o servidor uma primeira vez para rodar os scripts de
inicialização e o derruba. Esperar a primeira dá um banco que fecha na cara do
teste.

📚 [Testcontainers for Go](https://golang.testcontainers.org/)
📝 [Módulo de Postgres, com exemplo completo](https://golang.testcontainers.org/modules/postgres/) — inclusive as estratégias de espera

### Caddy

Proxy reverso com HTTPS automático. **Não é uma stack nossa**: o stacktrack
entra no Caddy que já servia o agendaGo no mesmo VPS, depositando um bloco de
site em `/home/deploy/caddy/sites`. Ver
[producao.md](producao.md), seção "Arquitetura no ar".

📚 [Caddy — reverse_proxy](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy)
📝 [Caddy — tutorial de proxy reverso](https://caddyserver.com/docs/quick-starts/reverse-proxy) — do zero ao HTTPS automático em cinco minutos

---

### Ansible

O **servidor** descrito como código, em `deploy/ansible/`. Não substitui a
esteira: o GitHub Actions continua levando o commit até o ar, e o Ansible cuida
da máquina que o recebe — diretórios, `.env`, cron do backup, bloco do Caddy e a
rede `borda`.

Entrou por um motivo específico: o `.env` de produção, com a senha do Postgres,
nascia de um heredoc copiado à mão. Era o único passo que impedia o servidor de
ser remontado sozinho, e o único lugar onde a senha existia — perder o arquivo
era perder o banco. Hoje ele sai de um template mais um vault cifrado
(`roles/stacktrack/templates/env.j2`), e ninguém digita a senha.

O ganho de fundo é a **idempotência declarada** em vez de reimplementada. O job
`implantar` do CI já fazia gerenciamento de configuração — só que em shell, com
`cmp -s` para o Caddy, `docker network inspect ||` para a rede e
`crontab -l | grep -v | crontab -` para o cron, este último com um comentário no
próprio workflow admitindo que um cancelamento no meio truncaria o agendamento.
O `ansible.builtin.cron` faz a mesma coisa gerenciando um bloco marcado por
`name:`, e a linha do agendaGo sobrevive por desenho, não por sorte.

Duas escolhas que valem registro. `community.docker.docker_compose_v2` chama a
CLI do Docker e por isso **não** exige o SDK Python no servidor; já
`docker_network` e `docker_login` exigem, então esses dois saem por
`ansible.builtin.command` com guarda — uma dependência a menos numa máquina
compartilhada vale mais que duas tasks mais bonitas. E a coleção tem teto de
versão (`<5.0.0`) porque o Ubuntu 24.04 empacota o ansible-core 2.16, e a série
5 exige 2.17.

Detalhes de uso, os segredos e o procedimento de recomeço estão em
[infraestrutura.md](infraestrutura.md).

📚 [Ansible — playbooks](https://docs.ansible.com/ansible/latest/playbook_guide/index.html) · [roles](https://docs.ansible.com/ansible/latest/playbook_guide/playbooks_reuse_roles.html) · [ansible-vault](https://docs.ansible.com/ansible/latest/vault_guide/index.html)
📝 [Idempotência](https://docs.ansible.com/ansible/latest/reference_appendices/glossary.html#term-Idempotency) — por que `changed=0` na segunda execução é o teste que importa

---

### Playwright

Ponta a ponta em navegador de verdade, sobre a stack do `docker compose`. A
configuração NÃO sobe a stack (`webServer`): são cinco serviços, contando o job
de migration e o Mailpit reservado, com
dependências na ordem certa, e o `make run` já os orquestra — duplicar isso
criaria uma segunda forma de subir o projeto, que divergiria da primeira.

📚 [Playwright](https://playwright.dev/docs/intro) · [browser contexts](https://playwright.dev/docs/browser-contexts) · [routeWebSocket](https://playwright.dev/docs/mock#mock-websockets)
📝 [Best practices do Playwright](https://playwright.dev/docs/best-practices) — por que seletor por papel e rótulo, e não por classe CSS

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
