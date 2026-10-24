package dto

import "time"

// CadastroRequest são os dados do formulário de cadastro.
//
// A senha só é marcada como obrigatória aqui: o tamanho mínimo é regra de
// negócio e vive no domínio (usuario.ValidarSenha), para não existirem duas
// réguas que podem divergir.
type CadastroRequest struct {
	Nome  string       `json:"nome" validate:"required,max=120"`
	Email EmailEntrada `json:"email" validate:"required,email,max=255"`
	Senha string       `json:"senha" validate:"required"`
}

// Validar checa o formato dos campos informados.
func (r CadastroRequest) Validar() error {
	return validate.Struct(r)
}

// LoginRequest são as credenciais informadas no login.
type LoginRequest struct {
	Email EmailEntrada `json:"email" validate:"required,email"`
	Senha string       `json:"senha" validate:"required"`
}

// Validar checa o formato dos campos informados.
func (r LoginRequest) Validar() error {
	return validate.Struct(r)
}

// SessaoResponse é a conta autenticada. É o corpo devolvido pelo cadastro,
// pelo login e pelo /auth/me — o frontend guarda uma coisa só.
//
// Não devolve nada de sessão (token, validade): o token vive no cookie
// HttpOnly e o JavaScript não deve conseguir lê-lo, senão o XSS que roubaria a
// sessão volta a ser possível.
type SessaoResponse struct {
	ID    string `json:"id"`
	Nome  string `json:"nome"`
	Email string `json:"email"`
}

// PedidoDeRecuperacaoRequest é o "esqueci minha senha": só o email.
type PedidoDeRecuperacaoRequest struct {
	Email EmailEntrada `json:"email" validate:"required,email,max=255"`
}

// Validar checa o formato dos campos informados.
func (r PedidoDeRecuperacaoRequest) Validar() error {
	return validate.Struct(r)
}

// PedidoDeRecuperacaoResponse é a resposta do pedido — a MESMA para email com
// conta e sem conta, de propósito.
type PedidoDeRecuperacaoResponse struct {
	Mensagem string `json:"mensagem"`
}

// RecuperacaoResponse diz de qual conta é o link, sem o email inteiro, e até
// quando ele vale.
type RecuperacaoResponse struct {
	EmailMascarado string    `json:"emailMascarado"`
	ExpiraEm       time.Time `json:"expiraEm"`
}

// RedefinirSenhaRequest é a senha nova.
type RedefinirSenhaRequest struct {
	Senha string `json:"senha" validate:"required"`
}

// Validar checa o formato dos campos informados.
func (r RedefinirSenhaRequest) Validar() error {
	return validate.Struct(r)
}
