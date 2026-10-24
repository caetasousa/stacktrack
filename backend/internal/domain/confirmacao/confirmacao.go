// Package confirmacao modela a confirmação de email da conta: o link que o
// cadastro manda e que prova que a caixa de entrada é de quem criou a conta.
//
// É a prova que libera os convites no painel. Sem ela, criar uma conta com o
// email de outra pessoa bastaria para ver — e aceitar — os convites dela.
package confirmacao

import (
	"errors"
	"net/url"
	"time"

	"stacktrack/internal/pkg/token"
)

// Validade é quanto o link de confirmação vale.
//
// Sete dias, como o convite, e não a hora da recuperação de senha: o link de
// confirmação não troca senha nem dá acesso a nada que a conta já não tenha —
// ele só prova o email. E quem acabou de se cadastrar nem sempre abre o email
// na hora.
const Validade = 7 * 24 * time.Hour

// MaximoPorHora é quantos links de confirmação uma conta pode pedir numa hora.
// Pedir manda email; o teto impede que o botão de reenviar vire um jeito de
// encher uma caixa de entrada.
const MaximoPorHora = 3

// ErrInvalida é retornado para link inexistente, vencido ou já usado — os três
// com a mesma mensagem.
var ErrInvalida = errors.New("este link é inválido ou já venceu")

// Confirmacao é um link de confirmação de email.
type Confirmacao struct {
	ID        string
	UsuarioID string
	// TokenHash é o SHA-256 do token do link. O token em si só existe no email.
	TokenHash string
	CriadoEm  time.Time
	ExpiraEm  time.Time
	// UsadaEm é quando o link confirmou a conta. Nil enquanto vale.
	UsadaEm *time.Time
}

// Nova cria o link e devolve, junto, o token puro que vai no email.
func Nova(id, usuarioID string, agora time.Time) (*Confirmacao, string, error) {
	t, err := token.Gerar()
	if err != nil {
		return nil, "", err
	}
	return &Confirmacao{
		ID:        id,
		UsuarioID: usuarioID,
		TokenHash: token.Hash(t),
		CriadoEm:  agora,
		ExpiraEm:  agora.Add(Validade),
	}, t, nil
}

// Vale informa se o link ainda confirma: não foi usado e não venceu.
func (c Confirmacao) Vale(agora time.Time) bool {
	return c.UsadaEm == nil && agora.Before(c.ExpiraEm)
}

// Link monta o endereço que vai no email: a tela de confirmação do frontend.
func Link(origem, tokenPuro string) string {
	return origem + "/confirmar-email/" + url.PathEscape(tokenPuro)
}
