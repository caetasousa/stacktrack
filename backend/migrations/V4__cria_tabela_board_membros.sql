-- Quem participa de qual quadro, e com que papel. É esta tabela que responde
-- "você pode ver este quadro?" — a pergunta que toda rota da fase 2 faz antes
-- de qualquer outra coisa.
--
-- Sem CHECK no papel: quais papéis existem e o que cada um pode fazer é regra
-- de negócio, e mora no domínio (internal/domain/membro). Uma constraint aqui
-- seria uma segunda fonte da verdade, que só se descobre desatualizada em
-- produção.
CREATE TABLE board_membros (
    board_id   UUID        NOT NULL REFERENCES boards (id)   ON DELETE CASCADE,
    usuario_id UUID        NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,
    papel      VARCHAR(10) NOT NULL,
    criado_em  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (board_id, usuario_id)
);

-- A listagem de quadros parte do usuário, não do quadro: a chave primária
-- começa por board_id e não serve para essa direção.
CREATE INDEX idx_board_membros_usuario ON board_membros (usuario_id);
