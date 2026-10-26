# Como o stacktrack pode quebrar

Este catálogo vem ANTES dos testes: cada item é uma maneira concreta de uma
funcionalidade deixar de funcionar para quem usa. Cada teste em `e2e/` cita no
título o id que cobre (`[E4] …`), ou os ids, quando o mesmo percurso prova mais
de um (`[F2][F3] …`), e o relatório de cada execução
(`e2e/relatorio/resumo.md`) lista o resultado por id — um item sem teste aparece
como lacuna declarada, não como esquecimento.

Os testes rodam contra a stack de desenvolvimento inteira (`make run`):
Postgres, API, frontend e Mailpit, que faz o papel do provedor de email. Nenhum
email sai para a internet: o Mailpit guarda as mensagens, e os testes as leem
pela API dele.

O que não passa pela tela — retentativa do envio, cifra do link no banco, a
corrida entre duas abas — está nos testes de integração do backend, com os
cenários escritos no comentário de cada um.

## A. Tempo real

| id  | Como quebra                                                            | O que o usuário vê                               |
| --- | ---------------------------------------------------------------------- | ------------------------------------------------ |
| A1  | a conexão de tempo real não sobe, ou cai sozinha                       | o quadro para de se atualizar sem aviso          |
| A2  | o card criado por uma pessoa não chega a quem está com o quadro aberto | a outra pessoa só vê o card depois de recarregar |
| A3  | duas abas da mesma conta não convergem                                 | a aba antiga mostra um quadro velho              |
| A4  | a coluna criada não chega aos outros                                   | coluna que só aparece depois de recarregar       |
| A5  | a queda da conexão passa despercebida, ou a conexão não volta          | a pessoa edita achando que está ao vivo          |
| A6  | a presença não aparece, ou fica presa depois que a pessoa sai          | avatar de quem já foi embora                     |
| A7  | o comentário não chega a quem está com o mesmo card aberto             | conversa descompassada                           |
| A8  | a segunda gravação sobrescreve a primeira em silêncio                  | o trabalho de alguém some                        |
| A9  | quem está editando uma coluna não aparece para os outros               | duas pessoas editam a mesma coisa sem saber      |
| A10 | a pessoa vê o próprio nome como se outra estivesse editando            | aviso falso de edição simultânea                 |

## B. Auditoria

| id  | Como quebra                                                              | O que o usuário vê                       |
| --- | ------------------------------------------------------------------------ | ---------------------------------------- |
| B1  | o card atribui a última movimentação a quem o criou                      | a auditoria aponta a pessoa errada       |
| B2  | o histórico do quadro não diz quem mexeu em quê                          | ninguém sabe quem arrastou os cards      |
| B3  | o filtro por pessoa mistura o que os outros fizeram                      | não dá para achar o que cada um fez      |
| B4  | quem só lê o quadro não consegue abrir o histórico                       | leitor sem como acompanhar               |
| B5  | o histórico ignora etiqueta, checklist, anexo, comentário ou responsável | buraco na auditoria                      |
| B6  | criar, renomear, mover ou apagar coluna e card fica fora do histórico    | mudança sem rastro                       |
| B7  | renomear depois reescreve o que o histórico já registrou                 | a auditoria muda de resposta com o tempo |

## C. Link público

| id  | Como quebra                                                   | O que o usuário vê                                       |
| --- | ------------------------------------------------------------- | -------------------------------------------------------- |
| C1  | o link público pede conta, ou não abre                        | quem recebeu o link não vê nada                          |
| C2  | a página pública mostra comentário ou quem trabalha no card   | dado interno exposto a qualquer um com o link            |
| C3  | revogar não derruba o link, ou republicar ressuscita o antigo | link que deveria estar morto continua mostrando o quadro |
| C4  | ligar e desligar o compartilhamento pela tela não tem efeito  | o dono acha que desligou e não desligou                  |

## D. Celular

| id  | Como quebra                                                        | O que o usuário vê                 |
| --- | ------------------------------------------------------------------ | ---------------------------------- |
| D1  | um toque no card não abre o modal                                  | no celular não dá para ver o card  |
| D2  | rolar a lista com o dedo arrasta o card                            | cards mudam de lugar sem querer    |
| D3  | o modal aberto por toque se fecha sozinho                          | impossível ler o card no celular   |
| D4  | arrastar pela alça da coluna reordena em vez de rolar              | colunas trocam de lugar sem querer |
| D5  | apagar um card não pede confirmação, ou cancelar apaga mesmo assim | perda de dado por um toque         |

