-- O outbox das exclusões de arquivo.
--
-- O PROBLEMA: apagar um card apaga a linha do anexo (por CASCADE) e o processo
-- apaga o arquivo do disco logo em seguida. As duas coisas não são a mesma
-- transação — não podem ser, porque o filesystem não participa do commit — e o
-- segundo passo é IRREVERSÍVEL. Se o backup mais recente for anterior à
-- exclusão, a restauração traz de volta uma linha cujo arquivo já não existe em
-- lugar nenhum. O anexo aparece na tela e não abre, para sempre.
--
-- A SAÍDA: a exclusão de domínio grava aqui, na MESMA transação do CASCADE, a
-- chave física de cada arquivo afetado. Os bytes continuam no disco. Um worker
-- só os remove quando um backup externo comprova que aquela exclusão está
-- coberta — ou seja, quando existe cópia posterior à exclusão de onde a linha
-- poderia ser restaurada sem o arquivo virar órfão.
--
-- Enquanto essa prova não existir (ela chega em A6), o worker opera em
-- FAIL-CLOSED: acumula e não remove nada. É a decisão certa — disco é barato,
-- e um anexo apagado cedo demais não volta.
--
-- ROLLOUT: expansiva. Tabela nova não afeta a versão anterior da aplicação, que
-- simplesmente não a conhece e continua apagando do disco como sempre fez.

CREATE TABLE arquivo_exclusoes (
    id            BIGSERIAL PRIMARY KEY,

    -- A chave física dentro do diretório de anexos. É o nome sorteado pelo
    -- armazém, nunca o nome original de quem enviou.
    --
    -- SEM chave estrangeira para `anexos`, e é o ponto da tabela: a linha do
    -- anexo já não existe quando esta é gravada. Ela é o que sobra dela.
    caminho       VARCHAR(255) NOT NULL,

    -- O quadro a que o arquivo pertencia, para o worker poder relatar e para a
    -- exclusão poder ser auditada por quadro.
    --
    -- Anulável e sem FK pelo mesmo motivo: apagar o quadro inteiro é um dos
    -- caminhos que chega aqui, e uma FK levaria a linha junto no CASCADE —
    -- apagando exatamente o registro que existe para sobreviver a ele.
    board_id      UUID,

    -- Quando o domínio apagou. É o instante que o backup precisa cobrir.
    excluido_em   TIMESTAMPTZ NOT NULL,

    -- Quando um backup externo comprovou que ESTA exclusão está coberta.
    -- NULL enquanto não houver prova. É o que o worker exige para remover.
    coberto_em    TIMESTAMPTZ,

    -- Quando os bytes saíram do disco. NULL enquanto ainda estão lá.
    removido_em   TIMESTAMPTZ,

    -- Quantas vezes a remoção falhou, e quando tentar de novo.
    tentativas    INTEGER NOT NULL,
    proxima_em    TIMESTAMPTZ,

    -- A última falha, SANITIZADA por quem grava: mensagem de erro de
    -- filesystem carrega caminho absoluto, e caminho absoluto num log é
    -- estrutura de diretório do servidor.
    ultimo_erro   VARCHAR(500)
);

-- A fila do worker: o que está coberto, ainda no disco, e na hora de tentar.
-- Parcial porque a tabela vira histórico depois de removido — e o histórico é
-- justamente o que não interessa a quem procura trabalho.
CREATE INDEX idx_arquivo_exclusoes_pendentes
    ON arquivo_exclusoes (proxima_em, id)
    WHERE removido_em IS NULL;

-- A consulta da cobertura: "quais exclusões deste intervalo ainda não têm
-- prova?". É por instante de exclusão porque é isso que um snapshot cobre.
CREATE INDEX idx_arquivo_exclusoes_sem_cobertura
    ON arquivo_exclusoes (excluido_em)
    WHERE coberto_em IS NULL;
