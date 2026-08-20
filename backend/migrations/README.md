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

## Por que o conjunto atual tem uma migration por tabela

Cada arquivo cria uma tabela (ou o par tabela + tabela de ligação, quando as
duas só fazem sentido juntas). **Não há `ALTER TABLE` no conjunto atual** porque
os arquivos foram consolidados uma vez, como explicado abaixo.

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

⚠️ **O `PLANO.md` preserva números antigos** ao narrar o roteiro: por exemplo,
`V15`/`V16` para responsáveis e comentários, `V18`/`V19` para a chave de
ordenação e `V20` para o arquivamento retirado. Eles são identificados como
históricos no próprio plano. A numeração vigente é sempre a dos arquivos deste
diretório.
