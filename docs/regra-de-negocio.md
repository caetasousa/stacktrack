# 📋 Regra de negócio

O que o produto faz, e as decisões de comportamento que não se leem no código
sem contexto.

---

## Quadro, coluna, card

Um **quadro** pertence a quem o criou e tem **colunas** em ordem; cada coluna tem
**cards** em ordem. Colunas e cards podem ter cor própria — verde no começo,
amarelo no meio, azul no fim é o uso típico, e é o que dá significado à etapa de
relance.

Apagar em cascata: quadro leva colunas, colunas levam cards, cards levam
etiquetas aplicadas, checklists e anexos.

## Papéis

| Papel | Ver | Editar | Administrar |
|---|---|---|---|
| dono | ✅ | ✅ | ✅ |
| editor | ✅ | ✅ | — |
| leitor | ✅ | — | — |

**Administrar** é convidar, trocar papel, remover membro, renomear e apagar o
quadro, e trocar o fundo. **Editar** é mexer em colunas, cards e no que pende
deles.

Papel desconhecido — dado estragado no banco, ou papel novo que alguém esqueceu
de tratar — **não pode nada**. É o padrão seguro, e há teste para isso.

## O 404 que não é engano

Quem não participa de um quadro recebe **404**, nunca 403. Não é imprecisão: um
403 confirmaria que o quadro existe, e a existência já é informação sobre dado
que aquela pessoa não pode ver. O 403 fica reservado para quem **é** membro e
esbarra no limite do próprio papel — aí não há o que esconder.

A mesma tradução vale em cadeia: pedir uma coluna de um quadro alheio devolve
"coluna não encontrada", e um anexo devolve "anexo não encontrado", em vez de
revelar em que etapa da cadeia a busca parou.

## Convites

O dono convida **informando o email**. Duas situações:

- **já existe conta com aquele email** → a pessoa entra no quadro na hora;
- **não existe** → nasce um convite com token, e o dono copia o link e envia por
  onde quiser.

Não há envio de email no projeto, então o link é o produto do convite, não um
efeito colateral dele. Quando o envio existir, ele só passa a mandar o mesmo
link.

Revogar um convite invalida o link. O token é gerado com `crypto/rand`.

## Auditoria: quem mexeu no quê

Num quadro com muitos cards, alguém arrasta trinta deles e ninguém sabe quem
foi. A informação **sempre esteve** no log de eventos — o histórico de cada card
já a mostrava —, mas só era alcançável abrindo card por card, que é o mesmo que
não estar.

Nada de novo é guardado. São duas leituras sobre a mesma tabela `board_events`:

**O selo no card.** Todo card que já foi movido mostra, no rodapé, quem o moveu
por último e há quanto tempo — o tempo relativo acompanha o relógio, e não
congela na hora em que a página abriu. O trajeto completo e a data exata ficam no
`title`. Card recém-criado **não** ganha selo: dizer "movido por quem criou"
transformaria auditoria em ficção.

**A tela de histórico.** Tudo o que aconteceu no quadro, do mais recente para o
mais antigo, com filtro por pessoa: card criado, movido, renomeado, apagado;
coluna criada, renomeada, reordenada, apagada; etiqueta criada, aplicada,
retirada; anexo, checklist, item, responsável, comentário; quadro renomeado,
fundo trocado; e quem entrou, mudou de papel ou saiu.

Ela começou mostrando só movimentações de card — a pergunta original era "quem
bagunçou a ordem". O recorte estreito continua a um clique, mas deixou de ser o
padrão: quem chega ali está investigando, e um filtro ligado de saída esconde
justamente o que a pessoa ainda não sabe que procura.

**Isso exigiu dar identidade aos eventos.** Doze ações diferentes — etiqueta,
anexo, checklist, responsável, renomear o quadro, trocar o fundo — eram gravadas
como um único `quadro.alterado` com payload vazio. Funcionava como aviso de
"recarregue o quadro", que era para o que fora feito, e era inútil como
histórico: o log registrava que ALGO mudou e nada mais. O tipo do evento é a
identidade do que aconteceu, e um tipo genérico apaga essa identidade no momento
da gravação — depois não há como recuperá-la.

Os tipos genéricos continuam sendo lidos: o log é append-only, e as linhas
antigas não mudam. A tela as descreve como pode ("mexeu no quadro") em vez de
escondê-las, porque um buraco onde houve atividade é pior que uma frase vaga.

**O email aparece sob cada nome.** Dois "Ana Silva" no mesmo quadro tornam o
histórico inútil exatamente quando ele é necessário. Não é exposição nova:
qualquer membro já lê o email de todos na tela de membros.

Três decisões que valem registrar:

- **Reordenar dentro da própria coluna conta como movimentação**, e tem dono. É
  justamente assim que uma coluna de prioridades é embaralhada. A frase muda
  ("reordenou em A fazer") porque o trajeto não existe, mas a linha existe.
