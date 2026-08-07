package board

import (
	dboard "kanbango/internal/domain/board"
	"kanbango/internal/domain/card"
	"kanbango/internal/domain/coluna"
	"kanbango/internal/domain/convite"
	"kanbango/internal/domain/membro"
	"kanbango/internal/domain/usuario"
)

// Todas as buscas devolvem (nil, nil) quando não encontram: "não existe" não é
// falha, e distinguir isso de erro real evita que um banco fora do ar seja
// respondido como 404.

type repositorioBoard interface {
	Salvar(b *dboard.Board) error
	Atualizar(b *dboard.Board) error
	BuscarPorID(id string) (*dboard.Board, error)
	Apagar(id string) error
	// ListarDoUsuario devolve os quadros de que o usuário participa, com o
	// papel dele em cada um, ordenados do mais recente para o mais antigo.
	ListarDoUsuario(usuarioID string) ([]Resumo, error)
}

type repositorioMembro interface {
	Salvar(m *membro.Membro) error
	Buscar(boardID, usuarioID string) (*membro.Membro, error)
	Atualizar(m *membro.Membro) error
	Remover(boardID, usuarioID string) error
	// Todos devolve só os vínculos, sem dados de pessoa: é o que a regra do
	// último dono precisa enxergar para decidir.
	Todos(boardID string) ([]membro.Membro, error)
	// Participantes devolve os vínculos já com nome e email, para a tela de
	// membros não precisar de uma consulta por pessoa.
	Participantes(boardID string) ([]Participante, error)
}

// buscadorUsuario resolve contas a partir do email — é como o convite descobre
// se quem foi convidado já existe no sistema.
type buscadorUsuario interface {
	BuscarPorID(id string) (*usuario.Usuario, error)
	BuscarPorEmail(email string) (*usuario.Usuario, error)
}

type repositorioConvite interface {
	Salvar(c *convite.Convite) error
	Atualizar(c *convite.Convite) error
	BuscarPorID(id string) (*convite.Convite, error)
	BuscarPorTokenHash(hash string) (*convite.Convite, error)
	BuscarPendentePorEmail(boardID, email string) (*convite.Convite, error)
	ListarPendentes(boardID string) ([]convite.Convite, error)
	Remover(id string) error
}

type repositorioColuna interface {
	Salvar(c *coluna.Coluna) error
	Atualizar(c *coluna.Coluna) error
	BuscarPorID(id string) (*coluna.Coluna, error)
	// ListarDoBoard devolve as colunas em ordem de posição.
	ListarDoBoard(boardID string) ([]coluna.Coluna, error)
	Apagar(id string) error
	// UltimaPosicao devolve a maior posição em uso no quadro, ou 0 se não
	// houver coluna nenhuma.
	UltimaPosicao(boardID string) (float64, error)
}

type repositorioCard interface {
	Salvar(c *card.Card) error
	Atualizar(c *card.Card) error
	BuscarPorID(id string) (*card.Card, error)
	// ListarDoBoard devolve todos os cards do quadro em ordem de posição,
	// numa consulta só — buscar coluna a coluna seria um N+1 que cresce com o
	// tamanho do quadro.
	ListarDoBoard(boardID string) ([]card.Card, error)
	Apagar(id string) error
	// UltimaPosicao devolve a maior posição em uso na coluna, ou 0 se a coluna
	// estiver vazia.
	UltimaPosicao(colunaID string) (float64, error)
}
