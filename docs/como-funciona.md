# 🧭 Como o stacktrack funciona

> **Em uma frase:** o mapa do projeto, sem jargão — o que ele faz, de que peças
> é feito, as regras que não se quebram e onde procurar cada coisa.

Se você nunca mexeu no projeto, comece por aqui. Os outros documentos são mais
técnicos e servem de referência quando você já souber o básico.

---

## O que o stacktrack faz

É um **quadro Kanban**: colunas ("A fazer", "Fazendo", "Pronto") com cartões —
os **cards** — que representam o trabalho e andam de coluna em coluna conforme
ele avança.

O que o diferencia é que **várias pessoas usam o mesmo quadro ao mesmo tempo**,
e cada uma vê a mudança da outra no instante em que ela acontece, sem
recarregar a página. Se a Ana arrasta um card para "Pronto", ele se move na tela
do Bruno também.

Em volta disso:

- **convidar pessoas por email**, cada uma com um papel — **dono**, **editor**
  ou **leitor**. Quem não tem conta cria uma pelo próprio convite; quem
  confirmou o email da conta encontra o convite também no painel, e aceita ou
  recusa por lá;
- cada card guarda **conversa, checklists, anexos, prazo, etiquetas,
  responsáveis** e o **histórico** do que aconteceu com ele;
- o **histórico do quadro** responde "quem mexeu nisso, e quando?";
- um **sino** no topo da tela avisa o que é com você: um convite para um quadro,
  um card que passou a ser seu, alguém que chamou você pelo nome num comentário
  (com "@Nome"), um prazo seu chegando — e o mesmo aviso chega por email, se você
  não o viu na tela;
- um **link público** mostra o quadro, só para leitura, a quem não tem conta —
  o cliente, a diretoria;
- **esqueceu a senha?** O link chega por email, vale uma hora, e trocar a senha
  tira a conta de todos os aparelhos;
- funciona no **celular**.

Tudo isso, uma funcionalidade por vez, está em
[funcionalidades.md](funcionalidades.md).

---

## As peças

O stacktrack não é um programa só. São algumas peças que conversam entre si.

Pense na **secretaria de uma escola**, com o quadro de avisos no corredor:

| Peça | Na escola seria | O que faz |
|---|---|---|
| **web** (SvelteKit) | o quadro de avisos | a tela que você vê, clica e arrasta |
| **API** (Go) | a secretaria | recebe cada pedido, confere se a pessoa pode, anota e avisa todo mundo |
| **PostgreSQL** | o livro de registros | onde tudo fica guardado — inclusive a lista, em ordem, de tudo o que aconteceu |
| **borda** (nginx) | a portaria | o único lugar por onde se entra; manda cada visita para a sala certa |
| **túnel da Cloudflare** | o endereço da escola | traz as visitas da internet até a portaria, sem abrir porta na rua |
| **Brevo** | os correios | levam os emails de convite, de senha nova e os avisos |

Dentro da secretaria trabalham ajudantes que **ninguém chama** — eles rodam
sozinhos, o tempo todo:

- o **despachante**: a cada anotação nova no livro, avisa todas as telas que
  estão com aquele quadro aberto;
- o **carteiro**: leva os emails da fila aos correios — e, se os correios
  estiverem fechados, tenta de novo mais tarde. Antes de levar um aviso, ele
  confere se ainda vale a pena: se você já viu o aviso na tela, ou se ele deixou
  de valer, o email não sai;
- o **vigia de prazos**: olha o relógio e, na véspera de um prazo, avisa quem é
  responsável pelo card;
- a **campainha**: quando chega um aviso para alguém, toca nas telas que essa
  pessoa tem abertas, e o número do sino muda sem recarregar;
- a **faxina**: de hora em hora, joga fora o que venceu — sessões, links de
  senha nova e de confirmação, convites velhos, emails que ficaram na fila e os
  avisos já lidos há mais tempo do que o dono do quadro escolheu guardar.

```
Você → tela (web) → portaria (borda) → secretaria (API) → livro (PostgreSQL)
                                          │
                     ┌────────────────────┴───────────────┐
                     ↓                                    ↓
          despachante: avisa as outras telas    carteiro: correios (Brevo)
```

A tela **nunca fala direto com o livro**. Tudo passa pela secretaria — é ela que
sabe as regras.

---

## Por que é assim

- **Uma secretaria só.** Um programa, num servidor só: é o mais simples de
  operar e de entender. O preço é não ter reserva — se o servidor cai, o quadro
  cai junto. Para o tamanho de hoje, é uma troca consciente.
- **A tela não decide nada.** Esconder um botão de quem não pode é cortesia, não
  proteção: quem confere é a API, a cada pedido. Um leitor que mandasse o pedido
  "na mão", sem a tela, levaria a mesma recusa.
- **Os ajudantes moram dentro da secretaria.** O aviso às telas e o envio de
  email não são programas à parte: são parte da API. Menos peças para subir,
  vigiar e manter em sincronia.

