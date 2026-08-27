# stacktrack — histórico do projeto

Este documento preserva o caminho que trouxe o stacktrack até o estado atual.
Ele **não é o backlog** e não deve ser usado para decidir a próxima entrega. O
roadmap executável, com prioridades, dependências e critérios de aceite, está em
[PLANO.md](../PLANO.md).

O histórico foi separado porque o plano anterior misturava quatro coisas:

- o que ainda precisava ser feito;
- o que já estava pronto;
- alternativas que já tinham sido descartadas;
- material de estudo e retrospectivas técnicas.

Tudo isso era útil durante a construção, mas escondia riscos de produção entre
fases concluídas. Aqui ficam apenas os fatos e aprendizados que continuam
relevantes. A história exata de cada alteração permanece no Git.

---

## Origem

O stacktrack nasceu como um projeto de aprendizagem para avançar além de CRUD,
autenticação e deploy tradicionais. O eixo escolhido foi **concorrência em Go e
sincronização de estado entre clientes**, usando um quadro Kanban como domínio.

A base tecnológica reaproveitou decisões que já eram conhecidas:

- Go, chi e pgx no backend;
- PostgreSQL e Flyway;
- arquitetura hexagonal;
- SvelteKit, Svelte 5 e TypeScript;
- Docker Compose em desenvolvimento e produção;
- Caddy como proxy e terminador TLS (depois substituído pela borda Traefik do
  projeto `loadbalancer` — ver [producao.md](producao.md));
- GitHub Actions e, posteriormente, Ansible.

O produto cresceu até permitir que várias pessoas trabalhassem no mesmo quadro,
com presença, eventos em tempo real, reconexão, comentários, auditoria, anexos,
responsáveis, filtros e link público de acompanhamento.

---

## Linha do tempo condensada

| Fase histórica | Resultado efetivamente entregue |
|---|---|
| 0 — Fundação | Ambiente local, API Go, frontend SvelteKit, PostgreSQL, Flyway, Compose e comandos de desenvolvimento |
| 1 — Contas e sessão | Cadastro, login, logout, perfil, Argon2id, sessão por cookie seguro e rate limiting; confirmação e recuperação por e-mail ficaram de fora |
| 2 — Quadro, colunas e cards | CRUD, papéis de dono/editor/leitor e autorização por recurso |
| 3 — Convites e colaboração | Conta existente era adicionada diretamente; para endereço sem conta, o dono recebia um link secreto. O primeiro caminho originou a contenção A1 |
| 4 — Arrastar e ordenar | Drag-and-drop e primeira ordenação fracionária numérica |
| 5 — Tempo real | WebSocket autenticado, hub por quadro e propagação de mudanças |
| 6 — Presença e conflito | Presença, indicação de edição de coluna e bloqueio otimista de cards |
| 7 — Reconexão e replay | `board_events`, cursor por sequência, backlog e reconexão com jitter |
| 8 — Base de produção | Testcontainers, CI, imagens de produção, Caddy, deploy e documentação operacional |
| 9 — Chave textual | Substituição do `double precision` por chaves textuais e ciclo expand/backfill/contract |
| 10 — Responsáveis e filtros | Responsáveis por card e filtros combináveis |
| 11 — Comentários e auditoria | Comentários Markdown, histórico de card e auditoria completa do quadro |
| 12 — Aplicação incremental | Não implementada; a decisão foi adiada com base numa medição pequena e depois absorvida pelo novo roadmap de capacidade |
| 13 — Arquivamento | Implementado e removido por decisão de produto; não deixou coluna ou migration pendente |
| 14 — Limite WIP | Nunca implementado e retirado do backlog ativo |
| 15 — Múltiplas APIs | Nunca implementada; deixou de ser fase e passou a depender de um gatilho real de escala |
| 16 — Infraestrutura como código | Provisionamento base em Ansible concluído; hardening, DR e promoção de artefato foram absorvidos pelo novo roadmap |

