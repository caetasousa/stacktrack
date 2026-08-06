-- O quadro. Não guarda quem é o dono: isso é vínculo, e vínculo tem tabela
-- própria (board_membros) — no dia em que o quadro tiver dois administradores,
-- uma coluna dono_id teria de virar tabela de qualquer forma.
CREATE TABLE boards (
    id            UUID         PRIMARY KEY,
    titulo        VARCHAR(120) NOT NULL,
    criado_em     TIMESTAMPTZ  NOT NULL,
    atualizado_em TIMESTAMPTZ  NOT NULL
);
