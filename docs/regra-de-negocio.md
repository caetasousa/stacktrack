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

O dono convida **por email**. Duas situações:

- **já existe conta com aquele email** → a pessoa entra no quadro na hora;
- **não existe** → nasce um convite com token, e o dono copia o link e envia por
  onde quiser.

Não há envio de email no projeto, então o link é o produto do convite, não um
efeito colateral dele. Quando o envio existir, ele só passa a mandar o mesmo
link.

Revogar um convite invalida o link. O token é gerado com `crypto/rand`.

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

## Ordenação: por que fracionária

A posição de cards e colunas é um **float**, não um inteiro sequencial. Com
`1, 2, 3, 4`, arrastar um item para o meio obrigaria a reescrever a numeração de
todos abaixo dele — muitas linhas alteradas e corrida garantida com duas pessoas
no mesmo quadro. Com fração, inserir entre `2048` e `3072` é gravar `2560`:

```
antes:   A(2048)   B(3072)            C(4096)
                       ▲ solta aqui
depois:  A(2048)   C(2560)   B(3072)          ← só C foi escrito
```

**A API recebe os vizinhos, não a posição.** O cliente manda `anteriorId` e
`proximoId`; quem calcula o número é o servidor. Três razões:

1. a cópia do quadro na tela pode estar velha, e a média entre posições que já
   mudaram põe o item no lugar errado;
2. o esgotamento da precisão só é detectável onde os valores reais estão;
3. posição vinda do cliente é entrada do usuário — embaralharia a ordem de um
   quadro inteiro.

O card continua se movendo na tela na hora. Ele só não decide o número.

**O limite do `double precision` é real e está medido.** Dividir sempre o mesmo
intervalo esgota a mantissa de 53 bits em **52 inserções seguidas no mesmo
ponto**, e há teste que mede isso. Quando acontece, o movimento responde `409`
em vez de gravar em silêncio duas posições iguais. A saída definitiva é trocar o
float por chave textual — é a fase 9 do [PLANO.md](../PLANO.md).

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
