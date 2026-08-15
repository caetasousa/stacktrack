package board

import (
	"context"
	"io"

	"stacktrack/internal/domain/anexo"
	dboard "stacktrack/internal/domain/board"
	"stacktrack/internal/domain/card"
	"stacktrack/internal/domain/checklist"
	"stacktrack/internal/domain/coluna"
	"stacktrack/internal/domain/comentario"
	"stacktrack/internal/domain/convite"
	"stacktrack/internal/domain/etiqueta"
	"stacktrack/internal/domain/membro"
	"stacktrack/internal/domain/usuario"
)

// Todas as buscas devolvem (nil, nil) quando não encontram: "não existe" não é
// falha, e distinguir isso de erro real evita que um banco fora do ar seja
// respondido como 404.

type RepositorioBoard interface {
	Salvar(ctx context.Context, b *dboard.Board) error
	Atualizar(ctx context.Context, b *dboard.Board) error
	BuscarPorID(ctx context.Context, id string) (*dboard.Board, error)
	Apagar(ctx context.Context, id string) error
	// ListarDoUsuario devolve os quadros de que o usuário participa, com o
	// papel dele em cada um, ordenados do mais recente para o mais antigo.
	ListarDoUsuario(ctx context.Context, usuarioID string) ([]Resumo, error)
}

type RepositorioMembro interface {
	Salvar(ctx context.Context, m *membro.Membro) error
	Buscar(ctx context.Context, boardID, usuarioID string) (*membro.Membro, error)
	Atualizar(ctx context.Context, m *membro.Membro) error
	Remover(ctx context.Context, boardID, usuarioID string) error
	// Todos devolve só os vínculos, sem dados de pessoa: é o que a regra do
	// último dono precisa enxergar para decidir.
	Todos(ctx context.Context, boardID string) ([]membro.Membro, error)
	// Participantes devolve os vínculos já com nome e email, para a tela de
	// membros não precisar de uma consulta por pessoa.
	Participantes(ctx context.Context, boardID string) ([]Participante, error)
}

// buscadorUsuario resolve contas a partir do email — é como o convite descobre
// se quem foi convidado já existe no sistema.
type buscadorUsuario interface {
	BuscarPorID(ctx context.Context, id string) (*usuario.Usuario, error)
	BuscarPorEmail(ctx context.Context, email string) (*usuario.Usuario, error)
}

type repositorioEtiqueta interface {
	Salvar(ctx context.Context, e *etiqueta.Etiqueta) error
	Atualizar(ctx context.Context, e *etiqueta.Etiqueta) error
	BuscarPorID(ctx context.Context, id string) (*etiqueta.Etiqueta, error)
	ListarDoBoard(ctx context.Context, boardID string) ([]etiqueta.Etiqueta, error)
	Apagar(ctx context.Context, id string) error
	UltimaPosicao(ctx context.Context, boardID string) (float64, error)
	// Aplicar e Remover ligam e desligam a etiqueta de um card.
	Aplicar(ctx context.Context, cardID, etiquetaID string) error
	Remover(ctx context.Context, cardID, etiquetaID string) error
	// EtiquetasDoBoardPorCard devolve, para cada card do quadro, os ids das
	// etiquetas aplicadas — numa consulta só, para a tela do quadro não fazer
	// uma por card.
	EtiquetasDoBoardPorCard(ctx context.Context, boardID string) (map[string][]string, error)
	EtiquetasDoCard(ctx context.Context, cardID string) ([]etiqueta.Etiqueta, error)
}

type repositorioChecklist interface {
	Salvar(ctx context.Context, c *checklist.Checklist) error
	Atualizar(ctx context.Context, c *checklist.Checklist) error
	BuscarPorID(ctx context.Context, id string) (*checklist.Checklist, error)
	ListarDoCard(ctx context.Context, cardID string) ([]checklist.Checklist, error)
	Apagar(ctx context.Context, id string) error
	UltimaPosicao(ctx context.Context, cardID string) (float64, error)

	SalvarItem(ctx context.Context, i *checklist.Item) error
	AtualizarItem(ctx context.Context, i *checklist.Item) error
	BuscarItem(ctx context.Context, id string) (*checklist.Item, error)
	ListarItens(ctx context.Context, checklistID string) ([]checklist.Item, error)
	ApagarItem(ctx context.Context, id string) error
	UltimaPosicaoItem(ctx context.Context, checklistID string) (float64, error)
	// ProgressoDoBoard devolve, por card, quantos itens estão concluídos e
	// quantos existem — é o "2/5" do card sem uma consulta por card.
	ProgressoDoBoard(ctx context.Context, boardID string) (map[string]Progresso, error)
}

// Responsavel é quem responde por um card. Guarda o mínimo que a tela precisa
// para desenhar um avatar — nada de email ou papel, que são da tela de membros.
type Responsavel struct {
	UsuarioID string
	Nome      string
}