- **Os nomes das colunas vêm do evento, não das colunas de hoje.** O log gravou
  o que era verdade na hora. Resolver o id agora mostraria o nome de hoje numa
  frase sobre ontem — e nada, se a coluna já tivesse sido apagada.
- **Qualquer membro audita, inclusive o leitor.** É a mesma régua do histórico
  de um card: ver o que aconteceu é ver, não mexer. Um leitor que precisasse
  virar editor para descobrir quem bagunçou o quadro seria o contrário do que se
  quer.

O nome de quem moveu **não** sai pelo link público — ver a seção seguinte.

## Link público de acompanhamento

Convidar resolve o caso de quem vai **trabalhar** no quadro. Não resolve o de
quem só precisa **saber como vai** — o cliente, a diretoria, a área vizinha.
Para essas pessoas, criar conta e conceder papel é atrito por nada: elas não vão
mexer em coisa nenhuma, e cada conta a mais é uma permissão a administrar depois.

O dono liga um link público e manda o endereço. Quem o recebe vê o quadro em
modo de leitura, sem conta e sem convite.

### O link é a credencial

Não existe `/publico/{id-do-quadro}`. O endereço carrega um **token** de 256 bits
gerado com `crypto/rand`, e é ele — não o id do quadro — que autoriza. As
consequências são as que interessam:

- quem descobrir o id de um quadro não chega a lugar nenhum com ele;
- revogar é apagar uma linha, e o endereço morre na hora;
- religar depois gera um token **diferente**. Quem guardou a URL antiga não
  volta a entrar. É o que separa revogar de esconder.

Publicar duas vezes devolve o **mesmo** link, de propósito: sem isso, só abrir a
tela de compartilhamento de novo invalidaria em silêncio o endereço que já
tinha sido enviado às pessoas.

### O que atravessa, e o que não

| Vai | Fica |
|---|---|
| colunas, cards, descrições, etiquetas, prazos, progresso de checklist | comentários, anexos, histórico, responsáveis, membros |

A coluna da direita é a regra, não um recorte de tela. Ela existe porque quem
decide publicar é o dono do quadro, e **o nome do colega não é dele para
publicar** — nem a conversa em que a equipe discute o que ainda não está
decidido. O que sai é o andamento do trabalho; quem o faz e como se chegou lá,
não.

Ids também não saem — nem do quadro, nem das colunas, nem dos cards. A resposta
pública não precisa endereçar nada, e um id que não sai não é um id que alguém
tenta noutra rota.

### Quem liga, e quem fica sabendo

Ligar e desligar é do **dono**: o token é o segredo, e quem o recebe pode
repassá-lo a quem quiser — um editor com acesso a ele publicaria o quadro por
conta própria.

Mas o **aviso** de que o quadro está público vai para todo membro, no cabeçalho
do quadro. Quem escreve num card precisa saber que aquilo está à vista de fora
antes de escrever, não depois.

## O que pende de um card

**Responsáveis** são quem responde pelo card — é o que faz o quadro responder
"o que é meu?", e não só "o que existe". Um card aceita mais de um: trabalho em
par é normal.

Só dá para atribuir **quem já participa do quadro**, e a regra é do domínio, não
do banco: a chave estrangeira aponta para `usuarios`, porque "quem pode ser
responsável" muda com a regra, ao contrário da existência da conta. Tentar
atribuir alguém de fora responde **422** — e não 404, porque quem pediu já
enxerga o card, então esconder o motivo não protegeria nada.

**Sair do quadro leva as atribuições junto.** Mantê-las deixaria a lista de
responsáveis mentindo (nomes de quem não tem mais acesso) e faria o filtro
"meus cards" mostrar à pessoa removida cards que ela não consegue mais abrir.

**Etiquetas** pertencem ao quadro, não ao card: renomear ou trocar a cor muda em
todos os cards de uma vez. O card guarda só os ids. No card elas aparecem com
**cor e nome**: a cor sozinha só significa algo para quem decorou a convenção do
quadro, e não significa nada para quem não a distingue.

**Comentários** são a conversa do card, em markdown. É o primeiro fluxo
**append-only** do projeto: um comentário acontece e fica — não tem posição, não
se reordena, e a ordem é a do tempo.

Quem pode o quê são três regras diferentes, e confundi-las seria deixar alguém
falar pela boca de outra pessoa:

| | quem pode |
|---|---|
| **escrever** | qualquer participante, inclusive o leitor — acompanhar e responder é ver, não mexer |
| **editar** | **só o autor**, nem o dono do quadro |
| **apagar** | o autor no próprio; quem administra o quadro, em qualquer um |

