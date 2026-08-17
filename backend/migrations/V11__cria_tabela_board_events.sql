-- O log de eventos de cada quadro: o que aconteceu, em que ordem.
--
-- Existe para a reconexão. Um cliente que perde a conexão por vinte segundos
-- volta e pergunta "o que houve desde o 41?", e recebe o intervalo antes de
-- voltar ao vivo.
--
-- É também o outbox transacional em miniatura: o evento é gravado na MESMA
-- transação da mudança que o originou, então ou o card move e o evento existe,
-- ou nenhum dos dois. Sem isso, um processo que morre entre as duas escritas
-- deixaria um buraco que nenhum cliente conseguiria perceber.
--
-- E é a fonte de dois read models sobre a mesma tabela: o histórico de um card
-- e a auditoria do quadro ("quem moveu o quê"). Nenhum dos dois tem tabela
-- própria — auditoria é leitura.

CREATE TABLE board_events (
    -- BIGSERIAL, e não UUID: o cliente precisa comparar "já apliquei até
    -- aqui", e isso exige ORDEM TOTAL. Um identificador aleatório não ordena,
    -- e um timestamp empata — dois eventos no mesmo microssegundo deixariam de
    -- ter sucessor definido. É também o cursor da paginação da auditoria.
    seq         BIGSERIAL PRIMARY KEY,
    board_id    UUID NOT NULL REFERENCES boards (id) ON DELETE CASCADE,
    tipo        VARCHAR(40) NOT NULL,
    -- JSONB, e não JSON: além de ser mais compacto, permite indexar por dentro
    -- do payload se um dia isso for preciso. O formato de cada tipo é decidido
    -- pelo domínio, não pelo banco.
    payload     JSONB,
    -- Quem causou. Fica anulável e SEM chave estrangeira de propósito: o
    -- histórico do quadro não pode desaparecer nem falhar porque a conta de
    -- quem agiu foi removida depois.
    autor_id    UUID,
    -- A que card o evento pertence, quando pertence a algum. Existe para o
    -- histórico ser lido por índice: a alternativa seria filtrar por
    -- `payload->>'cardId'`, varrendo o payload de todos os eventos do quadro —
    -- e o payload é JSONB justamente para NÃO ser o caminho de busca.
    --
    -- Sem chave estrangeira para `cards`, pelo mesmo motivo de `autor_id`: é
    -- justamente o evento "card apagado" que precisaria sobreviver ao CASCADE.
    card_id     UUID,
    criado_em   TIMESTAMPTZ NOT NULL
);

-- O índice que a reconexão usa: "os eventos deste quadro a partir do seq N".
-- Sem ele, cada reconexão varreria a tabela inteira — que cresce para sempre.
CREATE INDEX idx_board_events_board_seq ON board_events (board_id, seq);

-- O índice que o histórico de um card usa: "o que aconteceu com este card, do
-- mais recente para o mais antigo". Parcial porque a maioria dos eventos não é
-- de card, e indexar os nulos só engordaria a árvore.
CREATE INDEX idx_board_events_card ON board_events (card_id, seq DESC)
    WHERE card_id IS NOT NULL;
