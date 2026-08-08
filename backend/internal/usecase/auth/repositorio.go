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

// buscadorUsuario lê usuários. Devolve (nil, nil) quando não encontra: "não
// existe" não é falha, e distinguir isso de erro real evita que o login trate
// um banco fora do ar como credencial errada.
type buscadorUsuario interface {
	BuscarPorID(ctx context.Context, id string) (*usuario.Usuario, error)
	BuscarPorEmail(ctx context.Context, email string) (*usuario.Usuario, error)
}

// repositorioUsuario lê e grava usuários. Salvar devolve usuario.ErrEmailEmUso
// quando a unicidade do email é violada.
type repositorioUsuario interface {
	buscadorUsuario
	Salvar(ctx context.Context, u *usuario.Usuario) error
}

// repositorioSessao guarda as sessões abertas.
type repositorioSessao interface {
	Salvar(ctx context.Context, s *session.Session) error
	BuscarPorTokenHash(ctx context.Context, hash string) (*session.Session, error)
	Remover(ctx context.Context, hash string) error
	RemoverExpiradas(ctx context.Context) error
}

// hasherSenha transforma senha em hash e confere o par senha/hash. O domínio
// não conhece o algoritmo; quem escolhe é o adapter (ver adapter/security).
type hasherSenha interface {
	Gerar(senha string) (string, error)
	Verificar(senha, hash string) (bool, error)
}
