# Migrations

Aplicadas pelo Flyway (serviço `flyway` do `docker-compose.yml`) antes de a API
subir. Nomenclatura: `V{n}__descricao_em_snake_case.sql`, numeração sequencial e
**sem buracos**.

Na fase 0 ainda não havia tabela; a primeira migration
(`V1__cria_tabela_usuarios.sql`) entrou na fase 1. O diretório atual já contém o
schema completo consolidado.

As regras do que pode e do que não pode entrar numa migration estão no
[CLAUDE.md](../../CLAUDE.md#migrations-banco-de-dados) — em resumo: nada de
`DEFAULT`/`CHECK` de regra de negócio, e **migration não escreve dado**.

## Base consolidada e migrations incrementais

As migrations `V1`–`V14` formam a base consolidada: cada arquivo cria uma
tabela (ou o par tabela + tabela de ligação, quando as duas só fazem sentido
juntas). As `V15` e seguintes são a história incremental posterior e, por isso,
usam `ALTER TABLE` quando a mudança exige.

Isso é fotografia, não regra para a próxima mudança. Os arquivos atuais já
foram aplicados e são imutáveis: uma coluna nova em tabela existente deve entrar
por `ALTER TABLE` numa migration de número novo, seguindo o ciclo
expand/contract do `CLAUDE.md`. Editar um arquivo atual mudaria o checksum e
derrubaria a partida do Flyway.

## O conjunto foi consolidado uma vez

Este diretório teve 21 arquivos, com sete `ALTER TABLE` espalhados: `prazo` e
`cor` chegaram depois em `cards`, `fundo` em `boards`, `card_id` em
`board_events`, e a chave de ordenação veio num ciclo expand/contract de dois
passos que também derrubou a `posicao` em ponto flutuante.

Eles foram dissolvidos nos `CREATE TABLE` correspondentes quando o banco de
produção foi zerado — o único momento em que isso é seguro. A consolidação foi
verificada aplicando os dois conjuntos em bancos separados e comparando colunas,
tipos, nulidade, índices e constraints: a única diferença deliberada foram
`cards.arquivado_em` e `colunas.arquivado_em`, órfãs do arquivamento retirado,
que simplesmente deixaram de existir.

Relatos anteriores à consolidação podem mencionar outros números. Eles são
somente históricos; a sequência vigente é sempre a dos arquivos deste
diretório e a próxima migration usa o próximo número livre.

## O contract da chave de ordenação: unicidade da posição

A etapa A2 do [PLANO.md](../../PLANO.md) pede unicidade da posição dentro do
contêiner, e o **`V18` é essa migration** — o terceiro passo de um ciclo
expand/contract que levou três deploys:

1. **Expand** — nada a fazer no schema.
2. **Código novo em produção**: o domínio passa a detectar chave repetida e a
   redistribuir as chaves do contêiner antes de calcular a próxima, tudo sob o
   lock do quadro. Ver `internal/usecase/board/ordenacao.go` e
   `internal/domain/ordem/redistribuir.go`. A partir daqui, nenhuma versão em
   produção produz chave repetida.
3. **Contract** (`V18`): os índices únicos entram, e os não-únicos que existiam
   sobre as mesmas colunas saem — o único cobre as consultas dos dois.

A espera entre 2 e 3 não foi cerimônia. `UNIQUE` novo é migration que APERTA, e
o `CLAUDE.md` exige dois deploys para elas: durante o deploy a versão anterior
da aplicação continua no ar, e era ela que ainda conseguia gravar duas chaves
iguais numa rajada de inserções no mesmo ponto. Com o índice já criado, essa
gravação passaria a ser recusada — e a versão anterior quebraria no `INSERT`,
sem que o Flyway (forward-only) pudesse voltar atrás.

### Pré-condição do `V18`, antes do deploy que o aplica

O `CREATE UNIQUE INDEX` falha se ainda houver duplicidade herdada. A consulta
que responde antes de tentar:

```sql
SELECT coluna_id, chave, count(*)
  FROM cards GROUP BY coluna_id, chave HAVING count(*) > 1
UNION ALL
SELECT board_id, chave, count(*)
  FROM colunas GROUP BY board_id, chave HAVING count(*) > 1;
```

Vindo linha, o contract para — e para junto a partida do Flyway, que é o que
faz desta conferência um passo do deploy, e não uma recomendação. Antes dele
roda o comando de manutenção **pelo domínio**, que lista cada contêiner afetado,
toma o lock do quadro, redistribui as chaves com a mesma regra da aplicação e
emite relatório verificável:

```sh
# Na imagem de produção, que já traz o binário:
docker compose -f docker-compose.prod.yml run --rm --entrypoint manutencao api \
    reparar-ordenacao --conferir     # só relata; sai 1 se houver trabalho
docker compose -f docker-compose.prod.yml run --rm --entrypoint manutencao api \
    reparar-ordenacao                # repara e reconfere
```

O comando é idempotente e seguro com a aplicação no ar: ele repara **um quadro
por transação**, sob o lock daquele quadro, e relê as chaves lá dentro. Sai 0
apenas quando a reconferência não encontra mais duplicidade — é essa saída que
autoriza o contract. Ver `internal/usecase/board/reparo.go` e
`cmd/manutencao/`.

Nunca substituir por `UPDATE` na migration: a decisão de com que chave as linhas
antigas ficam é do domínio, e em SQL ela vira uma segunda fonte da verdade, sem
teste e sem conserto.

### Estimativa de lock

`CREATE UNIQUE INDEX` sem `CONCURRENTLY` toma `SHARE` na tabela e bloqueia
escrita enquanto constrói. O limite de 1.000 cards é por quadro, mas o índice
varre a tabela inteira; portanto a janela não pode ser estimada a partir desse
limite. Antes do deploy que aplica o `V18`, registrar quantidade de linhas,
tamanho das tabelas, tempo de construção numa cópia representativa e teto de
lock aceito. Se não couber na janela de manutenção, usar criação concorrente com
a configuração do Flyway que execute essa migration fora de transação.

### O que o contract muda para os testes

`backend/test/repository/reparo_test.go` derruba os dois índices antes de forçar
duplicidade e os recria no fim — a recriação É a asserção do contract, e é o que
o operador faz na janela de manutenção. Fixture que grave `chave` direto no
banco precisa do mesmo tratamento, ou o banco a recusa antes de o teste começar.