## E. Convite e compartilhamento

| id  | Como quebra                                                                                      | O que o usuário vê                                                 |
| --- | ------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------ |
| E1  | o dono convida pelo email e nada chega a quem foi convidado                                      | a pessoa nunca fica sabendo do convite                             |
| E2  | o email chega, mas o link dele não abre o convite (origem errada, token cortado)                 | "convite inválido" para quem foi convidado                         |
| E3  | quem não tem conta cria a conta pelo convite e cai no painel sem o quadro                        | precisa pedir o convite de novo                                    |
| E4  | quem já tem conta vai do cadastro para o login (ou entra direto) e perde o convite no caminho    | entra e não acha o quadro                                          |
| E5  | o quadro aceito não aparece no painel, ou aparece misturado com os próprios sem dizer de quem é  | não acha o quadro do outro, ou acha que é dela                     |
| E6  | logado com outra conta, o convite é aceito mesmo assim, ou a tela não oferece trocar de conta    | link encaminhado vale para qualquer um; ou a pessoa fica sem saída |
| E7  | revogar não derruba o link que já foi por email                                                  | quem não devia mais entrar entra                                   |
| E8  | reenviar não manda email novo, ou o link anterior continua valendo                               | o convidado não recebe; ou o link vazado volta a valer             |
| E9  | a tela do dono não diz se o email saiu, ou diz que saiu sem ter saído                            | o dono não sabe se precisa mandar o link por outro meio            |
| E10 | a página do convite mostra o email inteiro de quem foi convidado                                 | quem tem o link descobre o endereço de outra pessoa                |
| E11 | convidar, reenviar e revogar somem do histórico do quadro                                        | ninguém sabe quem chamou quem                                      |
| E12 | convidar quem já participa, ou já tem convite em aberto, cria duplicata ou erro genérico         | dois convites, ou "erro interno" sem explicação                    |
| E13 | a tela de membros descreve um caminho que não existe ("quem já tem conta entra na hora")         | o dono não entende o que acontece depois de convidar               |
| E14 | a página inicial promete o convite antigo ("quem já tem conta entra direto", "convite por link") | quem vai chamar a equipe espera um atalho que o produto não tem    |
| E15 | a lista de convites fica em "email na fila" depois que o email já saiu                           | o dono acha que o email nunca foi enviado                          |

## G. Confirmação de email e convites na conta

| id  | Como quebra                                                                      | O que o usuário vê                                                      |
| --- | -------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| G1  | o cadastro não manda o email de confirmação, ou o link dele não confirma a conta | a pessoa nunca consegue ver os convites no painel                       |
| G2  | uma conta que não confirmou o email vê no painel os convites para aquele email   | quem cadastrou o email de outra pessoa entra no quadro dela sem o link  |
| G3  | a conta confirmada não vê no painel o convite que chegou para o seu email        | o convite só existe no email, e some quando o email se perde            |
| G4  | aceitar pelo painel não põe a pessoa no quadro                                   | o botão não faz nada, ou faz pela metade                                |
| G5  | recusar não tira o convite do painel, ou o link do email continua valendo depois | o convite recusado volta a aparecer, ou alguém entra com ele            |
| G6  | o convite revogado continua aparecendo no painel de quem foi convidado           | um botão "Aceitar" que não leva a lugar nenhum                          |
| G7  | quem convidou não fica sabendo da recusa                                         | o dono espera para sempre por quem já disse não                         |
| G8  | a conta sem confirmar perde o caminho pelo link do email                         | quem acabou de criar a conta pelo convite não consegue entrar no quadro |

O que não passa pela tela — o token da confirmação guardado só como hash, o
teto de reenvios, o convite de outro email aceito pelo id, o convite vencido no
painel, aceitar e recusar ao mesmo tempo — está em
`backend/test/repository/confirmacao_test.go`.

## N. Notificações

O sino do cabeçalho junta o que pede a atenção da pessoa: convite para um
quadro, card que passou a ser dela, menção num comentário e prazo chegando. A
notificação é gravada no banco, e o tempo real (SSE) só avisa que algo mudou;
quem não estava olhando encontra tudo ao voltar.