Também entraram fora dessa sequência:

- etiquetas e cores;
- prazos;
- checklists;
- anexos de arquivo e link;
- compartilhamento público por token;
- confirmação própria para ações destrutivas;
- suporte móvel e gestos de toque;
- indicador de quem está editando uma coluna.

---

## Decisões que continuam valendo

### Arquitetura

Domínio e casos de uso não dependem de HTTP, PostgreSQL ou WebSocket. As portas
são declaradas por quem as consome, e os adaptadores ficam nas bordas. O
monólito modular continua sendo a arquitetura apropriada para o estágio atual.

O caminho de uma mutação colaborativa foi desenhado como:

```text
comando HTTP
  → autorização e regra de domínio
  → transação de dado + evento
  → commit
  → publicação para as conexões
  → reconciliação dos clientes
```

O novo roadmap mantém esse desenho e corrige as lacunas de atomicidade e ordem
que a auditoria posterior encontrou.

### Banco e migrations

- PostgreSQL permanece como fonte de verdade.
- SQL de aplicação é parametrizado.
- Migrations estruturais são aplicadas pelo Flyway.
- Backfill que depende de regra de domínio não é escrito em migration SQL.
- Aperto de schema segue expandir → código compatível/backfill → validar →
  contract.
- Números de migration não são reservados em planos futuros; usa-se a próxima
  versão disponível no momento da implementação.

As migrations foram consolidadas quando o banco de produção ainda podia ser
reconstruído. A sequência válida e as regras de compatibilidade estão em
[migrations/README.md](../backend/migrations/README.md); detalhes descartados
permanecem no histórico do Git, não neste documento.

### API e documentação

A tabela de rotas do README continua sendo a documentação humana da API.
Swagger UI ficou de fora. O roadmap novo acrescenta proteção de contrato por
fixtures compartilhadas e validação em runtime, sem criar uma segunda descrição
manual das rotas.

### Produção

- Frontend e API compartilham a mesma origem pública.
- O cookie de produção usa o prefixo `__Host-`.
- A borda do VPS é o único proxy externo (hoje o Traefik do projeto `loadbalancer`).
- O host recebe imagens prontas; nunca compila o projeto.
- PostgreSQL e anexos permanecem em volumes locais enquanto o perfil suportado
  for uma única instância.
- Ansible descreve configuração persistente; a esteira promove versões.

---

## Retrospectivas técnicas preservadas

### Ordenação: o limite mudou de lugar, mas não desapareceu

A primeira implementação usava `double precision`. Inserções repetidas no mesmo
intervalo esgotavam a precisão em aproximadamente 52 operações. A migração para
chaves textuais aumentou muito o espaço e evitou reescrever a lista inteira no
caso comum.

O ciclo ensinou três coisas que continuam importantes:

1. a ordem textual depende de `COLLATE "C"` no índice e na consulta;
2. teste que repete sempre os mesmos vizinhos não reproduz o estreitamento real;
3. chaves textuais ainda precisam de unicidade, retry e rebalanceamento para a
   colisão concorrente rara e para o limite de comprimento.

Esses pontos voltam como trabalho explícito na etapa A2 do roadmap.

### Realtime: fila limitada é parte da correção

O hub foi construído para que uma conexão lenta não bloqueie todas as outras.
Cada assinante possui fila limitada; quem deixa de acompanhar é desconectado e
se recupera pelo replay. Ping/pong detecta conexão morta, a origem do handshake
é validada e o acesso ao quadro é reavaliado periodicamente.

O race detector permaneceu limpo, mas isso não provou a correção do protocolo.
A auditoria posterior encontrou problemas de lógica:

- eventos da mesma conta eram filtrados em todas as suas conexões;
- havia uma janela entre o snapshot HTTP e o primeiro socket;
- o cursor avançava antes de a tela aplicar a mudança;
- `BIGSERIAL` representava ordem de alocação, não necessariamente ordem de
  commit.

