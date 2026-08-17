-- O quadro. Não guarda quem é o dono: isso é vínculo, e vínculo tem tabela
-- própria (board_membros) — no dia em que o quadro tiver dois administradores,
-- uma coluna dono_id teria de virar tabela de qualquer forma.
CREATE TABLE boards (
    id            UUID         PRIMARY KEY,
    titulo        VARCHAR(120) NOT NULL,
    -- Guarda o NOME do fundo escolhido (ex.: 'ardosia'), não uma cor nem uma
    -- URL: a paleta é decisão de design e vive no frontend, então gravar
    -- #1D2939 aqui congelaria no banco uma escolha que muda com o tema.
    --
    -- Anulável: quadro sem fundo escolhido usa o padrão, e o padrão é do
    -- domínio — não um DEFAULT daqui.
    fundo         VARCHAR(32),
    criado_em     TIMESTAMPTZ  NOT NULL,
    atualizado_em TIMESTAMPTZ  NOT NULL
);
