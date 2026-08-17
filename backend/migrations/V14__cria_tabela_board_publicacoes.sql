-- O link público de acompanhamento de um quadro: a porta de quem vê o andamento
-- sem ter conta e sem ser convidado.
--
-- Tabela própria, e não colunas em `boards`, pela mesma razão que o vínculo tem
-- a sua: publicar é um ato com vida própria — quem publicou, quando, e um
-- segredo a revogar. Em duas colunas anuláveis de `boards`, "nunca publicado" e
-- "publicado sem token" seriam estados indistinguíveis, e o segundo é um quadro
-- aberto sem ninguém saber.
--
-- board_id é a chave primária: um quadro tem no máximo um link vivo. Permitir
-- dois seria permitir revogar pela metade — e um segredo revogado pela metade
-- não está revogado.
--
-- O token vai em claro, ao contrário do de sessão e do de convite, que ficam só
-- como hash. A diferença não é descuido: hash protege contra quem lê o banco, e
-- quem lê o banco já lê os cards deste quadro direto, sem precisar de link
-- nenhum. Hash aqui não protegeria nada e custaria a única coisa que o dono
-- precisa — poder abrir a tela de novo amanhã e copiar o mesmo link.
CREATE TABLE board_publicacoes (
    board_id   UUID        PRIMARY KEY REFERENCES boards (id) ON DELETE CASCADE,
    token      VARCHAR(64) NOT NULL UNIQUE,
    -- ON DELETE CASCADE em quem publicou: a conta sumindo leva o link junto.
    -- É a direção segura — o contrário deixaria um quadro exposto por decisão
    -- de alguém que não existe mais para revogá-la.
    criado_por UUID        NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,
    criado_em  TIMESTAMPTZ NOT NULL
);
