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

É a parte mais sutil, e o que a torna possível é a **`revisao` do quadro**:
ela é incrementada sob o lock da linha de `boards`, na mesma transação da
mudança e do evento. Diferente do `seq` global, a revisão segue a ordem de
commit e não abre buraco quando duas transações terminam em ordem diferente da
chegada.

O cliente guarda somente a revisão que comprovou ter aplicado. Ao reconectar,
manda `?revisao=N`:

```
snapshot N   ── ?revisao=N ──►  servidor repõe N+1, N+2… e então o ao vivo

sem snapshot ── sem cursor ──►  servidor responde `sincronizado`; a tela baixa
                                 um snapshot antes de confirmar qualquer número

intervalo    ── ?revisao=7 ─►  mais de 200 eventos perdidos: em vez de
grande demais                  reproduzir, manda `recarregue.tudo`
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
inofensiva porque um evento em revisão já confirmada só é descartado depois de
o snapshot visível cobrir aquela revisão. `seq` continua como identidade do
evento e cursor legado de auditoria, nunca como cursor novo de reconexão — ver
`onmessage` em
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
**Messages**. É a forma mais rápida de entender a `revisao` na prática — feche
a aba, mexa no quadro noutra, reabra e observe o `?revisao=` no handshake.
`?desde=` aparece somente em clientes legados durante a janela de transição.

📚 [coder/websocket](https://pkg.go.dev/github.com/coder/websocket) · [RFC 6455](https://datatracker.ietf.org/doc/html/rfc6455) — leia as seções 1 (visão geral) e 4 (handshake); o resto é referência
📝 [O exemplo de chat do gorilla/websocket](https://github.com/gorilla/websocket/tree/main/examples/chat) — `hub.go` e `client.go` são a referência canônica do padrão hub em Go, e o desenho daqui vem deles
📝 [Go Concurrency Patterns, de Rob Pike](https://go.dev/talks/2012/concurrency.slide) — o modelo mental de canais que o hub usa
📝 [OWASP — WebSockets](https://cheatsheetseries.owasp.org/cheatsheets/HTML5_Security_Cheat_Sheet.html#websockets) — a seção do Cross-Site WebSocket Hijacking
📝 [MDN — WebSocket API](https://developer.mozilla.org/en-US/docs/Web/API/WebSockets_API) — o lado do navegador, que é o que `conexao.svelte.ts` usa

### Log de eventos (o outbox)

A tabela `board_events` guarda o que aconteceu em cada quadro. Ela tem **dois**
números por evento, e a diferença entre eles é o assunto mais sutil desta
seção.

**A mudança e o evento caem no mesmo commit.** Quem faz isso é
`repository.UnidadeDeTrabalho`: ela abre a transação, trava a linha do quadro,
entrega à operação os repositórios ligados a ela, grava o evento e comita. Ou o
card move e o evento existe, ou nenhum dos dois.

A peça que torna isso barato é a interface `consultante` — o que `*pgxpool.Pool`
e `pgx.Tx` têm em comum. Os repositórios dependem dela, e não do pool concreto,
então o mesmo SQL serve para uma consulta solta e para uma dentro de transação,
sem uma linha duplicada.

A publicação ao vivo fica **fora** da transação, de propósito: anunciar antes do
commit avisaria de uma mudança que o rollback ainda pode desfazer, e quem
recebesse o evento recarregaria o quadro para encontrar o estado anterior.

**Toda mutação não terminal de quadro passa por ali.** Já foi diferente: etiqueta, checklist
e anexo publicavam por um caminho sem transação comum, com o argumento de que o
evento deles é só um aviso de "recarregue o quadro". O argumento estava certo
sobre o sintoma e errado sobre a garantia — com duas transações separadas existe
também o caminho inverso, evento gravado sem a mudança, e um cliente que recebe
"etiqueta aplicada", recarrega e não encontra etiqueta nenhuma não tem como se
recuperar, porque o log afirma que aconteceu.

Apagar o próprio quadro é a exceção terminal: o `DELETE` remove também o seu
outbox por chave estrangeira, então não existe mais agregado no qual persistir
`quadro.apagado`. O commit publica depois um sinal efêmero para as abas abertas
saírem da sala; quem estava offline encontra `404` ao voltar. A entrega ao vivo
continua sendo feita pelo processo e pode ser perdida se ele morrer depois do
commit. O dispatcher durável e ordenado que elimina essa janela permanece na
etapa A3 do `PLANO.md`.

📚 [Transactional outbox (microservices.io)](https://microservices.io/patterns/data/transactional-outbox.html)

### `seq` e `revisao`: por que dois números

`seq` é `BIGSERIAL`, global à tabela. Ele é **identidade e ordem total** do log,
e serve muito bem a isso: identificador aleatório não ordena, e timestamp empata
— dois eventos no mesmo microssegundo ficariam sem sucessor definido.

O que ele **não** pode ser, e vinha sendo usado como se fosse, é **cursor de
reconexão**. Um `BIGSERIAL` registra a ordem de **alocação** do número, não a de
**commit**:

1. a transação A pega o `seq` 42; a transação B pega o 43;
2. B comita primeiro; A ainda está no meio da sua escrita;
3. um cliente reconecta e pergunta "o que houve desde o 41?";
4. ele recebe o 43, aplica, e avança o cursor para 43;
5. A comita um instante depois — e o 42 **nunca mais** será entregue a esse
   cliente, porque está abaixo do cursor dele.

O buraco é silencioso e permanente: nada falha, nenhum erro aparece, e a tela
daquela pessoa simplesmente discorda do banco para sempre.

`boards.revisao` conserta isso porque é incrementada **sob o lock da linha do
quadro**, no mesmo `UPDATE ... RETURNING` que toma o lock. Só uma transação por
vez a incrementa, então a ordem de numeração **é** a ordem de commit, e a
sequência é contígua dentro do quadro. É ela que viaja no snapshot
(`GET /boards/{id}` devolve `revisao`) e é ela que o cliente devolve em
`/ws?board=…&revisao=N`.

Cada evento leva também `indice` e `quantidade`: a posição dentro da revisão e
quantos eventos a formam. Hoje toda mutação produz exatamente um evento, então o
par é sempre `(0, 1)`. Eles existem assim mesmo porque é o que permite ao
cliente saber **quando o grupo está completo** — confirmar uma revisão pela
metade deixaria o cursor à frente do que foi aplicado, e o que faltou cairia no
mesmo buraco de sempre.

O cursor do cliente avança por **um caminho só**: depois de os snapshots
baixados terem sido aplicados (`conexao.confirmarRevisao`). Com o modal aberto,
isso inclui o quadro e a projeção do card; a confirmação usa a menor revisão que
as duas cobrem. Receber um evento nunca avança o cursor sozinho — confirmar é
dizer "apliquei até aqui", e o servidor repõe a partir dali.

⚠️ A armadilha que isso escondeu na primeira implementação: a unidade de
trabalho recebia o evento **por valor** e devolvia só o `seq`. A revisão era
gravada na cópia de dentro da transação, o log ficava perfeitamente correto, e o
evento **entregue ao vivo** saía com revisão zero. Nenhum teste pegou, porque
todos olhavam para o que foi *registrado*; quem pegou foi um smoke test contra a
API de verdade. Hoje `Escrever` devolve o evento carimbado, e
`test/usecase/outbox_test.go` fecha a porta.

### O lock do quadro

`READ COMMITTED` — o isolamento padrão do PostgreSQL — garante que cada comando
enxergue um instantâneo consistente. Ele **não** garante que duas transações
concorrentes cheguem a um resultado que faça sentido juntas.

O exemplo que dói: dois donos, cada um removendo o outro. As duas transações leem
"há dois donos", as duas concluem "posso remover", as duas comitam — e o quadro
fica órfão, sem ninguém que possa convidar ou apagá-lo. Nenhuma constraint pega
isso, porque a regra é sobre o **conjunto** de linhas, não sobre uma delas.

A unidade de trabalho abre toda escrita com um `UPDATE boards SET revisao = …
WHERE id = $1 RETURNING revisao`. Um `UPDATE` toma o lock exclusivo da linha,
exatamente como `SELECT … FOR UPDATE` faria, e já devolve a revisão nova — dois
efeitos, um round-trip.

É um lock **grosso**, de propósito: o quadro é a fronteira de consistência deste
domínio, o perfil desta rodada é de dezenas de conexões (não milhares), e um
lock por agregado é o desenho que se consegue **provar** correto com testes de
interleaving. Quadros diferentes não se esperam.

A ordem de aquisição é sempre board → convite/membro → coluna/card. Como o lock
do quadro é sempre o primeiro e é o único explícito, não há ciclo possível: o
deadlock é impossível por construção, não por sorte. `lock_timeout` de 2s e
`statement_timeout` de 5s entram por `SET LOCAL` na própria transação — `SET
LOCAL`, e não `SET`, porque o pool reaproveita conexões e um `SET` comum vazaria
o teto para as leituras longas de auditoria.

Estourado o prazo, o SQLSTATE `55P03` vira `ucboard.ErrQuadroOcupado` e a borda
responde **503 com `Retry-After`** — não 500. O servidor não falhou; ele
desistiu de esperar, e repetir é a ação certa.

Os testes que sustentam isso estão em `test/repository/concorrencia_test.go`, e
foram conferidos do jeito que importa: **desligando o lock e vendo o teste
falhar**.

📚 [PostgreSQL — Transaction Isolation](https://www.postgresql.org/docs/16/transaction-iso.html)
📚 [PostgreSQL — Explicit Locking](https://www.postgresql.org/docs/16/explicit-locking.html)

### Comandos estreitos (`UPDATE` só do que mudou)

Um `Atualizar(agregado)` que grava todas as colunas é conveniente e perde
trabalho alheio em silêncio: quem renomeia o quadro grava também o `fundo` que
leu há dez segundos, desfazendo a troca de fundo que outra pessoa fez nesse
meio-tempo. Nenhum erro, nenhum conflito — a mudança some.

Por isso `RepositorioBoard` tem `Renomear` e `DefinirFundo`; `RepositorioColuna`
tem `Renomear` e `DefinirChave`; `RepositorioChecklist` tem `EditarItem` e
`MarcarItem`. Duas edições de campos **diferentes** convivem; duas do **mesmo**
campo continuam sendo "a última vence", e aí a última realmente é a última.

O card é a exceção deliberada: ele tem edição integral e por isso mantém
**bloqueio otimista** (`WHERE id = $1 AND version = $2 - 1`), que devolve 409 em
vez de sobrescrever.

### O instantâneo do quadro

Montar o quadro custa dez consultas. Sob `READ COMMITTED`, cada uma enxerga o
banco no instante em que **ela** rodou: uma escrita no meio da sequência aparece
para as seguintes e não para as anteriores, e o snapshot devolvido descreve um
estado que nunca existiu — card na lista de cards e ausente da contagem de
comentários, coluna que sumiu deixando cards órfãos.

Isso passou a importar mais quando o snapshot ganhou uma `revisao`: um estado
incoerente **carimbado como coerente** faz o cliente aplicar os eventos
seguintes por cima dele sem nunca descobrir que partiu errado.

`repository.Instantaneo` abre a leitura em `REPEATABLE READ, READ ONLY`, que
congela um instantâneo no primeiro comando e o mantém até o fim. O estado da
publicação fica **dentro** dele, pois publicar e revogar possuem evento e revisão
próprios. O selo de "quem moveu por último" fica fora: ele é um resumo derivado
do próprio log, não estado que o cliente reconcilia por evento. O detalhe do
card usa a mesma disciplina para autorização, revisão e todos os agregados do
modal; a projeção do link público também lê token e conteúdo no mesmo
instantâneo.

📚 [Transactional outbox (microservices.io)](https://microservices.io/patterns/data/transactional-outbox.html)
📝 [Idempotência na API do Stripe](https://docs.stripe.com/api/idempotent_requests) — a explicação mais clara do conceito em API real
📝 [Exponential backoff and jitter (AWS Builders' Library)](https://aws.amazon.com/builders-library/timeouts-retries-and-backoff-with-jitter/) — por que o jitter importa quando cinquenta clientes reconectam juntos

### O perímetro: o que a API recusa antes de trabalhar

Cinco tetos, e cada um cobre um abuso que os outros não veem.

**Prazo por requisição.** Dez segundos de orçamento total, num `context`
(`middleware.Prazo`), e não num timer que responde por fora. A diferença é que o
contexto cancelado propaga até o pgx, que aborta a query e devolve a conexão ao
pool; um timeout que só escrevesse a resposta deixaria o trabalho rodando atrás
dela, segurando exatamente o recurso que o teto protege. Upload tem orçamento
próprio (dois minutos) — dez megabytes numa conexão móvel ruim passam de dez
segundos sem nada de errado acontecendo. O handshake do **WebSocket passa por
esse prazo** até concluir autenticação, autorização e o `101`; depois do upgrade
o handler destaca o contexto da conexão longa e usa prazos próprios para replay,
escrita, ping e revalidação. Manter o deadline HTTP depois do `101` a mataria
sempre no mesmo tempo, exatamente como o antigo `WriteTimeout` documentado em
`config.NovoServidor`.

**Cookie de sessão desconhecido, por IP.** Conferido **antes** da consulta ao
banco. Validar sessão é um `SELECT` indexado — barato por unidade e caro em
rajada: um laço com cookies aleatórios consome uma conexão do pool por
requisição e compete com o tráfego real sem nunca autenticar nada. O teto por
sessão do roteador não cobre isso, porque ele chaveia pelo **valor do cookie** e
quem manda um cookie novo a cada tentativa ganha um balde novo a cada tentativa.
Aqui a chave é o IP — que o cliente não escolhe, ver abaixo.

**IP confiável.** `middleware.IPReal` só obedece `X-Real-IP` quando o peer
direto da conexão TCP está na lista `PROXIES_CONFIAVEIS`; de qualquer outro
lugar, os cabeçalhos são **apagados** antes de seguir. A versão anterior confiava
sempre, com o raciocínio "só o Caddy fala com a API" — correto enquanto essa
topologia valer, e verificado em lugar nenhum. Escolher o próprio IP é escolher
o próprio balde de rate limit.

**Conexões de tempo real.** Teto por conta (5), teto global do processo (100) e
teto de handshakes por IP (10/min). Os três porque são três abusos: o global
sozinho é um recurso que a primeira conta a chegar consome inteiro; o por conta
não vê quem abre e fecha em laço, que nunca ocupa vaga e mesmo assim faz o
servidor pagar autorização, nome e replay a cada tentativa.

**Disco.** Disco cheio não degrada, ele **quebra**: o upload falha no meio, o
Postgres para de aceitar escrita porque o WAL não tem para onde ir, e nada disso
avisa antes. `middleware.PorteiroDeDisco` recusa **mutação** com 507 e deixa a
**leitura** passar — um quadro que não aceita card novo mas mostra o que já
existe é um sistema degradado; um que não abre é um sistema fora do ar, e a
diferença importa justamente quando alguém precisa ver o que está lá para decidir
o que apagar. `/ready` reflete os dois motivos separadamente: 503 se o banco caiu
(sem ele nem leitura existe), 200 com `escrita: false` se só faltou disco.

E, na entrada: `Content-Type` precisa ser JSON (sem isso, um formulário HTML de
outro site posta na API sem preflight de CORS), campo desconhecido é recusado
com o nome dele, lixo depois do objeto é recusado, e id de URL que não seja UUID
para em 400 antes de virar `invalid input syntax for type uuid` — que hoje
viraria 500 e gastaria uma conexão do pool por tentativa de varredura.

📝 [OWASP — Denial of Service Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Denial_of_Service_Cheat_Sheet.html)
📝 [Timeouts, retries and backoff with jitter (AWS)](https://aws.amazon.com/builders-library/timeouts-retries-and-backoff-with-jitter/)

### Convite: a única porta de entrada

Havia um atalho: convidar alguém que **já tinha conta** criava a participação na
hora, sem token. O raciocínio era que não há o que confirmar quando a pessoa já
provou ser dona daquele email ao se cadastrar.

O que ele produzia era outra coisa: conhecer o email de alguém bastava para pôr
essa pessoa dentro de um quadro, sem que ela concordasse e sem que ficasse
sabendo. A participação virava efeito de **conhecer um endereço**, não de provar
ser dono dele.

Hoje todo acesso nasce de um convite, e aceitar exige três coisas ao mesmo
tempo: sessão válida, token válido e email normalizado da sessão igual ao do
convite. Uma conta com outro email recebe 403 e **não consome** o convite.

A consulta pública do convite devolve o email **mascarado** (`a***@exemplo.com`).
Ela responde a quem tem o link, que não é necessariamente quem foi convidado — o
endereço inteiro ali transformaria um link encaminhado em vazamento de email. A
máscara ainda deixa a pessoa certa reconhecer o próprio endereço, que é a dúvida
real de quem chega nessa tela logado na conta errada. O cliente mascara o próprio
email com a mesma régua (`$lib/email.ts`) para decidir entre "aceitar" e "você
está na conta errada"; a decisão de verdade continua sendo do servidor.

Revogar **marca** (`revogado_em`) em vez de apagar: o `DELETE` levava junto a
resposta para "quem convidou quem, e quando" e, no caminho concorrente, tornava
indistinguível "revoguei agora" de "nunca existiu". Aceitar e revogar são
`UPDATE`s **condicionais** — a transição vai no `WHERE`, e quem perde a corrida
vê `RowsAffected = 0`. Sem isso, duas abas clicando no mesmo link liam
"pendente" as duas e o convite era consumido duas vezes.

O índice de pendência considera `aceito_em IS NULL AND revogado_em IS NULL`. O
**vencimento** não cabe num índice parcial (comparar com `now()` não é imutável,
e o PostgreSQL recusa), então é o domínio que revoga o convite vencido — na
mesma transação, sob o lock — antes de criar o novo.

### Senha: comprimento, e uma lista curta

Quinze caracteres, não oito. Oito caracteres de alfabeto completo caem em GPU
comum hoje, e o NIST subiu a recomendação por isso. O piso vale para senha
**nova**: invalidar as existentes trancaria todo mundo para fora de um sistema
que não tem recuperação por email para oferecer em troca.

O piso sozinho não basta. Quem é obrigado a digitar quinze caracteres escreve
`senhasenhasenha` ou sobe a linha do teclado — exatamente os primeiros palpites
de quem ataca uma base que exige senha longa. Daí a lista em
`internal/domain/usuario/senhas_comuns.txt`, **embutida no binário** com
`go:embed`: um arquivo externo poderia sumir no deploy e a validação passaria a
aceitar tudo em silêncio, que é falha aberta no lugar exatamente errado.

A lista é curta de propósito — o piso de comprimento já elimina quase toda a
cauda dos vazamentos públicos, dominada por senhas de 6 a 10 caracteres. Só uma
regra é estrutural em vez de estar na lista: o caractere repetido, que não dá
para enumerar.

📝 [NIST SP 800-63B — Digital Identity Guidelines](https://pages.nist.gov/800-63-3/sp800-63b.html) — a seção 5.1.1, que é a origem do "comprimento, não composição"

### A faxina, fora do caminho crítico

Quem limpava sessões vencidas era o **próprio login**: um `DELETE` sobre
`sessions` a cada sessão aberta. Barato com a tabela pequena, e deixa de ser
barato exatamente quando há mais gente usando — cada pessoa que entra paga a
limpeza de todo mundo, e a conta chega na rajada de logins de uma manhã de
segunda-feira, que é quando ninguém pode esperar.

`internal/usecase/manutencao` roda de hora em hora, fora de qualquer requisição,
em lotes de mil linhas com teto de lotes por passada. Lote, e não "tudo de uma
vez", porque um `DELETE` de cem mil linhas segura lock e infla o WAL por minutos
com o autovacuum ficando para trás; em lotes, cada transação dura milissegundos e
o trabalho é retomável por construção — a passada seguinte pega o que sobrou.

O `SELECT … LIMIT` no subselect usa `ctid`, o endereço físico da linha: o
planejador vai direto nela, sem passar de novo pelo índice que o subselect já
percorreu.

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

### Ansible — a infraestrutura, por dentro

Esta seção é longa de propósito. Ansible tem pouca sintaxe e muito modelo
mental, e quase todo erro de quem começa vem de imaginar a máquina errada
executando o código.

O **servidor** descrito como código, em `deploy/ansible/`. Não substitui a
esteira: o GitHub Actions continua levando o commit até o ar, e o Ansible cuida
da máquina que o recebe.

Entrou por um motivo específico: o `.env` de produção, com a senha do Postgres,
nascia de um heredoc copiado à mão. Era o único passo que impedia o servidor de
ser remontado sozinho, e o único lugar onde a senha existia — perder o arquivo
era perder o banco.

#### 1. Sem agente: onde o código roda

A primeira pergunta de todo mundo é "então instalo o Ansible no servidor?".
**Não.** O servidor não sabe que o Ansible existe; ele vê uma sessão SSH comum.

```
sua máquina (WSL)                          VPS srv1856874
─────────────────                          ──────────────
$ make infra-apply
   └─ ansible-playbook
        ├─ lê inventário e group_vars
        ├─ decifra o vault      ← o segredo só existe deste lado
        ├─ renderiza env.j2 em memória
        │
        └─ ssh deploy@… ───────────────────► python3 -c "<módulo>"
                                             (o módulo escreve o arquivo,
                                              devolve JSON, e some)