| id  | Como quebra                                                                                           | O que o usuário vê                                                         |
| --- | ----------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| N1  | a pessoa é posta como responsável e o sino dela não muda sem recarregar                              | só descobre o card novo quando dá F5, ou nunca                             |
| N2  | o email da atribuição não sai, ou o link dele não abre o card                                         | quem estava fora não fica sabendo, ou chega a um quadro sem saber qual card |
| N3  | desfazer a atribuição deixa a notificação contando, e o link do email abre como se ainda valesse      | o sino cobra um trabalho que já não é da pessoa                            |
| N4  | a menção num comentário não chega a quem foi mencionado, ou o campo não ajuda a mencionar quem participa | a pessoa chamada nunca lê o recado                                         |
| N5  | o convite para uma conta confirmada não aparece no sino, ou o clique não leva aos convites            | o convite fica escondido no email                                          |
| N6  | o convite recusado ou revogado continua contando no sino                                             | um aviso que não leva a nada                                               |
| N7  | ler numa aba não apaga o aviso nas outras abas da mesma conta                                         | a outra aba insiste num número que já foi lido                             |
| N8  | o prazo que vence amanhã não avisa quem responde pelo card                                            | o prazo estoura sem ninguém ser lembrado                                   |
| N9  | a retenção das notificações não é gravada, ou qualquer membro consegue trocá-la                        | o dono escolhe 7 dias e elas ficam para sempre; ou um editor decide por ele |
| N10 | o link de notificação de quem não está logado se perde no login                                       | entra e cai no painel, sem saber o que o email queria mostrar              |

O que não passa pela tela — a notificação e o email no mesmo commit, o email
que espera e é cancelado quando a pessoa lê antes, o teto diário de emails, a
retenção de cada quadro aplicada pela faxina, quem saiu do quadro deixando de ver
as notificações dele — está em `backend/test/repository/notificacao_test.go`. O
protocolo do SSE (cabeçalhos, batimento, sessão encerrada, limites) está em
`backend/test/realtime/sse_test.go`.

## I. Uso institucional

| id  | Como quebra                                                                        | O que o usuário vê                                         |
| --- | ---------------------------------------------------------------------------------- | ---------------------------------------------------------- |
| I1  | os Termos de Uso e a Política de Privacidade não existem, ou não há como chegar a eles | quem vai criar conta não sabe com o que está concordando  |
| I2  | a página inicial vende o projeto como de estudo ou fala de engenharia               | "código aberto", "Argon2id": o produto parece um protótipo |
| I3  | um endereço errado cai na página de erro crua do framework                          | um número e uma frase em inglês, sem caminho de volta      |

## F. Recuperar a senha

| id  | Como quebra                                                                                     | O que o usuário vê                                                           |
| --- | ----------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------- |
| F1  | a tela de login não leva a lugar nenhum para quem esqueceu a senha, ou o pedido não manda email | a pessoa fica sem como entrar na própria conta                               |
| F2  | o link do email não abre a tela de nova senha (origem errada, token cortado)                    | "link inválido" para quem acabou de pedir                                    |
| F3  | a senha nova não passa a valer, ou a antiga continua entrando                                   | a pessoa troca a senha e não entra; ou quem tinha a antiga continua entrando |
| F4  | o mesmo link redefine a senha duas vezes                                                        | um link vazado troca a senha de novo depois da dona                          |
| F5  | pedir com um email sem conta responde diferente de pedir com um email que tem conta             | dá para descobrir quem tem conta no stacktrack                               |
| F6  | redefinir não derruba as sessões abertas em outros aparelhos                                    | quem entrou com a senha antiga continua dentro                               |
| F7  | o link inválido ou vencido deixa a pessoa sem saída                                             | tela de erro sem caminho para pedir outro link                               |
| F8  | a senha nova aceita o que o cadastro recusaria (curta, comum)                                   | a recuperação vira o atalho para uma senha fraca                             |

O que não passa pela tela — o token guardado só como hash, a validade de uma
hora, o teto de pedidos por conta, duas redefinições simultâneas com o mesmo
link — está em `backend/test/repository/recuperacao_test.go`.
