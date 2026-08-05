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
