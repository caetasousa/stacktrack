-- Etiquetas do quadro. São do QUADRO, não do card: no template do Trello a
-- mesma "Flagged 🔴" aparece em vários cards, e renomeá-la precisa renomear em
-- todos de uma vez.
CREATE TABLE etiquetas (
    id        UUID         PRIMARY KEY,
    board_id  UUID         NOT NULL REFERENCES boards (id) ON DELETE CASCADE,
    nome      VARCHAR(60)  NOT NULL,
    -- Nome da cor na paleta (ex.: 'vermelho'), não o hex: a paleta é design e
    -- muda com o tema; o banco não pode congelar #F04438.
    cor       VARCHAR(20)  NOT NULL,
    posicao   DOUBLE PRECISION NOT NULL,
    criado_em TIMESTAMPTZ  NOT NULL
);

CREATE INDEX idx_etiquetas_board ON etiquetas (board_id);

-- Ligação entre card e etiqueta. A chave primária composta já garante que a
-- mesma etiqueta não entre duas vezes no mesmo card.
CREATE TABLE card_etiquetas (
    card_id     UUID        NOT NULL REFERENCES cards (id)     ON DELETE CASCADE,
    etiqueta_id UUID        NOT NULL REFERENCES etiquetas (id) ON DELETE CASCADE,
    criado_em   TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (card_id, etiqueta_id)
);

-- A leitura do quadro parte da etiqueta para saber quais cards a usam; a chave
-- primária começa por card_id e não serve para essa direção.
CREATE INDEX idx_card_etiquetas_etiqueta ON card_etiquetas (etiqueta_id);
