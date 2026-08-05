package dto

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
