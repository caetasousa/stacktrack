# 🧪 Testes

Oito camadas hoje e uma lacuna E2E conhecida. Este documento diz o que cada uma
cobre, o que ela **não** cobre e onde isso já custou caro.

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

Cobre: validação de título, papéis e o que cada um pode, a chave de ordenação
textual (inclusive os quatro padrões de reordenação no mesmo ponto, e o teto
real de ~750 que a coluna impõe — contra 52 da posição em float), cores da
paleta, prazo e checklist.

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

> **A camada que pegou três defeitos que nenhuma outra pegou.** Na fase 11, o
> domínio, o usecase e os testes de unidade estavam todos corretos, e mesmo
> assim a API respondia errado: um erro de domínio sem tradução virava **500**
> (dizendo "erro interno" para uma recusa que é regra de negócio), e dois campos
> que o DTO declarava não eram copiados pelo conversor — um deles saindo como
> `null` e quebrando a tela em tempo de execução.
>
> É o argumento a favor desta camada: a borda HTTP tem lógica própria — tradução
> de erro e montagem de DTO — e ela não é exercitada por nenhum teste de dentro.

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

`TestQuedaPorLentidaoAvisaQuemFicou` fecha o outro lado dessa regra: **cair por
lentidão também é sair**, e quem fica precisa saber. A remoção acontece dentro
de `Publicar`, e o `defer Cancelar` do handler roda depois — encontrando o
assinante já fora da sala, ele não anuncia nada. Sem o aviso vindo do próprio
`Publicar`, o avatar de quem caiu ficava preso na tela dos outros até o próximo
evento de presença, que pode nunca vir. O teste anterior não pegava isso porque
só conferia que a sala esvaziou, nunca que alguém foi avisado.

### O protocolo do WebSocket, sem stack

`ws_test.go` roda na suíte padrão: o handler sobe em `httptest` e o cliente é o
mesmo `coder/websocket` do outro lado, então o que se prova é o protocolo de
verdade — sem Docker, sem navegador, sem API no ar.

Existe porque o pacote `adapter/http/ws` não tinha cobertura nenhuma no
`go test ./...`. O que o exercitava era o teste de duas abas (build tag) e o
Playwright, ou seja: só rodava para quem lembrasse de subir tudo. A peça mais
delicada do projeto era a menos coberta no dia a dia.

Treze casos, em quatro grupos:

| grupo | o que tranca |
|---|---|
| handshake | 401 sem sessão, 400 sem quadro, **404** para quem não participa (nunca 403, que confirmaria a existência), e origem fora da lista recusada — o CSWSH |
| entrega | primeira conexão recebe a posição atual; evento ao vivo chega; o eco do próprio autor não volta |
| reposição | o intervalo volta em ordem; o eco é filtrado também no passado; um intervalo só do próprio autor **ainda fecha o seq**; backlog além do teto vira `recarregue.tudo` |
| revalidação | acesso revogado e sessão encerrada derrubam a conexão |

O último grupo usa `ComIntervaloDeRevalidacao` para não esperar os 30 segundos
de produção.

### A transação do outbox

Prova em duas camadas, porque são duas perguntas diferentes.

`test/usecase/outbox_test.go` prova o **contrato**: o usecase pede a escrita
atômica, e quando ela falha não registra nem publica — publicar ali anunciaria
mudança que não aconteceu. Não precisa de banco.

`test/repository/outbox_test.go` (tag `integracao`) prova o que só o Postgres
responde: que as duas escritas realmente compartilham a transação. O caso que
importa força o `INSERT` do evento a violar a chave estrangeira **depois** de o
`UPDATE` do card já ter acontecido, e verifica que o card voltou à posição
original. É exatamente a janela que o outbox fecha.

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
| o comentário do outro aparece no card aberto | o modal também é tempo real — ele nasceu mudo na fase 11 |

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

### O quadro no celular

`e2e/celular.spec.ts` roda o mesmo navegador com `devices['Pixel 5']` — tela
estreita e, o que importa, `hasTouch`. É o que faz o Playwright emitir eventos
de TOQUE em vez de mouse.

Existe porque o quadro estava inteiramente quebrado no celular com a suíte toda
verde: tocar num card não abria nada. Eram **três** defeitos independentes, e
cada um sozinho já bastava — nenhum deles alcançável com mouse:

| Caso | O que prova |
|---|---|
| um toque no card abre o modal | o clique sintetizado pelo toque vem sem coordenada, e o guarda de "isto foi arraste?" media por ela |
| o modal aberto por toque não se fecha sozinho | um toque gera DOIS cliques, e o segundo caía no fundo escuro que acabara de cobrir o card |
| arrastar o dedo pela lista não levanta o card | com o arraste começando no primeiro contato, rolar a coluna movia cards |
| arrastar o dedo pela alça da coluna rola o quadro | o mesmo, na zona de fora — o gesto lateral reordenava o quadro |
| apagar um card pede confirmação, e cancelar não apaga | o diálogo do produto no lugar do `confirm()`, e um `page.on('dialog')` que reprova se algo escapar para o nativo |

