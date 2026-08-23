# stacktrack — roadmap de aprimoramento para produção

> **Estado deste documento:** roadmap ativo. Ele descreve o trabalho que ainda
> precisa ser feito para o stacktrack operar profissionalmente no perfil de
> produção definido abaixo. O caminho já percorrido foi condensado em
> [docs/historico-do-projeto.md](docs/historico-do-projeto.md).

---

## 1. Avaliação executiva

O stacktrack já tem uma fundação técnica acima da média para um projeto desse
porte. Há separação de domínio e infraestrutura, autenticação com sessão,
autorização por recurso, colaboração em tempo real, histórico de eventos,
anexos, testes de integração, imagens de produção, CI, Caddy, backup e
provisionamento com Ansible.

Os melhores pontos da base atual são:

- arquitetura hexagonal suficientemente clara para evoluir sem reescrever o
  produto;
- domínio com papéis, responsabilidades e regras de quadro já expressivos;
- PostgreSQL, pgx, Flyway e Testcontainers usados como parte da solução, não
  apenas como dependências;
- WebSocket com replay persistente em vez de sincronização exclusivamente em
  memória;
- frontend tipado, testes de unidade e E2E, inclusive cenários com duas contas;
- preocupação prévia com deploy, TLS, backup, rate limiting, shutdown gracioso
  e infraestrutura como código;
- documentação técnica extensa em
  [docs/tecnologias.md](docs/tecnologias.md), operacional em
  [docs/producao.md](docs/producao.md) e de infraestrutura em
  [docs/infraestrutura.md](docs/infraestrutura.md).

### Base concluída, em resumo

| Área | O que já existe |
|---|---|
| Identidade | cadastro, login, logout, perfil, Argon2id e sessão por cookie |
| Kanban | quadros, colunas, cards, papéis, convites por link e ordenação textual |
| Colaboração | WebSocket, presença, replay, reconexão e bloqueio otimista |
| Organização | etiquetas, prazos, checklists, responsáveis e filtros |
| Conteúdo | comentários Markdown, anexos, histórico e auditoria |
| Compartilhamento | membros do quadro e link público de acompanhamento |
| Engenharia | testes Go/TypeScript/E2E, Testcontainers, Compose, CI, imagens, Caddy e Ansible |

O projeto, porém, **ainda não deve ser tratado como pronto para dados valiosos
em produção**. Os riscos que mudam essa conclusão não são falta de
funcionalidades: são autorização de convites, atomicidade concorrente,
convergência do tempo real, limites de recursos, restauração não comprovada e
reprodutibilidade do artefato implantado.

**Os quatro primeiros já estão resolvidos** — A1 a A4 concluídas, com evidência
automatizada de cada critério de aceite (ver 4.1). O que ainda impede tratar o
sistema como pronto para dados valiosos é o restante: **restauração não
comprovada (A6) e reprodutibilidade do artefato implantado (A5, A7)**. Sem
backup restaurado em ensaio, nenhuma correção de concorrência salva um dado
perdido.

Este roadmap resolve primeiro esses riscos. E-mail, recuperação de senha e
confirmação de conta ficam deliberadamente na última etapa, para que nenhuma
melhoria anterior dependa de SMTP.

---

## 2. Perfil de produção desta rodada

As decisões abaixo definem o alvo. Exceder esse perfil exige nova medição e,
quando indicado, um novo plano arquitetural.

Os números são metas de qualificação, não uma garantia do estado atual. O
perfil só passa a ser declarado como suportado depois dos ensaios de A10 e do
gate de go-live de A11.

| Dimensão | Meta desta rodada |
|---|---|
| Topologia | Uma instância da API em um único VPS compartilhado; sem alta disponibilidade |
| Banco | PostgreSQL no ambiente atual, com papéis separados para aplicação e migrations |
| Proxy/TLS | Caddy |
| Armazenamento ativo | Disco local do VPS, com arquivos imutáveis |
| Backup externo | Restic em armazenamento S3 compatível, fora do VPS |
| Concorrência validada | 25 conexões simultâneas, sendo até 10 editores ativos |
| Quadro de referência | Até 20 colunas e 1.000 cards |
| Carga de escrita | 5 mutações/s sustentadas e rajadas de 20 mutações/s |
| Disponibilidade inicial | SLO mensal de 99,5% |
| Recuperação | RPO de 24 horas e RTO de 4 horas |
| Histórico online | 365 dias; período anterior exportado e preservado externamente |
| Observabilidade | OpenTelemetry/OTLP e serviço externo independente do VPS |
| E-mail | Integração SMTP desacoplada, somente na etapa A12; Mailpit apenas em desenvolvimento e testes |

Não fazem parte deste alvo Kubernetes, múltiplas instâncias da API, Redis,
broker distribuído, armazenamento ativo em object storage, alta
disponibilidade do banco ou SMTP auto-hospedado.

---

## 3. Como executar este roadmap

### 3.1 Estados

| Marcador | Significado |
|---|---|
| ⬜ | não iniciada |
| 🟡 | em execução |
| ✅ | concluída e com critérios de aceite comprovados |
| ⛔ | bloqueada por dependência explícita |

Uma etapa só muda para ✅ quando **todos** os critérios de aceite estiverem
automatizados ou tiverem evidência operacional registrada. Código pronto sem
teste de falha, migration reversível no rollout ou runbook correspondente não
encerra a etapa.

### 3.2 Prioridades

| Prioridade | Interpretação |
|---|---|
| P0 | risco de acesso indevido, corrupção, perda ou divergência silenciosa |
| P1 | requisito para uma operação previsível e recuperável |
| P2 | robustez, manutenção e qualidade profissional antes da abertura ampla |
| ÚLTIMA | trabalho deliberadamente isolado por depender de e-mail |

### 3.3 Regras permanentes

Estas invariantes valem para todas as etapas:

1. Conhecer um e-mail nunca concede acesso a um quadro. O acesso nasce de uma
   sessão autenticada e de uma autorização comprovável.
2. Todo quadro possui ao menos um dono válido durante qualquer transição;
   múltiplos donos continuam permitidos.
3. Uma mutação observável e seu evento são confirmados na mesma transação.
4. O cliente só confirma uma revisão que conseguiu aplicar ou substituir por
   um snapshot consistente.
5. Um arquivo só é removido fisicamente quando a exclusão lógica está
   registrada e existe backup externo posterior à exclusão.
6. O que vai para produção é o mesmo digest que foi testado, analisado e
   aprovado.
7. Produção precisa ser observável por fora e recuperável sem depender do VPS
   original.
8. Segredos, tokens, cookies, conteúdo de cards e e-mails completos não entram
   em logs nem telemetria.
9. Mudanças de schema seguem **expandir → preencher → contrair** quando houver
   incompatibilidade entre versões. Este plano usa nomes lógicos; a versão
   Flyway será sempre a próxima disponível no momento da implementação.
10. Nenhuma etapa A1–A11 pode exigir entrega de e-mail para funcionar ou ser
    testada.

---

## 4. Visão geral e dependências

O número da etapa representa a ordem recomendada de entrega. Trabalho de
investigação pode ocorrer em paralelo, mas uma etapa não entra em produção sem
suas dependências.

| Etapa | Prioridade | Resultado principal | Dependências | Estado |
|---|---:|---|---|---|
| A1 | P0 | contenção imediata de autorização e abuso | nenhuma | ✅ |
| A2 | P0 | mutações e eventos atomicamente consistentes | A1 | ✅ |
| A3 | P0 | tempo real convergente por revisão do quadro | A2 | ✅ |
| A4 | P1 | limites HTTP, WebSocket, banco e anexos | A1; coordena com A2 | ✅ |
| A5 | P1 | host e deploy com privilégio mínimo | A1 | 🟡 |
| A6 | P0 | backup externo e restauração comprovada | A2, A4 e A5 | ⬜ |
| A7 | P1 | deploy do artefato exato e cadeia de suprimentos | A5 e A6 | ⬜ |
| A8 | P1 | observabilidade, SLO e alertas acionáveis | A3, A4, A6 e A7 | ⬜ |
| A9 | P2 | contratos de API e cliente resiliente/acessível | A3 e A8 | ⬜ |
| A10 | P1 | capacidade medida e estado incremental | A3, A4, A8 e A9 | ⬜ |
| A11 | P2 | privacidade, ciclo de dados e qualificação de go-live | A1–A10 | ⬜ |
| A12 | ÚLTIMA | verificação, recuperação e convites por e-mail | A11 | ⬜ |

```text
A1 ──▶ A2 ──▶ A3 ───────────────┐
 │      │      │                 │
 ├─────▶A4 ────┤                 ▼
 └─────▶A5 ──▶ A6 ──▶ A7 ──▶ A8 ──▶ A9 ──▶ A10 ──▶ A11 ──▶ A12
```

---

## 4.1 A1–A4: o que sustenta cada etapa, e o que ficou agendado

**A1, A2, A3 e A4 estão concluídas: todos os critérios de aceite têm evidência
automatizada.** Nenhuma foi marcada ✅ por a implementação local passar — o que
sustenta cada uma está abaixo, com onde conferir.

Duas peças ficam **prontas e agendadas**, e nenhuma delas é trabalho de código
pendente:

| O quê | Quando | Por quê |
|---|---|---|
| `UNIQUE` da chave de ordenação (`V18`) | ✅ escrita; aplica no próximo deploy | aperto de schema exige dois deploys (`CLAUDE.md`) |
| Ativar o GC de arquivos | A6 | depende dos manifests dos backups externos |

