-- A que card o evento pertence, quando pertence a algum.
--
-- Existe para o histórico de um card ser lido por índice. A alternativa seria
-- filtrar por `payload->>'cardId'`, o que obrigaria a varrer o payload de todos
-- os eventos do quadro — e o payload é JSONB justamente para NÃO ser o caminho
-- de busca.
--
-- Anulável, e não só por ser opcional: evento de coluna criada ou de membros
-- alterados não é de card nenhum, e coluna nova obrigatória quebraria a versão
-- anterior da aplicação, que continua no ar durante o deploy. É expand puro —
-- não há contract depois, porque a coluna nasce como deve ficar.
--
-- Sem chave estrangeira para `cards`, pelo mesmo motivo de `autor_id`: o
-- histórico não pode desaparecer porque o card foi apagado. É justamente o
-- evento "card apagado" que precisaria sobreviver ao CASCADE.
ALTER TABLE board_events ADD COLUMN card_id UUID;

-- O índice que o histórico usa: "o que aconteceu com este card, do mais recente
-- para o mais antigo". Parcial porque a maioria dos eventos não é de card, e
-- indexar os nulos só engordaria a árvore.
CREATE INDEX idx_board_events_card ON board_events (card_id, seq DESC)
    WHERE card_id IS NOT NULL;
