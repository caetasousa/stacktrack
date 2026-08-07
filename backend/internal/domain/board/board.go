// Package board modela o quadro Kanban. O quadro é só o continente: as etapas
// do fluxo são colunas, as tarefas são cards, e quem participa dele é membro —
// cada um no seu domínio.
package board

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// TamanhoMaximoTitulo é o limite de caracteres do título do quadro.
const TamanhoMaximoTitulo = 120

var (
	// ErrTituloObrigatorio é retornado quando o título está vazio ou só com espaços.
	ErrTituloObrigatorio = errors.New("título é obrigatório")
	// ErrTituloLongo é retornado quando o título passa de TamanhoMaximoTitulo caracteres.
	ErrTituloLongo = errors.New("título é longo demais")
	// ErrNaoEncontrado é retornado quando o quadro não existe — ou quando quem
	// pergunta não participa dele. Ver usecase/board: os dois casos respondem
	// igual de propósito.
	ErrNaoEncontrado = errors.New("quadro não encontrado")
)

// FundoPadrao é o fundo de quem nunca escolheu um.
const FundoPadrao = "padrao"

// Fundos é a paleta de fundos disponíveis, na ordem em que a tela os oferece.
//
// Guardamos o NOME, e não a cor: a paleta muda com o tema claro/escuro, e um
// hex gravado aqui congelaria no dado uma escolha que pertence ao CSS.
var Fundos = []string{FundoPadrao, "ardosia", "oceano", "floresta", "ameixa", "brasa"}

// ErrFundoInvalido é retornado quando o fundo não está na paleta.
var ErrFundoInvalido = errors.New("fundo de quadro inválido")

// FundoValido informa se o fundo pertence à paleta.
func FundoValido(fundo string) bool {
	for _, conhecido := range Fundos {
		if fundo == conhecido {
			return true
		}
	}
	return false
}

// Board representa um quadro Kanban.
type Board struct {
	ID     string
	Titulo string
	// Fundo é o nome do fundo escolhido; vazio significa FundoPadrao.
	Fundo        string
	CriadoEm     time.Time
	AtualizadoEm time.Time
}

// FundoEfetivo devolve o fundo a usar, resolvendo o vazio para o padrão. O
// padrão vive aqui, e não num DEFAULT do banco: qual fundo um quadro sem
// escolha tem é decisão do domínio.
func (b *Board) FundoEfetivo() string {
	if b.Fundo == "" {
		return FundoPadrao
	}
	return b.Fundo
}

// DefinirFundo troca o fundo do quadro.
func (b *Board) DefinirFundo(fundo string) error {
	if !FundoValido(fundo) {
		return ErrFundoInvalido
	}
	b.Fundo = fundo
	b.AtualizadoEm = time.Now()
	return nil
}

// Novo cria um quadro. Retorna ErrTituloObrigatorio se o título for vazio e
// ErrTituloLongo se ele passar do limite.
func Novo(id, titulo string) (*Board, error) {
	titulo, err := ValidarTitulo(titulo)
	if err != nil {
		return nil, err
	}

	agora := time.Now()
	return &Board{
		ID:           id,
		Titulo:       titulo,
		CriadoEm:     agora,
		AtualizadoEm: agora,
	}, nil
}

// Renomear troca o título do quadro, com as mesmas regras da criação.
func (b *Board) Renomear(titulo string) error {
	titulo, err := ValidarTitulo(titulo)
	if err != nil {
		return err
	}
	b.Titulo = titulo
	b.AtualizadoEm = time.Now()
	return nil
}

// ValidarTitulo apara os espaços das pontas e checa o título, devolvendo a
// versão já normalizada. Exportada porque coluna e card usam a mesma régua —
// título é título em qualquer um dos três, e ter uma regra por domínio faria
// as três divergirem com o tempo.
func ValidarTitulo(titulo string) (string, error) {
	titulo = strings.TrimSpace(titulo)
	if titulo == "" {
		return "", ErrTituloObrigatorio
	}
	if utf8.RuneCountInString(titulo) > TamanhoMaximoTitulo {
		return "", ErrTituloLongo
	}
	return titulo, nil
}
