-- O log de eventos de cada quadro: o que aconteceu, em que ordem.
--
-- Existe para a reconexão. Hoje um cliente que perde a conexão por vinte
-- segundos volta e não sabe o que perdeu; com esta tabela ele pergunta "o que
-- houve desde o 41?" e recebe o intervalo antes de voltar ao vivo.
--
-- É também o outbox transacional em miniatura: o evento é gravado na MESMA
-- transação da mudança que o originou, então ou o card move e o evento existe,
-- ou nenhum dos dois. Sem isso, um processo que morre entre as duas escritas
-- deixaria um buraco que nenhum cliente conseguiria perceber.

CREATE TABLE board_events (
    -- BIGSERIAL, e não UUID: o cliente precisa comparar "já apliquei até
    -- aqui", e isso exige ORDEM TOTAL. Um identificador aleatório não ordena,
    -- e um timestamp empata — dois eventos no mesmo microssegundo deixariam de
    -- ter sucessor definido.
    seq         BIGSERIAL PRIMARY KEY,
    board_id    UUID NOT NULL REFERENCES boards (id) ON DELETE CASCADE,
    tipo        VARCHAR(40) NOT NULL,
    -- JSONB, e não JSON: além de ser mais compacto, permite indexar por dentro
    -- do payload se um dia isso for preciso. O formato de cada tipo é decidido
    -- pelo domínio, não pelo banco.
    payload     JSONB,
    -- Quem causou. Fica anulável e sem chave estrangeira de propósito: o
    -- histórico do quadro não pode desaparecer nem falhar porque a conta de
    -- quem agiu foi removida depois.
    autor_id    UUID,
    criado_em   TIMESTAMPTZ NOT NULL
);

-- O índice que a reconexão usa: "os eventos deste quadro a partir do seq N".
-- Sem ele, cada reconexão varreria a tabela inteira — que cresce para sempre.
CREATE INDEX idx_board_events_board_seq ON board_events (board_id, seq);
