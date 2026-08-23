package auth

import (
	"context"
	"stacktrack/internal/domain/session"
	"stacktrack/internal/domain/usuario"
	"stacktrack/internal/pkg/token"
)

// novaSessao gera o token opaco e monta a sessão, SEM persistir.
//
// Separado de quem grava porque o cadastro grava dentro de uma transação (junto
// da conta) e o login grava sozinho. O token puro volta junto porque ele existe
// só aqui: o banco guarda o hash, e quem não o levar agora não o obtém depois.
func novaSessao(u *usuario.Usuario) (*session.Session, string, error) {
	t, err := token.Gerar()
	if err != nil {
		return nil, "", err
	}
	return session.Nova(token.Hash(t), u.ID, TTLSessao), t, nil
}

// sessaoAberta monta o resultado devolvido a quem chamou.
func sessaoAberta(s *session.Session, tokenPuro string, u *usuario.Usuario) *SessaoAberta {
	return &SessaoAberta{
		Token:     tokenPuro,
		ExpiraEm:  s.ExpiraEm,
		UsuarioID: u.ID,
		Nome:      u.Nome,
		Email:     u.Email,
	}
}

// abrirSessao gera o token, persiste a sessão e devolve o resultado. É o passo
// final do LOGIN, que não tem outra escrita a acompanhar.
//
// O cadastro NÃO passa por aqui: ele precisa gravar conta e sessão no mesmo
// commit, e por isso monta a sessão com novaSessao e grava dentro da unidade de
// trabalho.
func abrirSessao(ctx context.Context, sessoes RepositorioSessao, u *usuario.Usuario) (*SessaoAberta, error) {
	s, t, err := novaSessao(u)
	if err != nil {
		return nil, err
	}
	if err := sessoes.Salvar(ctx, s); err != nil {
		return nil, err
	}

	// A FAXINA SAIU DAQUI.
	//
	// Havia um `sessoes.RemoverExpiradas(ctx)` neste ponto — faxina oportunista,
	// barata quando a tabela é pequena. Com a tabela grande ela deixa de ser
	// barata e passa a ser um DELETE varrendo `sessions` inteira no caminho
	// crítico do login: cada pessoa que entra paga a limpeza de todo mundo, e a
	// conta cresce justamente quando há mais gente usando. Pior, ela roda
	// exatamente durante a rajada de logins de uma manhã de segunda-feira, que
	// é quando ninguém pode esperar.
	//
	// Agora quem limpa é um job horário, em lotes, fora de qualquer requisição
	// — ver internal/usecase/manutencao.
	return sessaoAberta(s, t, u), nil
}
