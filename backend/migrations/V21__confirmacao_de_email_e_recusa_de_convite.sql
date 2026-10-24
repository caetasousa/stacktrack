-- Confirmação de email das contas e recusa de convite.
--
-- O painel passa a mostrar os convites para o email da conta — e isso só é
-- seguro depois de a conta PROVAR que o email é dela. Sem a prova, qualquer um
-- cadastraria o email de outra pessoa e veria os convites dela. É o que
-- email_confirmado_em registra: nulo enquanto a conta não abriu o link que o
-- cadastro mandou.
--
-- As contas que já existem ficam nulas, e é o certo: nenhuma delas provou nada
-- ainda. Não há backfill — quem decide quando uma conta está confirmada é o
-- domínio, pelo link (ou pela troca de senha pelo email, que prova a mesma
-- coisa).
--
-- recusado_em distingue, no convite encerrado, o "não" de quem foi convidado do
-- cancelamento por quem convidou. A recusa também grava revogado_em: é ela que
-- faz o convite sair de toda consulta de pendentes e libera a vaga do índice de
-- "um convite em aberto por email", sem mudar consulta nem índice nenhum.
--
-- ROLLOUT: expansiva. Colunas novas anuláveis e tabela nova: a versão anterior
-- da aplicação não as conhece e continua funcionando. Nenhum dado é escrito.

ALTER TABLE usuarios ADD COLUMN email_confirmado_em TIMESTAMPTZ;

ALTER TABLE convites_board ADD COLUMN recusado_em TIMESTAMPTZ;

CREATE TABLE confirmacoes_email (
    id         UUID        PRIMARY KEY,
    -- CASCADE: a conta que some leva os links dela.
    usuario_id UUID        NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    criado_em  TIMESTAMPTZ NOT NULL,
    expira_em  TIMESTAMPTZ NOT NULL,
    -- Quando o link foi usado. Nulo enquanto vale.
    usada_em   TIMESTAMPTZ
);

-- O teto de reenvios: quantos links a conta pediu na última hora.
CREATE INDEX idx_confirmacoes_email_usuario
    ON confirmacoes_email (usuario_id, criado_em);

-- A faxina: o que já venceu.
CREATE INDEX idx_confirmacoes_email_expira
    ON confirmacoes_email (expira_em);