---

## As regras que não se quebram

Se você mexer no código, estas são as que mais causam estrago quando ignoradas.

### 1. Toda mudança é anotada junto com o aviso dela

Quando alguém move um card, duas coisas entram no livro **no mesmo instante**: a
mudança em si e a anotação "Ana moveu *Migração* para *Pronto*". Ou entram as
duas, ou nenhuma.

É dessa anotação que saem o aviso para as outras telas e o histórico. Se as duas
fossem gravadas separadas, um dia a mudança entraria sem o aviso — e a tela de
alguém passaria a mostrar um quadro velho, sem que ninguém percebesse.

### 2. Quem grava primeiro ganha — e a outra pessoa fica sabendo

Duas pessoas editando o mesmo card: a primeira a salvar ganha. A segunda vê o
aviso "alguém alterou este card enquanto você escrevia", **com o texto dela
ainda na tela**, para copiar antes de trazer a versão nova.

Sobrescrever calado seria apagar o trabalho de alguém sem ninguém ficar sabendo.

### 3. Mover um card mexe só nele

A posição de cada card é uma etiqueta que fica **entre a do vizinho de cima e a
do de baixo**. Mover um card troca só a etiqueta dele — os outros cards da coluna
ficam como estavam.

Por isso duas pessoas arrastando cards diferentes ao mesmo tempo não desfazem o
trabalho uma da outra. Se mover reescrevesse a coluna inteira, a última a soltar
apagaria a arrumação da primeira.

### 4. Quem não participa não descobre nem que o quadro existe

Um pedido sobre um quadro de que a pessoa não participa recebe **"não
encontrado"** — a mesma resposta de um quadro que não existe. Responder "você
não tem permissão" já confirmaria que ali há um quadro.

E participar de um quadro dá acesso **só a ele**: ser convidado para o quadro
da Ana não abre os outros quadros dela.

O convite que aparece no painel segue a mesma régua: ele só aparece para a conta
que **confirmou** o email — senão bastaria criar uma conta com o email de outra
pessoa para ver, e aceitar, os convites dela.

### 5. O que sai para fora é anotado antes

O email do convite entra numa **fila**, no mesmo instante em que o convite é
gravado; o carteiro o leva depois. Os correios fora do ar não desfazem convite
nenhum — o email só sai mais tarde.

Os avisos do sino também: o aviso de "você é responsável por este card" é
anotado no mesmo instante em que alguém marca você — se a anotação do card falha,
o aviso não existe, e vice-versa. O email desse aviso espera dois minutos na
fila, porque quem está com a tela aberta já viu.

O mesmo vale para os arquivos anexados: apagar um anexo anota a exclusão, e o
arquivo só sai do disco quando houver um backup garantido de que ele não vai
fazer falta.

---

## Como o código está organizado

O backend é dividido em camadas. A regra é simples: **as regras do negócio não
sabem nada sobre internet, telas ou banco de dados.**

```
Entra um pedido pela internet
        ↓
   [adapter/http]        traduz de "internet" para "programa"
        ↓
   [usecase]             coordena os passos: confere o acesso, trava o quadro,
        ↓                grava a mudança e a anotação, avisa o despachante
   [domain]              decide, segundo as regras do negócio
        ↓
   [adapter/repository]  guarda no banco
```

Por que separar assim: a regra "o último dono não pode sair do quadro" mora
**num lugar só**, no `domain`, e vale igual venha o pedido da tela, de um teste
ou de um comando de manutenção.

O frontend tem as **telas** em `frontend/src/routes/` e, em `frontend/src/lib/`,
o que é regra de tela e pode ser testado sozinho: o filtro de cards, o Markdown,
as cores, as frases do histórico e a conexão ao vivo.

O detalhe técnico está em [arquitetura.md](arquitetura.md).

---

## O caminho de uma mudança

Você vai mexer no código. E agora?

```
1. escreve como a funcionalidade pode quebrar   → frontend/e2e/CENARIOS.md
2. escreve o código
3. confere na sua máquina                        → make verificar
4. confere contra banco e navegador de verdade   → make test-integracao · make test-e2e
5. commita e envia para o GitHub                 → git commit · git push
6. publica as imagens                            → docs/entrega-continua.md
7. implanta no servidor                          → stacktrack-release release <sha>
8. confere no ar                                 → https://stacktrack.d4ffy.com
```

**Nenhum passo acontece sozinho.** Não há esteira: quem não roda o passo 4 não
fica sabendo do que só aparece com banco e navegador de verdade.

**Por que escrever primeiro como quebra?** Porque o teste escrito depois do código
tende a conferir o que o código faz, e não o que ele deveria fazer. Começar pela
lista de falhas é o que garante que cada uma tenha um teste que morre se ela
voltar.

O dia a dia completo está em [fluxo-de-trabalho.md](fluxo-de-trabalho.md).

