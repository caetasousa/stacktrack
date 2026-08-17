-- As etapas do fluxo, da esquerda para a direita.
CREATE TABLE colunas (
    id            UUID         PRIMARY KEY,
    board_id      UUID         NOT NULL REFERENCES boards (id) ON DELETE CASCADE,
    titulo        VARCHAR(120) NOT NULL,
    -- A ordem é uma chave de TEXTO, não um número, e isso é conserto de um
    -- defeito real: com `posicao` em DOUBLE PRECISION, inserir sempre no mesmo
    -- ponto esgota a mantissa em 52 inserções, e a partir daí o movimento
    -- responde 409 com um erro que ninguém resolve pela interface. Entre "b" e
    -- "c" cabe "bn", infinitamente.
    chave         VARCHAR(200) NOT NULL,
    -- Nome da cor na paleta ('verde'), como em etiquetas e no fundo do quadro:
    -- a paleta muda com o tema claro/escuro, e o hex pertence ao CSS.
    -- Anulável porque a cor é opcional — coluna sem cor usa o visual padrão.
    cor           VARCHAR(20),
    criado_em     TIMESTAMPTZ  NOT NULL,
    atualizado_em TIMESTAMPTZ  NOT NULL
);

CREATE INDEX idx_colunas_board ON colunas (board_id);

-- ⚠️ COLLATE "C" não é detalhe: a ordenação de texto no Postgres depende da
-- collation do banco, e uma que ignore caixa ou trate acentos ordenaria
-- diferente do que o domínio assume ao gerar a chave. "C" é a ordem de BYTES,
-- que é exatamente a que `ordem.ChaveEntre` usa para decidir o que vem antes.
--
-- O índice precisa da mesma collation da consulta: com collations diferentes o
-- Postgres simplesmente não o usa, e a ordenação vira sort em memória a cada
-- leitura do quadro.
CREATE INDEX idx_colunas_board_chave ON colunas (board_id, chave COLLATE "C");
