-- Data de entrega do card.
--
-- Anulável, e não só por ser opcional: coluna nova obrigatória quebraria a
-- versão anterior da aplicação, que continua no ar durante o deploy. Aqui não
-- há o que preencher — card sem prazo é o caso normal, não linha legada.
ALTER TABLE cards ADD COLUMN prazo TIMESTAMPTZ;

-- O quadro filtra e ordena por prazo; sem índice isso vira varredura da tabela
-- inteira à medida que os cards se acumulam.
CREATE INDEX idx_cards_prazo ON cards (prazo) WHERE prazo IS NOT NULL;
