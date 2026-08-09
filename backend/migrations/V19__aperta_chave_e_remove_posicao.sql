-- O CONTRACT da fase 9: a chave vira obrigatória e a posição em float sai.
--
-- Esta é a SEGUNDA metade do ciclo, e ela só é segura porque a primeira já está
-- em produção há um deploy:
--
--   V18 (expand)   `chave` nasceu anulável, e as duas colunas passaram a ser
--                  escritas juntas;
--   código novo    o `mover` passou a calcular pela chave, a leitura a ordenar
--                  por ela, e o comando de backfill preencheu as linhas antigas;
--   V19 (aqui)     só agora o aperto, quando não existe mais linha sem chave
--                  nem versão no ar que dependa de `posicao`.
--
-- ⚠️ PRÉ-REQUISITO, e ele não é verificável por SQL sem escrever dado: o
-- backfill precisa ter rodado. Se sobrar UMA linha com `chave` nula, o
-- SET NOT NULL falha e o deploy para — o que é o comportamento certo (falhar
-- alto em vez de corromper), mas dá trabalho no pior momento. Antes de
-- publicar, confira que o resultado é zero:
--
--   SELECT count(*) FROM cards   WHERE chave IS NULL;
--   SELECT count(*) FROM colunas WHERE chave IS NULL;
--
-- Se ainda assim falhar, o banco NÃO fica pela metade: no Postgres o DDL é
-- transacional, e o Flyway roda cada migration em uma transação. Verificado
-- com uma linha de chave nula — a V19 abortou no primeiro SET NOT NULL e as
-- duas colunas voltaram anuláveis, com `posicao` intacta. O estrago seria a
-- versão em produção não subir, não o schema meio migrado.
--
-- A migration NÃO preenche nada: preencher é decisão de negócio e mora no
-- domínio, como manda o CLAUDE.md. O comando que fez isso era
-- `internal/usecase/board/backfill.go`; ele saiu no mesmo commit desta
-- migration, porque um backfill que já rodou e não pode mais rodar (a coluna
-- de origem não existe) é código morto que só confunde. Para reler o que ele
-- fazia: `git log --diff-filter=D -- backend/internal/usecase/board/backfill.go`.
--
-- Declaração para o guard de expand/contract. Ele reprovaria estas quatro
-- mudanças por padrão, e é isso que se quer no dia em que uma delas for
-- acidente. Listá-las aqui autoriza EXATAMENTE estas — qualquer outra quebra
-- na mesma migration continua parando o build.
-- CONTRACT: cards.chave, colunas.chave, cards.posicao, colunas.posicao

ALTER TABLE cards   ALTER COLUMN chave SET NOT NULL;
ALTER TABLE colunas ALTER COLUMN chave SET NOT NULL;

-- A posição sai por último. Os índices que a usavam vão junto com ela, sem
-- precisar de DROP INDEX: o Postgres remove os índices que dependem da coluna.
ALTER TABLE cards   DROP COLUMN posicao;
ALTER TABLE colunas DROP COLUMN posicao;
