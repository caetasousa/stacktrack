package auth

import (
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
	BuscarPorID(id string) (*usuario.Usuario, error)
	BuscarPorEmail(email string) (*usuario.Usuario, error)
}

// repositorioUsuario lê e grava usuários. Salvar devolve usuario.ErrEmailEmUso
// quando a unicidade do email é violada.
type repositorioUsuario interface {
	buscadorUsuario
	Salvar(u *usuario.Usuario) error
}

// repositorioSessao guarda as sessões abertas.
type repositorioSessao interface {
	Salvar(s *session.Session) error
	BuscarPorTokenHash(hash string) (*session.Session, error)
	Remover(hash string) error
	RemoverExpiradas() error
}

// hasherSenha transforma senha em hash e confere o par senha/hash. O domínio
// não conhece o algoritmo; quem escolhe é o adapter (ver adapter/security).
type hasherSenha interface {
	Gerar(senha string) (string, error)
	Verificar(senha, hash string) (bool, error)
}
