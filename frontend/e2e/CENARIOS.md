# Como o stacktrack pode quebrar

Este catálogo vem ANTES dos testes: cada item é uma maneira concreta de uma
funcionalidade deixar de funcionar para quem usa. Cada teste em `e2e/` cita no
título o id que cobre (`[E4] …`), e o relatório de cada execução
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

| id | Como quebra | O que o usuário vê |
|---|---|---|
| A1 | a conexão de tempo real não sobe, ou cai sozinha | o quadro para de se atualizar sem aviso |
| A2 | o card criado por uma pessoa não chega a quem está com o quadro aberto | a outra pessoa só vê o card depois de recarregar |
| A3 | duas abas da mesma conta não convergem | a aba antiga mostra um quadro velho |
| A4 | a coluna criada não chega aos outros | coluna que só aparece depois de recarregar |
| A5 | a queda da conexão passa despercebida, ou a conexão não volta | a pessoa edita achando que está ao vivo |
| A6 | a presença não aparece, ou fica presa depois que a pessoa sai | avatar de quem já foi embora |
| A7 | o comentário não chega a quem está com o mesmo card aberto | conversa descompassada |
| A8 | a segunda gravação sobrescreve a primeira em silêncio | o trabalho de alguém some |
| A9 | quem está editando uma coluna não aparece para os outros | duas pessoas editam a mesma coisa sem saber |
| A10 | a pessoa vê o próprio nome como se outra estivesse editando | aviso falso de edição simultânea |

## B. Auditoria

| id | Como quebra | O que o usuário vê |
|---|---|---|
| B1 | o card atribui a última movimentação a quem o criou | a auditoria aponta a pessoa errada |
| B2 | o histórico do quadro não diz quem mexeu em quê | ninguém sabe quem arrastou os cards |
| B3 | o filtro por pessoa mistura o que os outros fizeram | não dá para achar o que cada um fez |
| B4 | quem só lê o quadro não consegue abrir o histórico | leitor sem como acompanhar |
| B5 | o histórico ignora etiqueta, checklist, anexo, comentário ou responsável | buraco na auditoria |
| B6 | criar, renomear, mover ou apagar coluna e card fica fora do histórico | mudança sem rastro |
| B7 | renomear depois reescreve o que o histórico já registrou | a auditoria muda de resposta com o tempo |

## C. Link público

| id | Como quebra | O que o usuário vê |
|---|---|---|
| C1 | o link público pede conta, ou não abre | quem recebeu o link não vê nada |
| C2 | a página pública mostra comentário ou quem trabalha no card | dado interno exposto a qualquer um com o link |
| C3 | revogar não derruba o link, ou republicar ressuscita o antigo | link que deveria estar morto continua mostrando o quadro |
| C4 | ligar e desligar o compartilhamento pela tela não tem efeito | o dono acha que desligou e não desligou |

## D. Celular

| id | Como quebra | O que o usuário vê |
|---|---|---|
| D1 | um toque no card não abre o modal | no celular não dá para ver o card |
| D2 | rolar a lista com o dedo arrasta o card | cards mudam de lugar sem querer |
| D3 | o modal aberto por toque se fecha sozinho | impossível ler o card no celular |
| D4 | arrastar pela alça da coluna reordena em vez de rolar | colunas trocam de lugar sem querer |
| D5 | apagar um card não pede confirmação, ou cancelar apaga mesmo assim | perda de dado por um toque |

## E. Convite e compartilhamento

| id | Como quebra | O que o usuário vê |
|---|---|---|
| E1 | o dono convida pelo email e nada chega a quem foi convidado | a pessoa nunca fica sabendo do convite |
| E2 | o email chega, mas o link dele não abre o convite (origem errada, token cortado) | "convite inválido" para quem foi convidado |
| E3 | quem não tem conta cria a conta pelo convite e cai no painel sem o quadro | precisa pedir o convite de novo |
| E4 | quem já tem conta vai do cadastro para o login (ou entra direto) e perde o convite no caminho | entra e não acha o quadro |
| E5 | o quadro aceito não aparece no painel, ou aparece misturado com os próprios sem dizer de quem é | não acha o quadro do outro, ou acha que é dela |
| E6 | logado com outra conta, o convite é aceito mesmo assim, ou a tela não oferece trocar de conta | link encaminhado vale para qualquer um; ou a pessoa fica sem saída |
| E7 | revogar não derruba o link que já foi por email | quem não devia mais entrar entra |
| E8 | reenviar não manda email novo, ou o link anterior continua valendo | o convidado não recebe; ou o link vazado volta a valer |
| E9 | a tela do dono não diz se o email saiu, ou diz que saiu sem ter saído | o dono não sabe se precisa mandar o link por outro meio |
| E10 | a página do convite mostra o email inteiro de quem foi convidado | quem tem o link descobre o endereço de outra pessoa |
| E11 | convidar, reenviar e revogar somem do histórico do quadro | ninguém sabe quem chamou quem |
| E12 | convidar quem já participa, ou já tem convite em aberto, cria duplicata ou erro genérico | dois convites, ou "erro interno" sem explicação |
| E13 | a tela de membros descreve um caminho que não existe ("quem já tem conta entra na hora") | o dono não entende o que acontece depois de convidar |

