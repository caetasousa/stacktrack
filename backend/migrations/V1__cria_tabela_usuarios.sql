-- Quem loga no kanbanGo. Um usuário é dono e membro de quadros (fases 2 e 3);
-- nada disso aparece aqui, porque a identidade não depende de quadro nenhum.
--
-- Sem coluna de confirmação de email: o cadastro desta fase cria a conta já
-- utilizável. Confirmar email exige enviar email, e o envio ainda não existe.
CREATE TABLE usuarios (
    id            UUID         PRIMARY KEY,
    nome          VARCHAR(120) NOT NULL,
    -- Guardado sempre em minúsculas, normalizado pelo domínio: sem isso
    -- "Ana@x.com" e "ana@x.com" viram duas contas e o UNIQUE não percebe.
    email         VARCHAR(255) NOT NULL UNIQUE,
    senha_hash    VARCHAR(255) NOT NULL,
    criado_em     TIMESTAMPTZ  NOT NULL,
    atualizado_em TIMESTAMPTZ  NOT NULL
);
