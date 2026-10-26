# 🔄 Fluxo de trabalho

> **Em uma frase:** escreva primeiro como a funcionalidade pode quebrar, depois o
> código; rode `make verificar`, a integração e o E2E; commite um contexto por
> vez — e publicar e implantar são passos seus, à mão.
>
> Novo por aqui? Leia [como-funciona.md](como-funciona.md) antes. Para subir a
> stack, [rodar-local.md](rodar-local.md).

**cenário → código → verificar → commitar → publicar → implantar**

```
escrever como quebra ......... frontend/e2e/CENARIOS.md, ou o comentário do teste
     │
     ├── make run ............ stack local de pé, recarga automática
     ├── escrever o código e o teste que cita o cenário
     │
     ├── make verificar ...... sem Docker: formato, vet, migrations, -race, lint, Vitest
     ├── make test-integracao  PostgreSQL de verdade (Docker)
     ├── make test-e2e ....... navegador de verdade (com o `make run` no ar)
     │
     ├── git add <arquivos>    você escolhe o que entra
     ├── git commit .......... um contexto por commit
     ├── git push ............ sempre você
     │
     ├── publicar as imagens . docs/entrega-continua.md
     └── stacktrack-release release <sha>
```

Não há esteira: nenhum desses passos roda sozinho, e quem pula um não fica
sabendo do que só ele pegaria.

---

## 1️⃣ Antes do código: como pode quebrar

