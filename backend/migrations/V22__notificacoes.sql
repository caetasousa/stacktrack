-- As notificações: o aviso pessoal de que algo pede a atenção de UMA pessoa —
-- o convite para um quadro, o card que passou a ser dela, a menção num
-- comentário, o prazo chegando.
--
-- O aviso é GRAVADO, e não só entregue pelo tempo real: quem estava fora
-- encontra tudo ao voltar, e o SSE só avisa as telas abertas de que a lista
-- mudou. A notificação nasce na mesma transação do que a causou — a atribuição
-- que comita leva o aviso junto, e a que desfaz não deixa aviso para trás.
--
-- O que NÃO é gravado é se o aviso ainda vale (a atribuição desfeita, o prazo
-- trocado, o convite recusado): isso é lido do estado atual a cada consulta.
-- Uma coluna de "não vale mais" teria de ser escrita por cada caminho que
-- desfaz alguma coisa, e o caminho esquecido viraria um aviso falso.
--
-- ROLLOUT: expansiva, num deploy só. Tabela nova e colunas novas anuláveis: a
-- versão anterior da aplicação não as conhece e continua funcionando — e é
-- para ela que um rollback volta. Nenhum dado é escrito aqui.

CREATE TABLE notificacoes (
    id            UUID         PRIMARY KEY,
    -- Quem recebe. CASCADE: a conta que some leva os avisos dela.
    usuario_id    UUID         NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,
    -- convite, atribuicao, mencao ou prazo. Os tipos são do domínio
    -- (internal/domain/notificacao), não de um CHECK daqui.
    tipo          VARCHAR(20)  NOT NULL,
    -- O quadro de que o aviso fala. Todo tipo tem um: é por ele que a retenção
    -- é escolhida e que sair do quadro esconde os avisos dele. CASCADE: apagar
    -- o quadro leva os avisos junto.
    board_id      UUID         NOT NULL REFERENCES boards (id) ON DELETE CASCADE,
    -- Os três abaixo apontam para o que o aviso mostra, conforme o tipo. Com
    -- CASCADE: um aviso sobre um card apagado não tem para onde levar ninguém,
    -- e o de um comentário apagado guardaria a lembrança de um texto que o
    -- autor retirou.
    card_id       UUID         REFERENCES cards (id) ON DELETE CASCADE,
    comentario_id UUID         REFERENCES comentarios (id) ON DELETE CASCADE,
    convite_id    UUID         REFERENCES convites_board (id) ON DELETE CASCADE,
    -- Quem causou. Anulável: o aviso de prazo não tem pessoa por trás.
    autor_id      UUID         REFERENCES usuarios (id) ON DELETE SET NULL,
    -- A data que motivou o aviso de prazo. O prazo do card pode mudar depois,
    -- e o aviso de uma data que já não vale precisa deixar de contar.
    prazo         TIMESTAMPTZ,
    -- Uma intenção, um aviso: o mesmo prazo visto por duas passadas do vigia,
    -- ou o mesmo convite reenviado, batem aqui em vez de virar dois.
    chave         VARCHAR(160) NOT NULL UNIQUE,
    criada_em     TIMESTAMPTZ  NOT NULL,
    lida_em       TIMESTAMPTZ
);

-- A lista do sino: os avisos de uma pessoa, do mais novo para o mais antigo.
-- O `id` desempata e é o cursor da página seguinte.
CREATE INDEX idx_notificacoes_usuario ON notificacoes (usuario_id, criada_em DESC, id DESC);

-- Os caminhos por onde o CASCADE e as consultas chegam a partir do que o aviso
-- aponta. Parciais: em cada um, a maioria das linhas é de outro tipo.
CREATE INDEX idx_notificacoes_board ON notificacoes (board_id);
CREATE INDEX idx_notificacoes_card ON notificacoes (card_id) WHERE card_id IS NOT NULL;
CREATE INDEX idx_notificacoes_comentario ON notificacoes (comentario_id) WHERE comentario_id IS NOT NULL;
CREATE INDEX idx_notificacoes_convite ON notificacoes (convite_id) WHERE convite_id IS NOT NULL;

-- Por quantos dias as notificações LIDAS deste quadro ficam guardadas,
-- escolhido pelo dono. Anulável e sem DEFAULT: o quadro que nunca escolheu usa
-- o padrão, e o padrão é do domínio.
ALTER TABLE boards ADD COLUMN retencao_notificacoes_dias INTEGER;

-- O email de uma notificação, quando ela tem um. CASCADE, e não SET NULL: a
-- notificação apagada (o card apagado, a menção retirada) não pode deixar na
-- fila um email avisando de algo que não existe mais.
ALTER TABLE email_outbox ADD COLUMN notificacao_id UUID REFERENCES notificacoes (id) ON DELETE CASCADE;

CREATE INDEX idx_email_outbox_notificacao ON email_outbox (notificacao_id) WHERE notificacao_id IS NOT NULL;

-- O vigia de prazos procura os cards cujo prazo chega nas próximas horas pelo
-- idx_cards_prazo, que já existe (V6) — parcial em prazo IS NOT NULL, que é
-- exatamente a busca dele.
