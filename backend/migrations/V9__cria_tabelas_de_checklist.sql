-- Checklists do card. Plural de propósito: no template do Trello cada card tem
-- duas — "To-do List" e "Task Review" — e tratá-las como uma lista única
-- perderia essa separação.
CREATE TABLE checklists (
    id            UUID             PRIMARY KEY,
    card_id       UUID             NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    titulo        VARCHAR(120)     NOT NULL,
    posicao       DOUBLE PRECISION NOT NULL,
    criado_em     TIMESTAMPTZ      NOT NULL,
    atualizado_em TIMESTAMPTZ      NOT NULL
);

CREATE INDEX idx_checklists_card ON checklists (card_id);

CREATE TABLE checklist_itens (
    id            UUID             PRIMARY KEY,
    checklist_id  UUID             NOT NULL REFERENCES checklists (id) ON DELETE CASCADE,
    texto         VARCHAR(500)     NOT NULL,
    -- Sem DEFAULT false: se um item nasce marcado ou desmarcado é decisão do
    -- domínio, e um padrão aqui seria uma segunda fonte da verdade.
    concluido     BOOLEAN          NOT NULL,
    posicao       DOUBLE PRECISION NOT NULL,
    criado_em     TIMESTAMPTZ      NOT NULL,
    atualizado_em TIMESTAMPTZ      NOT NULL
);

CREATE INDEX idx_checklist_itens_checklist ON checklist_itens (checklist_id);
