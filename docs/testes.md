# 🧪 Testes

Seis camadas hoje, e duas que faltam. Este documento diz o que cada uma cobre,
o que ela **não** cobre, e onde isso já custou caro.

```bash
make test            # backend (rápidos) + frontend
make test-backend    # go test ./...
make test-frontend   # vitest run

cd backend && make test        # com -race
cd frontend && npm run check   # tipos (svelte-check)
cd frontend && npx prettier --check "src/**/*.{svelte,ts}"

cd backend && make test-tempo-real   # exige a API no ar (build tag)
```

---

## 1. Domínio (Go)

`backend/test/domain/` — regra pura, sem I/O nenhum. Roda em milissegundos.

Cobre: validação de título, papéis e o que cada um pode, ordenação fracionária
(inclusive o teste que **mede** o esgotamento da mantissa em 52 inserções),
cores da paleta, prazo e checklist.

## 2. Usecases (Go, com fakes)

`backend/test/usecase/` — orquestração e **autorização**, contra repositórios em
memória (`test/repository/memoria/`).

É aqui que mora a garantia de que não-membro recebe 404, que leitor não move
card e que a cadeia card → coluna → quadro é percorrida antes de decidir.

⚠️ **É também o ponto cego mais perigoso do projeto.** Os fakes copiam a struct
inteira, então um campo que o SQL não grava passa por eles sem reclamar.
Aconteceu duas vezes:

| Campo | Existia em | Faltava em | Como apareceu |
|---|---|---|---|
| `cards.prazo` | domínio, DTO, migration | INSERT, UPDATE e os dois SELECTs | verificação manual na API |
| `boards.fundo` | domínio, DTO, handler, usecase | o repositório inteiro | o usuário viu que a cor não mudava |

Nos dois casos a API respondia `200` e o dado sumia.

## 3. Handlers (Go)

`backend/test/handler/` — o contorno HTTP: códigos de status, formato do corpo,
atributos do cookie de sessão.

## 4. Frontend (Vitest)

`frontend/src/**/*.test.ts` — funções puras (o renderizador de markdown, os
helpers de arraste, o cliente da API contra `fetch` falso) e **um teste de
componente**, que monta o `CardDoQuadro` de verdade em jsdom.

Esse último existe por um motivo específico: a ligação entre a cor do card e o
pixel já se perdeu em silêncio uma vez — a cor atravessava domínio, banco e API
corretamente, e a marcação que a pintava simplesmente não estava no arquivo.
Nem o `svelte-check` nem os testes de API viam isso, porque os dois olham o
JSON.

> Montar componente exige `resolve.conditions: ['browser']` no
> `vite.config.ts`. Sem isso o Vite entrega a build de **servidor** do Svelte e o
> `mount()` falha com "not available on the server".

## 5. Tempo real (Go, com -race)

`backend/test/realtime/hub_test.go` — o hub sozinho: salas isoladas, fan-out,
remoção idempotente, desligamento. O caso que importa é
`TestUsoConcorrenteNaoTemCorrida`: vinte goroutines publicando, vinte entrando e
saindo e uma lendo o tamanho da sala, tudo ao mesmo tempo. Sem o mutex
protegendo o mapa, o `-race` acusa.

E `TestAssinanteQueNaoLeEDesconectado` prova a regra que sustenta o desenho:
publicar não espera por ninguém. Um cliente que parou de ler é derrubado; sem
isso, a requisição HTTP de quem moveu o card ficaria bloqueada até ele voltar —
que pode ser nunca.

### O teste de duas abas

`duas_abas_test.go`, atrás da build tag `tempo_real` porque **exige a API no
ar**:

```bash
make run                        # noutro terminal
make -C backend test-tempo-real
```

Cobre o que só existe no handshake de verdade: ana age pela API REST e bruno
recebe pelo WebSocket; o autor não recebe o próprio eco; quem não participa
recebe **404**; quem vem de outra origem recebe **403** (o Cross-Site WebSocket
Hijacking, que o CORS não impede); e sem sessão, **401**.

> Um `429` ali não é defeito: é o teto por IP do rate limiter, que a suíte
> inteira divide. O teste vira *skip* com a causa escrita, em vez de vermelho.

## 6. Guard de schema (Go, sem banco)

`backend/test/repository/sql_cobre_as_colunas_test.go` — lê as migrations,
extrai toda coluna criada e falha se ela não aparecer em nenhum SQL do pacote de
repositórios.

É a rede que faltava para os dois casos da tabela acima. Não precisa de banco: é
comparação de texto, roda no `go test ./...` de sempre.

**O que ele não sabe dizer:** se a coluna está no `INSERT` mas falta no
`UPDATE`. Para isso são precisos os testes contra banco de verdade.

---

## O que a esteira roda

Ver [entrega-continua.md](entrega-continua.md). Além dos testes: `gofmt`,
`go vet`, `prettier --check`, `svelte-check`, `govulncheck`, `npm audit` e Trivy
nas três imagens.

O `go test` roda com **`-race`** — obrigatório desde que o hub existe.
Concorrência sem detector de corrida é fé, não engenharia. O teste de duas abas
NÃO roda na esteira: ele exige a stack no ar, e isso chega junto com o job de
ponta a ponta.

---

## O que ainda não existe

### Testes de repositório contra Postgres de verdade

**Fase 8, com Testcontainers.** É a camada que exercita o SQL real — a única que
teria pego `prazo` e `fundo` no ato, em vez de meses depois. O guard estático
acima é um paliativo consciente.

Junto vem o `compatibilidade_schema_test.go`, que compara o schema antes e
depois de uma migration e reprova coluna nova obrigatória, coluna removida e
coluna apertada — transformando a regra dos dois deploys em falha de build.

### Ponta a ponta (Playwright)

A lacuna mais cara até aqui. Dois defeitos passaram por todos os testes verdes:

- a **alça de arrastar coluna nunca funcionou** — a biblioteca só registra o
  `mousedown` quando `dragDisabled` já é falso, e o interruptor ligado no
  `pointerdown` chegava tarde;
- a **cor do card não pintava**, porque a marcação que a aplicava não estava no
  arquivo.

Os dois só apareceram quando alguém usou a tela.

O `duas_abas_test.go` já cobre o **protocolo** do tempo real de ponta a ponta —
handshake, autorização, origem, entrega —, mas não cobre a tela: se o cliente
deixar de aplicar o evento, ele continua verde. É esse pedaço que falta, com dois
`BrowserContext` no Playwright.
