// Package recuperacao modela o "esqueci minha senha": o link que vai por email
// e troca a senha de uma conta.
//
// O link é credencial — quem o tem troca a senha, e com ela fica com a conta.
// As regras daqui existem para encolher esse poder: ele vale por pouco tempo,
// uma vez só, e o banco guarda apenas o hash do token.
package recuperacao

import (
	"errors"
	"net/url"
	"time"

	"stacktrack/internal/pkg/token"
)

// Validade é quanto um link de recuperação vale depois de pedido.
//
// Uma hora, e não os sete dias do convite: o convite espera alguém que talvez
// nem saiba que vai receber; quem pede a recuperação está na frente da tela,
// esperando o email. Um link que vale pouco é um link vazado que vale pouco.
const Validade = time.Hour

// MaximoPorHora é quantos links uma conta pode pedir numa hora.
//
// Pedir manda email, e quem pede não precisa estar logado — basta saber o email
// de alguém. Sem teto, digitar o endereço de outra pessoa em laço encheria a
// caixa de entrada dela com emails do stacktrack. Três cobrem o "não chegou,
// pedi de novo" sem virar ferramenta de incômodo.
const MaximoPorHora = 3

// ErrInvalida é retornado para link inexistente, vencido ou já usado — os três
// com a mesma mensagem, para não ajudar quem testa links.
var ErrInvalida = errors.New("este link é inválido ou já venceu")

// Recuperacao é um pedido de recuperação de senha.
type Recuperacao struct {
	ID        string
	UsuarioID string
	// TokenHash é o SHA-256 do token do link. O token em si só existe no email.
	TokenHash string
	CriadoEm  time.Time
	ExpiraEm  time.Time
	// EncerradaEm é quando o link deixou de valer antes do prazo: foi usado, ou
	// outro link da conta trocou a senha. Nil enquanto está em aberto.
	EncerradaEm *time.Time
}

// Nova cria o pedido e devolve, junto, o token puro — que vai no link do email
// e não é guardado em lugar nenhum.
func Nova(id, usuarioID string, agora time.Time) (*Recuperacao, string, error) {
	t, err := token.Gerar()
	if err != nil {
		return nil, "", err
	}
	return &Recuperacao{
		ID:        id,
		UsuarioID: usuarioID,
		TokenHash: token.Hash(t),
		CriadoEm:  agora,
		ExpiraEm:  agora.Add(Validade),
	}, t, nil
}

// Vale informa se o link ainda troca a senha: não foi encerrado e não venceu.
func (r Recuperacao) Vale(agora time.Time) bool {
	return r.EncerradaEm == nil && agora.Before(r.ExpiraEm)
}

// Link monta o endereço que vai no email: a tela de senha nova do frontend.
func Link(origem, tokenPuro string) string {
	return origem + "/redefinir-senha/" + url.PathEscape(tokenPuro)
}
