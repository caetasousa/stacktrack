// Package coluna modela uma etapa do fluxo do quadro — "A fazer", "Fazendo",
// "Pronto". A ordem entre elas é a leitura da esquerda para a direita.
package coluna

import (
	"errors"
	"time"

	"stacktrack/internal/domain/board"
	"stacktrack/internal/domain/cor"
)

// ErrNaoEncontrada é retornado quando a coluna não existe — ou quando quem
// pergunta não participa do quadro dela.
var ErrNaoEncontrada = errors.New("coluna não encontrada")

// ErrConflito é devolvido quando a reordenação não conseguiu uma chave livre
// nem depois de redistribuir as chaves do quadro.
//
// É conflito, e não erro interno: quem chamou pediu algo legítimo e o estado
// mudou embaixo dele. A ação certa é recarregar e arrastar de novo, e é isso
// que um 409 comunica.
var ErrConflito = errors.New("a ordem das colunas mudou; recarregue e tente de novo")

// Coluna é uma etapa do fluxo dentro de um quadro.
type Coluna struct {
	ID      string
	BoardID string
	Titulo  string
	// Cor é opcional: coluna sem cor usa o visual padrão. Serve para dar
	// significado à etapa — verde no começo, amarelo no meio, azul no fim.
	Cor cor.Cor
	// Chave é a ordenação. Ver o comentário equivalente em domain/card.
	Chave        string
	CriadoEm     time.Time
	AtualizadoEm time.Time
}

// DefinirCor troca a cor da coluna. Cor vazia volta ao visual padrão.
func (c *Coluna) DefinirCor(nova cor.Cor) error {
	if !cor.ValidaOuVazia(nova) {
		return cor.ErrInvalida
	}
	c.Cor = nova
	c.AtualizadoEm = time.Now()
	return nil
}

// Nova cria uma coluna na posição informada. O título segue a mesma régua do
// quadro (board.ValidarTitulo).
func Nova(id, boardID, titulo string, cores cor.Cor, chave string) (*Coluna, error) {
	titulo, err := board.ValidarTitulo(titulo)
	if err != nil {
		return nil, err
	}
	if !cor.ValidaOuVazia(cores) {
		return nil, cor.ErrInvalida
	}

	agora := time.Now()
	return &Coluna{
		ID:           id,
		BoardID:      boardID,
		Titulo:       titulo,
		Cor:          cores,
		Chave:        chave,
		CriadoEm:     agora,
		AtualizadoEm: agora,
	}, nil
}

// Renomear troca o título da coluna.
func (c *Coluna) Renomear(titulo string) error {
	titulo, err := board.ValidarTitulo(titulo)
	if err != nil {
		return err
	}
	c.Titulo = titulo
	c.AtualizadoEm = time.Now()
	return nil
}

// MoverPara reposiciona a coluna. A posição é calculada por quem sabe quem são
// os vizinhos — ver usecase/board e o pacote ordem.
func (c *Coluna) MoverPara(chave string) {
	c.Chave = chave
	c.AtualizadoEm = time.Now()
}