```

O que viaja pelo SSH é um **módulo Python** gerado na hora, executado no
servidor, que devolve um JSON dizendo o que fez. Nada fica instalado lá. O único
pré-requisito no alvo é um `python3` — que qualquer Debian/Ubuntu já tem.

Três consequências que valem entender:

- **A senha do vault nunca sai da sua máquina.** O que chega ao servidor é o
  `.env` já renderizado, não a chave que o decifra.
- **Sua máquina precisa alcançar o servidor.** Não há daemon esperando conexão;
  é você que empurra. Por isso se chama modelo *push*.
- **O que a esteira faz é o mesmo.** O runner do GitHub também abre SSH e manda
  comandos. A diferença é que ela manda shell, e o Ansible manda módulos que
  sabem dizer se mudaram alguma coisa.

#### 2. Quem conecta como quem, e por quê

| Playbook | Conecta como | Precisa de root? | Quando roda |
| --- | --- | --- | --- |
| `preparar-host.yml` | `root` | sim | uma vez por **máquina** |
| `provisionar.yml` | `deploy` | **não** | sempre que quiser |

A separação não é organização, é necessidade. `preparar-host.yml` **cria** o
usuário `deploy` — e um playbook que entra *como* `deploy` não pode criá-lo.

É a mesma razão pela qual a esteira nunca vai provisionar um servidor do zero:
ela faz login com `VPS_USER=deploy`, um usuário que num host novo ainda não
existe. Não é limitação do Ansible; é ovo e galinha.

Do outro lado, `provisionar.yml` não usa `become` em lugar nenhum. Tudo mora em
`/home/deploy` e o acesso ao Docker vem do grupo — **o mesmo privilégio que a
esteira já tinha**. Provisionar não ampliou o poder de ninguém.

#### 3. Declarar, não mandar

A diferença que muda tudo:

```bash
# imperativo — o que o CI fazia
mkdir -p ~/stacktrack/scripts        # e se já existir? e se for um arquivo?
```

```yaml
# declarativo — o que a role faz
- name: diretórios da stack
  ansible.builtin.file:
    path: '{{ pasta_stack }}'
    state: directory
    owner: '{{ usuario_app }}'
    mode: '0755'
