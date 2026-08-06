-- As etapas do fluxo, da esquerda para a direita.
--
-- posicao é fracionária, e não um inteiro sequencial: inserir entre duas
-- colunas passa a ser gravar o ponto médio (2.5 entre 2.0 e 3.0), uma linha
-- só, em vez de renumerar todas as seguintes. A fase 4 explora isso de
-- verdade no arrastar-e-soltar — e encontra o limite de precisão do
-- double precision, que a fase 9 conserta.
CREATE TABLE colunas (
    id            UUID             PRIMARY KEY,
    board_id      UUID             NOT NULL REFERENCES boards (id) ON DELETE CASCADE,
    titulo        VARCHAR(120)     NOT NULL,
    posicao       DOUBLE PRECISION NOT NULL,
    criado_em     TIMESTAMPTZ      NOT NULL,
    atualizado_em TIMESTAMPTZ      NOT NULL
);

CREATE INDEX idx_colunas_board ON colunas (board_id);