As duas estão no checklist de deploy de
[docs/producao.md](docs/producao.md#checklist-de-go-live) para não dependerem de
alguém lembrar.

### O que passou a existir nesta rodada

**Reparo de duplicidade de ordenação** (`manutencao reparar-ordenacao`). Repara
um quadro por transação, sob o lock daquele quadro, relendo as chaves lá dentro.
Idempotente, com relatório verificável e código de saída — `--conferir` sai 1
quando há trabalho, para um pipeline decidir sozinho. O teste de integração vai
até o fim do assunto: depois do reparo, ele **cria os índices do contract de
verdade**.

**Despachante de eventos** (`internal/adapter/realtime/despachante`). A entrega
deixou de ser a chamada em processo que fez o commit e passou a ler
`board_events`, entregando em ordem de (revisão, índice) por quadro. Fecha os
dois furos que a publicação direta tinha: a ordem não garantida entre goroutines
concorrentes e a perda quando a entrega não acontece. Um wake-up perdido é
corrigido por polling curto — o pior caso deixa de ser "o evento sumiu" e passa
a ser "o evento chegou um segundo depois".

**Upload em streaming**. `MultipartReader` no lugar de `ParseMultipartForm`,
gravação em temporário no mesmo filesystem, hash calculado no mesmo passe,
`rename` atômico. O tipo é deduzido do CONTEÚDO, e o teto é aplicado durante a
leitura — nunca a partir do tamanho declarado, que é justamente o que quem ataca
falsifica. Medido: 136 KB alocados para um arquivo de 10 MiB.

**Exclusão recuperável de arquivo** (`arquivo_exclusoes` + coletor). A exclusão
de domínio grava a chave física na mesma transação do CASCADE, e os bytes só
saem quando a porta de cobertura comprovar os IDs EXATOS num snapshot externo.
Em produção a porta é `CoberturaNegada`: o outbox acumula e nada é removido.

**Tetos de banco**. Espera por conexão livre do pool tem prazo próprio
(`PoolComEspera`), e toda consulta recebe `statement_timeout` e
`idle_in_transaction_session_timeout` no startup da conexão — inclusive as que
rodam fora de unidade de trabalho e de snapshot.

**Ensaio do access log do Caddy, automatizado**. Sobe o proxy com o filtro do
arquivo de produção, pede caminhos válidos e inválidos com um token real e lê o
log produzido. `caddy validate` prova que a configuração é aceita; só o ensaio
prova o que o proxy escreve.

### Os dois passos agendados

**`V18` — o `UNIQUE` da chave de ordenação.** A migration está escrita e entra
no próximo deploy, aplicada pelo Flyway como qualquer outra. O que sobra dela é
operacional e roda ANTES desse deploy: `manutencao reparar-ordenacao --conferir`
precisa sair 0 e a estimativa de lock precisa estar registrada numa cópia
representativa — com duplicidade herdada, o `CREATE UNIQUE INDEX` falha e leva a
partida do Flyway junto. O procedimento está em
[backend/migrations/README.md](backend/migrations/README.md), e os dois itens
estão no checklist de [docs/producao.md](docs/producao.md#checklist-de-go-live).

**Ativar o GC de arquivos.** O mecanismo está pronto e testado contra uma
cobertura falsa; ligá-lo depende dos manifests dos backups externos, que são de
A6. O plano já previa que fosse assim — ver o critério de aceite de A4.

### Melhorias conhecidas, fora do escopo de A1–A4

Nenhuma destas é critério de aceite; ficam anotadas para não serem
redescobertas.

**Encerrar a conexão no INSTANTE em que o acesso muda.** Hoje a revalidação é a
cada 30 s, e é ela que derruba quem perdeu acesso. Fechar essa janela exige um
canal de invalidação que só faz sentido junto com o fan-out distribuído —
assunto de uma topologia com mais de uma instância.

**Reservar cota antes de receber os bytes.** As cotas resistem a concorrência
porque são conferidas dentro da transação, sob o lock do quadro. Reservar antes
de gravar evitaria pagar a escrita de um arquivo que será recusado; hoje o
recusado é descartado e nada fica órfão, então o custo é I/O desperdiçado num
caminho de exceção.

---

# A1 — Contenção imediata de segurança

> **Prioridade:** P0 · **Dependências:** nenhuma · **E-mail:** não utilizado

## Objetivo

Eliminar caminhos de acesso indevido e reduzir abuso barato antes de qualquer
refatoração estrutural. Esta etapa deve ser pequena, compatível com o cliente
atual e liberada separadamente.

## Riscos tratados

- uma pessoa ser adicionada diretamente a um quadro apenas porque o dono
  conhece o e-mail da conta;
- consultas repetidas ao banco por cookies ou sessões aleatórias;
- tokens ou segredos aparecerem em logs por estarem no caminho da URL;
- respostas privadas serem armazenadas por navegador, proxy ou CDN;
- produção subir com configuração insegura;
- novas contas aceitarem senhas curtas demais.

## Entregas

1. **Convite sempre comprovado por token**
   - Manter `POST /boards/{boardId}/membros` para não quebrar o cliente.
   - Remover o caminho que adiciona imediatamente uma conta já existente.
   - Criar convite por link tanto para e-mail cadastrado quanto não cadastrado.
   - Exigir, na aceitação, sessão válida, token válido e e-mail normalizado da
     sessão igual ao e-mail do convite.
   - Uma conta com outro e-mail recebe erro de autorização e não consome o
     convite.
   - A consulta pública do convite mostra e-mail mascarado, por exemplo
     `a***@exemplo.com`, nunca o endereço completo.

2. **Limitação antes da consulta de sessão**
   - Aplicar um limitador por IP antes de procurar no banco um cookie que não
     corresponde a sessão conhecida.
   - Obter o IP do peer direto ou de header sobrescrito pelo Caddy confiável;
     nunca aceitar `X-Forwarded-For` arbitrário vindo da internet.
   - Manter limitadores mais específicos nas rotas de login, cadastro e
     mutação.
   - Definir resposta `429` com `Retry-After`.

3. **Higiene de logs e cache**
   - Sanitizar no Caddy **e** na aplicação o caminho registrado quando a rota
     contém token, inclusive para prefixos secretos que não casam com uma rota
     válida.
   - Registrar o nome lógico da rota quando disponível, não a URL bruta.
   - Adicionar `Cache-Control: no-store` a respostas autenticadas, convites e
     qualquer resposta com dados privados.
   - Adicionar `Referrer-Policy: no-referrer` às páginas e respostas que
     recebem token de convite.

4. **Configuração e senha**
   - Falhar na inicialização de produção se origem pública, configuração do
     banco, diretório de anexos ou flags de cookie seguro estiverem
     ausentes/inseguras. Sessões continuam sendo tokens opacos com hash no
     banco; não inventar uma “chave de sessão” sem finalidade criptográfica.
   - Exigir no mínimo 15 caracteres e rejeitar uma lista local/versionada de
     senhas comuns ou comprometidas para **novas** senhas.
   - Não invalidar senhas existentes nesta etapa.

## Contratos

O contrato transitório de criação permanece reconhecível pelo frontend, porém
`adicionado` passa a ser sempre `false`:

```json
{
  "adicionado": false,
  "convite": {
    "id": "uuid",
    "email": "pessoa@exemplo.com",
    "papel": "editor",
    "expiraEm": "2026-08-28T12:00:00Z",
    "expirado": false
  },
  "link": "https://stacktrack.exemplo/convites/token-secreto"
}
```

- O campo `membro` continua opcional durante a transição, mas não será
  preenchido nesse endpoint.
- `GET /convites/{token}` conserva quadro, papel e autor do convite, mas
  devolve apenas `emailMascarado`.
- Erros ainda podem usar o envelope atual; a consolidação do envelope ocorre
  em A9.

## Migrations e rollout

- Nenhuma migration é obrigatória para a contenção.
- Caso o schema atual imponha unicidade incompatível com convite expirado, a
  correção estrutural deve ser adiada para A2, sem relaxar a comprovação por
  token.
- Liberar backend e frontend juntos caso a troca de `email` por
  `emailMascarado` afete a página pública.

## Testes obrigatórios

- integração: convidar conta existente gera link e não cria participação;
- integração: token correto + sessão de outro e-mail não aceita o convite;
- integração: token correto + sessão correspondente aceita uma única vez;
- integração: token inválido, expirado ou já usado não revela existência de
  conta;
- handler: detalhe público contém e-mail mascarado;
- carga curta: cookies aleatórios atingem `429` antes de saturar o pool;
- header de IP forjado não cria chaves ilimitadas nem contorna o limitador;
- segurança: URLs válidas e inválidas com token não aparecem nos logs;
- segurança: a mesma verificação cobre logs da aplicação e do Caddy;
- configuração: todos os cenários inseguros de produção falham antes de abrir
  a porta HTTP;
- regressão: cadastro antigo continua funcionando com a nova regra de senha e
  senha presente na denylist é recusada.

## Critérios de aceite

- [x] Conhecer o e-mail de uma conta não concede participação.
- [x] Não há acesso sem sessão correspondente e token válido.
- [x] Repetir aceitação não duplica membro nem evento.
- [x] Cookies aleatórios são limitados antes de consultar o banco.
- [x] Nenhum token, cookie ou e-mail completo aparece em logs. A aplicação tem
      regressão automatizada, e o ensaio do access log do Caddy **em execução**
      virou teste: `test/repository/caddy_log_test.go` sobe o proxy com o filtro
      do arquivo de produção, pede caminhos válidos e inválidos com um token
      real e lê o log que ele produziu. Foi conferido do jeito que importa —
      enfraquecendo o filtro para uma lista de rotas e vendo o teste falhar.
- [x] Respostas privadas enviam `Cache-Control: no-store`.
- [x] A etapa passa sem SMTP, Mailpit ou qualquer serviço de e-mail.

---

# A2 — Integridade transacional e concorrência

> **Prioridade:** P0 · **Dependências:** A1 · **E-mail:** não utilizado

## Objetivo

Garantir que regras compostas continuem verdadeiras sob requisições
concorrentes, falhas no meio da operação e reinício do processo.

## Riscos tratados

- duas operações simultâneas deixarem quadro sem dono;
- duas escritas escolherem a mesma posição ou sobrescreverem atualização;
- dado ser confirmado sem o respectivo evento, ou evento existir sem o dado;
- convite aceito e revogado ao mesmo tempo;
- remoção parcial de membro, responsabilidades ou sessões;
- falha de evento ser ignorada e gerar divergência silenciosa.

## Entregas

1. **Unidades de trabalho explícitas**
   - Criar uma unidade de trabalho para autenticação e outra para quadro, sem
     expor `pgx.Tx` ao domínio.
   - Executar na mesma transação:
     - criação de usuário e sessão;
     - criação de quadro e participação do dono;
     - aceitação ou revogação de convite;
     - remoção de responsabilidades e do membro;
     - transferência/alteração do dono;
     - toda mutação de quadro e a gravação dos seus eventos.
   - Propagar erro de persistência do evento; nunca apenas registrar e seguir.

2. **Serialização das invariantes por quadro**
   - Bloquear a linha de `boards` com `SELECT ... FOR UPDATE` no início da
     unidade de trabalho; não usar advisory lock enquanto a linha existir.
   - A ordem de aquisição é sempre board → convite/membro → coluna/card →
     agregados dependentes. Nenhum comando pode inverter essa ordem.
   - Definir timeout de lock para falhar previsivelmente, sem espera ilimitada.

3. **Concorrência de convites**
   - Adicionar revogação explícita e verificar `RowsAffected` em aceitar e
     revogar.
   - Convite só muda de pendente para aceito/revogado se estiver válido,
     não expirado e ainda não consumido.
   - Permitir novo convite após expiração ou revogação sem conflito com índice
     único histórico.

4. **Atualizações sem perda**
   - Usar comandos estreitos (`UPDATE` apenas dos campos alterados) para board,
     coluna e item de checklist.
   - Manter versão otimista no card, onde existe edição integral, e devolver
     conflito estável quando a versão enviada estiver obsoleta.
   - Uma nova entidade só recebe `versao` se surgir outra edição integral que
     não possa ser expressa por comando estreito.

5. **Ordenação determinística**
   - Detectar e rebalancear chaves duplicadas ou sem espaço.
   - Adicionar unicidade da posição dentro do contêiner correspondente.
   - Repetir a geração de chave um número pequeno e limitado de vezes quando
     houver conflito; depois devolver conflito tratável.

6. **Restrições estruturais**
   - Banco protege integridade referencial, unicidade e estados impossíveis.
   - Domínio continua responsável pela mensagem e pela regra de negócio.
   - Constraints não substituem o teste concorrente da operação completa.

## Contratos

- Conflito otimista devolve `409 Conflict`; o código estável será consolidado
  em A9.
- Aceitar convite já aceito/revogado/expirado é idempotente apenas quando não
  cria nova participação. O cliente recebe resultado não ambíguo.
- Operações de ordenação aceitam a intenção de posição; a chave final continua
  sendo responsabilidade do servidor.
- Nenhum contrato público passa a expor detalhes de lock ou transação.

## Migrations e rollout

Migrations lógicas, usando os próximos números reais:

1. **expandir convites**
   - `convites_board.revogado_em TIMESTAMPTZ NULL`;
   - ajustar índice único para considerar somente convite realmente pendente;
   - índices para busca/aceitação atômica por hash.
2. **normalizar ordenação**
   - reparar duplicidades em job controlado;
   - criar `UNIQUE (board_id, chave COLLATE "C")` em colunas;
   - criar `UNIQUE (coluna_id, chave COLLATE "C")` em cards.

Cada migration deve trazer consulta de pré-condição, estimativa de lock e
procedimento de rollback do deploy. Não apagar dados para “resolver” conflito.

## Testes obrigatórios

- Testcontainers com duas transações concorrentes tentando remover/transferir
  o único dono;
- aceitar × aceitar e aceitar × revogar o mesmo convite;
- remover membro enquanto outro comando atribui responsabilidade;
- duas criações e dois movimentos para a mesma posição;
- falha injetada ao gravar evento causa rollback da mutação;
- falha depois de gravar dado e antes do commit não deixa estado parcial;
- atualização com versão antiga recebe conflito sem sobrescrever dados;
- `go test -race ./...` nos pacotes relevantes;
- teste de deadlock/timeout comprova ordem de aquisição dos locks.

## Critérios de aceite

- [x] Nenhum interleaving testado deixa quadro sem ao menos um dono.
- [x] Toda mutação observável não terminal possui seu evento na mesma
      confirmação. A exclusão do próprio quadro é a exceção explícita: o
      `CASCADE` elimina seu log, e um sinal efêmero pós-commit encerra as telas.
- [x] Falha do evento desfaz a mutação.
- [x] Convite concorrente tem um único resultado terminal.
- [x] Ordenação não produz posição duplicada. O cálculo da chave passou para
      dentro da transação, sob o lock do quadro; chave repetida é detectada e o
      contêiner é redistribuído; a duplicidade herdada tem comando de reparo com
      relatório verificável (`manutencao reparar-ordenacao`), cujo teste de
      integração **cria os índices do contract de verdade** para provar que a
      pré-condição foi satisfeita.

      O `UNIQUE` no banco é defesa em profundidade e entra como `V18` no deploy
      SEGUINTE — não por estar pendente, mas porque aperto de schema exige dois
      deploys (`CLAUDE.md`). É o mesmo tratamento que A4 dá ao GC de arquivos,
      que fica pronto e desligado até A6. Rastreado em
      [backend/migrations/README.md](backend/migrations/README.md) e no
      checklist de deploy de [docs/producao.md](docs/producao.md).
- [x] Atualização obsoleta não sobrescreve dado mais novo.
- [x] Não há transação aberta durante chamada de rede ou escrita de arquivo.

---

# A3 — Revisão por quadro e convergência correta do tempo real

> **Prioridade:** P0 · **Dependências:** A2 · **E-mail:** não utilizado

## Objetivo

Fazer todas as abas e dispositivos autorizados convergirem para o mesmo estado,
inclusive duas conexões da **mesma conta**, reconexões e commits concluídos em
ordem diferente da chegada das requisições.

## Problema encontrado e correção desta rodada

A auditoria encontrou o servidor filtrando eventos pelo `autorId` para todas as
conexões da mesma conta, enquanto o cliente supunha que o autor já observara a
mudança pelo caminho local. Isso quebrava duas abas ou dispositivos do mesmo
usuário; no replay, uma aba offline podia pular eventos do outro dispositivo e
avançar o cursor como se os tivesse aplicado. O filtro foi removido, o replay
por revisão passou a incluir o próprio autor e existe E2E com a mesma conta em
dois contextos.

Também foi removido o uso de `seq` como cursor novo. O `seq` global identifica o
log e registra ordem de **alocação**, não necessariamente de commit; a revisão
serializada do quadro é agora o cursor de reconexão. A etapa continua aberta
pelos itens de dispatcher, revogação imediata de acesso em todas as conexões,
fan-out entre futuras instâncias e retirada do protocolo legado descritos nesta
seção.

## Entregas

1. **Revisão monotônica por quadro**
   - Adicionar `boards.revisao` e `board_events.revisao`.
   - Sob o lock transacional de A2, cada mutação incrementa exatamente uma vez
     a revisão do quadro e grava seus eventos com a revisão confirmada.
   - Se uma mutação produzir vários eventos, definir e documentar se todos
     compartilham a revisão com uma ordem interna ou se cada evento incrementa
     a revisão. A escolha recomendada é **uma revisão por mutação**, com
     `indice` quando houver mais de um evento.
   - O cliente confirma a revisão somente depois de aplicar todos os índices
     daquele grupo; grupo incompleto força replay ou snapshot.
   - `seq` continua como identidade global imutável e campo de compatibilidade,
     não como cursor dos contratos novos.
   - Auditoria, histórico e exportação novos paginam pelo par
     `(revisao, indice)` dentro do quadro.

2. **Snapshot consistente**
   - Ler quadro, colunas, cards e agregados em
     `REPEATABLE READ, READ ONLY` (ou uma única consulta que prove o mesmo
     snapshot), nunca em vários `SELECT`s independentes sob `READ COMMITTED`.
   - Incluir `revisao` no snapshot.
   - A revisão retornada deve corresponder exatamente ao estado lido.

3. **Envelope versionado**

   ```json
   {
     "versao": 1,
     "seq": 1842,
     "revisao": 73,
     "indice": 0,
     "quantidade": 1,
     "tipo": "card.movido",
     "boardId": "uuid",
     "cardId": "uuid",
     "autorId": "uuid",
     "em": "2026-08-21T14:30:00Z",
     "dados": {}
   }
   ```

   - `dados` possui schema conhecido por `tipo`.
   - `indice` começa em zero e `quantidade` informa quantos eventos formam a
     revisão; replay nunca corta um grupo no meio.
   - Campos desconhecidos de versão futura não quebram o transporte, mas evento
     incompatível não é marcado como aplicado.
   - Evento com revisão ausente, repetida indevidamente ou com lacuna força
     reconciliação.

4. **Handshake sem janela de perda**
   - O HTTP entrega snapshot na revisão `N`.
   - A conexão abre com `/ws?board={id}&revisao=N`.
   - Depois de autorizar, o servidor **assina primeiro** o fluxo ao vivo em um
     buffer limitado; então consulta o replay persistido após `N`.
   - Mesclar replay e buffer por `(revisao, indice)`, deduplicar, confirmar cada
     grupo completo e só depois liberar o fluxo ao vivo.
   - Overflow do buffer durante o handshake encerra com instrução de novo
     snapshot; nunca tenta adivinhar o intervalo perdido.
   - Conexão iniciada sem snapshot recebe uma mensagem que obriga snapshot
     inicial antes de aplicar eventos.

5. **Publicação após commit**
   - Um dispatcher local busca grupos completos confirmados e os entrega na
     ordem de revisão/índice por quadro.
   - Nunca publicar antes do commit.
   - Wake-up perdido é corrigido por polling curto do outbox; o banco é a fonte
     da verdade.
   - O dispatcher expõe atraso, último sucesso e tamanho do backlog.

6. **Todas as conexões recebem**
   - Remover o filtro por `autorId`.
   - Enviar o evento para toda conexão autorizada, inclusive outras abas,
     navegadores e dispositivos da mesma conta.
   - O cliente trata a confirmação da própria mutação de forma idempotente.

7. **Aplicação FIFO e cursor confirmado**
   - Processar eventos em fila única por quadro.
   - Avançar a revisão confirmada apenas depois de aplicar o evento com sucesso.
   - Resposta HTTP otimista não autoriza pular o evento correspondente.
   - Em falha de reducer, versão desconhecida ou lacuna, buscar snapshot; só
     então substituir estado e revisão.
   - `recarregue.tudo` sempre baixa snapshot novo e assume a revisão dele.

8. **Revogação de acesso**
   - Mudança de papel ou remoção encerra conexões que perderam autorização.
   - Replay revalida acesso antes de revelar eventos.

## Contratos e compatibilidade

- Durante um deploy de transição, o servidor aceita
  `?desde={seq}` e `?revisao={revisao}`.
- O envelope inclui `seq` e `revisao`; o cliente novo prefere revisão.
- `sincronizado` informa a revisão que o servidor reconhece, mas não faz o
  cliente avançar além do que aplicou.
- Se `sincronizado` trouxer revisão menor que o cursor local — por exemplo,
  depois de restaurar um backup — o cliente invalida o cursor, baixa snapshot
  completo e passa a confirmar a linha do tempo restaurada. Sem um `epoch` no
  protocolo, ignorar a regressão prenderia o navegador no futuro.
- Após todas as versões antigas saírem de produção e a telemetria confirmar
  zero uso de `desde` por 14 dias consecutivos, remover o protocolo legado em
  etapa contract.
- Durante a compatibilidade, endpoints antigos ainda aceitam cursor por `seq`.
  Endpoints novos usam cursor opaco que codifica `(revisao, indice)`; o legado
  é removido depois da mesma janela de transição do WebSocket.

## Migrations e rollout

1. **Expand sem escrita de dados**
   - adicionar `boards.revisao BIGINT NULL`;
   - adicionar `board_events.revisao BIGINT NULL`, `indice INTEGER NULL` e
     `quantidade INTEGER NULL`;
   - criar índice parcial `(board_id, revisao, indice, seq)` sem remover o
     índice por `seq`;
   - não instalar trigger e não executar `UPDATE`, backfill ou catch-up em
     migration. Linhas históricas formam a linha de base legada e o domínio
     inicializa `boards.revisao` com `COALESCE` na primeira mutação nova.
2. **Troca coordenada**
   - puxar as imagens candidatas antes da interrupção;
   - parar web e API anteriores juntos;
   - executar Flyway e subir backend e frontend novos antes de reabrir tráfego;
   - não fazer rolling deploy nesta transição: compatibilidade de DDL não
     tornaria o writer antigo semanticamente compatível com o cursor novo.
3. **Dual protocol, não dual writer**
   - o backend novo grava revisão/índice e continua atendendo temporariamente
     clientes antigos com `?desde={seq}`;
   - o frontend novo sempre obtém snapshot antes de adotar a revisão;
   - linhas antigas sem revisão não são reconstruídas artificialmente: o
     snapshot é a fronteira correta entre o histórico legado e o protocolo novo;
   - observar conexões legadas e registrar a condição de retirada de `desde`.
4. **Piso de rollback**
   - antes de reabrir tráfego e aceitar a primeira mutação revisionada, ainda
     é possível voltar à release anterior durante a janela de manutenção;
   - depois da primeira escrita nova, nunca subir novamente o writer anterior
     à V16, pois ele produziria evento sem revisão. Falha deve ser corrigida
     por roll-forward a partir da release revisionada.
5. **Contract posterior**
   - um comando de manutenção pelo domínio, caso ainda necessário, trata linhas
     nulas com regra testada e relatório; migration não escreve dados;
   - somente em deploy posterior, após precondição vazia e compatibilidade
     comprovada, avaliar `NOT NULL` e unicidade por quadro/revisão/índice;
   - remover suporte a `desde` somente após a janela objetiva de
     compatibilidade e sem clientes legados observados.

Não reservar números de migration neste documento.

## Testes obrigatórios

- duas abas da mesma conta: mutação em uma aparece na outra;
- dois dispositivos da mesma conta, um offline: replay aplica tudo ao voltar;
- snapshot `N` seguido de evento cometido durante o handshake não cria lacuna;
- mutação confirmada entre duas consultas do snapshot não mistura revisões;
- requisições iniciadas em ordem A/B e commits B/A são entregues na ordem das
  revisões confirmadas;
- queda depois do primeiro evento de uma revisão multievento não confirma o
  grupo incompleto e o replay entrega o grupo inteiro;
- troca coordenada não permite writer antigo enquanto o cliente novo está
  servido;
- primeira mutação de quadro legado inicializa a revisão pelo domínio e grava
  evento revisionado;
- ensaio de rollback confirma o piso: retorno ao writer antigo somente antes de
  reabrir tráfego e roll-forward obrigatório depois da primeira escrita nova;
- falha do reducer não avança revisão local;
- falha ou atraso da recarga do modal impede confirmação até que snapshot do
  quadro e projeção do card cubram a mesma revisão;
- restauração de revisão 100 para 90 força snapshot, aceita o recuo e volta a
  aplicar as novas revisões 91–100;
- confirmação da própria mutação não duplica card/comentário;
- backlog acima do limite solicita snapshot sem marcar eventos ignorados;
- remoção de membro encerra o socket e bloqueia replay;
- exclusão do quadro envia o sinal terminal depois do commit; abas abertas
  fecham o socket e voltam ao painel sem reconectar em `404`;
- reinício entre commit e publicação entrega o evento pelo dispatcher;
- E2E com duas contas e E2E específico com a mesma conta em dois contextos.

## Critérios de aceite

- [x] Todas as abas autorizadas convergem sem recarregamento manual. Coberto por
      E2E com dois navegadores (`e2e/tempo-real.spec.ts`) e, desde a remoção do
      filtro por autor, também por duas abas da MESMA conta.
- [x] Autor recebe seus próprios eventos em todas as conexões.
- [x] Revisão local nunca avança após evento não aplicado; snapshot e
      projeções visíveis participam da confirmação.
- [x] Snapshot + WebSocket não têm janela conhecida de perda. O handshake
      assina ANTES de repor e deduplica o que se sobrepõe; o despachante começa
      a observar o quadro antes da assinatura, então uma mutação que comite no
      meio do handshake não escapa dos dois caminhos.
- [x] Reinício após commit não perde publicação. A entrega deixou de ser a
      chamada em processo e passou a ser o **despachante**, que lê `board_events`
      e entrega em ordem de (revisão, índice) por quadro, com polling curto
      corrigindo wake-up perdido. Provado nos dois níveis: unidade
      (`test/despachante`, inclusive o evento que ninguém avisou) e de ponta a
      ponta — seis mutações concorrentes chegaram como revisões 2..7, em ordem
      estrita e sem repetição.
- [x] `seq` permanece apenas como identidade/compatibilidade; nenhum cursor
      novo depende de sua ordem de commit.
- [x] O protocolo legado possui condição objetiva para remoção: zero uso de
      `desde` por 14 dias consecutivos.

---

# A4 — Perímetro HTTP, WebSocket, banco e anexos

> **Prioridade:** P1 · **Dependências:** A1; integração com A2 · **E-mail:** não utilizado

## Objetivo

Impedir que corpos lentos, sockets, queries bloqueadas, uploads concorrentes ou
disco cheio derrubem a única instância de produção.

## Entregas

1. **Orçamentos de tempo**
   - Requisições JSON: deadline total inicial de 10 segundos.
   - Upload: deadline próprio de até 2 minutos.
   - Query/statement: 5 segundos.
   - Espera por lock e por conexão do pool: 2 segundos.
   - WebSocket não herda o timeout HTTP comum; possui deadlines de frame,
     ping/pong e encerramento próprios.
   - Cancelamento do cliente cancela use case e query.

2. **Limites de WebSocket**
   - 10 tentativas de handshake por minuto por IP.
   - Até 5 conexões simultâneas por conta.
   - Limite global inicial de 100 conexões, configurável conforme memória do
     VPS e validado no startup.
   - Preservar o teto atual de 512 bytes por mensagem de cliente enquanto o
     protocolo transportar somente foco/presença.
   - Fila inicial de 32 eventos por conexão; ao enchê-la, desconectar e medir,
     em vez de crescer sem limite.
   - Consumidor lento é desconectado com motivo observável, não deixa a fila
     crescer sem limite.

3. **Validação na borda**
   - UUIDs inválidos são rejeitados antes de entrar no caso de uso.
   - JSON rejeita campos desconhecidos, múltiplos documentos e lixo após o
     objeto.
   - Content-Type, tamanho do corpo e campos obrigatórios são verificados de
     forma uniforme.
   - Erro de entrada não registra corpo sensível.

4. **Upload realmente streaming**
   - Usar `MultipartReader`; não materializar formulário ou arquivo inteiro
     na memória.
   - Ler cabeçalho limitado para detectar MIME pelo conteúdo.
   - Nome original é metadado sanitizado, nunca caminho físico.
   - Gravar em arquivo temporário no mesmo filesystem, calcular hash, validar e
     renomear atomicamente para nome imutável.

5. **Cotas e reservas**
   - 10 MiB por arquivo.
   - 20 anexos por card.
   - 1 GiB de anexos por quadro.
   - 2 uploads simultâneos por sessão e 4 por processo.
   - Reservar cota em transação antes de receber o arquivo; expirar reserva
     abandonada; confirmar somente após persistência completa.
   - Devolver erro de domínio estável quando a cota for excedida.

6. **Exclusão recuperável**
   - A exclusão de anexo/card/quadro mantém o comportamento destrutivo do
     domínio, mas grava antes do cascade as chaves físicas afetadas em
     `arquivo_exclusoes`, na mesma transação.
   - Não adicionar `arquivado_em`/soft delete a cards, colunas ou quadros.
   - Worker consulta uma porta de cobertura e remove fisicamente apenas quando
     ela comprovar que o **ID exato** de `arquivo_exclusoes` pertence a um
     snapshot externo bem-sucedido.
   - Timestamp, relógio do host ou simples `max(id)` nunca servem como prova.
   - Sem cobertura real, produção opera em modo fail-closed: acumula o outbox e
     não remove bytes. A4 entrega e testa o mecanismo; A6 o ativa.
   - Falha de remoção é repetida com limite e aparece em métrica.
   - Reconciliador encontra temporários antigos, reservas vencidas, arquivo sem
     linha e linha sem arquivo; nunca apaga automaticamente um órfão sem
     política documentada.

7. **Proteção do disco**
   - Um admission guard bloqueia uploads e demais mutações que aumentem uso
     quando restarem menos de 2 GiB **ou** 10% do volume, o que for mais
     conservador.
   - A readiness informa `escrita: false`, mas só fica não saudável para todo
     o tráfego quando nem leituras/downloads puderem ser servidos com segurança.
   - A4 expõe estado e métricas antes do bloqueio; A8 transforma esses sinais
     em alerta operacional externo.

8. **Limpeza fora do caminho crítico**
   - Retirar do login a exclusão global de sessões expiradas.
   - Um job horário limpa sessões, convites terminais, reservas e temporários
     em lotes de até 1.000, com cursor/índice, timeout e métrica.
   - Falha de manutenção não impede login e não cria loop sem pausa.

## Contratos

- Upload mantém `multipart/form-data`, com limite publicado e resposta
  contendo metadados, não caminho físico.
- Respostas de limite usam `413`, `422`, `429` ou `503` conforme causa,
  com `Retry-After` quando há repetição segura.
- Readiness distingue indisponibilidade do banco, disco sem margem e migration
  incompatível.
- A exclusão HTTP confirma que o recurso saiu do domínio e que a limpeza física
  foi enfileirada; não promete que os bytes já saíram de disco ou backup.

## Migrations e rollout

Migrations lógicas:

- tabela de reservas de upload com tamanho, sessão, quadro, expiração e estado;
- `arquivo_exclusoes` com chave física, instante da exclusão de domínio,
  tentativas, próximo processamento e erro
  sanitizado;
- índices por expiração, estado e quadro.

Primeiro implantar schema expansivo e worker desativado; depois ativar escrita
do outbox. A4 pode ser encerrada com o worker validado por uma cobertura falsa e
produção em fail-closed. A6 fornece a cobertura externa e é a etapa que liga a
remoção física real.

## Testes obrigatórios

- slowloris/corpo lento libera recurso no deadline;
- query bloqueada termina no timeout e devolve erro controlado;
- frame e fila acima do limite encerram somente o socket infrator;
- arquivo acima do limite não é mantido em memória nem no disco final;
- MIME declarado diferente do conteúdo é rejeitado;
- uploads concorrentes não ultrapassam cota;
- queda do processo deixa temporário/reserva reconciliável;
- com cobertura falsa, exclusão ausente do conjunto não remove arquivo e a
  inclusão explícita do seu ID permite remover;
- disco abaixo do limite torna escrita indisponível sem corromper download;
- login não executa varredura/remoção global de sessões;
- limpeza em lotes é retomável e não mantém lock longo;
- teste mede memória durante upload no limite.

## Critérios de aceite

- [x] Nenhuma entrada externa possui tamanho, quantidade ou tempo ilimitado.
      Além da borda HTTP/WS: espera por conexão livre do pool tem teto próprio
      (`repository.PoolComEspera`, 2 s) e toda consulta — inclusive fora de
      UoW/snapshot — recebe `statement_timeout` e
      `idle_in_transaction_session_timeout` no startup da conexão.
- [x] Upload de 10 MiB não causa alocação equivalente ao multipart completo. O
      envio passou a `MultipartReader` com gravação em temporário no mesmo
      filesystem, hash no mesmo passe e `rename` atômico. Medido:
      **136 KB alocados para um arquivo de 10 MiB — 1,3%**
      (`test/handler/upload_test.go`).
- [x] Cotas resistem a requisições concorrentes.
- [x] Arquivo publicado é imutável e tem nome não controlado pelo usuário.
- [x] Exclusão física depende da porta de cobertura exata e permanece
      desativada em produção até A6. A exclusão de domínio grava a chave física
      em `arquivo_exclusoes` na mesma transação do CASCADE; o coletor só remove
      os IDs que a porta comprovar. Em produção a porta é `CoberturaNegada` —
      nada sai do disco.
- [x] Falha de banco, disco ou consumidor lento degrada de forma observável.
      `/ready` distingue os três: 503 com `banco: false`; 200 com
      `escrita: false` e o espaço livre; e `tempoReal` com atraso do
      despachante e conexões derrubadas por lentidão. Transformar esses sinais
      em alerta externo é A8.

---

# A5 — Infraestrutura como código e privilégio mínimo

> **Prioridade:** P1 · **Dependências:** A1 · **E-mail:** não utilizado

## Estado

**Todas as sete entregas estão escritas e verificadas até onde o repositório
alcança.** O que falta é execução contra o servidor e configuração no painel do
GitHub — trabalho de quem tem as credenciais, não código pendente. Cada linha
abaixo diz onde está a prova.

| Entrega | Onde está | Prova |
|---|---|---|
| 1 · Ansible como autoridade | `preparar-host.yml` ganha esteira e hardening; a esteira parou de copiar arquivo | job `infra` + comparação de sha256 no deploy |
| 2 · Vault explícito | `segredos/producao.yml` + `include_vars` | CI valida sem a senha do vault |
| 3 · Papéis de banco | `deploy/postgres/papeis.sql`, aplicado pelo playbook | `backend/test/repository/papeis_test.go` |
| 4 · Acesso SSH separado | `roles/acesso_esteira`, comando forçado + `sudo` restrito | `scripts/testa-wrapper-de-release.sh` |
| 5 · Hardening | `roles/hardening`: sshd, UFW, `unattended-upgrades` | `ansible-lint` + as duas travas de segurança da role |
| 6 · Mudanças controladas | `apt-mark hold` no Docker; Caddy validado antes da troca | tag `docker-upgrade`; handler restaura a cópia anterior |
| 7 · Proteção do repositório | `environment: production` no job de deploy | falta criar o Environment e proteger a `main` |

### Verificado em produção (23/08/2026)

O primeiro deploy pelo caminho novo rodou, e o que ele deixou no servidor foi
conferido de fora:

- `V18` aplicada (`flyway_schema_history`), com os dois índices únicos no lugar
  e os não-únicos removidos; zero duplicidade de ordenação;
- a API conecta como `stacktrack_app`, e `has_schema_privilege(..., 'CREATE')`
  responde `f` — **o critério de DDL deixou de ser só teste**;
- o wrapper responde `estado` no servidor, e a esteira implantou por ele;
- `/api/health` e `/` em 200;
- os quatro containers do agendaGo com duas semanas de uptime — a fronteira do
  host compartilhado não foi tocada.

E, depois da troca da chave, contra o servidor de verdade, com a chave da
esteira:

```
$ ssh -i stacktrack_deploy stacktrack-esteira@... 'stacktrack-release estado'
  → compose ps, sha256 dos três arquivos, fuso e cron

$ ssh -i stacktrack_deploy stacktrack-esteira@... 'bash'
  → stacktrack-release: verbo desconhecido: bash
```

A cadeia fecha: chave exclusiva → `restrict` + comando forçado → wrapper valida
→ `sudo` restrito a um arquivo → Docker. Uma chave vazada da esteira rende três
verbos.

### O hardening aplicado

Rodou com `--tags hardening`, e o resultado foi conferido no host e de fora:

- `ufw status`: ativo, `deny (incoming)`, com 22, 80 e 443 em IPv4 e IPv6;
- `sshd -T`: `passwordauthentication no`, `permitrootlogin no`,
  `kbdinteractiveauthentication no`, `allowagentforwarding no`,
  `x11forwarding no`;
- `apt-mark showhold`: os quatro pacotes do Docker travados;
- `20auto-upgrades` e `52stacktrack-unattended` no lugar;
- varredura de fora: 22, 80 e 443 abertas; 5432, 3000, 8080, 2375 e 2376
  fechadas;
- `https://stacktrack.duckdns.org` em 200, e os quatro containers do agendaGo
  intactos com duas semanas de uptime.

### O que só você pode fazer

1. ~~**Gerar a chave exclusiva da esteira**~~ — feito. Par próprio instalado em
   `stacktrack-esteira` com `restrict` e comando forçado, e os segredos do GitHub
   trocados. A chave do agendaGo continua em `deploy`, que é o acesso do
   operador e do vizinho.
2. ~~**`make infra-preparar` e `make infra-apply`**~~ — feito, hardening
   incluído. Usuário da esteira, wrapper, papel de runtime do banco, SSH sem
   senha, firewall e atualização automática de segurança estão no servidor, com
   o agendaGo intacto.
3. **GitHub**: criar o Environment `production` com os segredos `VPS_*` dentro
   dele e proteger a `main` com os checks da esteira. É o último critério aberto
   que depende só de configuração.
4. **Medir a idempotência**: dois `make infra-apply` seguidos, e o segundo tem
   de sair `changed=0`.

### Decisão tomada, e por quê

O usuário da esteira é **novo** (`stacktrack-esteira`), e não o `deploy`
apertado. O `deploy` é compartilhado com o agendaGo: tirar-lhe o shell ou o
grupo `docker` quebraria o deploy do vizinho, que não é deste repositório. O
novo não entra em grupo nenhum e chega ao Docker só pelo wrapper.

### Duas coisas que ficaram fora do escopo

**Rotação de chave exercitada sem indisponibilidade.** O mecanismo existe —
`authorized_key` com `exclusive: true` troca a chave numa aplicação —, mas
"exercitar a troca" é um ensaio contra o servidor, com a esteira rodando, e
entra junto com os ensaios de A10.

**`changed=0` na segunda aplicação.** O `--check` mostra `changed` na task
"sobe a stack" porque o módulo do Compose não sabe simular; a medição que vale é
a de dois `infra-apply` seguidos, contra o servidor.

## Objetivo

Tornar o host reproduzível e reduzir o impacto de uma credencial ou processo
comprometido.

## Entregas

1. **Ansible como autoridade**
   - Ansible passa a ser o único responsável por pacotes, usuários, diretórios
     persistentes, permissões, `.env`, timers/cron, firewall e configuração do
     Caddy.
   - Alterações manuais necessárias em incidente são registradas e depois
     reconciliadas no playbook.
   - Segunda execução sem mudança de inventário termina com `changed=0`.

2. **Vault carregado explicitamente**
   - Remover segredos do carregamento automático de `group_vars`.
   - Incluir o vault apenas no playbook de provisionamento/deploy que realmente
     precisa dele.
   - Permitir `ansible-playbook --syntax-check` e lint no CI sem senha do
     vault.
   - Nenhum segredo é renderizado em diff ou log.

3. **Papéis de banco**
   - Flyway usa papel proprietário de schema, exclusivo para migrations.
   - API usa papel de runtime com somente
     `SELECT/INSERT/UPDATE/DELETE` e sequências necessárias.
   - Definir `ALTER DEFAULT PRIVILEGES` para novas tabelas.
   - Runtime não cria/altera tabela, extensão, role ou database.
   - Timeouts de statement e lock são padrão do papel da aplicação.

4. **Acesso SSH separado**
   - Chave administrativa do Ansible fica fora do CI e é usada para
     provisionamento controlado.
   - Chave do CI é exclusiva do stacktrack e restrita em `authorized_keys`
     por comando forçado de deploy, sem shell, forwarding, agent forwarding,
     PTY ou acesso a outros projetos.
   - O comando forçado aceita apenas versão/digest validado e delega operações
     mínimas.
   - Usuário de deploy não entra no grupo `docker` com shell genérico — acesso
     ao socket equivale a root. O wrapper expõe somente as operações de release
     necessárias, com argumentos validados.
   - Definir rotação/revogação das chaves e exercitar a troca sem indisponibilidade.

5. **Hardening do host**
   - Desabilitar login SSH por senha e login direto de root.
   - UFW expõe somente 22, 80 e 443; banco e métricas não ficam públicos.
   - Atualizações automáticas apenas de segurança, sem reboot automático.
   - Reboot necessário gera alerta e janela planejada.
   - Serviço roda como usuário sem privilégios; diretórios usam menor
     permissão possível.

6. **Mudanças controladas**
   - Postgres e Docker não são atualizados implicitamente junto de cada deploy
     da aplicação.
   - Caddy é validado antes da troca e aplicado de modo atômico, com cópia
     anterior recuperável.
   - Aprofundamento e procedimentos de Ansible permanecem em
     [docs/tecnologias.md](docs/tecnologias.md), evitando duplicação aqui.

7. **Proteção do repositório e dos segredos**
   - Usar GitHub Environment `production`, com aprovação/proteção definida.
   - Proteger `main` com os checks obrigatórios da release.
   - Segredos de produção ficam escopados ao Environment e não chegam a jobs de
     pull request ou validação comum.

## Contratos operacionais

- Inventário declara ambiente, domínio, diretórios e versões não secretas.
- Vault contém somente valores secretos, com nomes documentados e sem defaults
  inseguros.
- O comando de deploy recebe referência de release/digests, nunca comando shell
  arbitrário.
- O runbook distingue chave administrativa, chave de deploy e credenciais do
  banco.

## Migrations e rollout

- Não há migration de domínio.
- Scripts idempotentes criam os papéis do PostgreSQL e transferem ownership
  antes de revogar privilégios do runtime.
- Fazer auditoria de grants antes e depois; manter janela de rollback que
  restaure grants, sem devolver superusuário à API.

## Testes obrigatórios

- `ansible-lint`, syntax check e checagem de idempotência;
- API consegue executar todas as operações normais;
- tentativa de `CREATE TABLE`, `ALTER TABLE` e `DROP TABLE` pela API falha;
- Flyway continua migrando com credencial própria;
- chave do CI não abre shell, não faz forwarding e não lê `.env`;
- chave de deploy é rotacionada e a anterior deixa de funcionar;
- job de pull request não consegue ler segredo do Environment de produção;
- varredura externa mostra somente portas esperadas;
- configuração inválida do Caddy não substitui a ativa;
- restauração das permissões é exercitada em ambiente isolado.

## Critérios de aceite

- [x] Estado persistente do host é reproduzível por Ansible.
- [x] CI valida Ansible sem possuir a senha do vault.
- [ ] Segunda aplicação do playbook é idempotente. *(medição contra o servidor)*
- [x] API não possui DDL nem privilégios administrativos.
- [x] Credencial de deploy não oferece shell genérico nem acesso a segredos.
- [ ] `main` e o Environment de produção aplicam os gates definidos. *(painel do
      GitHub)*
- [x] Portas internas não estão expostas à internet.

---

# A6 — Recuperação de desastre comprovada

> **Prioridade:** P0 · **Dependências:** A2, A4 e A5 · **E-mail:** não utilizado

## Objetivo

Ser capaz de perder completamente o VPS e recuperar banco, anexos,
configurações essenciais e versão da aplicação dentro de RPO 24 h / RTO 4 h.

## Entregas

1. **Snapshot externo coerente**
   - Restic envia para armazenamento S3 compatível fora do VPS.
   - Cada execução inclui:
     - dump PostgreSQL validado;
     - manifest da release e do schema;
     - anexos imutáveis;
     - inventário não secreto necessário à restauração.
   - O dump define o corte lógico do banco. Como arquivos publicados são
     imutáveis e o GC espera um backup posterior, a cópia pode conter um
     superset seguro: a restauração usa as linhas do dump como conjunto ativo
     e verifica que todo arquivo referenciado existe. Isso dispensa pausa de
     manutenção sem fingir atomicidade entre PostgreSQL e filesystem.
   - Abrir uma transação `REPEATABLE READ`, exportar seu snapshot e usar o
     mesmo identificador em `pg_dump --snapshot`. Nessa mesma visão, gerar a
     lista exata dos IDs pendentes em `arquivo_exclusoes`.
   - Proteger o repositório com versionamento e, quando disponível, Object
     Lock/imutabilidade.

2. **Execução robusta**
   - `flock` impede duas rotinas simultâneas.
   - Arquivos intermediários ficam em diretório temporário dedicado e são
     removidos com segurança.
   - Falha do dump, validação, upload ou prune torna a execução inteira falha.
   - Só depois de dump, arquivos, lista e manifest serem validados externamente,
     o sucesso atualiza heartbeat e marca como cobertos os IDs exatos usados
     pelo GC de anexos.

3. **Manifest**
   - Registrar data UTC, commit, digests completos das imagens, versão do
     PostgreSQL, última migration, hashes/tamanhos do dump e política de
     anexos.
   - Manifest tem versão de schema e validação automática.
   - Não incluir segredo.

4. **Retenção**
   - 14 snapshots diários;
   - 8 semanais;
   - 12 mensais;
   - prune somente após `restic check` e nunca como substituto de
     monitoramento de capacidade.

5. **Kit de recuperação fora do VPS**
   - Credenciais e senha do repositório armazenadas em cofre separado.
   - Runbook, inventário mínimo e chaves de verificação acessíveis mesmo com o
     Git/VPS indisponível.
   - Definir responsáveis e procedimento de rotação.

6. **Proteção antes de migration**
   - Deploy que contém migrations verifica se há dados.
   - Quando houver, exige snapshot externo recente e dispara backup
     pré-migration.
   - Migration não inicia se o backup/heartbeat falhar.

7. **Restauração verificada**
   - Restaurar em host/diretório isolado.
   - Usar `psql -v ON_ERROR_STOP=1`.
   - Reaplicar roles/grants e validar a versão das imagens.
   - Conferir que cada `anexos.caminho` ativo possui arquivo e hash esperado.
   - Executar smoke: login, abrir quadro, mutar card e baixar anexo.
   - Fazer exercício mensal e registrar tempo real, falhas e ação corretiva.

8. **Ativação do GC de arquivos**
   - Conectar a porta de cobertura de A4 aos manifests remotos validados.
   - Ativar o worker somente depois do primeiro restore drill aprovado.
   - Comprovar em ambiente operacional que ID ausente não remove e ID listado
     no snapshot converge, independentemente de timestamp ou clock skew.

## Contratos operacionais

O manifest precisa ter, no mínimo:

```json
{
  "versao": 1,
  "criadoEm": "2026-08-21T03:00:00Z",
  "commit": "sha",
  "imagens": {"api": "repo@sha256:...", "frontend": "repo@sha256:..."},
  "postgres": "major.minor",
  "migration": "versao-aplicada",
  "dump": {"arquivo": "db.dump", "sha256": "...", "bytes": 123},
  "anexos": {
    "exclusoesCobertas": {
      "arquivo": "arquivo-exclusoes.jsonl.gz",
      "sha256": "...",
      "quantidade": 42
    }
  }
}
```

Heartbeat externo informa último sucesso, duração e identificador do snapshot;
não depende da própria API nem do próprio VPS para alertar.

## Migrations e rollout

- Não há migration de domínio prevista.
- Mudanças de metadados de anexo necessárias ao hash/imutabilidade seguem A4.
- Antes de ativar GC, produzir pelo menos um snapshot externo e concluir uma
  restauração bem-sucedida.

## Testes obrigatórios

- restaurar snapshot em ambiente vazio;
- dump truncado ou hash divergente bloqueia restauração;
- credencial sem acesso ao VPS ainda obtém o kit e o repositório;
- falha do armazenamento externo não atualiza heartbeat/cobertura;
- duas execuções concorrentes resultam em uma execução e uma saída segura;
- caminho de anexo ausente faz o drill falhar;
- cobertura externa real libera somente os IDs que ela enumera;
- transação longa e relógio propositalmente divergente não fazem o GC liberar
  exclusão ausente do snapshot compartilhado;
- simulação de perda total mede RPO e RTO;
- restauração em versão incompatível do Postgres é detectada antes da carga.

## Critérios de aceite

- [ ] Um VPS vazio é reconstruído sem consultar arquivos do VPS perdido.
- [ ] O exercício completo termina em menos de 4 horas.
- [ ] Nenhum dado confirmado há mais de 24 horas fica fora do snapshot.
- [ ] Ausência de backup por 26 horas gera alerta externo.
- [ ] Backup pré-migration bloqueia deploy quando não é confiável.
- [ ] GC de anexo usa apenas cobertura exata do snapshot remoto confirmado.
- [ ] O GC real foi ativado somente após restore drill e testado ponta a ponta.

---

# A7 — Artefato exato, rollback e cadeia de suprimentos

> **Prioridade:** P1 · **Dependências:** A5 e A6 · **E-mail:** não utilizado

## Objetivo

Construir uma vez, testar exatamente o que será implantado, promover por digest
e voltar à release anterior sem recompilar.

## Entregas

1. **Build único**
   - API, frontend e migrations são construídos uma vez por release.
   - Publicar candidato no registry e capturar digest completo.
   - Testes, scanners, assinatura e deploy recebem esses digests.
   - Proibido reconstruir depois da aprovação.

2. **Ambiente de aprovação equivalente**
   - E2E sobe as imagens finais da API, migrations e frontend/Node.
   - Caddy interno usa TLS e a mesma topologia de rotas de produção.
   - Banco nasce limpo, recebe migrations da imagem candidata e dados de teste.
   - Rate limit pode ser elevado apenas por variável explícita do ambiente E2E.
   - Encontrar `429` inesperado falha o teste; nunca usar `test.skip` para
     esconder instabilidade.

3. **Deploy por digest**
   - Compose recebe referências completas
     `registry/imagem@sha256:...`.
   - Cada serviço pode ter digest diferente; não usar uma única tag mutável
     como “versão”.
   - `docker compose up -d --wait` só encerra após healthchecks.
   - `/api/ready` valida banco, compatibilidade de migration, disco e
     componentes críticos.
   - Ansible prepara bundles versionados de Compose, fragmento Caddy e
     configuração runtime com permissões restritas. O deploy só referencia e
     ativa um bundle cujo hash consta no manifest; o CI não reescreve `.env`.

4. **Rollback**
   - Guardar manifest e bundle de configuração da release atual e anterior.
   - Falha de readiness/smoke restaura atomicamente digests, Compose, Caddy e
     configuração runtime anteriores, valida Caddy e repete o smoke.
   - Migration incompatível com rollback precisa de expand/contract ou
     procedimento manual aprovado antes do deploy.
   - Postgres é fixado por digest e excluído do pull rotineiro da aplicação.

5. **Verificações de qualidade**
   - Go: testes, `go vet`, `staticcheck`, análise de segurança
     (`gosec` ou CodeQL) e `govulncheck`.
   - Frontend: typecheck, testes, ESLint, auditoria de dependências e build.
   - Infra: ShellCheck, actionlint, hadolint, ansible-lint e validação Caddy.
   - Scannear sistema operacional e dependências nas imagens.
   - Secret scanning cobre histórico novo, imagens e artefatos de build.

6. **Política de vulnerabilidade**
   - CRITICAL bloqueia promoção.
   - HIGH bloqueia ou exige exceção escrita, com mitigação, responsável e
     validade máxima de 30 dias.
   - Exceção expirada volta a bloquear.
   - Dependabot abre atualizações semanais para Go, npm, Docker e Actions.
   - Reescanear periodicamente o digest que está em produção, não somente
     candidatos novos, porque a base de vulnerabilidades muda sem novo build.

7. **Proveniência**
   - Gerar SBOM por imagem.
   - Gerar atestado de proveniência.
   - Assinar de forma keyless quando o registry/CI suportar.
   - Deploy verifica assinatura, proveniência e digest antes de executar o
     comando remoto.
   - Actions e imagens-base também ficam fixadas por SHA/digest.

## Contratos operacionais

O manifest de release relaciona commit, workflow, digests, SBOM, assinatura,
migrations esperadas, release anterior, revisão da infraestrutura e hashes do
Compose renderizado, fragmento Caddy e schema/bundle de configuração runtime.
Ele é a única entrada aceita pelo deploy; valores secretos não entram nele.

O endpoint de versão/readiness expõe somente identificadores não secretos:
commit curto, release e estado de compatibilidade. Digests completos ficam no
manifest e na telemetria operacional.

## Migrations e rollout

- Nenhuma migration de domínio.
- O pipeline classifica release com migration como compatível ou não com
  rollback.
- Antes de promover, executar migration em cópia representativa e restaurar a
  release anterior quando o ciclo expand/contract disser que isso é seguro.

## Testes obrigatórios

- comparar digest analisado, assinado e implantado;
- alteração de tag após aprovação não muda o artefato implantado;
- assinatura ou SBOM ausente bloqueia deploy;
- healthcheck falho restaura release anterior;
- smoke pós-deploy falho aciona rollback;
- rollback restaura também o bundle de configuração e o Caddy anterior;
- hash divergente em Compose/Caddy/runtime bloqueia a ativação;
- migration incompatível bloqueia rollback automático antes da produção;
- E2E roda com TLS, Caddy e imagens finais;
- `429` inesperado faz o job falhar;
- Postgres não é atualizado em deploy comum.

## Critérios de aceite

- [ ] O digest em produção é idêntico ao aprovado no CI.
- [ ] Nenhuma etapa reconstrói o artefato promovido.
- [ ] Rollback da aplicação foi exercitado com a release anterior.
- [ ] Configuração e proxy restaurados pertencem ao mesmo manifest dos digests.
- [ ] Release com migration declara estratégia de compatibilidade.
- [ ] Vulnerabilidades seguem política comprovável, sem exceção permanente.
- [ ] SBOM, proveniência e assinatura acompanham cada imagem.

---

# A8 — Observabilidade, SLO e alertas

> **Prioridade:** P1 · **Dependências:** A3, A4, A6 e A7 · **E-mail:** não utilizado

## Objetivo

Detectar indisponibilidade, degradação e risco de perda antes do usuário, com
evidência suficiente para localizar a release e o componente responsáveis.

## Entregas

1. **Instrumentação independente de fornecedor**
   - OpenTelemetry no backend e nos pontos críticos do frontend.
   - Exportação OTLP configurável para serviço externo.
   - Métricas internas não ficam expostas à internet.
   - Logs estruturados incluem `requestId`, release/digest, rota lógica,
     status e duração.
   - Frontend captura `window.onerror` e `unhandledrejection`, associa a release
     e usa source maps privados, nunca publicados junto aos assets.

2. **Métricas da aplicação**
   - requisições por rota/status, latência e tamanho;
   - espera, uso, timeout e erro do pool pgx;
   - duração de transação e conflito/lock timeout;
   - revisão, atraso e backlog do dispatcher;
   - conexões, reconexões, fila, consumidor lento e fechamento de WebSocket;
   - reservas, bytes, cota, órfãos e GC de anexos;
   - idade/duração/resultado de backup e restore drill;
   - memória, CPU, disco, inodes, OOM e reinícios do processo/host;
   - falha de reducer, reconciliação, loop de reconexão e Web Vitals do cliente.

3. **Tracing**
   - Propagar correlação HTTP → caso de uso → SQL → evento/dispatcher.
   - Não gravar statement com parâmetros sensíveis.
   - Amostrar erros e operações lentas com taxa superior às operações comuns.
   - Relacionar trace à release exata.

4. **Sondas externas**
   - Abrir página pública.
   - Consultar `/api/ready`.
   - Abrir WebSocket, receber handshake e encerrar.
   - Executar fora do VPS e do provedor quando possível.

5. **SLO e painéis**
   - Disponibilidade mensal inicial: 99,5%.
   - Antes da carga de A10, usar orçamentos provisórios já acionáveis:
     snapshot p95 < 500 ms, mutação p95 < 300 ms e entrega realtime p95 < 1 s.
     A10 confirma ou ajusta esses valores com baseline registrado, sem deixar
     o alerta desativado entre as etapas.
   - Painéis separados para experiência HTTP, banco, realtime, anexos, host e
     proteção de dados.
   - Registrar orçamento de erro e congelar releases de risco quando estiver
     consumido.
   - Reter inicialmente logs e sinais operacionais por 30 dias; qualquer prazo
     diferente precisa ser justificado na matriz de A11.

6. **Alertas acionáveis**
   - 5xx acima de 2% por 5 minutos;
   - latência p95 acima do orçamento por janela sustentada;
   - espera do pool p95 acima de 100 ms ou timeout de aquisição;
   - disco em 80% (aviso) e 90% (crítico);
   - inodes próximos do esgotamento e certificado TLS a menos de 14 dias do
     vencimento;
   - último backup bem-sucedido acima de 26 horas;
   - OOM/reinícios inesperados;
   - crescimento de fila WebSocket ou atraso do dispatcher;
   - loop de reconexão ou aumento anormal de reconciliações no frontend;
   - falha da sonda externa.
   - Cada alerta aponta para runbook e evita mensagem dependente de e-mail;
     canal operacional não SMTP é escolhido na configuração.

7. **Privacidade de telemetria**
   - Não coletar corpo de card/comentário, nome de arquivo, token, cookie,
     senha, link de convite ou e-mail completo.
   - Identificador de usuário, quando indispensável, é pseudonimizado.
   - Definir retenção e acesso do provedor de observabilidade.

## Contratos operacionais

- `requestId` entra no header de resposta e no envelope de erro de A9.
- `/api/live` significa apenas processo vivo.
- `/api/ready` significa apto a receber tráfego com dependências e espaço
  mínimos.
- Métricas têm nomes/unidades estáveis e labels de cardinalidade limitada;
  nunca usar board/card/user ID como label.

## Migrations e rollout

- Nenhuma migration de domínio prevista.
- Ativar exporters em modo controlado; validar custo/cardinalidade antes de
  aumentar retenção e amostragem.
- Instrumentar primeiro, criar painéis, depois ativar alertas com janela de
  calibração curta e data de término.

## Testes obrigatórios

- derrubar banco em ambiente de teste e receber alerta em menos de 5 minutos;
- interromper backup e receber alerta externo após o limiar;
- provocar consumidor lento e observar métrica/fechamento;
- localizar uma requisição de erro pelo `requestId` e pelo digest;
- scanner de logs/telemetria confirma ausência de tokens, cookies e PII;
- erro sintético de JavaScript é localizado pela release usando source map
  privado;
- teste de cardinalidade impede IDs livres em labels;
- sonda WebSocket detecta proxy/API indisponível.

## Critérios de aceite

- [ ] Incidente de banco é detectado em menos de 5 minutos.
- [ ] Ausência de backup é detectada mesmo com o VPS fora do ar.
- [ ] Um erro de usuário pode ser ligado à rota, trace e release.
- [ ] Painéis cobrem HTTP, banco, realtime, anexos, host e DR.
- [ ] Alertas possuem limiar, responsável e runbook.
- [ ] Telemetria não contém conteúdo de usuário ou segredo.

---

# A9 — Contratos, robustez do cliente e acessibilidade

> **Prioridade:** P2 · **Dependências:** A3 e A8 · **E-mail:** não utilizado

## Objetivo

Tornar falhas previsíveis entre backend e frontend, impedir feedback incorreto
ao usuário e estabelecer uma base acessível e testável para os fluxos críticos.

## Entregas

1. **Envelope de erro estável**

   ```json
   {
     "erro": {
       "codigo": "CARD_VERSAO_DESATUALIZADA",
       "mensagem": "O card foi alterado em outra sessão.",
       "requestId": "req_...",
       "campos": {
         "versao": "atualize os dados e tente novamente"
       }
     }
   }
   ```

   - Código estável e linguagem independente.
   - Mensagem segura para exibição.
   - `campos` opcional para validação.
   - Nunca expor SQL, stack trace ou erro interno.
   - Status HTTP continua semanticamente correto.

2. **Cliente HTTP tipado**
   - `ApiError` contém status, código, requestId, campos e `Retry-After`.
   - Parser transitório entende o envelope antigo e o novo.
   - Todas as funções aceitam `AbortSignal`.
   - Timeout padrão de 10 segundos; upload possui orçamento próprio.
   - Somente `GET` pode ter uma repetição automática, com jitter, para erro de
     rede, 502, 503 ou 504.
   - Escritas nunca são repetidas automaticamente sem chave idempotente.

3. **Contratos executáveis**
   - Validar respostas críticas no frontend com Zod.
   - Manter fixtures JSON versionadas e comprometidas no repositório.
   - Testes Go produzem/validam as fixtures; testes TypeScript consomem as
     mesmas fixtures.
   - Cobrir sessão, snapshot, eventos, convites, erro e anexos.
   - Não criar Swagger/OpenAPI manual ou uma segunda lista de rotas para
     manter.

4. **Resultado da mutação separado da atualização**
   - Sucesso de criar/mover/comentar não é transformado em “falha” porque uma
     atualização posterior do snapshot falhou.
   - Mostrar “alteração salva; atualização pendente” e reconciliar.
   - Para pesquisas e carregamentos concorrentes, somente a resposta mais nova
     pode substituir o estado.
   - Desabilitar duplo envio ou usar idempotência onde ele ainda for possível.

5. **Estado de sessão honesto**
   - Modelar `desconhecida`, `autenticada`, `anônima` e `indisponível`.
   - Erro temporário em `/auth/me` não vira logout silencioso.
   - Se logout falhar no servidor, limpar a experiência local com aviso de que
     a sessão remota pode persistir e oferecer nova tentativa.

6. **Redução de componentes críticos**
   - Extrair stores/controllers da página do quadro e do maior modal.
   - Separar aquisição de dados, comando, reducer e apresentação.
   - Evitar mais regras de realtime diretamente em componentes Svelte.

7. **Primitiva única de modal**
   - Stack de modais, focus trap, `inert` no fundo, restauração do foco e
     Escape apenas no modal superior.
   - Título/descrição associados e scroll lock consistente.
   - Confirmações destrutivas continuam específicas.

8. **Acessibilidade e semântica**
   - Remover elementos interativos aninhados e usos indevidos de `role`.
   - Fluxos completos por teclado, foco visível e ordem previsível.
   - `aria-live` para salvamento, conflito, reconexão e erro.
   - Drag-and-drop possui alternativa por teclado.
   - Contraste e estados não dependem somente de cor.

## Contratos

- Lista canônica de códigos de erro fica junto ao domínio/adapter e é testada.
- Fixtures incluem versão quando o formato puder evoluir.
- Alteração incompatível exige período de parser duplo ou deploy coordenado.
- `Retry-After` é respeitado pelo cliente, mas não dispara repetição de
  mutação.

## Migrations e rollout

- Nenhuma migration de banco prevista.
- Liberar primeiro o **frontend com parser duplo**, ainda consumindo o envelope
  antigo; depois ativar o envelope novo no backend.
- Manter respostas antigas compatíveis durante a janela definida. Remover o
  parser e o formato antigos apenas após telemetria confirmar ausência de
  clientes legados, inclusive abas que ficaram abertas durante um deploy.

## Testes obrigatórios

- fixtures compartilhadas falham quando Go e TypeScript divergem;
- resposta fora de ordem não substitui resultado mais novo;
- timeout/cancelamento não deixa loading permanente;
- duplo clique não cria duas entidades;
- mutação bem-sucedida + refresh falho não exibe “não foi salvo”;
- logout offline exibe estado verdadeiro;
- modal aninhado restaura foco e Escape fecha apenas o topo;
- smoke E2E em Chromium, Firefox e WebKit;
- auditoria automatizada com axe nas páginas e modais críticos;
- navegação completa por teclado e alternativa ao DnD.

## Critérios de aceite

- [ ] Todo erro crítico possui código, requestId e mensagem segura.
- [ ] DTO incompatível falha de forma visível no teste e de forma recuperável
      em produção.
- [ ] Cliente não repete escrita automaticamente.
- [ ] Estado de sessão não confunde indisponibilidade com anonimato.
- [ ] Fluxos críticos passam em três engines e em axe sem violação séria.
- [ ] Modais obedecem foco, pilha, Escape e restauração.

---

# A10 — Capacidade, estado incremental e ciclo de eventos

> **Prioridade:** P1 · **Dependências:** A3, A4, A8 e A9 · **E-mail:** não utilizado

## Objetivo

Comprovar o perfil de capacidade definido neste plano e substituir
recarregamentos amplos por atualização incremental segura, sem introduzir
infraestrutura distribuída antes da necessidade.

## Entregas

1. **Read model eficiente**
   - Snapshot executa consultas em lote e usa uma única conexão/transação.
   - Eliminar N+1 de responsáveis, etiquetas, checklists e contagens.
   - Card do quadro vira `CardResumo`: não inclui descrição, comentários,
     histórico ou anexos completos.
   - Detalhe pesado é carregado ao abrir o card.
   - Snapshot inclui revisão e `ETag`; requisição sem mudança pode receber
     `304`.

2. **Paginação e índices**
   - Comentários e históricos usam cursor estável, 50 itens por página.
   - Índices cobrem último movimento, auditoria por quadro/card e consultas de
     replay.
   - Validar planos com volume representativo, não somente banco vazio.

3. **Limites de produto**

   | Recurso | Limite inicial |
   |---|---:|
   | Colunas por quadro | 50 |
   | Cards por quadro | 1.000 |
   | Membros por quadro | 100 |
   | Etiquetas por quadro | 100 |
   | Checklists por card | 20 |
   | Itens de checklist por card | 200 |
   | Anexos | limites definidos em A4 |

   - Contagem e inserção são atômicas.
   - Erro possui código de domínio estável e instrução útil.
   - Limites são documentados como parte do perfil do produto.

4. **Estado normalizado no frontend**
   - Substituir `invalidateAll` por reducer tipado sobre entidades
     normalizadas.
   - Aplicar evento conhecido idempotentemente.
   - Evento inválido/desconhecido ou lacuna de revisão busca snapshot.
   - Modal aberto recebe apenas evento do card correspondente.
   - Lista de membros/etiquetas é cacheada por revisão e invalidada por evento
     específico.
   - Reconciliar ao voltar foco, rede ou conexão, sem invalidar
     `/auth/me` junto com o quadro.

5. **Renderização sob volume**
   - Medir DOM, scripting e memória com 1.000 cards em perfil de navegador e
     hardware versionado junto ao ensaio.
   - Se o orçamento não for atingido, usar
     `@tanstack/svelte-virtual` ou solução equivalente.
   - Virtualização precisa preservar teclado, foco, busca e drag-and-drop; não
     entra apenas para reduzir número de nós.

6. **Ciclo de vida de eventos**
   - Manter 365 dias consultáveis no PostgreSQL para **quadros existentes**.
     A exclusão destrutiva de um quadro é exceção explícita: o cascade atual
     remove seu histórico online e a confirmação informa isso ao dono.
   - Exportar período anterior como JSONL comprimido, com manifest, faixas de
     `(boardId, revisao, indice)`, identidades `seq`, hashes e contagem. `seq`
     não é usado para decidir que um commit já foi exportado.
   - Em transação curta sob o lock do quadro, reservar uma faixa **contígua**
     de revisões num ledger; gerar/enviar fora da transação; validar; marcar
     concluída; só então excluir exatamente a faixa concluída em lotes.
   - Enviar o pacote criptografado com chave por quadro/período. Remover nomes,
     e-mails e outras cópias diretas de identidade; `autorId` fica pseudônimo e
     títulos/comentários ficam classificados como conteúdo do quadro, com
     acesso operacional restrito e a mesma proteção de segredo dos backups.
     Esta é a política mínima obrigatória antes de ligar o exportador; A11 a
     documenta na matriz e integra o ledger de anonimização, sem ser
     pré-requisito oculto de A10.
   - Reter o arquivo por 24 meses após a exportação, salvo requisito aprovado
     que altere a matriz; nunca manter arquivo “para sempre” por omissão.
   - Exclusão do quadro cancela exportação pendente e elimina as chaves
     embrulhadas dos seus pacotes, produzindo apagamento criptográfico mesmo se
     os objetos imutáveis expirarem depois.
   - Validar primeiro e último evento e contagem após upload.
   - Cliente offline além da retenção recebe snapshot completo, não erro ou
     replay incompleto.

7. **Teste de capacidade**
   - Cenário: 25 conexões, 10 editores, 20 colunas, 1.000 cards.
   - Repetir o teste de fronteira com 50 colunas e os mesmos 1.000 cards, pois
     todo limite aceito pelo domínio precisa ser funcionalmente suportado.
   - Sustentar 5 mutações/s, com rajadas de 20, por 10 minutos.
   - Misturar movimento, comentário, checklist, responsável e leitura.
   - Medir HTTP, SQL, pool, dispatcher, WebSocket, CPU, memória e DOM.

## Orçamentos de aceite

| Indicador | Orçamento inicial |
|---|---:|
| Snapshot do quadro p95 | < 500 ms |
| Mutação p95 | < 300 ms |
| Evento visível em outro cliente p95 | < 1 s |
| Espera do pool p95 | < 100 ms |
| Erros 5xx no ensaio | 0 |
| Memória da API | < 80% do limite configurado |
| Observadores limitados indevidamente | 0 respostas 429 |
| Interação crítica no navegador p95 | < 200 ms |
| Maior long task no quadro | < 200 ms |
| Total blocking time após o snapshot | < 300 ms |
| Nós DOM com 1.000 cards | < 3.000 |
| Crescimento do heap do frontend | < 100 MiB |

## Contratos

- `CardResumo` e `CardDetalhe` são DTOs distintos.
- Paginação usa cursor opaco; não promete número de página.
- `ETag` representa a revisão do snapshot.
- Códigos de limite são específicos, por exemplo
  `BOARD_LIMITE_DE_CARDS`.
- Evento exportado conserva envelope/versionamento de A3.

## Migrations e rollout

Migrations lógicas:

- índices validados por `EXPLAIN (ANALYZE, BUFFERS)` em dataset de teste;
- metadados de último movimento quando a consulta não puder ser obtida com
  índice eficiente;
- `event_export_ranges` com board, revisão inicial/final, estado, manifest,
  hash, chave embrulhada e timestamps; unicidade impede reservar/exportar a
  mesma revisão duas vezes.

Criar índice de forma adequada ao tamanho real e remover índice antigo somente
após confirmar uso. Exclusão de eventos acontece em lotes pequenos, com
faixas concluídas no ledger e pausa sob pressão do banco.

O contrato é implantado de forma aditiva:

1. backend oferece `CardResumo`, detalhe e paginação novos sem retirar os campos
   e endpoints consumidos por abas antigas;
2. frontend novo migra para os contratos e envia a versão/capacidade esperada;
3. telemetria e testes cruzados comprovam que não há cliente legado relevante;
4. somente então uma release contract remove campos ou respostas não paginadas.

## Testes obrigatórios

- snapshot com 1.000 cards mantém quantidade limitada de queries;
- reducer aplica duplicata sem duplicar entidade;
- evento desconhecido ou revisão ausente força snapshot;
- evento de outro card não altera modal aberto;
- paginação não perde/duplica item em inserção concorrente;
- limite resistindo a duas criações simultâneas;
- exportar, verificar, restaurar e consultar pacote antigo;
- mutação concorrente à reserva de exportação recebe revisão maior e não fica
  fora do pacote atual nem do próximo;
- nenhuma faixa é apagada antes do ledger concluído e reprocessar exportação é
  idempotente;
- excluir quadro cancela o pacote pendente e torna pacotes concluídos
  criptograficamente inacessíveis;
- cliente offline por mais de 365 dias recebe snapshot;
- ensaio de 10 minutos cumpre todos os orçamentos;
- perfil do navegador mede nós DOM, memória e long tasks;
- frontend antigo funciona com backend expandido e frontend novo funciona
  durante a janela de compatibilidade.

## Critérios de aceite

- [ ] O perfil alvo passa pelos orçamentos sem erro 5xx.
- [ ] O cenário de fronteira com 50 colunas também respeita os orçamentos.
- [ ] Frontend não recarrega todo o quadro a cada evento conhecido.
- [ ] Snapshot não carrega conteúdo pesado de todos os cards.
- [ ] Limites são atômicos e comunicados por código de domínio.
- [ ] Eventos antigos só são removidos após exportação externa verificada.
- [ ] Retenção de 365 dias e a exceção de exclusão do quadro estão explícitas e
      testadas.
- [ ] Não foi adicionada segunda API, Redis ou pub/sub distribuído.

## Gatilhos para um plano de escala horizontal futuro

Somente abrir um novo roadmap para múltiplas APIs se, **depois** destas
otimizações, ocorrer de forma sustentada um dos casos:

- CPU acima de 70% no perfil alvo;
- pool de banco saturado ou espera acima do orçamento;
- SLO/realtime falha com recursos verticais economicamente razoáveis;
- disponibilidade exigida passa a ser incompatível com um VPS.

Esse novo plano deverá tratar pub/sub, presença distribuída, rate limit
compartilhado, afinidade/roteamento de sockets e armazenamento compartilhado.
Nada disso pertence ao backlog ativo atual.

---

# A11 — Privacidade, ciclo de dados e qualificação sem e-mail

> **Prioridade:** P2 · **Dependências:** A1–A10 · **E-mail:** não utilizado

## Objetivo

Definir como dados pessoais entram, permanecem, são exportados e são removidos,
e comprovar que o sistema está operacionalmente pronto antes de adicionar
fluxos dependentes de e-mail.

## Entregas

1. **Inventário e retenção**
   - Mapear dado, finalidade, origem, tabela/arquivo/log, acesso, retenção e
     destino na exclusão.
   - Incluir conta, sessão, convite, participação, auditoria, comentário,
     anexo, logs, traces, backups e exportações de eventos.
   - Documentar o que precisa sobreviver anonimizado por integridade do quadro.

2. **Minimização**
   - Remover e-mail completo de logs e telemetria; usar máscara ou hash
     rotacionável quando correlação for indispensável.
   - Não duplicar PII em payload de evento quando ID/valor mascarado bastar.
   - Restringir acesso operacional a dumps, traces e armazenamento externo.
   - Manter um ledger externo de anonimizações/apagamentos, separado dos
     snapshots que ele corrige; toda restauração o reaplica antes de liberar
     acesso aos dados.

3. **Exportação da conta**
   - Usuário autenticado e com senha atual pode gerar exportação.
   - Conteúdo inclui dados da própria conta e relações permitidas, sem revelar
     PII privada de outros membros além do que o produto já autoriza.
   - Geração tem limite, trilha de auditoria e arquivo temporário com expiração.
   - Solicitação e download exigem sessão válida e confirmação da senha na
     própria operação; não dependem de link por e-mail.

4. **Exclusão/anomização**
   - Exigir senha atual e confirmação destrutiva.
   - Bloquear exclusão do único dono até transferir ou excluir seus quadros.
   - Revogar sessões e convites emitidos/recebidos.
   - Remover participações e responsabilidades conforme invariantes.
   - Anonimizar a conta no lugar:
     - nome e e-mail substituídos por valores não identificáveis e únicos;
     - hash de senha torna-se inutilizável;
     - preencher `excluido_em`;
     - comentários, histórico e anexos preservados quando pertencem ao quadro,
       exibindo “Conta removida”;
     - limpar PII de payloads de auditoria que não precise permanecer.
   - Registrar no ledger externo o identificador pseudônimo da conta; leitores
     de arquivos históricos exibem “Conta removida” sem consultar cópia antiga
     de nome/e-mail.
   - Exclusão de quadro registra tombstone e destruição das chaves de arquivo
     de A10. Depois de restore, o ledger elimina novamente as chaves antes de
     qualquer pacote histórico ser acessado.
   - Documentar que backups imutáveis expiram pela retenção, não são
     reescritos.

5. **Runbooks**
   - incidente e comunicação;
   - rollback de aplicação;
   - restauração completa;
   - rotação de chave/segredo;
   - banco indisponível;
   - disco cheio;
   - backup atrasado;
   - fila/dispatcher de realtime atrasado;
   - vazamento de token ou credencial.

6. **Gate de go-live**
   - Sem P0/P1 aberto.
   - Restore, rollback e teste de carga dentro do orçamento.
   - E2E multi-browser e acessibilidade aprovados.
   - Nenhuma vulnerabilidade CRITICAL/HIGH sem exceção válida.
   - Alertas e sondas externos ativos.
   - Documentação representa a release efetiva.

## Contratos

- Endpoints de exportação, download e exclusão exigem sessão válida e senha
  atual verificada dentro da própria operação; não existe o conceito vago de
  “sessão recente”.
- Exportação pode ser assíncrona com consulta autenticada de status; nenhuma
  notificação por e-mail.
- Conta excluída recebe identificador técnico estável para referências, mas
  nenhum dado que permita reidentificação na interface.
- Respostas não confirmam dados de outras pessoas.

## Migrations e rollout

Migrations lógicas:

- `usuarios.excluido_em TIMESTAMPTZ NULL`;
- ajustes para manter e-mail normalizado único após anonimização;
- estado/expiração de exportações, se o processamento for persistente;
- índices para limpeza de sessões, convites e arquivos temporários.

Executar anonimização em transação para banco e outbox para arquivos. Testar
primeiro em cópia restaurada. Não apagar eventos em massa sem respeitar a
exportação e retenção de A10.

## Testes obrigatórios

- exportação contém o que deve e não vaza dados privados de outro usuário;
- senha incorreta ou sessão inválida bloqueia exportação/exclusão;
- único dono não consegue deixar quadro órfão;
- exclusão revoga todas as sessões e impede novo login;
- duas exclusões/requisições concorrentes são idempotentes;
- comentários/histórico preservam integridade com “Conta removida”;
- pacote histórico não contém nome/e-mail direto e respeita o tombstone da
  conta ao ser consultado;
- busca por e-mail antigo não encontra conta ativa;
- logs, traces, eventos e arquivos temporários respeitam a matriz;
- restauração de backup reaplica anonimizações e destruições de chave do ledger
  externo antes do smoke;
- tabletop dos runbooks e go-live checklist em ambiente restaurado.

## Critérios de aceite

- [ ] Existe matriz de dados e retenção aprovada.
- [ ] Usuário exporta e exclui a conta sem depender de e-mail.
- [ ] Exclusão não quebra quadros nem apaga histórico necessário.
- [ ] Sessões e acessos são revogados de forma atômica.
- [ ] Backups têm tratamento de expiração explicitamente comunicado.
- [ ] Arquivos históricos e restaurações respeitam o ledger externo de
      anonimização/apagamento.
- [ ] Todos os gates de produção A1–A10 possuem evidência.

---

# A12 — Serviço de e-mail, verificação e recuperação

> **Prioridade:** ÚLTIMA · **Dependências:** A11 concluída · **Única etapa que depende de e-mail**

## Objetivo

Adicionar confiança no endereço de e-mail, recuperação de senha e entrega
automática de convites sem tornar SMTP parte da transação principal ou um ponto
único de falha.

Esta etapa só começa depois da qualificação sem e-mail. Nenhuma etapa anterior
deve ser reaberta para “aguardar o provedor”.

## Entregas

1. **Abstração de entrega**
   - Interface interna de envio, independente do fornecedor.
   - Adaptador SMTP com TLS obrigatório em produção.
   - Mailpit somente em desenvolvimento e testes.
   - Não operar servidor SMTP próprio.
   - Templates versionados em texto e HTML, com URL pública validada.

2. **Outbox assíncrono**
   - Requisição grava intenção de e-mail na mesma transação do fluxo.
   - Worker envia após commit.
   - Retentativa exponencial com jitter, deduplicação e limite.
   - Estado terminal/dead letter observável e reprocessável por operação
     autorizada.
   - A tabela de tokens guarda somente hash. O token bruto necessário para
     montar o link existe temporariamente no payload criptografado do outbox,
     cuja chave fica no vault; ele é purgado após envio ou expiração.
   - Falha SMTP nunca desfaz criação de convite ou solicitação já confirmada.

3. **Verificação de endereço**
   - Pedido de cadastro novo fica em `cadastros_pendentes`, separado de
     `usuarios`, e não contém senha nem outra credencial utilizável.
   - O link comprova posse do endereço; só então a pessoa informa nome e senha,
     e a transação cria `usuarios` já verificado e sua primeira sessão.
   - Quem iniciou pedido para o e-mail de terceiro não escolhe a senha que será
     ativada pela vítima.
   - Respostas de cadastro/reenviar são genéricas para evitar enumeração.
   - Tokens usam 256 bits aleatórios, são de uso único e expiram em 24 horas
     para verificação.
   - Reenvio invalida ou limita tokens anteriores conforme regra documentada.

4. **Migração gradual das contas existentes**
   - Contas atuais permanecem funcionais, marcadas como não verificadas.
   - Não preencher `email_verificado_em` retroativamente sem prova.
   - Verificar conta existente exige, na mesma conclusão, sessão autenticada,
     senha atual e token recebido no e-mail. Token sozinho não transforma uma
     conta pré-cadastrada por terceiro em identidade confiável.
   - Fluxos novos que confiam no e-mail — recuperação, alteração sensível e
     entrega automática — exigem verificação.
   - Oferecer verificação progressiva sem bloquear o trabalho atual no quadro.

5. **Recuperação de senha**
   - Solicitação sempre responde de forma indistinguível.
   - Token de 256 bits, uso único, hash no banco e validade de 30 minutos.
   - Redefinição invalida o token e revoga todas as sessões da conta.
   - Senha nova respeita a política atual.
   - Limites por IP e por alvo pseudonimizado; `Retry-After` sem revelar
     existência.

6. **Convites automáticos**
   - Criação de convite continua devolvendo link manual.
   - Em paralelo, grava envio automático quando o endereço puder receber.
   - Indisponibilidade SMTP não impede o dono de copiar e compartilhar o link.
   - E-mail nunca contém informação além do necessário.

7. **Entregabilidade**
   - Configurar SPF, DKIM e DMARC no domínio.
   - Definir remetente, reply-to, tratamento de bounce e reputação.
   - Monitorar taxa de envio, falha, atraso, dead letter e reclamação sem
     colocar endereço completo em labels/logs.

## Contratos HTTP

```text
POST /auth/cadastro/solicitar
POST /auth/cadastro/concluir
POST /auth/email/verificacao/solicitar
POST /auth/email/verificar
POST /auth/senha/solicitar
POST /auth/senha/redefinir
```

- Solicitações usam respostas genéricas e tempos semelhantes.
- Links carregam o token no fragmento do navegador quando possível; o frontend
  o envia no corpo de verificação/redefinição, reduzindo vazamento em Caddy,
  histórico e header `Referer`.
- `cadastro/concluir` recebe token, nome e senha e é o único ponto que cria a
  conta/sessão de um novo endereço.
- `email/verificar` para conta preexistente recebe token e senha atual em sessão
  autenticada; não compartilha a semântica do cadastro pendente.
- Cookies e páginas que recebem token usam `Referrer-Policy: no-referrer` e
  `Cache-Control: no-store`.
- Alteração futura do próprio e-mail deve ser um fluxo separado, provando senha
  atual e o novo endereço; não entra automaticamente nesta entrega.

## Migrations e rollout

Migrations lógicas:

1. `usuarios.email_verificado_em TIMESTAMPTZ NULL`;
2. `cadastros_pendentes` com e-mail normalizado, expiração e timestamps, sem
   senha ou hash de senha;
3. `tokens_conta` com tipo, hash, usuário **ou** cadastro pendente, expiração,
   consumo e tentativas, com constraint de sujeito exclusivo;
4. `email_outbox` com template, payload criptografado, chave de deduplicação,
   estado, tentativas, próximo envio e timestamps.

Rollout:

1. expandir schema sem alterar login atual;
2. implantar uma release de compatibilidade que conhece o novo schema e pode
   desativar o cadastro legado, ainda sem enviar e-mail;
3. implantar frontend com o cadastro em duas etapas e worker desativado;
4. configurar SMTP/DNS, testar com Mailpit e enviar apenas contas internas;
5. desativar `POST /auth/cadastro` legado e ativar os novos cadastros; abas
   antigas recebem erro de atualização obrigatória, nunca criam conta sem
   verificação;
6. depois desse corte, rollback automático nunca volta a release anterior à de
   compatibilidade. Rollback excepcional para A11 bloqueia cadastro no Caddy
   até o forward fix;
7. oferecer verificação às contas existentes;
8. ativar recuperação e e-mail automático de convite;
9. observar entrega antes de remover qualquer compatibilidade transitória.

## Testes obrigatórios

- Mailpit valida destinatário, assunto, links e templates sem rede externa;
- cadastro não permite enumeração por corpo, status ou diferença grosseira de
  tempo;
- atacante inicia cadastro com e-mail da vítima, mas não define credencial; a
  vítima verificada escolhe a própria senha e o atacante não entra;
- registro em `cadastros_pendentes` não é autenticável por uma release A11;
- token expirado, usado, adulterado ou de outro tipo falha;
- duas verificações concorrentes têm um único consumo;
- em conta existente, token sem senha/sessão e senha/sessão sem token não
  verificam; o cenário de e-mail pré-cadastrado permanece não verificado;
- worker morre após SMTP aceitar e antes de confirmar: deduplicação evita efeito
  incorreto ou torna duplicata tolerável;
- falha SMTP entra em retentativa e depois dead letter sem rollback do domínio;
- redefinição revoga todas as sessões;
- limites de solicitação impedem abuso sem confirmar existência;
- contas antigas continuam entrando e trabalhando sem SMTP;
- rollback para a release de compatibilidade mantém login e bloqueia cadastro
  sem verificação; rollback excepcional A11 é testado com cadastro bloqueado;
- token bruto do outbox é purgado depois de envio ou expiração;
- suites A1–A11 passam com adaptador de e-mail desativado;
- configuração SPF/DKIM/DMARC é verificada antes da abertura ampla.

## Critérios de aceite

- [ ] Nenhuma resposta permite enumerar contas.
- [ ] Tokens têm 256 bits, uso único e validade definida; somente o hash fica
      em `tokens_conta`, e a cópia cifrada do outbox é temporária.
- [ ] Cadastro pendente não possui senha utilizável e não existe em `usuarios`.
- [ ] Falha ou duplicação de worker não corrompe estado do domínio.
- [ ] Redefinir senha revoga todas as sessões existentes.
- [ ] Contas antigas migram gradualmente e não são marcadas como verificadas
      sem prova.
- [ ] Convite manual continua disponível quando SMTP falha.
- [ ] Produção usa TLS, SPF, DKIM e DMARC; desenvolvimento usa Mailpit.
- [ ] Todas as etapas anteriores continuam independentes de SMTP.

---

# 5. Definition of Done transversal

Além dos critérios específicos, cada entrega de código precisa cumprir o que se
aplicar:

- teste unitário de domínio e casos de borda;
- teste de handler e contrato HTTP;
- integração real com PostgreSQL via Testcontainers;
- teste concorrente e `go test -race` para código compartilhado;
- Vitest para reducer/store/cliente;
- Playwright para fluxo crítico e realtime;
- migrations testadas em banco vazio e banco vindo da versão anterior;
- estratégia de expand/contract e rollback registrada;
- métricas e logs da nova falha; depois de A8, alerta quando a falha exigir
  ação operacional;
- limite de tempo, tamanho e concorrência documentado;
- runbook quando a operação puder exigir intervenção;
- documentação pública/técnica atualizada no mesmo contexto;
- `git diff --check`, linters, scanners e pipeline verdes;
- evidência do critério de aceite anexada à issue/release.

Teste não deve depender da ordem de execução, de SMTP real, do relógio local
sem controle ou de serviço externo não simulado, exceto nos ensaios
operacionais explicitamente marcados.

---

# 6. Política de migrations

- Este documento descreve mudanças lógicas e **não reserva números**.
- Usar sempre a próxima versão disponível no repositório.
- Migration aplicada nunca é editada.
- Mudança incompatível usa expandir → preencher → contrair em releases
  separadas.
- Preenchimento de dado antigo roda por comando do domínio, nunca dentro da
  migration; quando grande, é retomável, limitado em lotes e observável.
- Índice/constraint só entra após consulta de validação dos dados existentes.
- Toda migration informa lock esperado, duração medida, compatibilidade com a
  release anterior e caminho de recuperação.
- O histórico das migrations já consolidadas permanece em
  [backend/migrations/README.md](backend/migrations/README.md), não neste
  roadmap.

---

# 7. Itens explicitamente fora do backlog ativo

Os itens abaixo não são “pendências escondidas”. Foram retirados por decisão de
produto ou por não corresponderem ao perfil de produção atual:

- limite WIP;
- arquivamento de cards/colunas;
- CRDT;
- cursor colaborativo;
- indicador de edição de card;
- múltiplas instâncias/APIs;
- Redis, broker ou pub/sub distribuído;
- Kubernetes;
- object storage como armazenamento ativo de anexos;
- Swagger/OpenAPI mantido manualmente;
- SMTP auto-hospedado;
- novas funcionalidades de produto antes de encerrar os P0/P1.

Se algum deles voltar, precisa de problema mensurável, escopo próprio,
dependências e critérios de aceite; não deve ser recolocado informalmente em
uma etapa existente.

---

# 8. Ordem de encerramento

1. **Gate de segurança e consistência:** A1–A3.
2. **Gate operacional e recuperação:** A4–A8.
3. **Gate de experiência, capacidade e dados:** A9–A11.
4. **E-mail, por último:** A12.

O primeiro go-live controlado com dados importantes acontece somente depois de
A11. A12 amplia os fluxos de identidade e comunicação, mas não é pré-requisito
para que segurança, recuperação, observabilidade ou colaboração sejam
profissionais.

Ao concluir uma etapa, atualizar a tabela da seção 4, registrar a evidência e
mover aprendizados duráveis — não diários de implementação — para
[docs/historico-do-projeto.md](docs/historico-do-projeto.md).
