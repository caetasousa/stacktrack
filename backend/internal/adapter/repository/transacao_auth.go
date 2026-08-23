package repository

import (
	"context"

	ucauth "stacktrack/internal/usecase/auth"
)

// UnidadeDeAutenticacao grava conta e sessão na MESMA transação.
//
// É uma unidade separada da de quadro, e não a mesma com mais campos, porque as
// duas protegem invariantes diferentes e têm custos diferentes. A de quadro
// trava a linha do quadro — serializa as escritas de um agregado; esta não
// trava nada, porque a única garantia que ela precisa dar é "as duas linhas
// entram juntas ou nenhuma entra". Um lock aqui seria custo sem invariante que
// o justifique.
type UnidadeDeAutenticacao struct {
	pool Fonte
}

// NovaUnidadeDeAutenticacao cria a unidade sobre o pool informado.
func NovaUnidadeDeAutenticacao(pool Fonte) *UnidadeDeAutenticacao {
	return &UnidadeDeAutenticacao{pool: pool}
}

// Executar abre a transação, entrega os repositórios ligados a ela e comita.
func (u *UnidadeDeAutenticacao) Executar(ctx context.Context, trabalho func(ucauth.EscritaDeAuth) error) error {
	tx, err := u.pool.Begin(ctx)
	if err != nil {
		return err
	}
	// Rollback depois do commit não tem efeito, então o defer cobre só os
	// caminhos de erro — inclusive o panic.
	defer tx.Rollback(ctx) //nolint:errcheck // sem efeito depois do commit

	if err := trabalho(ucauth.EscritaDeAuth{
		Usuarios: &UsuarioPostgres{db: tx},
		Sessoes:  &SessionPostgres{db: tx},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
