-- As tarefas. Pertencem à coluna, e é mudando de coluna que o card "anda" pelo
-- fluxo.
CREATE TABLE cards (
    id            UUID         PRIMARY KEY,
    coluna_id     UUID         NOT NULL REFERENCES colunas (id) ON DELETE CASCADE,
    titulo        VARCHAR(200) NOT NULL,
    descricao     TEXT         NOT NULL,
    -- Chave textual de ordenação, pelo mesmo motivo da coluna: `posicao` em
    -- ponto flutuante esgota a precisão em 52 inserções no mesmo ponto.
    chave         VARCHAR(200) NOT NULL,
    -- O contador do bloqueio otimista: UPDATE ... WHERE id = $1 AND version =
    -- $2, com 409 quando nenhuma linha é afetada. É o que impede duas pessoas
    -- editando o mesmo card de uma sobrescrever a outra em silêncio.
    version       INTEGER      NOT NULL,
    -- Data de entrega. Anulável: card sem prazo é o caso normal.
    prazo         TIMESTAMPTZ,
    -- Nome da cor na paleta; vira uma tarja na lateral do card. Anulável.
    cor           VARCHAR(20),
    criado_em     TIMESTAMPTZ  NOT NULL,
    atualizado_em TIMESTAMPTZ  NOT NULL
);

CREATE INDEX idx_cards_coluna ON cards (coluna_id);

-- Ver o aviso sobre COLLATE "C" em colunas: o índice precisa da mesma
-- collation da consulta, senão o Postgres não o usa e a ordenação vira sort em
-- memória a cada leitura do quadro.
CREATE INDEX idx_cards_coluna_chave ON cards (coluna_id, chave COLLATE "C");

-- O quadro filtra e ordena por prazo; sem índice isso vira varredura da tabela
-- inteira à medida que os cards se acumulam. Parcial porque a maioria dos cards
-- não tem prazo, e indexar os nulos só engordaria a árvore.
CREATE INDEX idx_cards_prazo ON cards (prazo) WHERE prazo IS NOT NULL;
