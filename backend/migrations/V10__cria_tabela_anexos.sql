-- Anexos do card: arquivo enviado ou link.
--
-- Os dois na mesma tabela porque, para quem usa, são a mesma coisa — uma
-- referência pendurada no card. O que muda é onde o conteúdo mora, e é isso que
-- `tipo` diz: 'arquivo' guarda o nome do arquivo no disco em `caminho`;
-- 'link' guarda a URL em `url`. Duas tabelas obrigariam toda listagem a unir as
-- duas só para mostrar uma lista.
--
-- O binário NÃO fica aqui. BYTEA incharia backup e restore de um banco que
-- guarda texto curto no resto todo; o arquivo vai para o volume apontado por
-- ANEXOS_DIR.
CREATE TABLE anexos (
    id         UUID         PRIMARY KEY,
    card_id    UUID         NOT NULL REFERENCES cards (id)    ON DELETE CASCADE,
    tipo       VARCHAR(10)  NOT NULL,
    -- O que a pessoa vê: nome original do arquivo, ou o título dado ao link.
    nome       VARCHAR(255) NOT NULL,
    -- Preenchido só para tipo 'link'.
    url        TEXT,
    -- Preenchido só para tipo 'arquivo': nome do arquivo dentro de ANEXOS_DIR.
    -- Nunca o nome original — ele vem de quem envia e não pode virar caminho.
    caminho    VARCHAR(255),
    tamanho    BIGINT,
    mime       VARCHAR(120),
    criado_por UUID         NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,
    criado_em  TIMESTAMPTZ  NOT NULL
);

CREATE INDEX idx_anexos_card ON anexos (card_id);