A assimetria entre editar e apagar é deliberada: tirar do quadro o que não serve
é responsabilidade de quem administra, mas reescrever é pôr palavras na boca de
outra pessoa.

Um comentário editado carrega a marca disso (`editadoEm`), e o card mostra
quantos tem — a contagem vem junto com o quadro, numa consulta só.

**Histórico** é o que aconteceu com o card: quem criou, moveu, renomeou,
apagou e comentou. Não tem tabela própria — é um *read model* sobre o log de
eventos que a reconexão já usava.

O evento guarda **nomes**, e não só ids, e isso é decisão: um log registra o que
era verdade **no momento**. Resolver o id na hora de ler mostraria o título de
hoje numa frase sobre ontem — "moveu Migração para Pronto" viraria outra coisa
só porque alguém renomeou a coluna depois — e não mostraria nada quando a coluna
já tivesse sido apagada. Pelo mesmo motivo o evento de card apagado leva o nome
do card: depois do `DELETE` não há de onde tirá-lo.

**Prazo** é opcional. O campo `vencido` vem calculado **pelo servidor** — o
relógio do navegador pode estar errado, e um card vermelho por engano confunde
mais do que ajuda.

**Checklists** têm itens marcáveis; o card mostra o progresso (`3/7`).

**Anexos** são link ou arquivo. Três decisões que valem saber antes de mexer:

- **lista de permissão de tipos**, não de bloqueio. `text/html` e
  `image/svg+xml` ficam de fora de propósito: servidos da nossa origem,
  executariam script na nossa origem;
- **o nome no disco é sorteado**, nunca derivado do que veio de quem enviou —
  nome de arquivo é entrada do usuário, e entrada do usuário não vira caminho. O
  nome original sobrevive só como rótulo;
- **o download passa pela API** (`GET /anexos/{id}`), e não por um caminho
  estático: é lá que se confere se quem pede participa do quadro. A resposta sai
  como `attachment` com `nosniff`.

