package auth

import (
	"context"
	"time"

	"stacktrack/internal/domain/confirmacao"
	demail "stacktrack/internal/domain/email"
	"stacktrack/internal/domain/recuperacao"
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
//
// Os campos depois dos dois primeiros servem à recuperação de senha e à
// confirmação de email. São portas à parte, e não métodos a mais em
// RepositorioUsuario e RepositorioSessao, porque só esses fluxos precisam
// deles — e só dentro da transação.
type EscritaDeAuth struct {
	Usuarios RepositorioUsuario
	Sessoes  RepositorioSessao

	Contas         ContaDaTransacao
	SessoesDaConta EncerradorDeSessoes
	Recuperacoes   RepositorioRecuperacao
	Confirmacoes   RepositorioConfirmacao
	// Emails é nil com o envio de email desligado.
	Emails FilaDeEmail
}

// ContaDaTransacao é o que se escreve numa conta fora do cadastro: a senha e a
// prova de que o email é dela.
type ContaDaTransacao interface {
	DefinirSenha(ctx context.Context, usuarioID, hash string, em time.Time) error
	// ConfirmarEmail marca a conta como dona do email. Não é erro confirmar de
	// novo: a primeira data é a que fica.
	ConfirmarEmail(ctx context.Context, usuarioID string, em time.Time) error
}

// RepositorioConfirmacao guarda os links de confirmação de email.
type RepositorioConfirmacao interface {
	Salvar(ctx context.Context, c *confirmacao.Confirmacao) error
	// BuscarPorTokenHash devolve (nil, nil) quando não encontra e, numa
	// transação, trava a linha.
	BuscarPorTokenHash(ctx context.Context, hash string) (*confirmacao.Confirmacao, error)
	ContarDoUsuarioDesde(ctx context.Context, usuarioID string, desde time.Time) (int, error)
	// EncerrarDoUsuario marca como usados todos os links ainda abertos da conta.
	EncerrarDoUsuario(ctx context.Context, usuarioID string, em time.Time) error
}

// EncerradorDeSessoes apaga todas as sessões de uma conta — o "sair de todos os
// aparelhos" que acompanha a troca de senha.
type EncerradorDeSessoes interface {
	RemoverDoUsuario(ctx context.Context, usuarioID string) error
}

// RepositorioRecuperacao guarda os pedidos de recuperação de senha.
type RepositorioRecuperacao interface {
	Salvar(ctx context.Context, r *recuperacao.Recuperacao) error
	// BuscarPorTokenHash devolve (nil, nil) quando não encontra. Dentro da
	// transação, trava a linha: duas redefinições com o mesmo link esperam uma
	// pela outra, e a segunda já a encontra encerrada.
	BuscarPorTokenHash(ctx context.Context, hash string) (*recuperacao.Recuperacao, error)
	// TravarConta segura a linha da conta até o fim da transação, para que dois
	// pedidos simultâneos não passem os dois pelo último lugar do teto.
	TravarConta(ctx context.Context, usuarioID string) error
	// ContarDoUsuarioDesde é a base do teto: quantos pedidos a conta fez.
	ContarDoUsuarioDesde(ctx context.Context, usuarioID string, desde time.Time) (int, error)
	// EncerrarDoUsuario encerra todos os links ainda abertos da conta.
	EncerrarDoUsuario(ctx context.Context, usuarioID string, em time.Time) error
}

// FilaDeEmail é o outbox de email, visto pela autenticação: só enfileirar. A
// mensagem entra em claro, e é o adaptador que a cifra.
type FilaDeEmail interface {
	Enfileirar(ctx context.Context, e *demail.Envio, m demail.Mensagem) error
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
