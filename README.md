# kanbanGo

Quadro Kanban colaborativo em tempo real: várias pessoas movem cards no mesmo
quadro e todas veem na hora, sem recarregar a página.

Projeto de estudo. O eixo é **concorrência em Go e sincronização de estado entre
clientes** — o roteiro completo, fase a fase e com as fontes de estudo de cada
uma, está em [PLANO.md](PLANO.md).

**Fase atual: 0 — fundação.** A stack sobe, a API responde e fala com o banco. O
quadro começa na fase 2; o tempo real, na 5.

## Stack

| Camada | Tecnologia |
|---|---|
| Backend | Go 1.26 · [chi](https://github.com/go-chi/chi) · [pgx](https://github.com/jackc/pgx) · [log/slog](https://pkg.go.dev/log/slog) |
| Banco | PostgreSQL 16 · migrations com [Flyway](https://flywaydb.org/) |
| Frontend | [Svelte 5](https://svelte.dev) + [SvelteKit](https://svelte.dev/docs/kit) · TypeScript · [Tailwind 4](https://tailwindcss.com/) · [Vite](https://vite.dev/) |
| Testes | `go test` · [Vitest](https://vitest.dev/) |
| Ambiente | Docker Compose · [Mailpit](https://mailpit.axllent.org/) (email de dev, a partir da fase 1) |

Arquitetura hexagonal no backend (`domain` / `usecase` / `adapter`) — as
convenções do projeto estão no [CLAUDE.md](CLAUDE.md).

## Quick start

```bash
make run
```

Sobe tudo em primeiro plano; `Ctrl+C` encerra. Na primeira vez o alvo também
prepara o que faltar, e pode repetir à vontade:

- cria o `.env` a partir do exemplo, com `POSTGRES_PASSWORD` aleatória
- cria o `frontend/.env`
- roda o `npm install` do host — **antes** do compose, e isso importa: o
  container `web` monta `node_modules` e `.svelte-kit` em volumes próprios (a
  instalação dele é Alpine/musl, a sua é glibc), e o Docker cria o ponto de
  montagem como **root** quando o diretório ainda não existe. Depois disso todo
  `npm install` seu falharia com `EACCES`. Se já tiver acontecido:
  `docker run --rm -v "$PWD/frontend:/app" alpine chown 1000:1000 /app/node_modules /app/.svelte-kit`

| Serviço | Endereço |
|---|---|
| Frontend | http://localhost:5173 |
| API | http://localhost:8080 |
| Mailpit (emails de dev) | http://localhost:8025 |
| Postgres | `localhost:5432` |

> As portas são as mesmas do agendaGo. Rodando os dois ao mesmo tempo, o
> segundo `docker compose up` falha com "port is already allocated" — derrube o
> outro projeto antes (`docker compose down`).

## Rotas

| Método | Rota | O que faz |
|---|---|---|
| `GET` | `/health` | Liveness: o processo está no ar. Não toca em dependência nenhuma. |
| `GET` | `/ready` | Readiness: faz ping no Postgres. Responde 503 se o banco estiver fora. |

## Testes

```bash
make run             # sobe a stack inteira (Ctrl+C encerra)
make test            # backend (rápidos) + frontend
make test-backend    # go test ./...
make test-frontend   # vitest run

cd backend && make test    # com -race, o detector de corrida
cd frontend && npm run check   # tipos (svelte-check)
```
