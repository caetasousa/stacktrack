# Colunas órfãs — existem no banco, e ninguém as usa

Este arquivo é lido por `test/repository/sql_cobre_as_colunas_test.go`.

Aquele guard cobra que toda coluna criada nas migrations apareça em algum SQL do
repositório. Ele existe porque isso já falhou duas vezes — `cards.prazo` e
`boards.fundo` atravessaram domínio, DTO e handler sem nunca serem gravados, e a
API respondia `200` enquanto o dado sumia.

Há um caso legítimo de coluna sem SQL, e ele aparece quando uma funcionalidade é
**retirada**: entre o código parar de usar a coluna e ela ser derrubada vão dois
deploys, e no meio-tempo ela fica no banco sem ninguém que a leia. Declarar aqui
é o que separa essa situação deliberada do defeito que o guard caça.

A declaração é cobrada nos **três** sentidos:

- coluna sem SQL e sem declaração → reprova;
- declaração de coluna que não existe mais nas migrations → reprova;
- coluna declarada que voltou a aparecer no SQL → reprova.

Uma autorização que sobra é a que ninguém relê no dia do acidente.

## Por que aqui, e não dentro da migration

Foi a primeira tentativa, e ela **quebra o deploy**. O Flyway guarda o checksum
de cada migration aplicada e valida na subida; editar o arquivo — ainda que só
para acrescentar um comentário — muda o checksum e o start falha:

```
Migration checksum mismatch for migration version 20
-> Applied to database : 960633743
-> Resolved locally    : 502790743
```

Migration aplicada é imutável, inclusive nos comentários. Este arquivo é `.md`,
então o Flyway não o enxerga: ele só recolhe os que casam com o padrão de nome
(`V*.sql`).

## Declarações

<!-- Uma por linha, no formato `- ORFA: tabela.coluna` seguido do porquê. -->

*(nenhuma no momento)*

A lista já teve `cards.arquivado_em` e `colunas.arquivado_em`, órfãs do
arquivamento que foi construído inteiro e depois retirado por decisão de
produto. Elas esperavam o contract do deploy seguinte.

O contract nunca precisou ser escrito: o banco de produção foi zerado e as
migrations foram consolidadas, então as duas colunas simplesmente deixaram de
existir. **Não foi o ciclo expand/contract funcionando — foi ele deixando de ser
necessário**, e é uma saída que só existe uma vez, com o banco vazio.

Este arquivo continua aqui porque o mecanismo continua valendo: no dia em que
uma funcionalidade for retirada com dado real no banco, a coluna vai ficar órfã
por um deploy e a declaração vai ter de aparecer.
