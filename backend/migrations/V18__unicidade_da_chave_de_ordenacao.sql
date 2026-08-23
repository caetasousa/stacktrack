-- O CONTRACT da chave de ordenação: duas linhas do mesmo contêiner não podem
-- mais ocupar a mesma posição.
--
-- É o terceiro passo de um ciclo expand/contract que começou dois deploys
-- atrás, e a espera não foi cerimônia. `UNIQUE` novo é migration que APERTA: a
-- versão ANTERIOR da aplicação continua no ar durante o deploy e é para onde um
-- rollback volta, e era justamente ela que conseguia gravar duas chaves iguais
-- — duas inserções simultâneas no mesmo ponto calculavam a mesma chave. Criar o
-- índice junto com o código que corrigiu isso faria a versão anterior quebrar no
-- primeiro INSERT, sem que o Flyway (forward-only) pudesse voltar atrás.
--
-- O que já aconteceu, e é o que autoriza este arquivo:
--
--   1. Expand — nada no schema.
--   2. Código novo em produção: o domínio detecta chave repetida e redistribui
--      as chaves do contêiner antes de calcular a próxima, sob o lock do quadro
--      (`internal/usecase/board/ordenacao.go`). Nenhuma versão em produção
--      produz duplicidade a partir daí, e a herdada é reparada PELO DOMÍNIO,
--      com `manutencao reparar-ordenacao` — nunca por UPDATE aqui, que seria
--      uma segunda fonte da verdade sobre com que chave a linha antiga fica.
--   3. Este contract.
--
-- PRÉ-CONDIÇÃO: `manutencao reparar-ordenacao --conferir` precisa sair 0 antes
-- deste deploy. Havendo duplicidade herdada, o CREATE UNIQUE INDEX falha e a
-- partida do Flyway para com ele — o que é o comportamento certo, mas na hora
-- errada. O procedimento está em backend/migrations/README.md.
--
-- JANELA: sem CONCURRENTLY, o CREATE UNIQUE INDEX toma SHARE na tabela e
-- bloqueia escrita enquanto constrói. É deliberado no perfil desta rodada — uma
-- instância, quadros de até 1.000 cards, janela de manutenção curta —, e a
-- estimativa numa cópia representativa é registrada antes de aplicar. Se um dia
-- não couber na janela, a saída é criação concorrente com a migration marcada
-- para rodar fora de transação, não abrir mão da unicidade.

-- ⚠️ COLLATE "C" pelo mesmo motivo dos índices que estes substituem: é a ordem
-- de BYTES, a mesma que `ordem.ChaveEntre` usa para decidir o que vem antes.
-- Com collation diferente da consulta o Postgres não usa o índice, e a
-- ordenação vira sort em memória a cada leitura do quadro.
CREATE UNIQUE INDEX idx_colunas_chave_por_board
    ON colunas (board_id, chave COLLATE "C");

CREATE UNIQUE INDEX idx_cards_chave_por_coluna
    ON cards (coluna_id, chave COLLATE "C");

-- Os índices NÃO-únicos que existiam sobre exatamente as mesmas colunas, na
-- mesma collation, saem agora: os de cima os cobrem inteiramente. Manter os dois
-- pares custaria escrita e disco em toda mutação para nada.
--
-- Derrubá-los é seguro para a versão anterior da aplicação — ela não nomeia
-- índice nenhum, e as consultas que os usavam passam a usar os únicos, com o
-- mesmo plano. A ordem importa: os novos nascem ANTES, para que nunca exista um
-- instante sem índice servindo `ORDER BY chave`.
DROP INDEX idx_colunas_board_chave;

DROP INDEX idx_cards_coluna_chave;
