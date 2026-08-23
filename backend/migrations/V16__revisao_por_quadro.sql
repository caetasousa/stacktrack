-- A REVISÃO do quadro: um contador monotônico por quadro, e a posição de cada
-- evento dentro dele.
--
-- O QUE `seq` NÃO PODE FAZER
--
-- `seq` é BIGSERIAL, global à tabela, e serve muito bem ao que foi feito para
-- servir: identidade imutável e ordem total do log. O que ele NÃO é, e vinha
-- sendo usado como se fosse, é cursor de reconexão.
--
-- A razão é que um BIGSERIAL registra a ordem de ALOCAÇÃO do número, não a de
-- COMMIT. Duas transações concorrentes pegam 42 e 43 nessa ordem e podem
-- comitar na ordem inversa. Um cliente que reconecte no meio disso pede "o que
-- houve desde o 42?", recebe o 43 — que já comitou — e avança o cursor para 43.
-- Quando o 42 comitar, um instante depois, ele nunca mais será entregue a esse
-- cliente: está abaixo do cursor. O buraco é silencioso e permanente.
--
-- A revisão conserta isso porque é atribuída SOB O LOCK do quadro: só uma
-- transação por vez a incrementa, então a ordem de numeração é a ordem de
-- commit, e ela é contígua dentro do quadro. `seq` continua existindo como
-- identidade e como campo de compatibilidade.
--
-- ROLLOUT: schema expansivo, com troca coordenada da aplicação.
--
-- Todas as colunas nascem ANULÁVEIS e sem DEFAULT. A versão anterior da
-- aplicação, que não as conhece, continua inserindo eventos normalmente (as
-- colunas ficam NULL) e continua lendo pelo `seq`. Nenhum backfill: quadro
-- antigo tem revisão NULL, e quem a lê trata NULL como zero — a mesma coisa que
-- "nunca teve mutação numerada". A decisão sobre o valor das linhas antigas é
-- do domínio (COALESCE no incremento), e não de um UPDATE aqui.
--
-- Compatibilidade de schema não significa compatibilidade do protocolo. Antes
-- de executar esta migration, o deploy interrompe web e API antigos; depois
-- sobe primeiro o backend novo e então serve o cliente novo. Isso impede que o
-- writer anterior grave uma mutação com revisão NULL enquanto alguém já usa a
-- revisão como cursor. O procedimento está automatizado na esteira e descrito
-- em docs/producao.md.

ALTER TABLE boards
    -- A revisão ATUAL do quadro. Incrementada uma vez por mutação, sob o lock
    -- da própria linha — é o que garante que ela seja contígua e que a ordem de
    -- numeração seja a de commit.
    ADD COLUMN revisao BIGINT;

ALTER TABLE board_events
    -- A revisão do quadro em que este evento foi confirmado.
    ADD COLUMN revisao BIGINT,
    -- A posição do evento DENTRO da revisão, começando em zero, e quantos
    -- eventos formam o grupo.
    --
    -- Hoje toda mutação produz exatamente um evento, então o par é sempre
    -- (0, 1). As colunas existem assim mesmo porque o cliente precisa saber
    -- QUANDO um grupo está completo para confirmar a revisão: sem `quantidade`
    -- ele teria de adivinhar se ainda vem mais, e adivinhar errado é confirmar
    -- uma revisão que ele aplicou pela metade.
    ADD COLUMN indice INTEGER,
    ADD COLUMN quantidade INTEGER;

-- O índice do replay por revisão. O de `(board_id, seq)` continua existindo
-- para a auditoria e para o cliente antigo; este é o do protocolo novo.
--
-- `seq` entra como terceira coluna para desempatar dentro do mesmo (revisão,
-- índice) caso um evento legado, com revisão NULL, apareça no intervalo.
CREATE INDEX idx_board_events_revisao
    ON board_events (board_id, revisao, indice, seq)
    WHERE revisao IS NOT NULL;
