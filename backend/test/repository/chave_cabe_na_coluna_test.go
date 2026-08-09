//go:build integracao

// O teto da chave de ordenação mora em DOIS lugares: a constante do domínio e o
// VARCHAR da migration. Se eles divergirem, o domínio gera uma chave que o
// banco recusa — e o que era um 409 previsto vira um 500 vindo do driver, num
// caminho que só aparece depois de centenas de movimentos no mesmo ponto, ou
// seja, nunca em teste manual.
//
// Este teste pergunta ao banco de verdade qual é o tamanho da coluna.
package repository_test

import (
	"context"
	"testing"

	"stacktrack/internal/adapter/repository"
	"stacktrack/internal/domain/card"
	"stacktrack/internal/domain/ordem"

	"github.com/google/uuid"
)

func TestOTetoDaChaveNoDominioEhOTamanhoDaColuna(t *testing.T) {
	ctx := context.Background()

	for _, tabela := range []string{"cards", "colunas"} {
		var limiteDaColuna int
		err := pool.QueryRow(ctx,
			`SELECT character_maximum_length
			   FROM information_schema.columns
			  WHERE table_name = $1 AND column_name = 'chave'`, tabela).Scan(&limiteDaColuna)
		if err != nil {
			t.Fatalf("%s: consultar o schema: %v", tabela, err)
		}
		if limiteDaColuna != ordem.TamanhoMaximo {
			t.Errorf("%s.chave é VARCHAR(%d), mas ordem.TamanhoMaximo é %d — "+
				"o domínio geraria chave que o banco recusa",
				tabela, limiteDaColuna, ordem.TamanhoMaximo)
		}
	}
}

// E a chave no teto CABE mesmo — a constante estar certa não prova que o
// caminho de escrita aceita o valor extremo.
func TestChaveNoTetoEhAceitaPeloBanco(t *testing.T) {
	ctx := context.Background()
	_, colunaID, _ := cenario(t)

	c, err := card.Novo(uuid.NewString(), colunaID, "No teto", "", "", ordem.ChaveInicial)
	if err != nil {
		t.Fatalf("montar: %v", err)
	}
	if err := repository.NovoCardPostgres(pool).Salvar(ctx, c); err != nil {
		t.Fatalf("salvar: %v", err)
	}

	// Uma chave do tamanho máximo, respeitando a invariante (não termina no
	// menor caractere).
	limite := make([]byte, ordem.TamanhoMaximo)
	for i := range limite {
		limite[i] = 'n'
	}
	chave := string(limite)

	if _, err := pool.Exec(ctx, `UPDATE cards SET chave = $1 WHERE id = $2`, chave, c.ID); err != nil {
		t.Fatalf("o banco recusou uma chave de %d caracteres: %v", len(chave), err)
	}

	// E um caractere a mais é recusado — é isso que torna o teto real, e não
	// uma constante decorativa.
	if _, err := pool.Exec(ctx, `UPDATE cards SET chave = $1 WHERE id = $2`, chave+"n", c.ID); err == nil {
		t.Errorf("o banco aceitou %d caracteres — a coluna não é o teto que o domínio supõe",
			len(chave)+1)
	}
}
