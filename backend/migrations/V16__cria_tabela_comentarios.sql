-- A conversa de um card.
--
-- É o primeiro fluxo append-only do projeto: comentário não se move, não se
-- reordena e não tem posição — ele acontece, e a ordem é a do tempo. Por isso
-- não há `posicao` aqui, ao contrário de coluna, card, etiqueta e checklist.
CREATE TABLE comentarios (
    id        UUID         PRIMARY KEY,
    card_id   UUID         NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    -- ON DELETE CASCADE, como em anexos.criado_por: é o que o schema já faz
    -- com conteúdo autorado, e não há caminho de exclusão de conta no produto.
    -- A alternativa (guardar o comentário órfão de uma conta apagada) obrigaria
    -- a tabela a carregar um autor anulável e a tela a ter um estado "conta
    -- removida" que nada hoje produz.
    autor_id  UUID         NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,
    texto     VARCHAR(2000) NOT NULL,
    criado_em TIMESTAMPTZ  NOT NULL,
    -- NULL enquanto o comentário nunca foi editado. É o que permite a tela
    -- dizer "editado" sem comparar timestamps que sempre diferem por
    -- microssegundos.
    editado_em TIMESTAMPTZ
);

-- A leitura é sempre "os comentários deste card, do mais antigo para o mais
-- novo". Sem o índice, cada abertura de card varreria a tabela inteira.
CREATE INDEX idx_comentarios_card ON comentarios (card_id, criado_em);
