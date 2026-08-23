package repository

import (
	"context"

	ucboard "stacktrack/internal/usecase/board"
)

// DuplicidadePostgres encontra chaves de ordenação repetidas.
type DuplicidadePostgres struct {
	db consultante
}

// NovoDuplicidadePostgres cria o leitor sobre o pool informado.
func NovoDuplicidadePostgres(pool Fonte) *DuplicidadePostgres {
	return &DuplicidadePostgres{db: pool}
}

// DuplicidadesDeChave devolve os contêineres com chave repetida.
//
// É a MESMA consulta que backend/migrations/README.md documenta como
// pré-condição do contract, com uma diferença que importa: ela devolve também o
// `board_id` do caso dos cards. Sem ele o comando de reparo não teria como saber
// que linha travar — o lock é da linha do quadro, e a duplicidade de cards é
// descoberta pela coluna.
//
// O `COLLATE "C"` aparece no SELECT e no GROUP BY para ser exatamente a mesma
// expressão que o índice do contract vai indexar. Com as chaves que o domínio
// produz — só `[a-z]`, ver ordem.chaveValida — nenhuma collation as agruparia
// diferente; escrever a mesma expressão dos dois lados é o que mantém a
// equivalência garantida por construção em vez de por hipótese sobre o dado.
//
// (Ela precisa aparecer no SELECT porque o PostgreSQL exige que a coluna
// projetada seja a mesma expressão agrupada: `SELECT c.chave … GROUP BY c.chave
// COLLATE "C"` é recusado com 42803.)
func (r *DuplicidadePostgres) DuplicidadesDeChave(ctx context.Context) ([]ucboard.Duplicidade, error) {
	linhas, err := r.db.Query(ctx,
		`SELECT col.board_id, c.coluna_id, c.chave COLLATE "C", count(*)
		   FROM cards c
		   JOIN colunas col ON col.id = c.coluna_id
		  GROUP BY col.board_id, c.coluna_id, c.chave COLLATE "C"
		 HAVING count(*) > 1
		 UNION ALL
		 SELECT l.board_id, NULL, l.chave COLLATE "C", count(*)
		   FROM colunas l
		  GROUP BY l.board_id, l.chave COLLATE "C"
		 HAVING count(*) > 1`,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	duplicidades := make([]ucboard.Duplicidade, 0)
	for linhas.Next() {
		var d ucboard.Duplicidade
		var colunaID *string
		if err := linhas.Scan(&d.BoardID, &colunaID, &d.Chave, &d.Ocorrencias); err != nil {
			return nil, err
		}
		// NULL é o ramo das COLUNAS: ali o contêiner é o próprio quadro.
		d.ColunaID = valorOuVazio(colunaID)
		duplicidades = append(duplicidades, d)
	}
	return duplicidades, linhas.Err()
}
