# stacktrack

Quadro Kanban colaborativo em tempo real: várias pessoas movem cards no mesmo
quadro e todas veem na hora, sem recarregar a página.

Projeto de estudo. O eixo é **concorrência em Go e sincronização de estado entre
clientes** — o roteiro completo, fase a fase e com as fontes de estudo de cada
uma, está em [PLANO.md](PLANO.md).

**Fase atual: 4 — arrastar e soltar.** Dá para criar conta, montar um
quadro e dividi-lo com outras pessoas, com papéis de dono, editor e leitor.
Cards e colunas se arrastam. Ainda **sem tempo real**: cada pessoa vê só o que
ela mesma faz, e a tela se atualiza porque pergunta de novo à API. É esse
contraste que a fase 5 resolve.

> **Sem envio de email.** Confirmação de cadastro e recuperação de senha ficaram
> de fora por isso, e o convite virou **link**: quem já tem conta com o email
> convidado entra na hora; quem não tem recebe um link que o dono copia e envia
> por onde quiser. Quando o envio existir, ele só passa a mandar o mesmo link.

## Stack

| Camada | Tecnologia |
|---|---|
| Backend | Go 1.26 · [chi](https://github.com/go-chi/chi) · [pgx](https://github.com/jackc/pgx) · [log/slog](https://pkg.go.dev/log/slog) |
| Banco | PostgreSQL 16 · migrations com [Flyway](https://flywaydb.org/) |
| Frontend | [Svelte 5](https://svelte.dev) + [SvelteKit](https://svelte.dev/docs/kit) · TypeScript · [Tailwind 4](https://tailwindcss.com/) · [Vite](https://vite.dev/) |
| Testes | `go test` · [Vitest](https://vitest.dev/) |
| Ambiente | Docker Compose · [Mailpit](https://mailpit.axllent.org/) (email de dev, a partir da fase 1) |

Arquitetura hexagonal no backend (`domain` / `usecase` / `adapter`) — as
convenções do projeto estão no [CLAUDE.md](CLAUDE.md).

## Design

O sistema visual é o [Untitled UI](https://www.untitledui.com/), a base que o
[shelf.nu](https://www.shelf.nu/) publica no `tailwind.config.ts` deles: rampa de
cinza azulado de `#FCFCFD` a `#101828`, laranja `#EF6820` como marca, Inter,
raios de 6 e 8 px e sombras de 5% a 10%.

Tudo vive em tokens no topo de [`frontend/src/routes/layout.css`](frontend/src/routes/layout.css).
Os componentes leem os tokens por `var()` e pelas utilitárias que o
`@theme inline` gera (`bg-canvas`, `text-mute`, `border-hairline`), então trocar
um tema é trocar valores — nenhum `.svelte` conhece cor.

**Escuro é o padrão; quem usa escolhe.** O seletor no cabeçalho grava a
preferência, e [`frontend/static/tema.js`](frontend/static/tema.js) a aplica no
`<html>` **antes da primeira pintura** — sem ele a página apareceria escura e
piscaria para clara depois da hidratação. Ele é um script externo, e não inline,
porque a CSP do projeto é `script-src 'self'`.

## Quick start

```bash
make run
```

Sobe tudo em primeiro plano; `Ctrl+C` encerra. Na primeira vez o alvo também
prepara o que faltar, e pode repetir à vontade:

- cria o `.env` a partir do exemplo, com `POSTGRES_PASSWORD` aleatória
- cria o `frontend/.env`
- roda o `npm install` do host — **antes** do compose, e isso importa: o
  container `web` monta `node_modules` e `.svelte-kit` em volumes próprios (a
  instalação dele é Alpine/musl, a sua é glibc), e o Docker cria o ponto de
  montagem como **root** quando o diretório ainda não existe. Depois disso todo
  `npm install` seu falharia com `EACCES`. Se já tiver acontecido:
  `docker run --rm -v "$PWD/frontend:/app" alpine chown 1000:1000 /app/node_modules /app/.svelte-kit`

| Serviço | Endereço |
|---|---|
| Frontend | http://localhost:5173 |
| API | http://localhost:8080 |
| Mailpit (emails de dev) | http://localhost:8025 |
| Postgres | `localhost:5432` |

> As portas são as mesmas do agendaGo. Rodando os dois ao mesmo tempo, o
> segundo `docker compose up` falha com "port is already allocated" — derrube o
> outro projeto antes (`docker compose down`).

## Rotas

| Método | Rota | O que faz |
|---|---|---|
| `GET` | `/health` | Liveness: o processo está no ar. Não toca em dependência nenhuma. |
| `GET` | `/ready` | Readiness: faz ping no Postgres. Responde 503 se o banco estiver fora. |
| `POST` | `/auth/cadastro` | Cria a conta e já abre a sessão (201 + cookie). 409 se o email já existir. |
| `POST` | `/auth/login` | Autentica e abre a sessão (200 + cookie). 401 genérico; 429 no teto por conta. |
| `POST` | `/auth/logout` | Encerra a sessão no servidor e apaga o cookie (204, sempre). |
| `GET` | `/auth/me` | Devolve a conta autenticada. 401 sem sessão válida. |

A sessão viaja num cookie `HttpOnly` + `SameSite=Lax` (com prefixo `__Host-` e
`Secure` em produção). O banco guarda só o SHA-256 do token; a senha, um hash
Argon2id.

Todas as rotas abaixo exigem sessão e têm teto de requisições por sessão.

| Método | Rota | O que faz |
|---|---|---|
| `GET` | `/boards` | Lista os quadros de que você participa, com o seu papel em cada um. |
| `POST` | `/boards` | Cria um quadro e vincula você como dono (201). |
| `GET` | `/boards/{id}` | O quadro com colunas e cards, em ordem, numa requisição só. |
| `PATCH` | `/boards/{id}` | Renomeia o quadro. Só o dono (403 para os demais). |
| `DELETE` | `/boards/{id}` | Apaga o quadro; colunas, cards e vínculos vão junto (204). Só o dono. |
| `POST` | `/boards/{id}/colunas` | Cria uma coluna no fim do quadro (201). |
| `PATCH` | `/colunas/{id}` | Renomeia a coluna. |
| `DELETE` | `/colunas/{id}` | Apaga a coluna e os cards dela (204). |
| `POST` | `/colunas/{id}/cards` | Cria um card no fim da coluna (201). |
| `PATCH` | `/cards/{id}` | Edita título e descrição, incrementando a versão. |
| `DELETE` | `/cards/{id}` | Apaga o card (204). |
| `GET` | `/boards/{id}/membros` | Quem participa. Para o dono, também os convites pendentes. |
| `POST` | `/boards/{id}/membros` | Convida por email (201). Só o dono. |
| `PATCH` | `/boards/{id}/membros/{usuarioId}` | Troca o papel. Só o dono. |
| `DELETE` | `/boards/{id}/membros/{usuarioId}` | Remove do quadro (204). Só o dono. |
| `DELETE` | `/boards/{id}/convites/{conviteId}` | Revoga um convite, invalidando o link (204). |
| `POST` | `/convites/{token}/aceitar` | Aceita o convite e entra no quadro. |
| `GET` | `/cards/{id}` | O card com etiquetas, checklists e anexos — o que o modal mostra. |
| `PATCH` | `/cards/{id}/prazo` | Marca a data de entrega; `null` limpa. |
| `PATCH` | `/cards/{id}/mover` | Move o card. Recebe os **vizinhos**, não a posição. 409 sem espaço. |
| `PATCH` | `/colunas/{id}/mover` | Reordena a coluna no quadro, pelos vizinhos. |
| `PATCH` | `/boards/{id}/fundo` | Troca o fundo do quadro. Só o dono. |
| `GET`/`POST` | `/boards/{id}/etiquetas` | Lista e cria as etiquetas do quadro. |
| `PATCH`/`DELETE` | `/etiquetas/{id}` | Edita nome e cor, ou apaga (some de todos os cards). |
| `PUT`/`DELETE` | `/cards/{id}/etiquetas/{etiquetaId}` | Aplica e tira a etiqueta do card. |
| `POST` | `/cards/{id}/checklists` | Cria uma checklist no card. |
| `PATCH`/`DELETE` | `/checklists/{id}` | Renomeia ou apaga (os itens vão junto). |
| `POST` | `/checklists/{id}/itens` | Cria uma linha. |
| `PATCH`/`DELETE` | `/itens/{id}` | Marca, renomeia ou apaga a linha. |
| `POST` | `/cards/{id}/anexos/link` | Anexa uma URL. |
| `POST` | `/cards/{id}/anexos/arquivo` | Envia um arquivo (multipart, até 10 MB). |
| `GET`/`DELETE` | `/anexos/{id}` | Baixa ou apaga o anexo. |

E uma rota **sem** sessão:

| Método | Rota | O que faz |
|---|---|---|
| `GET` | `/convites/{token}` | Descreve o convite para quem abriu o link. |

**404 e 403 querem dizer coisas diferentes.** Quadro de outra pessoa responde
`404`, e não `403`: dizer "proibido" confirmaria que aquele quadro existe, o que
basta para varrer ids e mapear o que os outros têm. O `403` fica só para quem
**já enxerga** o quadro e esbarra no próprio papel — leitor tentando editar,
editor tentando apagar o quadro. Coluna e card são endereçados pelo próprio id
e o servidor descobre sozinho a que quadro pertencem (card → coluna → quadro);
informar um quadro pela URL não daria acesso a nada.

## Testes

```bash
make run             # sobe a stack inteira (Ctrl+C encerra)
make test            # backend (rápidos) + frontend
make test-backend    # go test ./...
make test-frontend   # vitest run

cd backend && make test    # com -race, o detector de corrida
cd frontend && npm run check   # tipos (svelte-check)
```

## Ao adicionar uma dependência Go

O `air` recompila a cada `.go` salvo, mas quando a build falha ele **mantém no
ar o binário anterior** — a API responde normalmente e só as rotas novas dão
404, o que parece erro de código e não de build. É o que acontece depois de um
`go get`, porque o container ainda não baixou o módulo novo:

```bash
go get <módulo>              # no host, atualiza go.mod/go.sum
docker compose restart api   # o container baixa e recompila
```

Na dúvida, `docker compose logs api` mostra a falha de build.

## Anexos

Arquivo enviado vai para o disco, num **volume próprio** (`ANEXOS_DIR`, padrão
`/var/lib/stacktrack/anexos`) — não para o banco, que incharia backup e restore de
um schema que guarda texto curto no resto todo, nem para dentro de `./backend`,
que é a árvore de código montada do host.

Três decisões que valem saber antes de mexer:

- **Lista de permissão de tipos**, não de bloqueio. `text/html` e `image/svg+xml`
  ficam de fora de propósito: servidos da nossa origem, executariam script na
  nossa origem.
- **O nome do arquivo no disco é sorteado**, nunca derivado do que veio de quem
  enviou — nome de arquivo é entrada do usuário, e entrada do usuário não vira
  caminho. O nome original sobrevive só como rótulo.
- **O download passa pela API** (`GET /anexos/{id}`), e não por um caminho
  estático: é lá que se confere se quem pede participa do quadro. A resposta sai
  como `attachment` com `nosniff`.

A descrição do card aceita um subconjunto de Markdown, renderizado por
[`lib/markdown.ts`](frontend/src/lib/markdown.ts) — escrito à mão, sem
biblioteca: o texto é escapado inteiro **antes** de as marcas virarem tags, então
nada que alguém escreveu chega ao HTML como marcação.

## Ordenação (arrastar e soltar)

A posição de cards e colunas é **fracionária**, não um inteiro sequencial. Com
`1, 2, 3, 4`, arrastar um item para o meio obrigaria a reescrever a numeração de
todos abaixo dele — muitas linhas alteradas e corrida garantida com duas pessoas
no mesmo quadro. Com fração, inserir entre `2048` e `3072` é gravar `2560`:

```
antes:   A(2048)   B(3072)            C(4096)
                       ▲ solta aqui
depois:  A(2048)   C(2560)   B(3072)          ← só C foi escrito
```

**A API recebe os vizinhos, não a posição.** O cliente manda `anteriorId` e
`proximoId`; quem calcula o número é o servidor. Três razões: a cópia do quadro
na tela pode estar velha e a média entre posições que já mudaram põe o item no
lugar errado; o esgotamento da precisão só é detectável onde os valores reais
estão; e posição vinda do cliente é entrada do usuário, que embaralharia a ordem
de um quadro inteiro. O card continua se movendo na tela na hora — ele só não
decide o número.

**O limite do `double precision` é real e está medido.** Dividir sempre o mesmo
intervalo esgota a mantissa de 53 bits em **52 inserções seguidas no mesmo
ponto** (há teste que mede isso). Quando acontece, o movimento responde `409` em
vez de gravar em silêncio duas posições iguais. A saída definitiva é trocar o
float por chave textual — é a fase 9 do plano.