A diferença entre data race e erro de protocolo passou a ser uma lição central
do projeto. A etapa A3 trata o conjunto como uma única correção de convergência.

### Evento persistido e evento ao vivo têm responsabilidades diferentes

`board_events` nasceu para replay e depois virou também fonte do histórico e da
auditoria. Payloads passaram a carregar valores anteriores e nomes relevantes
para que a frase histórica descrevesse o que era verdade no momento da ação.

A ampliação do uso revelou que evento não pode ser apenas um aviso descartável:
se dado e evento não pertencem ao mesmo commit, replay e auditoria podem ficar
incompletos. Por isso o roadmap estende a transação a todas as mutações.

### O modal também é parte do tempo real

Recarregar a página do quadro não atualizava comentários, histórico e anexos já
abertos no modal. Um pulso de atualização foi introduzido para recarregar esses
dados e a versão usada pelo bloqueio otimista passou a ser congelada no início
da edição.

O conserto preservou consistência, mas ampliou o número de requisições por
evento. A etapa A10 substitui esse caminho por eventos tipados, estado
normalizado e reconciliação explícita.

### Testes de navegador precisam provar o gesto real

Testes de usecase e handler não encontraram problemas de toque, foco, layout ou
integração de DTO. A suíte móvel revelou falhas independentes que deixavam o
quadro inutilizável mesmo com todo o backend verde.

Por isso os gates futuros diferenciam:

- regra pura;
- integração PostgreSQL;
- contrato HTTP;
- protocolo WebSocket;
- navegador e gesto real;
- imagem exata de produção atrás do Caddy.

### Infraestrutura como código é convergência, não apenas instalação

O primeiro ganho do Ansible foi tornar `.env`, diretórios, cron, backup e bloco
do Caddy reproduzíveis. O critério que permaneceu foi executar novamente e
obter `changed=0`.

O material detalhado sobre o modelo sem agente, vault, check mode e a fronteira
com a borda Traefik já está em
[tecnologias.md](tecnologias.md#ansible--a-infraestrutura-por-dentro). A operação
fica em [infraestrutura.md](infraestrutura.md).

> **Nota (agosto de 2026):** a borda deixou de ser o Caddy compartilhado com o
> agendaGo e passou a ser o projeto `loadbalancer` (Traefik + ACME, dono do
> host). O `deploy/ansible/` do stacktrack descreve só a aplicação. Detalhes em
> [producao.md](producao.md).

---

## Ideias retiradas do backlog ativo

Os itens abaixo não são compromissos futuros. Se algum voltar, deverá nascer de
uma necessidade nova e ganhar plano próprio:

- arquivamento de cards e colunas;
- limite WIP;
- cursor de cada pessoa flutuando no quadro;
- indicador de edição dentro do card;
- edição colaborativa de texto com CRDT;
- múltiplas instâncias da API sem evidência de saturação.

O canal de e-mail é diferente: não foi descartado. Foi deliberadamente movido
para a **última etapa** do roadmap, depois que segurança, consistência,
recuperação, observabilidade e capacidade estiverem prontas.

---

## Onde estudar e operar

- [tecnologias.md](tecnologias.md): modelo mental e decisões técnicas.
- [testes.md](testes.md): camadas de teste e limites de cada uma.
- [regra-de-negocio.md](regra-de-negocio.md): comportamento atual do produto.
- [producao.md](producao.md): topologia, deploy, backup e operação.
- [infraestrutura.md](infraestrutura.md): Ansible e reconstrução do host.
- [entrega-continua.md](entrega-continua.md): CI, imagens e deploy.

Quando uma etapa do roadmap mudar o comportamento real, esses documentos devem
ser atualizados no mesmo conjunto de alterações. O histórico, por outro lado,
só muda quando uma etapa inteira termina e deixa um aprendizado que vale
preservar.