**Primeiro** se escreve como a funcionalidade pode deixar de funcionar para quem
usa; **depois**, o código. A regra está no [CLAUDE.md](../CLAUDE.md#-testes), e
o motivo é prático: o teste escrito depois do código tende a conferir o que o
código faz, e não o que ele deveria fazer.

- **O que passa pela tela** ganha uma linha em
  [`frontend/e2e/CENARIOS.md`](../frontend/e2e/CENARIOS.md) — id, como quebra e o
  que o usuário vê:

  ```
  | E8 | reenviar não manda email novo, ou o link anterior continua valendo | o convidado não recebe; ou o link vazado volta a valer |
  ```

  O teste cita o id no título (`test('[E8] …')`), e o relatório lista o resultado
  por id. Item sem teste aparece como **lacuna declarada**, e não como
  esquecimento.

- **O que não passa pela tela** — a retentativa do email, a corrida entre duas
  transações, a cifra do link no banco — tem os cenários escritos no comentário
  do teste de integração que os cobre.

Qual camada cobre o quê, e o que é proibido (fake de porta nossa no lugar do
banco, por exemplo), está em [testes.md](testes.md#a-política).

---

## 2️⃣ Desenvolver

```bash
make run                          # sobe tudo → http://localhost:5173
docker compose logs -f api        # acompanhar a API, noutro terminal
docker compose up flyway          # aplicar uma migration nova sem derrubar a stack
```

Salvou um `.go`, a API recompila sozinha; salvou um `.svelte`, a tela recarrega.
O que pede passo manual — migration, `.air.toml`, dependência nova — está em
[rodar-local.md](rodar-local.md#3️⃣-no-dia-a-dia).

### Ao mexer em…

| Você mexeu em… | Leve junto |
|---|---|
| uma **rota** | a linha dela na tabela de [arquitetura.md](arquitetura.md#rotas), com os códigos de erro que importam para quem chama |
| uma **regra ou tela** | a seção dela em [funcionalidades.md](funcionalidades.md) |
| uma **migration** | as regras do [CLAUDE.md](../CLAUDE.md#migrations-banco-de-dados) (não escreve dado; o que aperta exige dois deploys) e a linha em [backend/migrations/README.md](../backend/migrations/README.md) |
| uma **tecnologia nova** ou decisão de arquitetura | a entrada em [tecnologias.md](tecnologias.md): o que é → por que está aqui → fontes |
| uma **variável de ambiente** | o `.env.example` e, se for de produção, o `vars.yml` ou o vault e o `env.j2` |
| a **infraestrutura** | a role do Ansible — nada à mão no servidor ([infraestrutura.md](infraestrutura.md)) |

Documentação e código discordando, vale o código: investigue a implementação e
corrija o documento. O mapa de qual documento responde por qual assunto está no
[CLAUDE.md](../CLAUDE.md#fontes-de-verdade).

---

## 3️⃣ Verificar

```bash
make verificar
```

É o que a esteira conferia sozinha, reunido num comando — e que agora só
acontece se alguém rodar:

| | O que roda | Onde |
|---|---|---|
| 🐹 | `gofmt -l` · `go vet` · `go test -race ./...` | `backend/` |
| 🐘 | o grep de que **migration não escreve dado** | `backend/migrations/` |
| 🧡 | Prettier · `svelte-check` · Vitest | `frontend/` |

O que precisa de Docker fica de fora dele, de propósito:

```bash
make test-integracao   # casos de uso e o servidor HTTP contra PostgreSQL de verdade
make test-e2e          # Playwright contra a stack — exige o `make run` no ar
make test-tempo-real   # duas abas pelo WebSocket — também exige o `make run`
```

> [!WARNING]
> A integração roda num PostgreSQL efêmero do Testcontainers, nunca no banco
> local. O E2E, ao contrário, usa a stack de desenvolvimento **inteira** — o
> banco local ganha as contas e os quadros que a suíte cria.

### O artefato do E2E

O `make test-e2e` termina gerando `frontend/e2e/relatorio/resumo.md` — mesmo
quando a suíte falha, que é quando ele mais importa. Cada cenário do catálogo
aparece com o resultado, o print do estado final e o commit testado (marcado
"com alterações locais" quando a árvore não está limpa). É o que se confere antes
de publicar.

---

## 4️⃣ Commitar

```bash
git config --local --list | grep user        # a identidade é sempre caetasousa
git add backend/internal/usecase/board/membros.go frontend/e2e/convite.spec.ts
git commit -m "feat: reenviar o convite troca o link e renova a validade"
```

- **Um contexto por commit.** Nunca `feat` e `docs` juntos: a tarefa que mexe nos
  dois vira dois commits.
- **Conventional Commits, em português**, no imperativo:

  | | Tipo | Quando |
  |---|---|---|
  | ✨ | `feat` | funcionalidade nova |
  | 🐛 | `fix` | correção de bug |
  | ♻️ | `refactor` | reorganização sem mudar comportamento |
  | 🧪 | `test` | só testes |
  | 📝 | `docs` | só documentação |
  | 🔧 | `chore` | configuração, build, `.gitignore`, dependências |

- **Você escolhe o que entra.** `git add` por arquivo, e não `git add -A`: o que
  entra sem ser olhado — um print esquecido, uma chave gerada sem querer na raiz
  — fica para sempre no histórico. O `.env` e a `.senha-vault` estão no
  `.gitignore`, mas o `.gitignore` só protege do que alguém já previu.
- **A autoria é sempre `caetasousa`.** Se a configuração local sumir (clone novo),
  restaure-a antes de qualquer commit — os comandos estão no
  [CLAUDE.md](../CLAUDE.md#identidade-no-git).

> [!IMPORTANT]
> **Com o assistente (Claude Code):** ele pergunta antes de **cada** commit — a
> autorização vale só para aquele pedido — e **nunca dá push**, nem pergunta se
> pode. Publicar é sempre decisão sua.

---

## 5️⃣ Publicar e implantar

```bash
git push                                          # você
```

Depois, um comando ([entrega-continua.md](entrega-continua.md)):

```bash
make deploy                                       # verifica, publica e implanta
```

Ele constrói e publica as três imagens no GHCR com a tag no SHA do commit,
aplica a configuração do servidor (`make infra-apply`), faz o release e confere
que a versão nova está no ar. Quando o compose, o `.env`, o `backup.sh` ou os
papéis do banco mudaram, rode antes o `make infra-check`, que mostra o diff do
que vai mudar. Depois, o teste que vale mais que qualquer checagem: cadastro →
login → criar quadro → F5.

### 📦 O que o deploy faz

O servidor **não compila nada**: ele só puxa imagem pronta. O wrapper
`stacktrack-release` — o único comando que a chave de deploy alcança — faz, nessa
ordem: backup do banco, `pull`, para a `web` e a `api` **juntas**, `up -d` (o
Flyway roda antes de a API subir) e limpa as imagens órfãs.

Voltar atrás é o mesmo comando com o SHA anterior — e o banco **não** volta junto:
[producao.md](producao.md#voltar-atrás) explica quando isso importa.

Montar o servidor do zero (a conta de deploy, as chaves, o primeiro
provisionamento) roda uma vez só: [producao.md](producao.md#-roteiro-do-primeiro-deploy)
e [infraestrutura.md](infraestrutura.md).

---

## 📖 Resumo dos alvos

`make ajuda` lista todos. Os que importam no dia a dia:

| Alvo | Para quê |
|---|---|
| `make run` | a stack local, em primeiro plano (`make` sozinho faz o mesmo) |
| `make verificar` | o que a esteira conferia, sem Docker |
| `make test` · `test-backend` · `test-frontend` | só os testes rápidos |
| `make test-integracao` | PostgreSQL de verdade (Docker) |
| `make test-e2e` · `test-tempo-real` | o app inteiro pelo navegador · duas abas pelo WebSocket (exigem o `make run`) |
| `make infra-check` · `infra-apply` | ver e aplicar a configuração do servidor |
| `make infra-validar` | sintaxe e lint do Ansible e o teste do wrapper, sem tocar no servidor |
| `make infra-segredos` · `infra-preparar` | uma vez só: o vault e a conta de deploy |
