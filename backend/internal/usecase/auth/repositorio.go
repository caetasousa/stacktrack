package auth

import (
	"context"
	"stacktrack/internal/domain/session"
	"stacktrack/internal/domain/usuario"
)

// As portas ficam neste pacote, e não junto dos adapters, porque quem define o
// que precisa é o usecase — o adapter é que se molda a elas. São minúsculas
// (não exportadas) de propósito: ninguém de fora precisa implementá-las senão
// os adapters, que satisfazem a interface estruturalmente.

// BuscadorUsuario lê usuários. Devolve (nil, nil) quando não encontra: "não
// existe" não é falha, e distinguir isso de erro real evita que o login trate
// um banco fora do ar como credencial errada.
type BuscadorUsuario interface {
	BuscarPorID(ctx context.Context, id string) (*usuario.Usuario, error)
	BuscarPorEmail(ctx context.Context, email string) (*usuario.Usuario, error)
}

// RepositorioUsuario lê e grava usuários. Salvar devolve usuario.ErrEmailEmUso
// quando a unicidade do email é violada.
type RepositorioUsuario interface {
	BuscadorUsuario
	Salvar(ctx context.Context, u *usuario.Usuario) error
}

// RepositorioSessao guarda as sessões abertas.
type RepositorioSessao interface {
	Salvar(ctx context.Context, s *session.Session) error
	BuscarPorTokenHash(ctx context.Context, hash string) (*session.Session, error)
	Remover(ctx context.Context, hash string) error
	RemoverExpiradas(ctx context.Context) error
}

// HasherSenha transforma senha em hash e confere o par senha/hash. O domínio
// não conhece o algoritmo; quem escolhe é o adapter (ver adapter/security).
type HasherSenha interface {
	Gerar(senha string) (string, error)
	Verificar(senha, hash string) (bool, error)
}

// EscritaDeAuth são os repositórios de autenticação ligados a uma transação em
// curso.
type EscritaDeAuth struct {
	Usuarios RepositorioUsuario
	Sessoes  RepositorioSessao
}

// UnidadeDeAutenticacao executa um trabalho com conta e sessão na MESMA
// transação.
//
// O que ela conserta: o cadastro gravava a conta numa transação e a sessão
// noutra, logo depois. Uma falha no meio deixava conta criada SEM sessão — a
// pessoa terminava o formulário, via um erro, tentava de novo e recebia "já
// existe uma conta com este email", sem nunca ter conseguido entrar e sem
// nenhuma recuperação de senha para oferecer. Era um caminho que produzia
// contas inacessíveis.
//
// Nada aqui sabe o que é uma transação de Postgres — só que existe um jeito de
// as duas escritas caírem ou valerem juntas. Quem a implementa é
// adapter/repository.UnidadeDeAutenticacao.
type UnidadeDeAutenticacao interface {
	Executar(ctx context.Context, trabalho func(EscritaDeAuth) error) error
}