Dois detalhes que fizeram a diferença entre um teste que prova e um que só passa:

- **a verificação acontece com o dedo ainda na tela.** A classe do item no ar
  some no `touchEnd`, então uma versão que só olhasse no fim passava com o
  defeito de volta. Foi assim que a primeira tentativa deixou o mutante vivo;
- **o alvo do arraste da coluna é a alça `⠿`, não o cabeçalho.** A biblioteca se
  recusa a arrastar quando o toque começa num elemento com `value`, e `<button>`
  tem — mirar o título testava um ponto onde o defeito não podia aparecer.

Os gestos de arraste vão pelo CDP (`Input.dispatchTouchEvent`): a API pública do
Playwright sabe tocar, não sabe arrastar o dedo.

As quatro correções foram verificadas por mutação — desfeitas uma a uma, cada
uma derruba o seu teste.

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

### O guard da coluna sem SQL, e como declarar uma órfã

`sql_cobre_as_colunas_test.go` cobra que toda coluna criada nas migrations
apareça em algum SQL do repositório. É grosseiro de propósito — não sabe se ela
está no INSERT mas falta no UPDATE —, e mesmo assim é o guard que fecha o buraco
por onde `cards.prazo` e `boards.fundo` passaram.

Existe um caso legítimo de coluna sem SQL, e ele aparece quando uma
funcionalidade é **retirada**: entre parar de usar a coluna e derrubá-la vão
dois deploys, e no meio-tempo ela fica no banco sem ninguém que a leia. A
declaração vive em `backend/migrations/COLUNAS-ORFAS.md`. Durante a retirada do
arquivamento, por exemplo, ela teve estas duas linhas:

```
- ORFA: cards.arquivado_em
- ORFA: colunas.arquivado_em
```

Hoje a lista está vazia: o banco de produção foi zerado antes do contract, as
migrations foram consolidadas e as duas colunas deixaram de existir. O arquivo
continua porque o mecanismo será necessário na próxima retirada que ocorrer
com dados reais.

⚠️ **E ela não pode morar dentro da migration.** Foi a primeira tentativa: o
Flyway guarda o checksum de cada migration aplicada e valida na partida, então
acrescentar um comentário a um arquivo já aplicado muda o checksum e derruba o
start com `Migration checksum mismatch`. Migration aplicada é imutável inclusive
nos comentários. O `.md` existe por isso — o Flyway só recolhe `V*.sql`.

A diferença entre uma órfã deliberada e o defeito é uma frase escrita por
alguém — e por isso a declaração é cobrada nos **três** sentidos: sem ela a
coluna acusa; se a coluna sumir e a linha ficar, acusa; e se o código voltar a
usar a coluna sem a linha sair, acusa também. Uma autorização que sobra é a que
ninguém relê no dia do acidente.

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

**Mas o contract da fase 9 precisava fazer exatamente as três coisas proibidas.**
Um guard sem escape vira um guard desligado no dia em que atrapalha, então ele
ganhou uma declaração: uma linha `-- CONTRACT: tabela.coluna, ...` na própria
migration autoriza **exatamente** aquelas colunas. Qualquer outra quebra na mesma
migration continua reprovando.

Ele falha nos dois sentidos, e o segundo é o que importa: **declaração que sobra
também reprova**. Autorização esquecida é justamente a que ninguém relê no dia do
acidente. Verificado nos dois — tirando um item da lista o guard acusa a coluna
removida; deixando um item a mais, acusa a declaração sem uso.

### O teto da chave de ordenação

`chave_cabe_na_coluna_test.go` pergunta ao banco o tamanho de `cards.chave` e
compara com `ordem.TamanhoMaximo`. São dois lugares que precisam concordar, e
eles moram em arquivos que ninguém edita junto — a migration e o domínio.

Foi assim que apareceu que a promessa "a chave nunca esgota" era falsa: ela
esgota, perto de 750 reordenações no mesmo ponto, porque a coluna é
`VARCHAR(200)`. Os testes de domínio afirmavam mil e passavam só porque nunca
perguntaram ao banco.

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

### Arrastar e soltar com mouse, sob o Playwright

A suíte de ponta a ponta cobre criação, propagação e — desde `celular.spec.ts` —
o que o toque NÃO deve arrastar. Falta o arraste que dá certo: pegar um card e
soltá-lo em outra coluna, com mouse. É onde moravam os dois defeitos históricos
(a alça da coluna e a cor do card).

Automatizar isso exige uma sequência de `mouse.down/move/up` com passos
intermediários, e um teste de arraste mal calibrado falha por motivo errado com
frequência suficiente para virar ruído.

É o candidato natural para o teste de duas abas completo: uma pessoa arrasta, a
outra vê o card mudar de coluna.
