-- A chave TEXTUAL de ordenação — o conserto do limite do double precision.
--
-- `posicao` em float funciona até a mantissa acabar: o teste do domínio mede o
-- esgotamento em 52 inserções seguidas no mesmo ponto, e a partir daí o
-- movimento responde 409 com um erro que a pessoa não tem como resolver pela
-- interface. Com chave textual isso deixa de existir — entre "b" e "c" cabe
-- "bn", infinitamente.
--
-- Este é o EXPAND do ciclo de dois deploys, e por isso a coluna nasce ANULÁVEL:
-- a versão anterior da aplicação continua no ar durante o deploy e não sabe
-- escrever nela. Obrigatória de saída, ela quebraria todo INSERT da versão
-- velha — e o rollback, que é para onde se volta quando algo dá errado, subiria
-- sem erro e falharia no primeiro card criado.
--
-- O CONTRACT (`SET NOT NULL` e `DROP` de `posicao`) é a migration do deploy
-- SEGUINTE, depois de o backfill ter rodado. Ele não pode vir junto: entre
-- aplicar a migration e o código novo estar no ar existe uma janela, e é ela
-- que o ciclo de dois passos protege.
ALTER TABLE cards   ADD COLUMN chave VARCHAR(200);
ALTER TABLE colunas ADD COLUMN chave VARCHAR(200);

-- Os índices que a leitura ordenada usa.
--
-- ⚠️ COLLATE "C" não é detalhe: a ordenação de texto no Postgres depende da
-- collation do banco, e uma que ignore caixa ou trate acentos ordenaria
-- diferente do que o domínio assume ao gerar a chave. "C" é a ordem de BYTES,
-- que é exatamente a que `ordem.ChaveEntre` usa para decidir o que vem antes.
--
-- O índice precisa da mesma collation da consulta: com collations diferentes o
-- Postgres simplesmente não o usa, e a ordenação vira sort em memória a cada
-- leitura do quadro.
CREATE INDEX idx_cards_coluna_chave   ON cards   (coluna_id, chave COLLATE "C");
CREATE INDEX idx_colunas_board_chave  ON colunas (board_id, chave COLLATE "C");
