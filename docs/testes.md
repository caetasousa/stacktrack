# 🧪 Testes

Oito camadas hoje, e uma que falta. Este documento diz o que cada uma cobre,
o que ela **não** cobre, e onde isso já custou caro.

```bash
make test            # backend (rápidos) + frontend
make test-backend    # go test ./...
make test-frontend   # vitest run

cd backend && make test        # com -race
cd frontend && npm run check   # tipos (svelte-check)
cd frontend && npx prettier --check "src/**/*.{svelte,ts}" "e2e/*.ts"

cd backend && make test-tempo-real   # protocolo do tempo real (build tag)
make test-e2e                        # navegador de verdade sobre a stack
```

Os dois últimos exigem `make run` noutro terminal.

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

## 6. Ponta a ponta (Playwright)

`frontend/e2e/` — navegador de verdade contra a stack inteira. É a camada que
fecha a fase 5, e a que faltava quando dois defeitos passaram por todo o resto
verde: a alça de arrastar coluna que nunca funcionou e a cor de card que não
pintava.

O teste que define o projeto abre **dois `BrowserContext`** — cookies e
armazenamento independentes, então duas contas convivem no mesmo navegador:

| Caso | O que prova |
|---|---|
| a conexão fica no ar | o indicador só aparece quando ela NÃO está; a ausência dele é a prova |
| ana cria um card, bruno vê | propagação sem nenhum reload chamado pelo teste |
| bruno cria uma coluna, ana vê | o caminho inverso |
| a queda aparece e volta sozinha | a tela não mente sobre ter parado de se atualizar |
| o avatar aparece e some | presença entrando e saindo, sem recarregar |
| quem grava por último é avisado | o 409 do bloqueio otimista, com o texto preservado |

Contas, quadro e convite são semeados **pela API**, não pela tela: um teste de
tempo real que quebra porque o botão de cadastro mudou de rótulo aponta para o
lugar errado.

> Todas as contas nascem numa rajada só, no `beforeAll`. Criar uma no meio da
> suíte esbarrava no teto por IP e deixava um teste **intermitente** — que é pior
> que um teste vermelho, porque ensina a ignorar a suíte.

> A queda é simulada com `routeWebSocket`, que deixa o teste interceptar a
> conexão e fechá-la. `context.setOffline()` **não** serve: ele não derruba um
> socket já estabelecido, que só perceberia a rede ausente no ping seguinte,
> 30 segundos depois.

Sem paralelismo (`workers: 1`), pelo mesmo motivo do teste em Go: cadastro e
login têm teto por IP, e uma suíte paralela derrubaria a si mesma com 429.

## 7. Integração contra Postgres real (Testcontainers)

`backend/test/repository/`, atrás da build tag `integracao` porque exige Docker:

```bash
make -C backend test-integracao
```

É a camada que os fakes nunca puderam cobrir. `TestTodoCampoDoCardSobreviveAoBanco`
escreve um card com todos os campos preenchidos e **lê de volta do banco** — se
um faltar no INSERT, no UPDATE ou em qualquer SELECT, ele volta zerado. É
exatamente assim que `cards.prazo` e `boards.fundo` sobreviveram meses.

Cobre também o `WHERE version` do bloqueio otimista contra o SQL de verdade (o
fake repete a regra, mas quem escreve o SQL errado não é o fake) e o log de
eventos da fase 7: ordem do `seq`, o intervalo do `Desde`, o payload
sobrevivendo ao JSONB, e o `ON DELETE CASCADE` levando a história junto com o
quadro.

### O guard de expand/contract

`compatibilidade_schema_test.go` aplica as migrations até a **penúltima**,
fotografa o schema, aplica a última e compara. Reprova três coisas, cada uma
com a instrução de como partir em dois deploys:

| O que a migration fez | Por que quebra |
|---|---|
| criou coluna já obrigatória | a versão anterior não a preenche, e o INSERT dela falha durante o deploy |
| removeu coluna | a versão anterior ainda a escreve, e é para onde o rollback volta |
| apertou para NOT NULL | idem: a versão anterior insere linhas sem ela |

Tabela **nova** com colunas obrigatórias passa, e não é descuido: a versão
anterior não a conhece e nunca insere nela. Foi o primeiro falso positivo do
guard, e o conserto está no código.

> Verifiquei que ele reprova de verdade acrescentando uma migration que aperta o
> schema — um guard que nunca falhou é decoração.

## 8. Guard de schema (Go, sem banco)

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
Concorrência sem detector de corrida é fé, não engenharia.

O job `e2e` sobe a stack inteira com `docker compose up -d --wait`, espera
`/ready` e a página responderem, e roda o Playwright. Ele é pré-requisito da
publicação: imagem só vai para o registry depois de o navegador ter aberto o
quadro.

O `duas_abas_test.go` do backend NÃO roda na esteira — o job `e2e` cobre o
mesmo caminho pela tela, que é o que faltava. Ele fica como ferramenta de
diagnóstico: quando o E2E falha, ele diz se o problema é do protocolo ou do
cliente.

---

## O que ainda não existe

### A transação do outbox

O evento é gravado logo **depois** da mudança, e não dentro da mesma transação.
Um processo que morra entre as duas escritas deixa um buraco no log — e o
cliente que reconectar pedindo "desde o 41" receberia o 43 sem saber que o 42
existiu.

O que segura isso hoje é o caminho de recarga completa, sempre correto. Fechar a
transação exige levar o `pgx.Tx` até os repositórios, e é o próximo passo aqui.

### Arrastar e soltar sob o Playwright

A suíte de ponta a ponta cobre criação e propagação, mas **ainda não o arraste**
— que é justamente onde os dois defeitos históricos moravam. Automatizar o
`svelte-dnd-action` exige uma sequência de `mouse.down/move/up` com passos
intermediários, e um teste de arraste mal calibrado falha por motivo errado com
frequência suficiente para virar ruído.

É o próximo caso a escrever, e o candidato natural para o teste de duas abas
completo: uma pessoa arrasta, a outra vê o card mudar de coluna.
