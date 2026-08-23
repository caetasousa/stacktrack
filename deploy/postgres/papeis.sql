-- Os dois papéis do banco: quem MIGRA e quem SERVE.
--
-- Até aqui a API se conectava com o dono do banco, que pode criar, alterar e
-- derrubar qualquer tabela. Uma falha de execução remota na API herdava esse
-- poder: o mesmo processo que serve `/api/boards` conseguia um `DROP TABLE
-- cards`. Separar os papéis não impede a falha — impede que ela alcance o
-- schema.
--
-- A divisão:
--
--   dono (POSTGRES_USER)   nasce com o initdb, é dono do schema e das tabelas,
--                          e é com ele que o Flyway migra. Ninguém mais o usa.
--   aplicação (DB_USER)    só DML nas tabelas que existem, mais as sequências.
--                          Sem CREATE, sem ALTER, sem DROP, sem extensão, sem
--                          role, sem database.
--
-- Aplicado pelo Ansible (roles/stacktrack) e exercitado pelo teste de
-- integração backend/test/repository/papeis_test.go, que é quem prova que o DDL
-- falha — um GRANT esquecido não aparece em teste que só faz SELECT.
--
-- Roda com `psql -v ON_ERROR_STOP=1`, e recebe:
--
--   -v dono=...  -v db=...  -v app_user=...  -v app_password=...
--
-- É IDEMPOTENTE de ponta a ponta: rodar de novo não muda nada e não falha. Tem
-- de ser, porque roda a cada provisionamento.
--
-- Sobre a sintaxe: `:'x'` vira literal e `:"x"` vira identificador, os dois
-- escapados pelo psql. Não há bloco `DO $$`, e a ausência é técnica — o psql
-- NÃO interpola variável dentro de string com cifrão, então um DO block
-- receberia os dois-pontos literais. Onde é preciso condicional, o caminho é
-- `\gexec`: o SELECT monta o comando e o psql executa o que voltar.

-- O papel da aplicação, se ainda não existir.
SELECT format('CREATE ROLE %I LOGIN', :'app_user')
 WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'app_user')
\gexec

-- A senha vem do vault, e é gerenciada AQUI — ao contrário de
-- POSTGRES_PASSWORD, que o initdb gravou no volume e que só um ALTER ROLE
-- manual muda. Trocar esta é editar o vault e reaplicar o playbook.
ALTER ROLE :"app_user" WITH LOGIN PASSWORD :'app_password';

-- Nada de administrativo, explicitamente. São os padrões de um CREATE ROLE
-- simples; declará-los protege contra o papel ter sido criado à mão, um dia,
-- com mais do que precisava.
ALTER ROLE :"app_user" WITH NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;

-- Tetos de tempo como PADRÃO DO PAPEL, não só do código.
--
-- A API já aplica statement_timeout e idle_in_transaction no startup de cada
-- conexão, e lock_timeout por transação (A4). Estes valem para o que o código
-- não alcança: um psql aberto com a credencial da aplicação, um cliente futuro,
-- uma versão que esqueça de configurá-los.
--
-- São FOLGADOS de propósito — acima dos do backend (5s, 10s, 2s). Um teto de
-- papel mais apertado que o do código não seria defesa em profundidade, seria
-- um segundo lugar para ajustar toda vez que o primeiro mudasse, e a query
-- normal passaria a morrer por um limite que ninguém lembra que existe.
ALTER ROLE :"app_user" SET statement_timeout = '15s';
ALTER ROLE :"app_user" SET idle_in_transaction_session_timeout = '30s';
ALTER ROLE :"app_user" SET lock_timeout = '5s';

-- --- o que a aplicação PODE ------------------------------------------------

-- Entrar no banco e atravessar o schema. `USAGE` não dá direito a nada dentro
-- dele: é só a permissão de passar pela porta.
GRANT CONNECT ON DATABASE :"db" TO :"app_user";
GRANT USAGE ON SCHEMA public TO :"app_user";

-- DML nas tabelas que já existem. Sem TRUNCATE e sem REFERENCES: nenhum dos
-- dois é usado pelo domínio, e TRUNCATE esvazia tabela inteira sem passar por
-- FK nem por trigger.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO :"app_user";

-- As sequências dos BIGSERIAL (board_events.seq, arquivo_exclusoes.id). Sem
-- isto o INSERT falha ao pedir o próximo valor, e o erro fala de permissão numa
-- sequência que ninguém lembra que existe.
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO :"app_user";

-- O mesmo, para as tabelas que ainda NÃO existem.
--
-- Sem isto, toda migration futura criaria tabela invisível para a aplicação, e
-- a descoberta seria em produção, no primeiro INSERT depois do deploy. O `FOR
-- ROLE` é o dono porque é ele quem o Flyway usa para criar: privilégio padrão é
-- definido por quem cria, não por quem lê.
ALTER DEFAULT PRIVILEGES FOR ROLE :"dono" IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO :"app_user";
ALTER DEFAULT PRIVILEGES FOR ROLE :"dono" IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO :"app_user";

-- --- o que a aplicação NÃO pode --------------------------------------------

-- CREATE no schema `public` é o privilégio que transforma o papel de runtime em
-- papel de DDL: com ele a API cria tabela — e quem cria, é dono.
--
-- No PostgreSQL 15+ o `public` já não vem com esse direito para todo mundo, mas
-- o REVOKE fica explícito: a versão do banco é infraestrutura e pode mudar, a
-- intenção não.
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
REVOKE CREATE ON SCHEMA public FROM :"app_user";

-- `PUBLIC` no PostgreSQL inclui todo papel que existir, inclusive o da
-- aplicação: revogar tudo e devolver só o CONNECT é o que deixa o direito de
-- entrar sendo uma concessão, e não um resto do padrão.
REVOKE ALL ON DATABASE :"db" FROM PUBLIC;
GRANT CONNECT ON DATABASE :"db" TO :"app_user";

-- --- conferência -----------------------------------------------------------
--
-- A saída é o que o Ansible e o operador leem. Um GRANT que não pegou aparece
-- aqui, e não no primeiro INSERT depois do deploy.
SELECT
    :'app_user'                                                    AS papel,
    has_database_privilege(:'app_user', :'db', 'CONNECT')          AS conecta,
    has_schema_privilege(:'app_user', 'public', 'USAGE')           AS enxerga_schema,
    has_schema_privilege(:'app_user', 'public', 'CREATE')          AS cria_tabela,
    (SELECT count(*) FROM information_schema.table_privileges
      WHERE grantee = :'app_user' AND privilege_type = 'INSERT')   AS tabelas_com_insert;