```

A task não diz "crie". Diz "isto deve ser um diretório, do `deploy`, com modo
0755". O módulo verifica o estado atual e decide: se já bate, reporta `ok`; se
não, corrige e reporta `changed`.

É daí que vem o teste que vale mais que todos:

```
PLAY RECAP
srv1856874 : ok=14  changed=0  failed=0
```

**`changed=0` na segunda execução** significa que o código descreve o servidor,
e não apenas sabe construí-lo uma vez. Um playbook que sempre reporta `changed`
é um script disfarçado — ele age sem olhar, e você perdeu a capacidade de
perguntar "o servidor ainda é o que eu penso que é?".

#### 4. O caminho de um `make infra-apply`

O que aconteceu de verdade na reconstrução, em ordem:

1. **`pre_tasks`** — `docker compose version` responde? Se não, o playbook para
   ali com a instrução de rodar `make infra-preparar`. Falhar cedo, com a causa.
2. **diretórios** — `~/stacktrack`, `scripts/`, `~/backups`, `~/caddy/sites`.
   O módulo `file` cria os pais recursivamente, como `mkdir -p`.
3. **GHCR** — `slurp` do `~/.docker/config.json` para ver se já há login. As três
   imagens estão públicas, o token no vault está vazio, e a task foi **pulada**.
4. **o `assert` do `initdb`** — lê o `.env` que existir e compara com o vault.
   Num servidor vazio não há `.env`, então ele é pulado pelo
   `when: env_atual.content is defined`. A mesma role serve aos dois cenários.
5. **`.env`** — `template` renderiza `env.j2` com as variáveis e o vault. Nasceu
   `-rw------- deploy deploy`, que é o `mode: '0600'` declarado.
6. **compose, `backup.sh`, bloco do Caddy** — `copy` a partir dos arquivos da
   raiz do repositório. Nada é duplicado dentro da role.
7. **cron** — e aqui está o resultado que prova o desenho:

```
0 1 * * * $HOME/agendago/scripts/backup.sh >> $HOME/backups/backup.log 2>&1
#Ansible: backup do stacktrack
20 3 * * * /home/deploy/stacktrack/scripts/backup.sh >> ...
```

A linha do agendaGo **intacta**. O `ansible.builtin.cron` não reescreve o
arquivo: ele procura o marcador `#Ansible: <name>` e gerencia só aquele bloco.
Compare com o que o CI fazia — `crontab -l | grep -v … | crontab -` —, que
reconstruía o arquivo inteiro e tinha um comentário no próprio workflow
admitindo que um cancelamento no meio truncaria o agendamento.

