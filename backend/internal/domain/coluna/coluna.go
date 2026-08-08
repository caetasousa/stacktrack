// Package coluna modela uma etapa do fluxo do quadro — "A fazer", "Fazendo",
// "Pronto". A ordem entre elas é a leitura da esquerda para a direita.
package coluna

import (
	"errors"
	"time"

	"stacktrack/internal/domain/board"
	"stacktrack/internal/domain/cor"
	"stacktrack/internal/domain/ordem"
)

// ErrNaoEncontrada é retornado quando a coluna não existe — ou quando quem
// pergunta não participa do quadro dela.
var ErrNaoEncontrada = errors.New("coluna não encontrada")

// Coluna é uma etapa do fluxo dentro de um quadro.
type Coluna struct {
	ID      string
	BoardID string
	Titulo  string
	// Cor é opcional: coluna sem cor usa o visual padrão. Serve para dar
	// significado à etapa — verde no começo, amarelo no meio, azul no fim.
	Cor     cor.Cor
	Posicao float64
	// Chave é a ordenação TEXTUAL, que substitui Posicao. Durante o expand as
	// duas convivem. Vazia só nas linhas antigas, até o backfill passar.
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
func Nova(id, boardID, titulo string, cores cor.Cor, posicao float64, chave string) (*Coluna, error) {
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
		Posicao:      posicao,
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

// PassoPosicao é o intervalo entre posições. Mantido como alias do pacote
// ordem, que é onde a regra de ordenação fracionária vive desde a fase 4.
const PassoPosicao = ordem.Passo

// PosicaoNoFim devolve a posição de um item acrescentado depois do último.
// Recebe a maior posição em uso (0 quando não há nenhum item).
func PosicaoNoFim(ultimaPosicao float64) float64 {
	return ordem.NoFim(ultimaPosicao)
}

// MoverPara reposiciona a coluna. A posição é calculada por quem sabe quem são
// os vizinhos — ver usecase/board e o pacote ordem.
func (c *Coluna) MoverPara(posicao float64, chave string) {
	c.Chave = chave
	c.Posicao = posicao
	c.AtualizadoEm = time.Now()
}

// DefinirChaveDeOrdem grava a chave textual de ordenação. É o que o comando de
// backfill usa para preencher as colunas criadas antes do expand.
func (c *Coluna) DefinirChaveDeOrdem(chave string) {
	c.Chave = chave
}
