// Package board contém os usecases do quadro: criar, listar, detalhar e
// alterar quadros, colunas e cards.
//
// É aqui que a autorização acontece — em toda operação, antes de qualquer
// leitura ou escrita. O handler só traduz o erro em código HTTP; se a checagem
// vivesse lá, uma rota nova nasceria aberta por esquecimento.
package board

import (
	"time"

	"kanbango/internal/domain/board"
	"kanbango/internal/domain/card"
	"kanbango/internal/domain/coluna"
	"kanbango/internal/domain/membro"
)

// Resumo é um quadro na listagem, com o papel de quem pediu.
type Resumo struct {
	Board board.Board
	Papel membro.Papel
}

// ColunaComCards é uma coluna e os cards dela, já em ordem de posição.
type ColunaComCards struct {
	Coluna coluna.Coluna
	Cards  []card.Card
}

// Detalhado é o quadro inteiro: dados, papel de quem pediu e o conteúdo.
type Detalhado struct {
	Board   board.Board
	Papel   membro.Papel
	Colunas []ColunaComCards
}

// Participante é alguém que participa do quadro, com os dados que a tela de
// membros mostra.
type Participante struct {
	UsuarioID string
	Nome      string
	Email     string
	Papel     membro.Papel
	CriadoEm  time.Time
}
