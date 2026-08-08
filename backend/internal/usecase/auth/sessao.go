package auth

import (
	"context"
	"stacktrack/internal/domain/session"
	"stacktrack/internal/domain/usuario"
	"stacktrack/internal/pkg/token"
)

// abrirSessao gera o token opaco, persiste a sessão com o hash dele e devolve
// o token puro para virar cookie. É o passo final do cadastro e do login — os
// dois abrem sessão exatamente do mesmo jeito, e ter isso num lugar só evita
// que um deles ganhe TTL ou tratamento diferente por descuido.
func abrirSessao(ctx context.Context, sessoes repositorioSessao, u *usuario.Usuario) (*SessaoAberta, error) {
	t, err := token.Gerar()
	if err != nil {
		return nil, err
	}

	s := session.Nova(token.Hash(t), u.ID, TTLSessao)
	if err := sessoes.Salvar(ctx, s); err != nil {
		return nil, err
	}

	// Faxina oportunista: sessão vencida não é lida por ninguém (a validação
	// checa a expiração), mas acumula linha para sempre. Aqui é barato e
	// dispensa um job agendado; o erro é ignorado de propósito, porque falhar
	// a limpeza não pode derrubar um login que já deu certo.
	sessoes.RemoverExpiradas(ctx)

	return &SessaoAberta{
		Token:     t,
		ExpiraEm:  s.ExpiraEm,
		UsuarioID: u.ID,
		Nome:      u.Nome,
		Email:     u.Email,
	}, nil
}
