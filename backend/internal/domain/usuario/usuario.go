// Package usuario modela a identidade de quem loga no kanbanGo. Um Usuario não
// sabe nada de quadros, colunas ou cards: o que liga a pessoa a um quadro é o
// domínio membro, que chega na fase 3.
package usuario

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// Usuario representa uma conta do sistema.
type Usuario struct {
	ID           string
	Nome         string
	Email        string
	SenhaHash    string
	CriadoEm     time.Time
	AtualizadoEm time.Time
}

var (
	// ErrNomeObrigatorio é retornado quando o nome está vazio ou só com espaços.
	ErrNomeObrigatorio = errors.New("nome é obrigatório")
	// ErrEmailObrigatorio é retornado quando o email está vazio.
	ErrEmailObrigatorio = errors.New("email é obrigatório")
	// ErrEmailInvalido é retornado quando o email não tem a forma "algo@algo".
	ErrEmailInvalido = errors.New("email inválido")
	// ErrSenhaObrigatoria é retornado quando o hash de senha está vazio.
	ErrSenhaObrigatoria = errors.New("senha é obrigatória")
	// ErrSenhaCurta é retornado quando a senha em texto puro tem menos que TamanhoMinimoSenha.
	ErrSenhaCurta = errors.New("a senha precisa ter ao menos 8 caracteres")
	// ErrEmailEmUso é retornado pela persistência quando já existe conta com o
	// email informado. Vive aqui, e não no usecase, porque quem descobre a
	// colisão é o UNIQUE do banco: o adapter precisa de um erro do domínio para
	// traduzir a violação de constraint, e a checagem prévia do usecase é só
	// conveniência — entre consultar e gravar cabe outro cadastro igual.
	ErrEmailEmUso = errors.New("já existe uma conta com este email")
)

// TamanhoMinimoSenha é o piso de comprimento da senha, em caracteres.
//
// Só comprimento, e nenhuma exigência de "uma maiúscula e um símbolo": regras
// de composição empurram a pessoa para senhas curtas e previsíveis
// (Senha@123), enquanto o comprimento é o que de fato encarece o ataque. É a
// recomendação atual do NIST (SP 800-63B).
const TamanhoMinimoSenha = 8

// ValidarSenha checa a senha em TEXTO PURO antes de ela virar hash.
//
// Fica no domínio, e não no DTO, porque é regra de negócio e não formato de
// entrada: quem decide o que é uma senha aceitável é o domínio, e qualquer
// caminho novo que crie ou troque senha (recuperação, convite) precisa passar
// pela mesma régua sem ter que lembrar de copiar uma anotação de validação.
func ValidarSenha(senha string) error {
	if senha == "" {
		return ErrSenhaObrigatoria
	}
	if utf8.RuneCountInString(senha) < TamanhoMinimoSenha {
		return ErrSenhaCurta
	}
	return nil
}

// NormalizarEmail devolve o email sem espaços nas pontas e em minúsculas.
//
// É o que garante que "Ana@X.com " e "ana@x.com" sejam a mesma conta: sem
// normalizar antes de gravar, o UNIQUE do banco aceitaria as duas linhas e a
// pessoa teria duas contas sem entender por quê.
func NormalizarEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Novo cria um Usuario. Recebe o hash da senha já calculado — o domínio não
// conhece o algoritmo de hash usado. O email é normalizado aqui; validar a
// senha em texto puro é responsabilidade de quem chama (ver ValidarSenha).
// Retorna erro se o nome estiver vazio, se o email for inválido ou se o hash
// estiver vazio.
func Novo(id, nome, email, senhaHash string) (*Usuario, error) {
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return nil, ErrNomeObrigatorio
	}
	email = NormalizarEmail(email)
	if err := validarEmail(email); err != nil {
		return nil, err
	}
	if senhaHash == "" {
		return nil, ErrSenhaObrigatoria
	}

	agora := time.Now()
	return &Usuario{
		ID:           id,
		Nome:         nome,
		Email:        email,
		SenhaHash:    senhaHash,
		CriadoEm:     agora,
		AtualizadoEm: agora,
	}, nil
}

// DefinirSenha troca o hash da senha. Recebe o hash já calculado.
func (u *Usuario) DefinirSenha(senhaHash string) error {
	if senhaHash == "" {
		return ErrSenhaObrigatoria
	}
	u.SenhaHash = senhaHash
	u.AtualizadoEm = time.Now()
	return nil
}

// validarEmail faz uma checagem estrutural mínima: exige exatamente um "@" com
// conteúdo dos dois lados. Não tenta implementar a RFC 5322 nem verificar se o
// endereço existe — a única prova de que um email é real é mandar mensagem
// para ele, e é isso que a confirmação por email fará quando entrar.
func validarEmail(email string) error {
	if email == "" {
		return ErrEmailObrigatorio
	}
	partes := strings.Split(email, "@")
	if len(partes) != 2 || partes[0] == "" || partes[1] == "" {
		return ErrEmailInvalido
	}
	if !strings.Contains(partes[1], ".") {
		return ErrEmailInvalido
	}
	return nil
}
