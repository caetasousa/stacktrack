-- Sessões autenticadas (cookie HttpOnly). Guarda só o SHA-256 do token: se o
-- banco vazar, os tokens vazados não servem para se passar por ninguém.
CREATE TABLE sessions (
    token_hash CHAR(64)    PRIMARY KEY,
    usuario_id UUID        NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,
    criado_em  TIMESTAMPTZ NOT NULL,
    expira_em  TIMESTAMPTZ NOT NULL
);

-- A varredura de sessões vencidas roda a cada login; sem índice ela vira um
-- seq scan na tabela inteira à medida que as sessões se acumulam.
CREATE INDEX idx_sessions_expira_em ON sessions (expira_em);
