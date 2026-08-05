// Package auth contém os usecases de autenticação: cadastro, login, logout,
// validação de sessão e consulta do perfil autenticado.
//
// Não há confirmação de email nem recuperação de senha: as duas dependem de
// enviar mensagem, e o envio ainda não existe no projeto. O cadastro desta
// fase cria a conta já utilizável.
package auth

import (
	"errors"
	"time"
)

// ErrCredenciaisInvalidas é retornado tanto para email inexistente quanto para
// senha incorreta — resposta genérica de propósito, para não revelar quais
// emails estão cadastrados.
var ErrCredenciaisInvalidas = errors.New("credenciais inválidas")

// ErrSessaoInvalida é retornado quando o token não corresponde a nenhuma
// sessão ativa ou a sessão já expirou.
var ErrSessaoInvalida = errors.New("sessão inválida")

// TTLSessao é a validade fixa de uma sessão a partir do login.
const TTLSessao = 7 * 24 * time.Hour

// CadastroInput contém os dados informados no cadastro.
type CadastroInput struct {
	Nome  string
	Email string
	Senha string
}

// LoginInput contém as credenciais informadas no login.
type LoginInput struct {
	Email string
	Senha string
}

// SessaoAberta é o resultado de um cadastro ou login bem-sucedido. Token é o
// token de sessão em texto puro — vai apenas para o cookie de resposta, nunca
// é persistido.
type SessaoAberta struct {
	Token     string
	ExpiraEm  time.Time
	UsuarioID string
	Nome      string
	Email     string
}

// Identidade representa o usuário autenticado em uma requisição.
type Identidade struct {
	UsuarioID string
}
