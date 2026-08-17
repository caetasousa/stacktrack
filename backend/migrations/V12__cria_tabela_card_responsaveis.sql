-- Quem é responsável por cada card.
--
-- É o que faz o quadro responder "o que é meu?", e não só "o que existe". Um
-- card pode ter mais de um responsável — trabalho em par é normal —, e por isso
-- é tabela de ligação, não uma coluna responsavel_id em cards.
CREATE TABLE card_responsaveis (
    card_id    UUID        NOT NULL REFERENCES cards (id)    ON DELETE CASCADE,
    -- Aponta para usuarios, e NÃO para board_membros. O vínculo com o quadro é
    -- regra de negócio ("só dá para atribuir quem participa"), e regra de
    -- negócio mora no domínio: uma chave estrangeira composta aqui seria uma
    -- segunda fonte da verdade, e ainda amarraria a atribuição ao formato atual
    -- da tabela de membros.
    usuario_id UUID        NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,
    criado_em  TIMESTAMPTZ NOT NULL,
    -- A chave composta já impede atribuir a mesma pessoa duas vezes ao mesmo
    -- card, sem precisar de UNIQUE separado nem de checagem no código.
    PRIMARY KEY (card_id, usuario_id)
);

-- A pergunta "quais cards são meus?" parte da PESSOA, e a chave primária começa
-- por card_id — ela não serve para essa direção.
CREATE INDEX idx_card_responsaveis_usuario ON card_responsaveis (usuario_id);