// RepositorioResponsavel liga pessoas a cards.
//
// É porta própria, e não um punhado de métodos no repositório de cards, porque
// a pergunta que ela responde não é sobre o card: é sobre quem trabalha nele.
type RepositorioResponsavel interface {
	// Atribuir e Remover são idempotentes: repetir a operação não é erro,
	// porque o resultado pretendido já vale.
	Atribuir(ctx context.Context, cardID, usuarioID string) error
	Remover(ctx context.Context, cardID, usuarioID string) error
	// RemoverDoBoard apaga as atribuições da pessoa em todos os cards do
	// quadro. É o que roda quando ela sai dele.
	RemoverDoBoard(ctx context.Context, boardID, usuarioID string) error
	DoCard(ctx context.Context, cardID string) ([]Responsavel, error)
	// DoBoardPorCard devolve, para cada card do quadro, quem responde por ele —
	// numa consulta só, pelo mesmo motivo das etiquetas.
	DoBoardPorCard(ctx context.Context, boardID string) (map[string][]Responsavel, error)
}

// ComentarioComAutor é um comentário já com o nome de quem escreveu — o JOIN
// resolvido, para a tela não precisar de uma consulta por autor.
type ComentarioComAutor struct {
	Comentario comentario.Comentario
	AutorNome  string
}

// RepositorioComentario guarda a conversa dos cards.
type RepositorioComentario interface {
	Salvar(ctx context.Context, c *comentario.Comentario) error
	Atualizar(ctx context.Context, c *comentario.Comentario) error
	BuscarPorID(ctx context.Context, id string) (*comentario.Comentario, error)
	Apagar(ctx context.Context, id string) error
	// ListarDoCard devolve a conversa em ordem de tempo, do mais antigo para o
	// mais novo — é como se lê uma conversa.
	ListarDoCard(ctx context.Context, cardID string) ([]ComentarioComAutor, error)
	// ContarPorCardDoBoard devolve quantos comentários cada card do quadro tem,
	// para o selo do card, numa consulta só.
	ContarPorCardDoBoard(ctx context.Context, boardID string) (map[string]int, error)
}

type repositorioAnexo interface {
	Salvar(ctx context.Context, a *anexo.Anexo) error
	BuscarPorID(ctx context.Context, id string) (*anexo.Anexo, error)
	ListarDoCard(ctx context.Context, cardID string) ([]anexo.Anexo, error)
	Apagar(ctx context.Context, id string) error
	// ContarPorCardDoBoard devolve quantos anexos cada card do quadro tem.
	ContarPorCardDoBoard(ctx context.Context, boardID string) (map[string]int, error)
	// Caminhos dos arquivos que ficarão órfãos ao apagar. O ON DELETE CASCADE
	// limpa a tabela e NÃO o volume: sem isto, apagar um card deixava os
	// arquivos anexados no disco para sempre, sem nenhuma linha apontando para
	// eles. Quem chama coleta ANTES do DELETE.
	CaminhosDeArquivoDoCard(ctx context.Context, cardID string) ([]string, error)
	CaminhosDeArquivoDaColuna(ctx context.Context, colunaID string) ([]string, error)
	CaminhosDeArquivoDoBoard(ctx context.Context, boardID string) ([]string, error)
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
	Salvar(ctx context.Context, c *convite.Convite) error
	Atualizar(ctx context.Context, c *convite.Convite) error
	BuscarPorID(ctx context.Context, id string) (*convite.Convite, error)
	BuscarPorTokenHash(ctx context.Context, hash string) (*convite.Convite, error)
	BuscarPendentePorEmail(ctx context.Context, boardID, email string) (*convite.Convite, error)
	ListarPendentes(ctx context.Context, boardID string) ([]convite.Convite, error)
	Remover(ctx context.Context, id string) error
}

type RepositorioColuna interface {
	Salvar(ctx context.Context, c *coluna.Coluna) error
	Atualizar(ctx context.Context, c *coluna.Coluna) error
	BuscarPorID(ctx context.Context, id string) (*coluna.Coluna, error)
	// ListarDoBoard devolve as colunas em ordem de chave.
	ListarDoBoard(ctx context.Context, boardID string) ([]coluna.Coluna, error)
	Apagar(ctx context.Context, id string) error
	// UltimaChave devolve a maior chave em uso no quadro, ou vazio quando o
	// quadro não tem coluna nenhuma.
	UltimaChave(ctx context.Context, boardID string) (string, error)
}

type RepositorioCard interface {
	Salvar(ctx context.Context, c *card.Card) error
	Atualizar(ctx context.Context, c *card.Card) error
	BuscarPorID(ctx context.Context, id string) (*card.Card, error)
	// ListarDoBoard devolve todos os cards do quadro em ordem de chave,
	// numa consulta só — buscar coluna a coluna seria um N+1 que cresce com o
	// tamanho do quadro.
	ListarDoBoard(ctx context.Context, boardID string) ([]card.Card, error)
	Apagar(ctx context.Context, id string) error
	// UltimaChave devolve a maior chave em uso na coluna, ou vazio.
	UltimaChave(ctx context.Context, colunaID string) (string, error)
}