8. **`docker compose up -d`** — Postgres → Flyway (aplica as migrations e sai) →
   API → web. Três containers `healthy`, e os quatro do agendaGo em `Up 2 weeks`:
   o reload do Caddy não reinicia nada.

#### 5. Quando um módulo custa caro demais

`community.docker` tem `docker_network` e `docker_login`, e os dois exigem o
**SDK do Docker para Python instalado no servidor**. Numa máquina compartilhada,
uma dependência a mais para ganhar duas tasks é mau negócio — então esses dois
saem por `command`, com a idempotência escrita à mão:

```yaml
- name: verifica se a rede compartilhada do Caddy existe
  ansible.builtin.command: docker network inspect {{ rede_borda }}
  register: rede_existente
  changed_when: false      # ler nunca é mudar
  failed_when: false       # "não existe" é resposta, não erro
  check_mode: false        # precisa rodar mesmo em --check

- name: cria a rede compartilhada do Caddy
  ansible.builtin.command: docker network create {{ rede_borda }}
  when: rede_existente.rc != 0
```

Os quatro atributos são o vocabulário que transforma um comando cru em task
honesta. Sem `changed_when: false`, a consulta apareceria como alteração e
destruiria o `changed=0`. Sem `check_mode: false`, ela seria pulada em `--check`
e a task seguinte decidiria com uma variável indefinida.

