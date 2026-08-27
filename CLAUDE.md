# stacktrack

Quadro Kanban colaborativo em tempo real (Go + SvelteKit). O roteiro completo,
fase a fase, com as fontes de estudo de cada uma, está em [PLANO.md](PLANO.md).

---

## Identidade no Git

**Toda alteração deste repositório é feita com o usuário do GitHub `caetasousa`.**

O repositório vem configurado localmente:

```
git config --local user.name  caetasousa
git config --local user.email caetasousa@gmail.com
```

Nenhum commit pode ser autorado com outra identidade — nem a global da máquina,
nem uma passada por `--author`, nem a de qualquer assistente. Antes de commitar,
conferir com `git config --local --list | grep user`; se a configuração local
tiver sumido (clone novo, `.git` recriado), restaurá-la com os dois comandos
acima antes de qualquer outra coisa.

A autoria é sempre do usuário. A participação do assistente vai no trailer
`Co-Authored-By:` da mensagem, nunca no campo de autor.

---

## Migrations (banco de dados)

O banco é um detalhe de infraestrutura — ele não conhece regras de negócio.

Constraints permitidas nas migrations:
- `NOT NULL` — o campo deve sempre existir
- `UNIQUE` — unicidade técnica (ex: email)
- `PRIMARY KEY`, `FOREIGN KEY` — relações entre tabelas
- Tipos corretos (`UUID`, `VARCHAR`, `TIMESTAMPTZ`)

Não usar nas migrations:
- `DEFAULT` com valores que representam regras de negócio — essa responsabilidade é do domínio
- `CHECK` constraints que validam regras de negócio — essa responsabilidade é do domínio
- `UPDATE`, `INSERT` ou `DELETE` — **migration não escreve dado**. Todo backfill
  precisa decidir com que valor as linhas antigas ficam, e essa decisão é do
  domínio; em SQL ela vira uma segunda fonte da verdade, sem teste e sem
  conserto, porque migration aplicada não se corrige. A partir da fase 8 o CI
  reprova (job `backend`, passo "migrations não podem escrever dado"); até lá a
  regra vale igual, só não há quem a faça cumprir sozinha.

### Migration que aperta exige dois deploys

`SET NOT NULL`, `DROP COLUMN`, `RENAME` e `UNIQUE` novo quebram a versão
ANTERIOR da aplicação, que continua no ar durante o deploy e é para onde um
rollback volta. Como o Flyway é forward-only, o banco não volta junto com a
imagem — e o rollback sobe sem erro, quebrando no primeiro `INSERT`.

O ciclo é de três passos, e o do meio não é opcional:

1. **Expand** (migration): coluna nova entra **anulável**. As duas versões do
   código funcionam contra este schema.
2. **Código novo em produção**: passa a escrever a coluna em todos os caminhos.
   As linhas legadas são preenchidas por um comando pontual que roda **pelo
   domínio** — nunca por SQL na migration.
3. **Contract** (migration do deploy seguinte): só o `SET NOT NULL`.

A partir da fase 8, `backend/test/repository/compatibilidade_schema_test.go`
transforma isso em falha de build: compara o schema antes e depois e acusa
coluna nova obrigatória, coluna removida e coluna apertada.

---

## Infraestrutura

**Nada é configurado à mão no VPS. Se não está no playbook, não existe.**

