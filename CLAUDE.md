# stacktrack

Quadro Kanban colaborativo em tempo real (Go + SvelteKit). O mapa da
documentação está em [README.md](README.md); o que ainda falta, em
[docs/historico-do-projeto.md](docs/historico-do-projeto.md#-o-que-ainda-falta).

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
  conserto, porque migration aplicada não se corrige. O `make verificar` roda o
  grep que a confere — mas só se alguém rodar: não há esteira que o faça
  cumprir sozinha.

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

**Nada é configurado à mão no servidor. Se não está no playbook, não existe.**

O servidor é a distro **`ubuntu-server` do WSL desta máquina**, publicada por um
**túnel da Cloudflare** (`stacktrack.d4ffy.com` → `http://localhost:8580`, rota
administrada no painel). Ele foi montado pelo `site.yml` do **studygo**
(`~/studygo/ansible`), que é **dono do host** — sshd na 2222, pacotes do Docker
rootless, `cloudflared`, nginx de sistema. O stacktrack só pressupõe isso pronto.

O que `deploy/ansible/` descreve é só **o que é do stacktrack** — o usuário de
deploy e o Docker rootless dele, diretórios, `.env`, compose, cron do backup,
papéis do banco. Um comando dado por SSH que não tenha uma task equivalente é
deriva: ele funciona hoje e desaparece no próximo servidor. Ao resolver algo, a
correção vai para a role, e o `make infra-apply` é quem aplica.

- o roteamento vive no serviço `borda` (nginx) do `docker-compose.prod.yml`,
  com a config embutida — nada de config de proxy fora dele, e nada no nginx
  nem no `cloudflared` do servidor
- só a `borda` publica porta, e só em `127.0.0.1:8580`: as distros do WSL dividem
  a rede, e a 8480 é do studygo
- a stack roda no Docker **rootless** do usuário `stacktrack`; nunca o Docker de
  sistema, nunca o grupo `docker`
- nunca reescrever o crontab inteiro — `ansible.builtin.cron` com `name:`
  gerencia só o bloco marcado
- não mexer no que é do studygo no servidor (o usuário dele, o `cloudflared`, o
  nginx de sistema). A conta `studygo` só serve para o primeiro
  `make infra-preparar`, quando o `stacktrack` ainda não existe
- as chaves SSH do servidor são só do stacktrack: `~/.ssh/stacktrack_servidor`
  (operador, shell e sudo) e `~/.ssh/stacktrack_release` (presa ao wrapper,
  três verbos, nenhum que escreva arquivo). Trocar uma é pôr a antiga em
  `chaves_revogadas` no `vars.yml` — com `exclusive: false`, ela continuaria
  valendo

**Não há esteira.** O GitHub Actions foi removido: testar, buildar, publicar as
imagens no GHCR e implantar são passos manuais — ver
[docs/entrega-continua.md](docs/entrega-continua.md). O `make verificar` reúne o
que ela conferia sozinha; antes de publicar, ele, a integração e o E2E.

O deploy é `stacktrack-release release <sha>` (o wrapper faz backup, `pull`,
para web e api, `up -d`). O `docker-compose.prod.yml` e o `backup.sh` são do
Ansible (`make infra-apply`).

Segredos ficam em `ansible-vault`, nunca em claro no repositório. `POSTGRES_DB`,
`POSTGRES_USER` e `POSTGRES_PASSWORD` são gravados pelo `initdb` no volume:
mudá-los no vault depois **não** muda o papel no banco, só quebra a conexão da
API — trocar de verdade exige `ALTER ROLE`.

Detalhes em [docs/infraestrutura.md](docs/infraestrutura.md).

---

## 📚 Documentação

### Fontes de verdade

Cada assunto tem **um** documento dono. Evite duplicar o que está nele — aponte
para ele:

| | Arquivo | Contém |
|---|---|---|
| 🛠️ | `Makefile` | os comandos (`make ajuda` lista; a descrição fica no próprio alvo) |
| 📖 | `README.md` | o produto, a stack e o mapa da documentação |
| 🐣 | `docs/como-funciona.md` | o projeto sem jargão, para quem chega agora |
| 🧭 | `docs/funcionalidades.md` | cada funcionalidade: na tela, por baixo, as regras e os limites |
| 📐 | `docs/arquitetura.md` | camadas, decisões centrais, modelo de dados, contrato HTTP, **rotas** e vocabulário |
| 🚀 | `docs/rodar-local.md` | ambiente local, o `.env`, o dia a dia e as armadilhas |
| 🔄 | `docs/fluxo-de-trabalho.md` | o caminho de uma mudança: cenário, código, checagens, commit, publicação |
| 🧪 | `docs/testes.md` | cada camada de teste: build tags, Testcontainers, Playwright |
| 🧪 | `frontend/e2e/CENARIOS.md` | como cada funcionalidade pode quebrar, com os ids que os testes citam |
| 📖 | `docs/tecnologias.md` | o stack peça por peça: o que é → por que está aqui → onde estudar |
| 🔁 | `docs/entrega-continua.md` | publicação, varredura de vulnerabilidades e deploy, todos manuais |
| 🚢 | `docs/producao.md` | o servidor no ar: primeiro deploy, backup, email, diagnóstico e manutenção |
| 🏗️ | `docs/infraestrutura.md` | o Ansible: playbooks, segredos, recomeço do zero |
| 🐘 | `backend/migrations/README.md` | a sequência vigente das migrations e o porquê de cada uma |
| 📜 | `docs/historico-do-projeto.md` | o caminho já percorrido e o que ainda falta |
| 📌 | `go.mod`, `package.json`, lockfiles, Dockerfiles, compose | versões |

**Quando documentação e código divergirem, vale o código:** investigue a
implementação e corrija a documentação relacionada no mesmo conjunto de
alterações. Documento que descreve o que o sistema fazia é pior que documento
nenhum, porque é lido com confiança.

### O que muda junto com o código

- **Rota nova** entra na tabela de rotas de
  [docs/arquitetura.md](docs/arquitetura.md#rotas), com a descrição do que ela
  faz — inclusive os códigos de erro que importam para quem chama (o 409 do card
  em disputa, o 404 de quem não participa).
- **Regra ou tela nova** muda a seção dela em
  [docs/funcionalidades.md](docs/funcionalidades.md), no roteiro que o documento
  já usa: na tela → por baixo → regras → no código.
- **Tecnologia nova ou decisão de arquitetura relevante** entra em
  `docs/tecnologias.md` no padrão estabelecido (o que é → por que está aqui, com
  referência a arquivo real do código → fontes para estudo) — e não só uma
  menção de passagem.
- **Comando novo** ganha a descrição `## …` na linha do alvo do `Makefile`.

**Não há Swagger, e isso é decisão tomada, não pendência.** O plano original
previa Swaggo na fase 8; ficou de fora porque a tabela de rotas já responde à
mesma pergunta, e uma segunda descrição das rotas — gerada de anotações no
código — seria uma fonte da verdade a mais para manter alinhada, com o custo de
poluir cada handler com blocos `@Summary`/`@Router`. Se a API um dia passar a ser
consumida por terceiros, aí a conta muda e vale reabrir.

### Como os documentos são escritos

- O `README.md` é **cartão de visitas**: o que o produto faz, a stack, como
  começar, os comandos e o mapa de `docs/`. Detalhe vai para o documento dono.
- Todo documento de `docs/` abre com **"Em uma frase:"** — o que ele responde —
  e aponta para `como-funciona.md` quem chega sem contexto.
- `como-funciona.md` não usa jargão: o termo técnico, quando aparece, vem
  explicado.
- Descreve-se o sistema **como ele é hoje**. Quando uma decisão substituiu outra,
  o motivo fica registrado só se ainda restringe o que pode mudar; a história vai
  para `docs/historico-do-projeto.md`.
- O que ainda falta fica em `docs/historico-do-projeto.md`; procedimento
  operacional, em `docs/producao.md`. Números históricos de migration são marcados como tais e
  apontam para `backend/migrations/README.md`, que é a sequência vigente.

---

## Comentários no código

Comentários sempre em português.

- Identificadores exportados (Go) recebem doc comment no padrão `// Nome é/faz X`, descrevendo o comportamento — inclusive casos de erro relevantes (ex: `// Executar valida os dados, verifica duplicidade de email e persiste o novo prestador.`).
- Arquivo com papel não óbvio pelo nome/caminho ganha um comentário de cabeçalho explicando para que ele serve (ex: `// Cliente HTTP fino sobre fetch para falar com a API Go.`).
- Fora isso, comentário só quando o "porquê" não é óbvio pelo código — uma decisão, um trade-off, uma limitação do ambiente (ex: `// localStorage indisponível: mantém a escolha só nesta sessão`). Nunca comentário descrevendo o que o código já deixa claro sozinho.
- Anotações Swag/godoc (`@Summary`, `@Router` etc.) seguem o formato exigido pela ferramenta, não esse padrão.

---

## 🧪 Testes

- Um teste confere o que o usuário vê ou o que fica gravado — nunca a ordem
  das chamadas a uma porta.
- **E2E primeiro**, para o que passa pela tela. No final, produza um artefato
  verificável e repetível.
- **Integração** (tag `integracao`, PostgreSQL efêmero do Testcontainers) para
  o resto do backend: casos de uso ligados aos repositories reais e à
  `UnidadeDeTrabalho` de verdade (`backend/test/repository/convite_email_test.go`)
  e o servidor inteiro por HTTP, com a mesma montagem da produção
  (`backend/cmd/api/servidor_test.go`).
- **Teste sem Docker só para** função pura de domínio ou de parser cujo
  resultado o usuário vê (a chave de ordenação, o texto do email de convite,
  o Markdown do comentário, o filtro de cards, as frases da auditoria) e para os
  guarda-corpos estruturais (toda coluna das migrations coberta pelo SQL, o
  wrapper de release).
- **Proibido fake ou mock de port nosso no lugar do banco.** Dublê só na
  fronteira de fora: relógio, servidor SMTP (`backend/test/smtpfalso`; o Mailpit
  no E2E). Falha de gravação se provoca no próprio banco — lock segurado por
  outra transação, `statement_timeout` (ver `tetos_de_banco_test.go`). Os fakes
  em memória de `backend/test/repository/memoria` são legado: não crescem, e
  teste novo não os usa.
- **NUNCA** escreva o teste depois do código: **PRIMEIRO** escreva como a
  funcionalidade pode quebrar (em `frontend/e2e/CENARIOS.md` ou no comentário
  do teste), **DEPOIS** o código.

A suíte E2E vive em `frontend/e2e/`: o catálogo de falhas em
`frontend/e2e/CENARIOS.md` (cada teste cita no título o id que cobre) e o
relatório de cada execução em `frontend/e2e/relatorio/resumo.md`, com o
resultado e o print de cada cenário. Como rodar está em
[docs/testes.md](docs/testes.md).

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
(publicar, implantar, validar em produção). Nesses casos, descrever o que
aconteceria e deixar a decisão de lado do usuário.
