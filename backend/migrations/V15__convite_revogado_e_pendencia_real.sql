-- Revogação explícita de convite, e o índice de pendência que passa a
-- considerá-la.
--
-- O QUE ESTAVA ERRADO
--
-- `idx_convites_board_pendente` era UNIQUE (board_id, email) WHERE aceito_em IS
-- NULL. Um convite VENCIDO continua com aceito_em NULL, então ele seguia
-- ocupando a vaga: convidar de novo o mesmo email, depois do vencimento,
-- batia na constraint e virava 500. O caminho quase não era alcançado porque
-- quem já tinha conta era adicionado direto, sem convite; com esse atalho
-- removido (A1), todo convite passa por aqui e o defeito vira rotina.
--
-- Revogar, por sua vez, era um DELETE — a linha sumia, e com ela a resposta
-- para "quem convidou quem, e quando". Agora revogar é marcar.
--
-- ROLLOUT: expansiva, aplicável num deploy só.
--
--   * a coluna nova é ANULÁVEL e sem DEFAULT: a versão anterior da aplicação,
--     que não a conhece, continua inserindo e lendo normalmente;
--   * o índice novo é MAIS FROUXO que o antigo (menos linhas entram na
--     unicidade), então nada que a versão anterior conseguia gravar passa a
--     ser recusado. Trocar um índice por outro mais permissivo não é apertar.
--
-- Nenhum backfill: `revogado_em` nasce NULL em toda linha existente, que é
-- exatamente o significado certo — nenhum convite antigo foi revogado. Não há
-- decisão de negócio escondida aqui, e por isso não há UPDATE nesta migration.

ALTER TABLE convites_board
    ADD COLUMN revogado_em TIMESTAMPTZ;

-- A vaga de "convite pendente para este email neste quadro" deixa de ser
-- ocupada por convite revogado. O vencido é resolvido pelo domínio, que revoga
-- o antigo antes de criar o novo — na MESMA transação, sob o lock do quadro.
-- Vencimento não cabe num índice parcial: o predicado teria de comparar com
-- now(), que não é imutável, e o PostgreSQL recusa.
DROP INDEX idx_convites_board_pendente;

CREATE UNIQUE INDEX idx_convites_board_pendente
    ON convites_board (board_id, email)
    WHERE aceito_em IS NULL AND revogado_em IS NULL;

-- A aceitação busca por hash e depois atualiza a MESMA linha condicionalmente.
-- O token_hash já é UNIQUE (o índice vem da constraint), então a busca é
-- indexada; o que falta é o caminho da listagem, que filtra por quadro e
-- pendência.
CREATE INDEX idx_convites_board_pendentes_do_quadro
    ON convites_board (board_id, criado_em DESC)
    WHERE aceito_em IS NULL AND revogado_em IS NULL;
