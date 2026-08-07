package board

import (
	"io"

	"kanbango/internal/domain/anexo"
	dboard "kanbango/internal/domain/board"
	"kanbango/internal/domain/card"
	"kanbango/internal/domain/checklist"
	"kanbango/internal/domain/coluna"
	"kanbango/internal/domain/convite"
	"kanbango/internal/domain/etiqueta"
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

type repositorioEtiqueta interface {
	Salvar(e *etiqueta.Etiqueta) error
	Atualizar(e *etiqueta.Etiqueta) error
	BuscarPorID(id string) (*etiqueta.Etiqueta, error)
	ListarDoBoard(boardID string) ([]etiqueta.Etiqueta, error)
	Apagar(id string) error
	UltimaPosicao(boardID string) (float64, error)
	// Aplicar e Remover ligam e desligam a etiqueta de um card.
	Aplicar(cardID, etiquetaID string) error
	Remover(cardID, etiquetaID string) error
	// EtiquetasDoBoardPorCard devolve, para cada card do quadro, os ids das
	// etiquetas aplicadas — numa consulta só, para a tela do quadro não fazer
	// uma por card.
	EtiquetasDoBoardPorCard(boardID string) (map[string][]string, error)
	EtiquetasDoCard(cardID string) ([]etiqueta.Etiqueta, error)
}

type repositorioChecklist interface {
	Salvar(c *checklist.Checklist) error
	Atualizar(c *checklist.Checklist) error
	BuscarPorID(id string) (*checklist.Checklist, error)
	ListarDoCard(cardID string) ([]checklist.Checklist, error)
	Apagar(id string) error
	UltimaPosicao(cardID string) (float64, error)

	SalvarItem(i *checklist.Item) error
	AtualizarItem(i *checklist.Item) error
	BuscarItem(id string) (*checklist.Item, error)
	ListarItens(checklistID string) ([]checklist.Item, error)
	ApagarItem(id string) error
	UltimaPosicaoItem(checklistID string) (float64, error)
	// ProgressoDoBoard devolve, por card, quantos itens estão concluídos e
	// quantos existem — é o "2/5" do card sem uma consulta por card.
	ProgressoDoBoard(boardID string) (map[string]Progresso, error)
}

type repositorioAnexo interface {
	Salvar(a *anexo.Anexo) error
	BuscarPorID(id string) (*anexo.Anexo, error)
	ListarDoCard(cardID string) ([]anexo.Anexo, error)
	Apagar(id string) error
	// ContarPorCardDoBoard devolve quantos anexos cada card do quadro tem.
	ContarPorCardDoBoard(boardID string) (map[string]int, error)
}

// armazemDeArquivos guarda e devolve o conteúdo dos anexos. É porta porque o
// disco de hoje pode virar S3 amanhã sem o usecase saber.
type armazemDeArquivos interface {
	// Guardar grava o conteúdo e devolve o nome do arquivo gravado.
	Guardar(conteudo io.Reader, extensao string) (string, error)
	Abrir(caminho string) (io.ReadCloser, error)
	Remover(caminho string) error
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
