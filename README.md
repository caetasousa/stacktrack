# stacktrack

**Quadro Kanban colaborativo em tempo real.** Várias pessoas movem cards no
mesmo quadro e todas veem na hora, sem recarregar a página.

🔗 **[stacktrack.duckdns.org](https://stacktrack.duckdns.org)**

[![CI](https://github.com/caetasousa/stacktrack/actions/workflows/ci.yml/badge.svg)](https://github.com/caetasousa/stacktrack/actions/workflows/ci.yml)

Projeto de estudo. O eixo é **concorrência em Go e sincronização de estado entre
clientes** — o roteiro completo, fase a fase e com as fontes de cada uma, está em
[PLANO.md](PLANO.md).

> **Estado atual: fases 0–11 concluídas, com ajustes de escopo.** O quadro tem
> colaboração ao vivo, presença, replay após reconexão, bloqueio otimista,
> responsáveis, comentários, histórico completo e link público de
> acompanhamento. A fase 12 (aplicar eventos sem recarregar o quadro) está
> adiada porque a medição ainda não justificou o risco. O estado de todas as
> fases está no início do [PLANO.md](PLANO.md).

## Documentação

| | |
|---|---|
| [docs/producao.md](docs/producao.md) | arquitetura no ar, primeiro deploy, backup e operação |
| [docs/entrega-continua.md](docs/entrega-continua.md) | a esteira, a varredura de vulnerabilidades e o deploy automático |
| [docs/tecnologias.md](docs/tecnologias.md) | o stack peça por peça: o que é, por que está aqui, onde estudar |
| [docs/regra-de-negocio.md](docs/regra-de-negocio.md) | o que o produto faz — papéis, convites, cards e ordenação |
| [docs/testes.md](docs/testes.md) | as camadas de teste, o que cada uma cobre e o que não cobre |
| [CLAUDE.md](CLAUDE.md) | convenções do repositório |

## Stack

| Camada | Tecnologia |
|---|---|
| Backend | Go 1.26 · [chi](https://github.com/go-chi/chi) · [pgx](https://github.com/jackc/pgx) · [log/slog](https://pkg.go.dev/log/slog) |
| Banco | PostgreSQL 16 · migrations com [Flyway](https://flywaydb.org/) |
| Frontend | [Svelte 5](https://svelte.dev) + [SvelteKit](https://svelte.dev/docs/kit) · TypeScript · [Tailwind 4](https://tailwindcss.com/) |
| Testes | `go test` · [Vitest](https://vitest.dev/) |
| Ambiente | Docker Compose · [Mailpit](https://mailpit.axllent.org/) |
| Produção | imagens no GHCR pelo GitHub Actions · deploy contínuo por SSH · Caddy + Let's Encrypt |

Arquitetura hexagonal no backend (`domain` / `usecase` / `adapter`). O detalhe de
cada escolha está em [docs/tecnologias.md](docs/tecnologias.md).

> **Sem envio de email.** Confirmação de cadastro e recuperação de senha ficaram
> de fora por isso, e o convite virou **link**: quem já tem conta com o email
> convidado entra na hora; quem não tem recebe um link que o dono copia e envia
> por onde quiser.

## Quick start

Requer Docker com Compose, Node/npm e OpenSSL. Go só é necessário para executar
os testes do backend diretamente no host.

```bash
make run
```

Sobe tudo em primeiro plano; `Ctrl+C` encerra. Na primeira vez o alvo prepara o
que faltar (`.env` com senha aleatória, `frontend/.env`, `npm install` no host) e
pode repetir à vontade.

| Serviço | Endereço |
|---|---|
| Frontend | http://localhost:5173 |
| API | http://localhost:8080 |
| Mailpit (reservado para envio futuro) | http://localhost:8025 |
| Postgres | `localhost:5432` |

As portas são as mesmas do agendaGo — rodando os dois ao mesmo tempo, o segundo
`docker compose up` falha com "port is already allocated". Armadilhas do
ambiente de desenvolvimento (o `EACCES` do `node_modules`, o `air` servindo
binário velho) estão em
[docs/tecnologias.md](docs/tecnologias.md#armadilhas-do-ambiente).

## Testes

```bash
make test            # backend (rápidos) + frontend
make test-backend    # go test ./...
make test-frontend   # vitest run

make test-e2e        # Playwright sobre a stack (exige `make run` no ar)

cd backend && make test-integracao   # repositórios contra Postgres real (Docker)
cd backend && make test-tempo-real   # protocolo do WebSocket (exige a stack no ar)

cd backend && make test        # com -race, o detector de corrida
cd frontend && npm run check   # tipos (svelte-check)
```

Detalhe de cada camada em [docs/testes.md](docs/testes.md).

## Rotas

A sessão viaja num cookie `HttpOnly` + `SameSite=Lax` (com prefixo `__Host-` e
`Secure` em produção). O banco guarda só o SHA-256 do token; a senha, um hash
Argon2id.

| Método | Rota | O que faz |
|---|---|---|
| `GET` | `/health` | Liveness: o processo está no ar. Não toca em dependência nenhuma. |
| `GET` | `/ready` | Readiness: faz ping no Postgres. Responde 503 se o banco estiver fora. |
| `POST` | `/auth/cadastro` | Cria a conta e já abre a sessão (201 + cookie). 409 se o email já existir. |
| `POST` | `/auth/login` | Autentica e abre a sessão (200 + cookie). 401 genérico; 429 no teto por conta. |
| `POST` | `/auth/logout` | Encerra a sessão no servidor e apaga o cookie (204, sempre). |
| `GET` | `/auth/me` | Devolve a conta autenticada. 401 sem sessão válida. |
| `GET` | `/convites/{token}` | Detalhes do convite, **sem sessão** — é o que a tela do link mostra. |
| `GET` | `/publico/{token}` | O quadro de um link de acompanhamento, **sem sessão**: colunas, cards, descrições, etiquetas, prazos e checklists. Sem responsáveis, comentários, anexos, histórico nem ids. **404** para token inventado, revogado ou de quadro apagado — os três iguais. Responde `Cache-Control: no-store` e `X-Robots-Tag: noindex`. |

As demais exigem sessão e têm teto de requisições por sessão.

| Método | Rota | O que faz |
|---|---|---|
| `GET`/`POST` | `/boards` | Lista os quadros de que você participa (com o seu papel) e cria um novo. |
| `GET` | `/boards/{id}` | O quadro com colunas e cards, em ordem, numa requisição só. |
| `PATCH`/`DELETE` | `/boards/{id}` | Renomeia ou apaga o quadro. Só o dono. |
| `PATCH` | `/boards/{id}/fundo` | Troca o fundo do quadro. Só o dono. |
| `GET` | `/boards/{id}/publicacao` | O estado do link público, com a URL. Só o dono — **403** para editor e leitor, porque o token é o segredo do link. |
| `PUT` | `/boards/{id}/publicacao` | Liga o link público e devolve a URL. Só o dono. Idempotente: repetir devolve o **mesmo** link. |
| `DELETE` | `/boards/{id}/publicacao` | Revoga o link (204, mesmo que já não houvesse). O endereço morre na hora, e publicar de novo gera um token **diferente**. |
| `POST` | `/boards/{id}/colunas` | Cria uma coluna no fim do quadro (201). |
| `PATCH`/`DELETE` | `/colunas/{id}` | Renomeia (título e cor) ou apaga a coluna e os cards dela. |
| `PATCH` | `/colunas/{id}/mover` | Reordena a coluna no quadro, **pelos vizinhos**. |
| `POST` | `/colunas/{id}/cards` | Cria um card no fim da coluna (201). |
| `GET` | `/cards/{id}` | O card com etiquetas, checklists e anexos — o que o modal mostra. |
| `PATCH`/`DELETE` | `/cards/{id}` | Edita título, descrição e cor, ou apaga. **409** se `version` estiver defasada. |
| `PATCH` | `/cards/{id}/mover` | Move o card. Recebe os **vizinhos**, não a ordem. **409** se eles vierem fora de ordem (a tela estava velha) ou se a lista já foi reordenada vezes demais naquele ponto. |
| `PATCH` | `/cards/{id}/prazo` | Marca a data de entrega; `null` limpa. |
| `GET`/`POST` | `/boards/{id}/membros` | Quem participa (com os convites pendentes, para o dono) e convida informando o email. Se ainda não houver conta, devolve o link para o dono enviar. |
| `PATCH`/`DELETE` | `/boards/{id}/membros/{usuarioId}` | Troca o papel ou remove do quadro. Só o dono. |
| `DELETE` | `/boards/{id}/convites/{conviteId}` | Revoga um convite, invalidando o link (204). |
| `POST` | `/convites/{token}/aceitar` | Aceita o convite e entra no quadro. |
| `GET`/`POST` | `/boards/{id}/etiquetas` | Lista e cria as etiquetas do quadro. |
| `PATCH`/`DELETE` | `/etiquetas/{id}` | Edita nome e cor, ou apaga (some de todos os cards). |
| `PUT`/`DELETE` | `/cards/{id}/etiquetas/{etiquetaId}` | Aplica e tira a etiqueta do card. |
| `PUT`/`DELETE` | `/cards/{id}/responsaveis/{usuarioId}` | Marca e desmarca quem responde pelo card. **422** se a pessoa não participa do quadro. |
| `GET` | `/cards/{id}/atividade` | O histórico do card: o que aconteceu, quem fez e quando. Read model sobre o log de eventos. |
| `GET` | `/boards/{id}/atividade` | O histórico do quadro: tudo o que aconteceu — card, coluna, etiqueta, anexo, checklist, responsável, comentário, participação —, do mais recente para o mais antigo. Qualquer membro lê. Parâmetros opcionais: `filtro=movimentacoes\|tudo` (padrão `movimentacoes`), `autor={usuarioId}` e `antesDe={seq}` — o cursor da página seguinte, e não um número de página. Devolve `temMais` para a tela não oferecer um "carregar mais" vazio. |
| `GET`/`POST` | `/cards/{id}/comentarios` | A conversa do card, do mais antigo para o mais novo, e escrever nela. Basta participar — comentar não exige papel de edição. |
| `PATCH`/`DELETE` | `/comentarios/{id}` | Edita ou apaga. **403** ao editar o de outra pessoa: só o autor edita. Apagar, o autor no próprio e o dono em qualquer um. |
| `POST` | `/cards/{id}/checklists` | Cria uma checklist no card. |
| `PATCH`/`DELETE` | `/checklists/{id}` | Renomeia ou apaga (os itens vão junto). |
| `POST` | `/checklists/{id}/itens` | Cria uma linha. |
| `PATCH`/`DELETE` | `/itens/{id}` | Marca, renomeia ou apaga a linha. |
| `POST` | `/cards/{id}/anexos/link` | Anexa uma URL. |
| `POST` | `/cards/{id}/anexos/arquivo` | Envia um arquivo (multipart, até 10 MB). |
| `GET`/`DELETE` | `/anexos/{id}` | Baixa ou apaga o anexo. |
| `GET` | `/ws?board={id}&desde={seq}` | Abre o WebSocket do quadro. Com `desde`, repõe o que se perdeu antes de voltar ao vivo. Fora do teto por sessão: é uma requisição só que dura horas. |

**A rota nunca informa o quadro.** Card, coluna e etiqueta são identificados por
si, e o servidor descobre sozinho a que quadro pertencem (card → coluna →
quadro). Informar um quadro pela URL não daria acesso a nada.
