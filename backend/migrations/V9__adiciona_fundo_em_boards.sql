-- Fundo do quadro. Guarda o NOME do fundo escolhido (ex.: 'ardosia'), não uma
-- cor nem uma URL: a paleta é decisão de design e vive no frontend, então
-- gravar #1D2939 aqui congelaria no banco uma escolha que muda com o tema.
--
-- Anulável: quadro sem fundo escolhido usa o padrão, e o padrão é do domínio.
ALTER TABLE boards ADD COLUMN fundo VARCHAR(32);
