// Package board contém os usecases do quadro: criar, listar, detalhar e
// alterar quadros, colunas e cards.
//
// É aqui que a autorização acontece — em toda operação, antes de qualquer
// leitura ou escrita. O handler só traduz o erro em código HTTP; se a checagem
// vivesse lá, uma rota nova nasceria aberta por esquecimento.
package board

import (
	"time"

	"stacktrack/internal/domain/anexo"
	"stacktrack/internal/domain/board"
	"stacktrack/internal/domain/card"
	"stacktrack/internal/domain/checklist"
	"stacktrack/internal/domain/coluna"
	"stacktrack/internal/domain/etiqueta"
	"stacktrack/internal/domain/membro"
)

// Resumo é um quadro na listagem, com o papel de quem pediu.
type Resumo struct {
	Board board.Board
	Papel membro.Papel
}

// Progresso é o "2/5" de checklist de um card.
type Progresso struct {
	Concluidos int
	Total      int
}

// CardNoQuadro é um card com o resumo do que está pendurado nele. Os resumos
// vêm juntos para a tela do quadro não precisar abrir card por card só para
// desenhar os selos.
type CardNoQuadro struct {
	Card card.Card
	// Responsaveis vêm com nome, e não só o id: o avatar precisa das iniciais,
	// e resolver nome por card no cliente exigiria uma segunda requisição.
	Responsaveis   []Responsavel
	Etiquetas      []string
	Checklist      Progresso
	QtdAnexos      int
	QtdComentarios int
}

// ColunaComCards é uma coluna e os cards dela, já em ordem de chave.
type ColunaComCards struct {
	Coluna coluna.Coluna
	Cards  []CardNoQuadro
}

// Detalhado é o quadro inteiro: dados, papel de quem pediu e o conteúdo.
type Detalhado struct {
	Board     board.Board
	Papel     membro.Papel
	Colunas   []ColunaComCards
	Etiquetas []etiqueta.Etiqueta
}

// CardDetalhado é o que o modal do card mostra: o card e tudo que pende dele.
type CardDetalhado struct {
	Card         card.Card
	BoardID      string
	Responsaveis []Responsavel
	Etiquetas    []etiqueta.Etiqueta
	Checklists   []ChecklistComItens
	Anexos       []anexo.Anexo
	Comentarios  []ComentarioComAutor
}

// ChecklistComItens é uma checklist e as linhas dela, em ordem.
type ChecklistComItens struct {
	Checklist checklist.Checklist
	Itens     []checklist.Item
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
