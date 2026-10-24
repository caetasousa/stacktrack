-- O outbox de email: a intenção de mandar uma mensagem, gravada na MESMA
-- transação do fluxo que a pede.
--
-- O PROBLEMA: mandar o email dentro da requisição amarra o convite ao SMTP. Se
-- o provedor demora, a requisição demora; se cai, ou o convite some junto (o
-- email virou parte da transação) ou o convite fica gravado sem o email ter
-- saído, e ninguém fica sabendo. E um processo que morre entre o commit e o
-- envio perde a mensagem em silêncio.
--
-- A SAÍDA: a requisição grava aqui a mensagem já redigida, junto com o convite.
-- Um worker a envia depois do commit, com retentativa. A falha de SMTP nunca
-- desfaz o convite, e o convite desfeito (rollback) nunca deixa mensagem para
-- trás — os dois valem ou caem juntos.
--
-- O link do convite é credencial: quem o tem entra na tela do convite. Por isso
-- a mensagem fica CIFRADA (payload_cifrado, AES-256-GCM com chave do vault) e só
-- enquanto é útil — some no envio, no cancelamento e no vencimento. A tabela de
-- convites continua guardando apenas o hash do token.
--
-- ROLLOUT: expansiva. Tabela nova não afeta a versão anterior da aplicação, que
-- não a conhece. Nenhum dado é escrito aqui.

CREATE TABLE email_outbox (
    id                 UUID         PRIMARY KEY,

    -- O modelo e a versão que redigiram a mensagem (ex.: 'convite.v1'). Fica em
    -- claro para a operação saber o que está na fila sem decifrar nada.
    modelo             VARCHAR(40)  NOT NULL,

    -- Uma intenção, uma mensagem: um reenvio de requisição (duplo clique,
    -- retentativa do cliente) bate aqui em vez de virar dois emails.
    chave_deduplicacao VARCHAR(120) NOT NULL UNIQUE,

    -- A mensagem inteira, cifrada. Anulável: é apagada assim que deixa de ser
    -- necessária, e a linha continua como registro de que houve envio.
    payload_cifrado    BYTEA,

    -- pendente, enviado, falhou, cancelado ou expirado. As transições são do
    -- domínio (internal/domain/email), não de um CHECK daqui.
    estado             VARCHAR(12)  NOT NULL,
    tentativas         INTEGER      NOT NULL,
    proxima_em         TIMESTAMPTZ  NOT NULL,
    criado_em          TIMESTAMPTZ  NOT NULL,
    enviado_em         TIMESTAMPTZ,

    -- Até quando a mensagem faz sentido. Passado isso, o link dentro dela já não
    -- abre nada, e mandá-la seria entregar um convite morto.
    expira_em          TIMESTAMPTZ  NOT NULL,

    -- A última falha, SANITIZADA por quem grava: resposta de servidor SMTP
    -- costuma repetir o endereço do destinatário.
    ultimo_erro        VARCHAR(500),

    -- Quem pediu o envio. É a base da cota diária por conta: com email de
    -- verdade, convidar passa a ser um jeito de mandar mensagem a qualquer
    -- endereço, e precisa de teto.
    autor_id           UUID         NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,

    -- O convite que esta mensagem entrega. CASCADE: apagar o quadro leva os
    -- convites, e com eles as mensagens que ainda não saíram.
    convite_id         UUID         REFERENCES convites_board (id) ON DELETE CASCADE
);

-- A fila do worker: o que está pendente e já na hora. Parcial porque o resto da
-- tabela é histórico, e o histórico é justamente o que não interessa a quem
-- procura trabalho.
CREATE INDEX idx_email_outbox_fila
    ON email_outbox (proxima_em)
    WHERE estado = 'pendente';

-- O estado de entrega de cada convite, na tela de membros.
CREATE INDEX idx_email_outbox_convite
    ON email_outbox (convite_id, criado_em DESC);

-- A cota: quantos envios esta conta pediu nas últimas horas.
CREATE INDEX idx_email_outbox_autor
    ON email_outbox (autor_id, criado_em);