`docker_compose_v2` é a exceção que ficou: ele chama a CLI do `docker compose`,
que já está no servidor, e não precisa de SDK nenhum.

#### 6. Handlers: agir só quando algo mudou

O Caddy pertence ao agendaGo e atende outro serviço em produção. Recarregá-lo a
cada `infra-apply` seria mexer num processo alheio sem motivo.

```yaml
- name: bloco de roteamento no Caddy do agendaGo
  ansible.builtin.copy:
    src: '{{ raiz_repo }}/deploy/caddy/stacktrack.caddy'
    dest: '{{ pasta_caddy_sites }}/stacktrack.caddy'
  notify: recarrega o Caddy
```

O `notify` **não** executa o handler. Ele o enfileira — e só se a task reportou
`changed`. Os handlers rodam uma vez, no fim do play, mesmo que dez tasks os
tenham notificado. É o `cmp -s` do CI, mas como propriedade da ferramenta em vez
de condicional escrita à mão.

#### 7. Os limites do `--check`

`--check` é o ensaio: nenhuma escrita acontece, e cada módulo reporta o que
*faria*. Com `--diff`, mostra o conteúdo linha a linha — foi assim que o diff do
crontab acima apareceu antes de qualquer alteração real.

Mas ele tem um limite que dá nó na cabeça na primeira vez. Num servidor vazio, a
task que sobe a stack falhava com:

