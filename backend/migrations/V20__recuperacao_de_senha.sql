-- Os links de recuperação de senha.
--
-- Cada linha é um pedido de "esqueci minha senha": o link que foi por email
-- vale por uma hora e uma vez só. Quem decide isso — a validade, o teto de
-- pedidos por conta, o que encerra um link — é o domínio
-- (internal/domain/recuperacao), não esta tabela.
--
-- O link é credencial: quem o tem troca a senha da conta. Por isso aqui só
-- fica o HASH do token, como nas sessões e nos convites. A mensagem que leva o
-- link vai pelo email_outbox (V19), cifrada.
--
-- ROLLOUT: expansiva. Tabela nova não afeta a versão anterior da aplicação, que
-- não a conhece. Nenhum dado é escrito aqui.

CREATE TABLE recuperacoes_senha (
    id           UUID        PRIMARY KEY,
    -- CASCADE: a conta que some leva os pedidos dela.
    usuario_id   UUID        NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,
    token_hash   VARCHAR(64) NOT NULL UNIQUE,
    criado_em    TIMESTAMPTZ NOT NULL,
    expira_em    TIMESTAMPTZ NOT NULL,
    -- Quando o link deixou de valer antes do prazo: foi usado, ou outro link da
    -- mesma conta trocou a senha. Nulo enquanto está em aberto.
    encerrada_em TIMESTAMPTZ
);

-- O teto de pedidos: quantos esta conta fez na última hora.
CREATE INDEX idx_recuperacoes_senha_usuario
    ON recuperacoes_senha (usuario_id, criado_em);

-- A faxina: o que já venceu.
CREATE INDEX idx_recuperacoes_senha_expira
    ON recuperacoes_senha (expira_em);
