-- Convite para participar de um quadro.
--
-- Existe para quem ainda NÃO tem conta: quem já tem entra direto como membro,
-- sem token nenhum. O dono copia o link e manda por onde quiser — enquanto não
-- houver envio de email, é o próprio dono quem entrega o convite.
--
-- Guarda só o SHA-256 do token, pelo mesmo motivo das sessões: o link é a
-- credencial inteira de quem o possui, e um vazamento do banco não pode
-- entregar convites utilizáveis.
CREATE TABLE convites_board (
    id         UUID         PRIMARY KEY,
    board_id   UUID         NOT NULL REFERENCES boards (id)   ON DELETE CASCADE,
    -- Para quem o convite foi feito. Normalizado pelo domínio, como em usuarios.
    email      VARCHAR(255) NOT NULL,
    papel      VARCHAR(10)  NOT NULL,
    token_hash CHAR(64)     NOT NULL UNIQUE,
    criado_por UUID         NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,
    criado_em  TIMESTAMPTZ  NOT NULL,
    expira_em  TIMESTAMPTZ  NOT NULL,
    -- NULL enquanto pendente. Aceito vira histórico: não some, para dar para
    -- responder "quem convidou fulano, e quando".
    aceito_em  TIMESTAMPTZ
);

-- Um convite pendente por email por quadro. Parcial porque a unicidade só vale
-- enquanto o convite está de pé: depois de aceito ou revogado, convidar de novo
-- é legítimo.
CREATE UNIQUE INDEX idx_convites_board_pendente
    ON convites_board (board_id, email)
    WHERE aceito_em IS NULL;

CREATE INDEX idx_convites_board_board ON convites_board (board_id);