A **borda** do VPS é o projeto [`loadbalancer`](https://github.com/caetasousa/loadbalancer)
(repo à parte): um nginx em container recebe 80/443, termina o TLS (certbot) e
roteia por domínio. Esse repo é **dono do host** — Docker, rede `borda`,
firewall, hardening de SSH, e o bloco `:443` de `stacktrack.caetasousa.tech`
(com o split `/api` → API). O stacktrack **não guarda nada de nginx**: os
containers sobem na rede `borda` e o proxy os alcança por nome.

O que `deploy/ansible/` descreve é só **a aplicação** — o usuário de deploy,
diretórios, `.env`, compose, cron do backup, papéis do banco. Um comando dado
por SSH que não tenha uma task equivalente é deriva: ele funciona hoje e
desaparece no próximo servidor. Ao resolver algo, a correção vai para a role, e
o `make infra-apply` é quem aplica.

- nunca escrever configuração de nginx daqui — o roteamento é do `loadbalancer`
- nunca reescrever o crontab inteiro — `ansible.builtin.cron` com `name:`
  gerencia só o bloco marcado
- nunca remover a rede `borda`, que é compartilhada com o `loadbalancer`

**A esteira não escreve arquivo no servidor.** Compose e `backup.sh` são
instalados pelo playbook; o job de deploy só manda `release <sha>` a um comando
forçado (`/usr/local/bin/stacktrack-release`) e falha quando o sha256 do que
está no servidor não bate com o do commit. Acrescentar um `scp` ao CI é recriar
a segunda fonte da verdade que essa divisão desfez.

Segredos ficam em `ansible-vault`, nunca em claro no repositório. `POSTGRES_DB`,
`POSTGRES_USER` e `POSTGRES_PASSWORD` são gravados pelo `initdb` no volume:
mudá-los no vault depois **não** muda o papel no banco, só quebra a conexão da
API — trocar de verdade exige `ALTER ROLE`.

Detalhes em [docs/infraestrutura.md](docs/infraestrutura.md).

---

## README

Sempre que uma nova rota for criada, ela deve ser adicionada à tabela de rotas no `README.md`, com a descrição do que ela faz — inclusive os códigos de erro que importam para quem chama (o 409 do card em disputa, o 404 de quem não participa).

**Não há Swagger, e isso é decisão tomada, não pendência.** O plano original previa Swaggo na fase 8; ficou de fora porque a tabela do README já responde à mesma pergunta, e uma segunda descrição das rotas — gerada de anotações no código — seria uma fonte da verdade a mais para manter alinhada, com o custo de poluir cada handler com blocos `@Summary`/`@Router`. Se a API um dia passar a ser consumida por terceiros, aí a conta muda e vale reabrir.

### Estrutura de documentação

O `README.md` é enxuto — cartão de visitas: badges, stack resumida, quick start, tabela de rotas, comandos de teste, e links para `docs/`. Não crescer o README com conteúdo detalhado; extrair para arquivos dedicados em `docs/`:

- `docs/tecnologias.md` — guia de estudo do stack (o que é, por que está no projeto, fontes oficiais para aprofundar)
- `docs/testes.md` — instruções detalhadas de cada camada de teste (build tags, Testcontainers, Playwright)
- `docs/regra-de-negocio.md` — modelo de negócio
- `docs/producao.md` — arquitetura no VPS, primeiro deploy, backup e operação
- `docs/entrega-continua.md` — CI, segurança de dependências, publicação e deploy

O `PLANO.md` começa pelo estado atual das fases e depois preserva o roteiro e as
retrospectivas. Backlog de produto fica nele; procedimento operacional fica em
`docs/producao.md`. Números históricos de migration precisam ser marcados como
tais e apontar para `backend/migrations/README.md`, que é a sequência vigente.

Ao adicionar uma tecnologia nova ou uma decisão de arquitetura relevante, documentar em `docs/tecnologias.md` seguindo o padrão já estabelecido (o que é → por que está aqui, com referência a arquivo real do código → fontes para estudo), não só mencionar em passant no README.

---

## Comentários no código

Comentários sempre em português.

- Identificadores exportados (Go) recebem doc comment no padrão `// Nome é/faz X`, descrevendo o comportamento — inclusive casos de erro relevantes (ex: `// Executar valida os dados, verifica duplicidade de email e persiste o novo prestador.`).
- Arquivo com papel não óbvio pelo nome/caminho ganha um comentário de cabeçalho explicando para que ele serve (ex: `// Cliente HTTP fino sobre fetch para falar com a API Go.`).
- Fora isso, comentário só quando o "porquê" não é óbvio pelo código — uma decisão, um trade-off, uma limitação do ambiente (ex: `// localStorage indisponível: mantém a escolha só nesta sessão`). Nunca comentário descrevendo o que o código já deixa claro sozinho.
- Anotações Swag/godoc (`@Summary`, `@Router` etc.) seguem o formato exigido pela ferramenta, não esse padrão.

---

## Convenção de commits

Cada commit deve ter um único contexto — nunca misturar feat, fix, docs, chore ou refactor no mesmo commit. Se uma tarefa envolve múltiplos contextos, separar em commits distintos.

Mensagens de commit seguem o padrão Conventional Commits, sempre em português:

- **feat** — nova funcionalidade
- **fix** — correção de bug
- **docs** — só documentação
- **chore** — tarefa de manutenção que não afeta o código de produção (configuração, build, `.gitignore`, dependências)
- **refactor** — reorganização de código sem mudar comportamento
- **test** — adição ou ajuste de testes

### Nunca commitar sem perguntar

**Sempre perguntar antes de fazer `git commit`**, sem exceção — mesmo com a
alteração pronta, revisada e os testes passando. Terminar o trabalho, apresentar
o que mudou e **aguardar confirmação explícita**.

Uma autorização vale só para os commits daquele pedido: pedir de novo na próxima
vez. Pedir para *construir* alguma coisa **não** autoriza commitar.

### Nunca dar push — e nem perguntar

**`git push` é sempre do usuário, nunca do assistente.** Não executar, e
**não perguntar se pode**: a pergunta já empurra uma decisão que não é sua.

Terminado o trabalho e feitos os commits autorizados, apenas informar que está
pronto — quantos commits, o que entrou — e parar por aí. Quem decide se e quando
publicar é o usuário.

Isso vale mesmo quando o push parece a consequência natural do que foi pedido
(implantar, ver a esteira rodar, validar em produção). Nesses casos, descrever o
que aconteceria e deixar a decisão de lado do usuário.