O arquivo vai para um volume próprio (`ANEXOS_DIR`), não para o banco — que
incharia backup e restore de um schema que guarda texto curto no resto todo. Por
isso o backup tem [dois artefatos](producao.md#backup).

A **descrição** aceita um subconjunto de Markdown, renderizado por
[`lib/markdown.ts`](../frontend/src/lib/markdown.ts) — escrito à mão, sem
biblioteca: o texto é escapado inteiro **antes** de as marcas virarem tags, então
nada que alguém escreveu chega ao HTML como marcação.

---

## Ordenação: por que uma chave de texto

Arrastar um card não pode renumerar a coluna inteira. Com posições `1, 2, 3, 4`,
soltar um item no meio obrigaria a reescrever todos abaixo dele — muitas linhas
alteradas e corrida garantida com duas pessoas no mesmo quadro. A saída é dar a
cada item uma posição *entre* as dos vizinhos, escrevendo uma linha só.

A primeira tentativa foi um **float**, e ela tinha um fundo raso: dividir sempre
o mesmo intervalo esgotava a mantissa de 53 bits em **52 inserções seguidas no
mesmo ponto**, e o movimento respondia `409` — um erro que a pessoa não tinha
como resolver pela interface. Cards e colunas passaram a usar uma **chave
textual**: entre `"b"` e `"c"` cabe `"bn"`, e entre `"b"` e `"bn"` cabe `"bg"`.

No papel isso é infinito. Na prática a chave mora num `VARCHAR(200)`, e o fundo
continua existindo — só que **catorze vezes mais fundo**: medido, a chave cresce
cerca de 0,27 caractere por movimento no mesmo ponto, e o teto chega perto de
**750 reordenações consecutivas exatamente no mesmo lugar**. O domínio conhece
esse teto e responde `409` por conta própria; se ele se calasse, quem reclamaria
seria o driver do Postgres, e um erro previsto viraria `500`.

Se esse dia chegar, a saída **não** é uma coluna mais larga — isso só adia o
mesmo problema. É redistribuir as chaves daquela lista, aceitando de propósito a
reescrita em massa que este esquema evita no caso comum.

```
antes:   A("b")   B("n")           C("t")
                      ▲ solta aqui
depois:  A("b")   C("g")   B("n")          ← só C foi escrito
```

Duas invariantes sustentam o esquema:

- **Nenhuma chave termina no menor caractere.** Sem isso, uma chave `"a"` seria
  um beco sem saída: não existe string entre `""` e `"a"`, e inserir no topo
  passaria a ser impossível.
- **A chave nova é sorteada dentro da folga**, não fixada no meio exato. Duas
  pessoas que soltam um card no mesmo ponto ao mesmo tempo calculariam a mesma
  chave determinística — e a partir daí não caberia mais nada entre elas. O
  sorteio faz o empate ser improvável em vez de garantido.

**A API recebe os vizinhos, não a chave.** O cliente manda `anteriorId` e
`proximoId`; quem calcula é o servidor. Três razões:

1. a cópia do quadro na tela pode estar velha, e calcular entre vizinhos que já
   se moveram põe o item no lugar errado;
2. o servidor é o único lugar onde os valores reais estão;
3. chave vinda do cliente é entrada do usuário — embaralharia um quadro inteiro.

O card continua se movendo na tela na hora. Ele só não decide a chave.

A ordenação por chave depende de comparar texto **byte a byte**: a consulta e o
índice usam `COLLATE "C"`, porque a collation padrão do banco pode ignorar caixa
ou tratar acentos, e aí a ordem lida não seria a que o domínio calculou.

**Etiqueta e checklist continuam em float**, e isso não é dívida: elas só
acrescentam no fim, nunca inserem entre dois vizinhos. Sem divisão repetida do
mesmo intervalo, o limite da mantissa não é alcançável. Se um dia a etiqueta
ganhar arrastar-e-soltar, ela migra — o cálculo já está pronto.

A troca foi feita em **dois deploys**, como manda qualquer aperto de schema: a
coluna `chave` entrou anulável (*expand*), o código passou a escrevê-la, um
comando **do domínio** preencheu as linhas antigas preservando a ordem que
tinham, e só o deploy seguinte apertou a coluna e derrubou a `posicao`
(*contract*) — ver [PLANO.md](../PLANO.md), fase 9.

---

## Duas pessoas no mesmo card

Editar não é primeiro-a-chegar nem último-a-vencer: é **quem gravou primeiro
ganha, e o segundo é avisado**.

Cada card tem uma `version`, que sobe a cada escrita. A tela manda a versão que
estava mostrando; se o banco já foi além, a API responde **409** e a tela diz
"alguém alterou este card enquanto você escrevia" — mantendo o texto digitado
para ser copiado antes de trazer a versão nova.

Sobrescrever seria pior do que parece: o trabalho da outra pessoa sumiria **sem
ninguém ficar sabendo**. É o *lost update* clássico, e o isolamento padrão do
Postgres (`READ COMMITTED`) não protege contra ele — os dois UPDATEs são
válidos isoladamente.

São duas redes, com alcances diferentes:

- a **conferência no usecase** pega o conflito lento: abriu o card, foi tomar
  café, salvou meia hora depois;
- o **`WHERE version` do SQL** pega o simultâneo: duas requisições que leram a
  mesma linha e escrevem no mesmo instante, quando nenhuma das versões
  informadas está errada.

**Arrastar não confere versão.** Mover é posicional, a última pessoa a soltar
decide, e não há texto de ninguém para perder.

## Quem está no quadro agora

Os avatares no cabeçalho vêm do **mapa de conexões do hub**, não do banco. É
estado efêmero: não tem tabela, não tem migration, e morre com o processo — o
que é correto, porque "quem está olhando agora" não é fato histórico.

Duas abas da mesma conta são duas conexões e **um** avatar. Sem deduplicar, quem
abrisse o quadro em dois monitores apareceria como duas pessoas, e a contagem
deixaria de significar algo.

## Quem cai e volta

Uma conexão de horas cai — é o estado normal, não a exceção. O que não pode é
voltar fingindo que nada aconteceu.

Cada mudança no quadro entra num log com um `seq` crescente. O cliente guarda o
último que aplicou; ao reconectar, pergunta `?desde=41` e recebe o intervalo
**antes** de voltar ao vivo. A ordem importa: entregar o passado depois do
presente faria a tela aplicar o velho por cima do novo.

A assinatura da sala acontece **antes** da reposição, de propósito. Isso faz os
eventos que chegam durante a reposição ficarem na fila e serem entregues em
seguida — sem buraco entre o fim da história e o começo do ao vivo. O preço é
que alguns chegam duas vezes, e é por isso que o cliente descarta tudo com `seq`
menor ou igual ao último aplicado. **Preferimos repetir a arriscar buraco**: o
`seq` torna a repetição inofensiva, e nada torna um buraco perceptível.

Intervalo grande demais (mais de 200 eventos) não é reproduzido: o servidor
manda recarregar o quadro inteiro. Uma requisição resolve, e o resultado é o
mesmo — com a vantagem de ser sempre correto.

## Sessão

Token opaco, gerado com `crypto/rand`. O banco guarda **só o SHA-256** dele: um
vazamento do dump não devolve sessão utilizável.

O cookie é `HttpOnly` + `SameSite=Lax`, e em produção ganha `Secure` e o prefixo
`__Host-` — que exige `Path=/` e **nenhum** `Domain`, amarrando o cookie a um
host exato. É por isso que o stacktrack vive num subdomínio próprio, e não num
caminho do domínio do vizinho: o navegador enviaria o token para ele também.