---

## Onde o sistema fica no ar

Num servidor próprio: uma segunda distro Linux (`ubuntu-server`) dentro do WSL
deste mesmo computador, separada da que se usa para programar. Ela não tem porta
aberta para a internet: um programa da Cloudflare, o **túnel**, abre uma conexão
de dentro para fora, e as visitas chegam por ela.

| | |
|---|---|
| Endereço | `https://stacktrack.d4ffy.com` |
| Servidor | a distro `ubuntu-server` do WSL, dividida com o studygo |
| Como publica | à mão, imagem por imagem, com o SHA do commit |
| Backup | antes de cada deploy e às 03:20 — só nos dias em que algo mudou |

> ⚠️ **O servidor é este computador.** Com ele desligado, o site sai do ar. E o
> backup ainda não sai da máquina: perder o disco é perder os dados
> ([producao.md](producao.md#o-que-ainda-falta)).

---

## Palavras que você vai encontrar

| Palavra | O que significa aqui |
|---|---|
| **quadro** (*board*) | o Kanban inteiro, com suas colunas |
| **card** | um cartão: uma tarefa, com tudo o que pende dela |
| **dono / editor / leitor** | os papéis: administra / mexe / só olha (e comenta) |
| **convite** | o link, mandado por email, que põe alguém num quadro |
| **link público** | o endereço que mostra o quadro, só para leitura, sem conta |
| **evento** | a anotação de uma mudança no livro ("Ana moveu X") |
| **revisão** | o número de ordem dos eventos de um quadro: 41, 42, 43… |
| **tempo real** | a tela se atualizando sozinha quando outra pessoa mexe |
| **notificação** (aviso) | o que aparece no sino: convite, card que passou a ser seu, menção, prazo |
| **menção** | chamar alguém num comentário escrevendo "@" e o nome da pessoa |
| **SSE** | o jeito como a campainha chega à tela: uma conexão que fica aberta e por onde só o servidor fala |
| **outbox** | a fila de saída: o que vai para fora é anotado nela antes de sair |
| **borda** | a portaria (nginx) por onde toda visita entra |
| **túnel** | a conexão da Cloudflare que traz as visitas até o servidor |
| **imagem** | o pacote pronto de uma peça, publicado no GHCR e puxado pelo servidor |
| **deploy** | pôr uma versão nova no ar |
| **rollback** | voltar para a versão anterior |
| **migration** | uma mudança na estrutura do banco de dados |

---

## Se algo der errado

**O site está fora do ar?**

```bash
curl -fsS https://stacktrack.d4ffy.com/api/health   # a API respondeu?
ssh -i ~/.ssh/stacktrack_release -p 2222 stacktrack@127.0.0.1 "stacktrack-release estado"
```

O segundo mostra os containers e as impressões digitais dos arquivos da stack,
para comparar com o repositório. Os logs e o resto do diagnóstico estão em
[producao.md](producao.md#diagnóstico).

**Publiquei e quebrou?** Volte para o commit anterior com o mesmo comando do
deploy, mudando só o SHA — [producao.md](producao.md#voltar-atrás). O banco não
volta junto, e a seção explica quando isso importa.

**O sino não muda sozinho?** Recarregar a página mostra os avisos de qualquer
jeito — eles moram no banco, não na conexão. Se nunca muda sem recarregar, a
conexão da campainha está sendo barrada no caminho: veja
[producao.md](producao.md#diagnóstico).

**O convite não chegou?** A tela de Membros mostra o que houve com cada email
("na fila", "enviado", "não saiu"), e **Reenviar** resolve a maioria dos casos. A
fila por dentro está em [producao.md](producao.md#-email-dos-convites).

**Quebrou na minha máquina?** Veja as armadilhas conhecidas em
[rodar-local.md](rodar-local.md#-armadilhas-conhecidas) — a maioria é
`node_modules` com dono errado, porta ocupada ou o recarregador servindo um
binário velho.

---

## Onde procurar cada coisa

| Você quer... | Vá para |
|---|---|
| saber o que o produto faz, tela por tela | [funcionalidades.md](funcionalidades.md) |
| rodar o projeto na sua máquina | [rodar-local.md](rodar-local.md) |
| entender o dia a dia de trabalho | [fluxo-de-trabalho.md](fluxo-de-trabalho.md) |
| entender as decisões técnicas, o banco e as rotas | [arquitetura.md](arquitetura.md) |
| rodar e escrever testes | [testes.md](testes.md) |
| publicar ou implantar | [entrega-continua.md](entrega-continua.md) |
| operar o servidor (backup, email, diagnóstico) | [producao.md](producao.md) |
| mexer no Ansible e no host | [infraestrutura.md](infraestrutura.md) |
| estudar as tecnologias do projeto | [tecnologias.md](tecnologias.md) |
| saber o que ainda falta | [historico-do-projeto.md](historico-do-projeto.md#-o-que-ainda-falta) |
