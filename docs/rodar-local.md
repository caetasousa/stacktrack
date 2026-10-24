# 🚀 Rodar localmente

> **Em uma frase:** `make run` sobe tudo — banco, migrations, API, tela e o
> Mailpit —, com recarga automática nos dois lados, e `Ctrl+C` derruba.
>
> Novo por aqui? Leia [como-funciona.md](como-funciona.md) antes.

---

## 📋 Pré-requisitos

| Ferramenta | Para quê |
|---|---|
| Docker com Compose v2 | a stack inteira roda em containers |
| Go 1.26 | os testes do backend no host (`make verificar`, `make test-integracao`) |
| Node 24 e npm | o `node_modules` do host e os testes do frontend |
| `openssl` | a senha aleatória do banco na primeira subida |
| Chromium do Playwright | só para o E2E: `cd frontend && npx playwright install chromium` |

---

## 1️⃣ Subir a stack

```bash
make run
```

Na primeira vez ele prepara o que falta, e só o que falta — repetir é seguro:

- cria o `.env` a partir do `.env.example`, com uma **senha aleatória** para o
  Postgres;
- cria o `frontend/.env`;
- roda o `npm install` **no host**, antes do compose (o porquê está em
  [Armadilhas](#-armadilhas-conhecidas)).

Depois sobe os serviços em primeiro plano, com os logs de todos juntos:

| Serviço | Endereço | O que é |
|---|---|---|
| `web` | <http://localhost:5173> | a tela (Vite, com recarga ao salvar) |
| `api` | <http://localhost:8080> | a API Go (`air`, recompila ao salvar) |
| `mailpit` | <http://localhost:8025> | a caixa de entrada de mentira: todo email do dev cai aqui |
| `postgres` | `localhost:5432` | o banco, com volume próprio |
| `flyway` | — | aplica as migrations e termina; a API só sobe depois dele |

`Ctrl+C` encerra tudo — e exercita, de quebra, o desligamento gracioso da API.

### Primeiro uso

1. Abra <http://localhost:5173>, **crie uma conta** e um quadro.
2. Para ver o convite de ponta a ponta: em **Membros**, convide um email
   qualquer (`bia@exemplo.com`), abra o **Mailpit** e clique no link do email
   numa **janela anônima** — é outra sessão, e lá você cria a conta da Bia.
3. Com as duas janelas no mesmo quadro, mova um card numa e veja a outra se
   atualizar sozinha.

---

## 2️⃣ O que muda em relação à produção

O mesmo código, com outros valores — e cada diferença é decisão:

| | Desenvolvimento | Produção |
|---|---|---|
| email | **sempre o Mailpit**; o compose não lê `SMTP_*` do `.env` | Brevo, com STARTTLS obrigatório |
| chave da fila de email | efêmera — a fila não sobrevive a um reinício | `EMAIL_CHAVE`, do vault |
| tetos de requisição | frouxos (200 cadastros/min…) | os do produto (5 cadastros/min…), obrigatórios |
| intervalo entre reenvios de convite | 0 | 2 minutos |
| origem | tela em `:5173`, API em `:8080` (CORS por `FRONTEND_ORIGIN`) | uma origem só, `/api` é um caminho |
| cookie de sessão | `stacktrack_session`, sem `Secure` | `__Host-…`, com `Secure` |

O email nunca vai para um provedor de verdade em desenvolvimento **de
propósito**: a suíte E2E convida dezenas de endereços inventados por execução, e
cada `make test-e2e` gastaria a cota e a reputação do remetente mandando email
para domínios de terceiros.

Os tetos frouxos também são de propósito: com os de produção, a suíte E2E levaria
429 no meio do caminho e os testes se pulariam sozinhos — verde sem ter
exercitado nada.

### 🔑 O `.env`

Em desenvolvimento só o bloco do Postgres importa (`POSTGRES_DB`,
`POSTGRES_USER`, `POSTGRES_PASSWORD`); o resto do `.env.example` está comentado e
documenta os padrões. As variáveis de produção moram no **vault**
(`deploy/ansible/segredos/producao.yml`) e chegam ao servidor pelo `make
infra-apply` — ver [producao.md](producao.md).

---

## 3️⃣ No dia a dia

| Você mudou… | O que acontece | Precisa fazer algo? |
|---|---|---|
| um `.go`, `.txt` ou `.html` do backend | o `air` recompila e reinicia a API | não — confira o log se a rota nova der 404 |
| um `.svelte` ou `.ts` do frontend | o Vite recarrega a tela | não |
| uma migration nova | nada: o Flyway só roda na subida | `docker compose up flyway` |
| o `.air.toml` | nada: o `air` lê a configuração só ao iniciar | `docker compose restart api` |
| uma dependência Go (`go get`) | o container não tem o módulo | `docker compose restart api` |
| uma dependência do frontend | o host já tem; o container não | `npm install <pacote>` **no host** e reinicie o `web` |

### O banco

```bash
docker compose exec postgres psql -U stacktrack -d stacktrack   # um psql
docker compose down -v                                          # APAGA tudo e recomeça
```

O `down -v` apaga os volumes — banco, anexos e caches. É também o único jeito de
trocar a `POSTGRES_PASSWORD`: ela é gravada pelo `initdb` na primeira subida, e
mudá-la no `.env` depois só quebra a conexão da API.

---

## 4️⃣ Checagens

```bash
make verificar         # sem Docker: formato, vet, migrations, -race, lint, svelte-check, Vitest
make test-integracao   # casos de uso e servidor HTTP contra PostgreSQL de verdade (Docker)
make test-e2e          # Playwright — exige o `make run` no ar, noutro terminal
make test-tempo-real   # duas abas pelo WebSocket — também exige o `make run`
make ajuda             # a lista dos alvos
```

O `make test-e2e` termina gerando `frontend/e2e/relatorio/resumo.md`, com o
resultado e o print de cada cenário. Cada camada, e quando usar qual, está em
[testes.md](testes.md).

---

## 🪤 Armadilhas conhecidas

Coisas que já custaram tempo aqui.

### `EACCES` no `node_modules`

O container `web` monta `node_modules` e `.svelte-kit` em volumes próprios — a
instalação dele é Alpine/musl, a sua é glibc, e misturar quebra os binários
nativos do Vite. O Docker cria o ponto de montagem como **root** quando o
diretório ainda não existe, e depois disso todo `npm install` seu falha.

Por isso o `make run` instala no host **antes** do compose. Se já aconteceu:

```bash
docker run --rm -v "$PWD/frontend:/app" alpine \
  chown 1000:1000 /app/node_modules /app/.svelte-kit
```

### O `air` servindo binário velho

Quando a build falha, o `air` **mantém no ar o binário anterior**: a API responde
normalmente e só as rotas novas dão 404 — o que parece erro de código e é erro de
build. É o que acontece logo depois de um `go get`, porque o container ainda não
baixou o módulo:

```bash
go get <módulo>              # no host, atualiza go.mod/go.sum
docker compose restart api   # o container baixa e recompila
```

Na dúvida, `docker compose logs api` mostra a falha.

### O `air` compilando só um arquivo

O `.air.toml` compila o **pacote** `./cmd/api`, e não o `main.go`: a montagem da
aplicação mora em `cmd/api/montagem.go`, e um `go build main.go` não a enxerga.
Mudou o `.air.toml`? Reinicie a API — ele só é lido na partida do `air`.

### Porta já alocada

Rodando outra stack local nas mesmas portas (5173, 8080, 5432, 8025, 1025), o
segundo `docker compose up` falha com "port is already allocated" — derrube a
outra antes.

### A tela recarregando sozinha durante o E2E

O Vite recarrega a página quando um arquivo vigiado muda, e o E2E escreve prints
e o relatório dentro de `frontend/`. Por isso o `vite.config.ts` ignora
`e2e/relatorio/` no vigia. Se mover o relatório de lugar, leve a exceção junto —
sem ela a suíte inteira falha com páginas que "somem" no meio do teste.
