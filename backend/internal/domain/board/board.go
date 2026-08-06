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

// Board representa um quadro Kanban.
type Board struct {
	ID           string
	Titulo       string
	CriadoEm     time.Time
	AtualizadoEm time.Time
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
