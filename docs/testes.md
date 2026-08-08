# 🧪 Testes

Quatro camadas hoje, e duas que faltam. Este documento diz o que cada uma cobre,
o que ela **não** cobre, e onde isso já custou caro.

```bash
make test            # backend (rápidos) + frontend
make test-backend    # go test ./...
make test-frontend   # vitest run

cd backend && make test        # com -race
cd frontend && npm run check   # tipos (svelte-check)
cd frontend && npx prettier --check "src/**/*.{svelte,ts}"
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

## 5. Guard de schema (Go, sem banco)

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

O `go test` roda com **`-race`** desde já, antes mesmo de existir concorrência —
para o hábito estar pronto quando o hub da fase 5 chegar. Concorrência sem
detector de corrida é fé, não engenharia.

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

**Fase 5.** A lacuna mais cara até aqui. Dois defeitos passaram por todos os
testes verdes:

- a **alça de arrastar coluna nunca funcionou** — a biblioteca só registra o
  `mousedown` quando `dragDisabled` já é falso, e o interruptor ligado no
  `pointerdown` chegava tarde;
- a **cor do card não pintava**, porque a marcação que a aplicava não estava no
  arquivo.

Os dois só apareceram quando alguém usou a tela. O teste que define o projeto —
dois `BrowserContext`, um arrasta e o outro vê — chega junto com o tempo real.
