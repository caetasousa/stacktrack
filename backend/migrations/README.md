# Migrations

Aplicadas pelo Flyway (serviço `flyway` do `docker-compose.yml`) antes de a API
subir. Nomenclatura: `V{n}__descricao_em_snake_case.sql`, numeração sequencial e
**sem buracos**.

A fase 0 não tem tabela nenhuma — a primeira migration (`V1__cria_tabela_usuarios.sql`)
chega na fase 1. Com a pasta vazia, o Flyway registra "no migrations found" e
termina com sucesso, que é o esperado.

As regras do que pode e do que não pode entrar numa migration estão no
[CLAUDE.md](../../CLAUDE.md#migrations-banco-de-dados) — em resumo: nada de
`DEFAULT`/`CHECK` de regra de negócio, e **migration não escreve dado**.

## Uma migration por tabela

Cada arquivo cria uma tabela (ou o par tabela + tabela de ligação, quando as
duas só fazem sentido juntas). **Não há `ALTER TABLE` no diretório**: uma coluna
nova de uma tabela que já existe entra no `CREATE TABLE` dela.

Isso vale **enquanto nada estiver aplicado em produção**. No dia em que houver
banco com histórico, a regra se inverte e passa a valer a do CLAUDE.md: migration
aplicada é imutável, e coluna nova vira `ALTER TABLE` num arquivo novo. Editar um
arquivo já aplicado muda o checksum e derruba a partida do Flyway.

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

⚠️ **O `PLANO.md` cita números antigos** (`V18`/`V19` para a chave de ordenação,
`V20` para o arquivamento). Ele é o registro do roteiro, e descreve o que
aconteceu em cada fase — os números de lá são os de antes desta consolidação.