```
"/home/deploy/stacktrack" is not a directory
```

As tasks anteriores apenas **simularam** criar o diretório. O `docker compose`,
que é um programa de verdade, não participa da simulação. Daí a guarda:

```yaml
  when: not ansible_check_mode or stacktrack_compose_no_servidor.stat.exists
```

Depois do primeiro apply o arquivo existe, e o `--check` volta a exercitar a
task — que é o que o critério de `changed=0` precisa medir.

#### 8. O vault

`ansible-vault` cifra um arquivo de variáveis com AES-256 e o deixa versionado
junto do código que o consome. `segredos/producao.yml` começa assim:

```
$ANSIBLE_VAULT;1.1;AES256
33616337343837353130393237663438353462636663313362323538653362646534633536663832
```

A senha que o abre fica em `deploy/ansible/.senha-vault`, no `.gitignore`.

**Onde o arquivo cifrado mora é uma decisão, não arrumação.** Em
`group_vars/producao/` ele seria carregado por precedência, ao montar as
variáveis de qualquer playbook do grupo — e o Ansible tenta decifrá-lo antes de
saber se alguém vai usá-lo, o que fazia até um `--syntax-check` exigir a senha.
Fora de lá, quem o carrega é uma task:

```yaml
    - name: carrega os segredos de produção
      ansible.builtin.include_vars: segredos/producao.yml
      no_log: true
```

