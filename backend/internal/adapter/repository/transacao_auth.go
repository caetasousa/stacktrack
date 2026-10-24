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
	// cifraDeEmail liga o outbox de email à transação — é por ele que sai o
	// link da recuperação de senha. Nil com o envio desligado.
	cifraDeEmail cifraDeEmail
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

	usuarios := &UsuarioPostgres{db: tx}
	sessoes := &SessionPostgres{db: tx}
	escrita := ucauth.EscritaDeAuth{
		Usuarios:       usuarios,
		Sessoes:        sessoes,
		Contas:         usuarios,
		SessoesDaConta: sessoes,
		Recuperacoes:   &RecuperacaoPostgres{db: tx},
		Confirmacoes:   &ConfirmacaoPostgres{db: tx},
	}
	// Atribuição condicional, e não um *EmailPostgres nil no campo: a interface
	// com ponteiro nil dentro NÃO é nil, e o caso de uso tentaria enfileirar
	// sem cifra.
	if u.cifraDeEmail != nil {
		escrita.Emails = &EmailPostgres{db: tx, cifra: u.cifraDeEmail}
	}
	if err := trabalho(escrita); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ComCifraDeEmail liga o outbox de email às transações de autenticação.
func (u *UnidadeDeAutenticacao) ComCifraDeEmail(cifra cifraDeEmail) *UnidadeDeAutenticacao {
	u.cifraDeEmail = cifra
	return u
}
