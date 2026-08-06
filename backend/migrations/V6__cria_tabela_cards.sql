-- As tarefas. Pertencem à coluna, e é mudando de coluna que o card "anda" pelo
-- fluxo.
--
-- version existe desde já, ainda sem ninguém checá-la: ela é o contador do
-- bloqueio otimista da fase 6 (UPDATE ... WHERE id = $1 AND version = $2, com
-- 409 quando nenhuma linha é afetada). Nasce aqui porque acrescentar coluna
-- obrigatória depois exigiria o ciclo expand/contract de dois deploys — e nesta
-- fase a tabela ainda está vazia.
CREATE TABLE cards (
    id            UUID             PRIMARY KEY,
    coluna_id     UUID             NOT NULL REFERENCES colunas (id) ON DELETE CASCADE,
    titulo        VARCHAR(200)     NOT NULL,
    descricao     TEXT             NOT NULL,
    posicao       DOUBLE PRECISION NOT NULL,
    version       INTEGER          NOT NULL,
    criado_em     TIMESTAMPTZ      NOT NULL,
    atualizado_em TIMESTAMPTZ      NOT NULL
);

CREATE INDEX idx_cards_coluna ON cards (coluna_id);