`include_vars` roda dentro da play, não antes dela: analisar o playbook deixou
de ser a mesma coisa que abrir os segredos de produção. Pelo mesmo motivo o
`ansible.cfg` não declara `vault_password_file` — ali a senha valeria para todo
comando do diretório, inclusive os do CI, que não a tem. Quem precisa dela passa
`--vault-password-file`, e os alvos `infra-*` do Makefile já o fazem.

O que isso muda: o segredo deixa de ser um artefato que só existe no servidor e
passa a ser reproduzível. O custo é que a senha do vault vira o segredo que não
pode se perder — sem ela o arquivo não abre, e a senha do banco existe apenas
dentro do volume do Postgres, de onde não sai.

**A armadilha específica deste projeto:** `POSTGRES_DB`, `POSTGRES_USER` e
`POSTGRES_PASSWORD` são gravados pelo `initdb` no volume, na primeira subida.
Mudá-los no vault depois **não** muda o papel no banco — só faz o `.env` mentir,
e a API para de conectar no deploy seguinte. É o que o `assert` do passo 4
recusa, com a instrução no texto do erro.

#### Como estudar isto

A ordem que funciona é mexer, não ler:

1. `make infra-check` — leia o `PLAY RECAP`. Todo `ok` é uma coisa que o
   servidor já tem; todo `changed` é uma diferença entre o código e a realidade.
