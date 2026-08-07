-- Cor da coluna e do card.
--
-- Guarda o NOME da cor ('verde'), como já fazem etiquetas e o fundo do quadro:
-- a paleta muda com o tema claro/escuro, e o hex pertence ao CSS.
--
-- Anuláveis por dois motivos: a cor é opcional — item sem cor usa o visual
-- padrão — e coluna nova obrigatória quebraria a versão anterior da aplicação,
-- que continua no ar durante o deploy.
ALTER TABLE colunas ADD COLUMN cor VARCHAR(20);
ALTER TABLE cards   ADD COLUMN cor VARCHAR(20);
