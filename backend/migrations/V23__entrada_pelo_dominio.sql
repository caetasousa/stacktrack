-- A entrada pelo domínio do email: o dono abre o quadro a "quem tiver email
-- @acme.com.br confirmado", e a colega entra sem convite.
--
-- O domínio gravado é sempre o do email confirmado de quem abriu — quem deriva
-- e valida é o domínio (internal/domain/dominio), não um CHECK daqui. Os dois
-- campos andam juntos: nulos, o quadro está fechado ao domínio.
--
-- ROLLOUT: expansiva, num deploy só. Colunas novas anuláveis e tabela nova: a
-- versão anterior da aplicação não as conhece e continua funcionando — e é
-- para ela que um rollback volta. Nenhum dado é escrito aqui.

ALTER TABLE boards ADD COLUMN dominio_acesso VARCHAR(253);
-- editor ou leitor. Os papéis são do domínio (internal/domain/membro).
ALTER TABLE boards ADD COLUMN dominio_papel VARCHAR(20);

-- O painel pergunta "que quadros estão abertos ao MEU domínio?". Parcial: a
-- maioria dos quadros nunca é aberta.
CREATE INDEX idx_boards_dominio_acesso ON boards (dominio_acesso) WHERE dominio_acesso IS NOT NULL;

-- Quem o dono tirou do quadro. Sem isto, remover uma colega de um quadro aberto
-- ao domínio dela não tiraria ninguém: ela voltaria sozinha pela porta do
-- domínio. Só um convite aceito desfaz o bloqueio — a decisão volta a ser de
-- quem administra.
CREATE TABLE board_bloqueios (
    -- CASCADE nos dois: o quadro apagado e a conta apagada levam o bloqueio.
    board_id   UUID        NOT NULL REFERENCES boards (id) ON DELETE CASCADE,
    usuario_id UUID        NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,
    criado_em  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (board_id, usuario_id)
);

-- O CASCADE da conta apagada chega por aqui.
CREATE INDEX idx_board_bloqueios_usuario ON board_bloqueios (usuario_id);