2. Mude `backup_minuto` no `group_vars` e rode `make infra-check`. Veja o diff do
   crontab aparecer, e a linha do agendaGo continuar fora dele.
3. Desfaça, e confirme que volta a `changed=0`.
4. `cd deploy/ansible && ansible-vault view --vault-password-file .senha-vault segredos/producao.yml`
   — veja o segredo que a esteira nunca precisou conhecer.
5. Apague `~/stacktrack/.env` no servidor e rode `make infra-check`. Uma task
   `changed`, as outras `ok` — é a convergência funcionando: o Ansible conserta
   o que falta, sem refazer o que está certo.

📚 [Ansible — playbooks](https://docs.ansible.com/ansible/latest/playbook_guide/index.html) · [roles](https://docs.ansible.com/ansible/latest/playbook_guide/playbooks_reuse_roles.html) · [handlers](https://docs.ansible.com/ansible/latest/playbook_guide/playbooks_handlers.html)
📚 [ansible-vault](https://docs.ansible.com/ansible/latest/vault_guide/index.html) — segredo cifrado versionado junto do código que o consome
📝 [Como os módulos são executados](https://docs.ansible.com/ansible/latest/dev_guide/developing_program_flow_modules.html) — o que realmente viaja pelo SSH, e por que não há agente
📝 [Idempotência](https://docs.ansible.com/ansible/latest/reference_appendices/glossary.html#term-Idempotency) — por que `changed=0` na segunda execução é o teste que importa
📚 [community.docker](https://docs.ansible.com/ansible/latest/collections/community/docker/) — `docker_compose_v2` chama a CLI e dispensa o SDK Python no servidor

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
