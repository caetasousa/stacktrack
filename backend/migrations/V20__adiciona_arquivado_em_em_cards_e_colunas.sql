-- Arquivar em vez de apagar: o card sai do quadro e continua existindo.
--
-- Hoje o DELETE é definitivo e leva CINCO tabelas por cascata — comentários,
-- checklists, anexos, responsáveis e etiquetas aplicadas. Não há desfazer.
--
-- Este é o caso mais fácil do ciclo expand/contract, e o único até agora que
-- **não tem contract**: a coluna nasce anulável e assim fica. NULL significa
-- "não arquivado", que já é o estado verdadeiro de toda linha existente — não
-- há backfill a fazer, nem decisão de negócio escondida num DEFAULT. Comparar
-- com a V18/V19, onde `chave` nascia sem valor nenhum para as linhas antigas e
-- precisou de um comando do domínio antes de poder virar NOT NULL.
--
-- É TIMESTAMPTZ e não BOOLEAN de propósito: "quando foi arquivado" responde
-- tudo o que um booleano responderia, e mais — ordenar a tela de arquivados
-- pelo mais recente, e um dia limpar o que está arquivado há muito tempo.
ALTER TABLE cards   ADD COLUMN arquivado_em TIMESTAMPTZ;
ALTER TABLE colunas ADD COLUMN arquivado_em TIMESTAMPTZ;

-- O índice é PARCIAL, e a condição é a mesma que as leituras do quadro usam.
--
-- A leitura quente é "os cards não arquivados desta coluna", e um índice parcial
-- só indexa as linhas que interessam: ele não cresce com o arquivo morto, que é
-- justamente a parte que tende a virar a maioria das linhas com o tempo.
--
-- A chave vai junto, na mesma collation do índice que a V18 criou: assim a
-- consulta do quadro resolve filtro e ordenação no mesmo índice.
CREATE INDEX idx_cards_coluna_ativos
    ON cards (coluna_id, chave COLLATE "C")
    WHERE arquivado_em IS NULL;

CREATE INDEX idx_colunas_board_ativas
    ON colunas (board_id, chave COLLATE "C")
    WHERE arquivado_em IS NULL;

-- E o caminho inverso, para a tela de arquivados: os mais recentes primeiro.
CREATE INDEX idx_cards_arquivados   ON cards   (coluna_id, arquivado_em DESC) WHERE arquivado_em IS NOT NULL;
CREATE INDEX idx_colunas_arquivadas ON colunas (board_id, arquivado_em DESC)  WHERE arquivado_em IS NOT NULL;
