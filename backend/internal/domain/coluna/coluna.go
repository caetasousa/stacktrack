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

var (
	// ErrJaArquivada é retornado ao arquivar uma coluna que já está arquivada.
	ErrJaArquivada = errors.New("a coluna já está arquivada")
	// ErrNaoArquivada é retornado ao desarquivar uma coluna que não está arquivada.
	ErrNaoArquivada = errors.New("a coluna não está arquivada")
)

// Coluna é uma etapa do fluxo dentro de um quadro.
type Coluna struct {
	ID      string
	BoardID string
	Titulo  string
	// Cor é opcional: coluna sem cor usa o visual padrão. Serve para dar
	// significado à etapa — verde no começo, amarelo no meio, azul no fim.
	Cor cor.Cor
	// Chave é a ordenação. Ver o comentário equivalente em domain/card.
	Chave string
	// ArquivadoEm é nil enquanto a coluna está no quadro. Ver o comentário
	// equivalente em domain/card.
	//
	// Arquivar a coluna NÃO arquiva os cards dela: eles continuam com
	// `arquivado_em` nulo e voltam junto quando ela volta. Arquivar em cascata
	// pareceria mais arrumado e seria pior — desarquivar a coluna teria de
	// adivinhar quais cards já estavam arquivados ANTES, e devolveria ao quadro
	// cards que alguém tinha tirado de lá de propósito.
	ArquivadoEm  *time.Time
	CriadoEm     time.Time
	AtualizadoEm time.Time
}

// Arquivada informa se a coluna saiu do quadro.
func (c *Coluna) Arquivada() bool { return c.ArquivadoEm != nil }

// Arquivar tira a coluna do quadro sem apagá-la, junto com os cards dela.
// Erra com ErrJaArquivada se ela já estiver fora.
func (c *Coluna) Arquivar() error {
	if c.Arquivada() {
		return ErrJaArquivada
	}
	agora := time.Now()
	c.ArquivadoEm = &agora
	c.AtualizadoEm = agora
	return nil
}

// Desarquivar devolve a coluna ao quadro, na posição em que estava, com os
// cards que ela tinha. Erra com ErrNaoArquivada se ela não estiver arquivada.
func (c *Coluna) Desarquivar() error {
	if !c.Arquivada() {
		return ErrNaoArquivada
	}
	c.ArquivadoEm = nil
	c.AtualizadoEm = time.Now()
	return nil
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
