// Package session modela a sessão autenticada — o vínculo entre um token
// opaco no navegador e o usuário dono dele.
package session

import "time"

// Session representa uma sessão autenticada. Guarda apenas o hash do token —
// o token em texto puro nunca é persistido, então um vazamento do banco não
// entrega sessões utilizáveis.
type Session struct {
	TokenHash string
	UsuarioID string
	CriadoEm  time.Time
	ExpiraEm  time.Time
}

// Nova cria uma sessão com validade de ttl a partir do momento atual.
func Nova(tokenHash, usuarioID string, ttl time.Duration) *Session {
	agora := time.Now()
	return &Session{
		TokenHash: tokenHash,
		UsuarioID: usuarioID,
		CriadoEm:  agora,
		ExpiraEm:  agora.Add(ttl),
	}
}

// Expirada informa se a sessão já passou da validade em relação a agora.
//
// A expiração é checada aqui, no domínio, e não confiada ao Max-Age do cookie:
// o cookie vive no cliente e pode ser mantido além do prazo — só o servidor
// pode decidir que uma sessão acabou.
func (s *Session) Expirada(agora time.Time) bool {
	return agora.After(s.ExpiraEm)
}
