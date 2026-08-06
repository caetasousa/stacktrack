// Package coluna modela uma etapa do fluxo do quadro — "A fazer", "Fazendo",
// "Pronto". A ordem entre elas é a leitura da esquerda para a direita.
package coluna

import (
	"errors"
	"time"

	"kanbango/internal/domain/board"
)

// ErrNaoEncontrada é retornado quando a coluna não existe — ou quando quem
// pergunta não participa do quadro dela.
var ErrNaoEncontrada = errors.New("coluna não encontrada")

// Coluna é uma etapa do fluxo dentro de um quadro.
type Coluna struct {
	ID           string
	BoardID      string
	Titulo       string
	Posicao      float64
	CriadoEm     time.Time
	AtualizadoEm time.Time
}

// Nova cria uma coluna na posição informada. O título segue a mesma régua do
// quadro (board.ValidarTitulo).
func Nova(id, boardID, titulo string, posicao float64) (*Coluna, error) {
	titulo, err := board.ValidarTitulo(titulo)
	if err != nil {
		return nil, err
	}

	agora := time.Now()
	return &Coluna{
		ID:           id,
		BoardID:      boardID,
		Titulo:       titulo,
		Posicao:      posicao,
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

// PassoPosicao é o intervalo deixado entre posições ao acrescentar no fim.
//
// Não é 1: o espaço entre duas posições vizinhas é o que permite inserir no
// meio sem renumerar ninguém, e um passo largo adia por muito tempo o dia em
// que as divisões pela metade esgotam a precisão do float. A fase 4 vive
// dentro desses intervalos.
const PassoPosicao = 1024.0

// PosicaoNoFim devolve a posição de um item acrescentado depois do último.
// Recebe a maior posição em uso (0 quando não há nenhum item).
func PosicaoNoFim(ultimaPosicao float64) float64 {
	return ultimaPosicao + PassoPosicao
}
