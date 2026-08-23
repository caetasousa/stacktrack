package board

import (
	"context"
	"errors"
	"io"
	"time"

	"stacktrack/internal/domain/anexo"
	dboard "stacktrack/internal/domain/board"
	"stacktrack/internal/domain/card"
	"stacktrack/internal/domain/checklist"
	"stacktrack/internal/domain/coluna"
	"stacktrack/internal/domain/comentario"
	"stacktrack/internal/domain/convite"
	"stacktrack/internal/domain/cor"
	"stacktrack/internal/domain/etiqueta"
	"stacktrack/internal/domain/membro"
	"stacktrack/internal/domain/publicacao"
	"stacktrack/internal/domain/usuario"
)

// Todas as buscas devolvem (nil, nil) quando não encontram: "não existe" não é
// falha, e distinguir isso de erro real evita que um banco fora do ar seja
// respondido como 404.

// ErrQuadroOcupado é devolvido quando a unidade de trabalho não conseguiu o
// lock do quadro dentro do prazo.
//
// Fica declarado AQUI, no pacote que define as portas, e não no adaptador que o
// produz: a borda HTTP precisa reconhecê-lo para responder 503 com Retry-After,
// e importar o pacote de repositório só para comparar um erro furaria a
// direção das dependências.
//
// É erro do TEMPO, e não da regra: quem chamou pediu algo legítimo e o quadro
// estava ocupado. Repetir daqui a pouco é a ação certa, e é isso que um 503 com
// Retry-After comunica — um 500 diria "deu errado, não tente de novo".
var ErrQuadroOcupado = errors.New("o quadro está ocupado por outra operação; tente de novo")

// RepositorioBoard guarda os quadros.
//
// Renomear e DefinirFundo são comandos ESTREITOS, e não um `Atualizar` que
// grava o agregado inteiro. Com o UPDATE largo, quem renomeia regrava também o
// fundo que leu há dez segundos — desfazendo, sem erro e sem conflito, a troca
// de fundo que outra pessoa fez nesse meio-tempo.
type RepositorioBoard interface {
	Salvar(ctx context.Context, b *dboard.Board) error
	Renomear(ctx context.Context, id, titulo string, em time.Time) error
	DefinirFundo(ctx context.Context, id, fundo string, em time.Time) error
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

// RepositorioPublicacao guarda o link público de cada quadro.
//
// Não tem Atualizar: o token de uma publicação não muda. Gerar um link novo é
// remover a publicação e criar outra, e é essa a razão de a porta ser assim —
// um Atualizar convidaria a trocar o token no lugar, e quem estivesse com o
// link antigo continuaria dentro de uma publicação que o dono acha que revogou.
type RepositorioPublicacao interface {
	Salvar(ctx context.Context, p *publicacao.Publicacao) error
	// BuscarPorBoard responde "este quadro está publicado?" — é o que a tela do
	// dono mostra e o que o quadro exibe como aviso a quem edita.
	BuscarPorBoard(ctx context.Context, boardID string) (*publicacao.Publicacao, error)
	// BuscarPorToken é a única autorização do caminho público: quem apresenta o
	// token vê o quadro, quem não apresenta não existe para esta rota.
	BuscarPorToken(ctx context.Context, token string) (*publicacao.Publicacao, error)
	// Remover revoga. Revogar o que não está publicado não é erro: o resultado
	// pretendido já vale.
	Remover(ctx context.Context, boardID string) error
}

// buscadorUsuario resolve contas a partir do email — é como o convite descobre
// se quem foi convidado já existe no sistema.
type buscadorUsuario interface {
	BuscarPorID(ctx context.Context, id string) (*usuario.Usuario, error)
	BuscarPorEmail(ctx context.Context, email string) (*usuario.Usuario, error)
}

type RepositorioEtiqueta interface {
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

type RepositorioChecklist interface {
	Salvar(ctx context.Context, c *checklist.Checklist) error
	Atualizar(ctx context.Context, c *checklist.Checklist) error
	BuscarPorID(ctx context.Context, id string) (*checklist.Checklist, error)
	ListarDoCard(ctx context.Context, cardID string) ([]checklist.Checklist, error)
	Apagar(ctx context.Context, id string) error
	UltimaPosicao(ctx context.Context, cardID string) (float64, error)

	SalvarItem(ctx context.Context, i *checklist.Item) error
	// EditarItem e MarcarItem são separados porque reescrever o texto e marcar
	// a caixa são ações independentes que duas pessoas fazem ao mesmo tempo o
	// tempo todo. Com um UPDATE largo, renomear o item desmarcava a caixa que a
	// outra pessoa acabara de marcar.
	EditarItem(ctx context.Context, id, texto string, em time.Time) error
	MarcarItem(ctx context.Context, id string, concluido bool, em time.Time) error
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

type RepositorioAnexo interface {
	Salvar(ctx context.Context, a *anexo.Anexo) error
	// ContarDoCard e BytesDoBoard alimentam as cotas, e são chamadas DENTRO da
	// transação que grava o anexo. Uma contagem lida antes dela seria vista por
	// dois envios simultâneos, que gravariam os dois por cima do teto.
	ContarDoCard(ctx context.Context, cardID string) (int, error)
	BytesDoBoard(ctx context.Context, boardID string) (int64, error)
	BuscarPorID(ctx context.Context, id string) (*anexo.Anexo, error)
	ListarDoCard(ctx context.Context, cardID string) ([]anexo.Anexo, error)
	// Apagar informa se esta chamada realmente removeu a linha. O resultado
	// distingue o vencedor de duas exclusões concorrentes: só ele pode emitir
	// anexo.removido e mandar o arquivo físico para descarte.
	Apagar(ctx context.Context, id string) (bool, error)
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

// ErrArquivoAcimaDoLimite é devolvido pelo armazém quando o conteúdo passa do
// teto informado a Receber.
//
// Fica declarado AQUI, no pacote das portas, e não no adaptador que o produz: o
// usecase precisa reconhecê-lo para traduzi-lo no erro de domínio, e importar o
// pacote de armazém só para comparar um erro furaria a direção das dependências.
//
// É detectado DURANTE a leitura, e não pelo Content-Length: o cabeçalho é
// entrada do cliente, e quem mente nele é exatamente quem se quer barrar.
var ErrArquivoAcimaDoLimite = errors.New("o arquivo passa do tamanho máximo")

// ArquivoRecebido é um upload já em disco e ainda NÃO publicado.
//
// O usecase só conhece o que precisa para decidir: quanto ocupou, o que é, e a
// impressão digital. Onde o temporário está é assunto do armazém.
type ArquivoRecebido interface {
	// Bytes é o tamanho medido durante a leitura — nunca o Content-Length, que
	// é entrada do cliente.
	Bytes() int64
	// Tipo é o MIME deduzido do CONTEÚDO.
	Tipo() string
	// Digest é o SHA-256 do conteúdo, para a verificação de restauração de A6.
	Digest() string
}

// armazemDeArquivos guarda e devolve o conteúdo dos anexos. É porta porque o
// disco de hoje pode virar S3 amanhã sem o usecase saber.
//
// RECEBER e PUBLICAR são separados de propósito, e essa separação é o upload
// streaming inteiro. O conteúdo chega por um socket e não cabe decidir se ele é
// aceitável antes de tê-lo por inteiro: o tamanho real só se conhece no fim, e o
// tipo só depois de olhar o começo. Recebendo primeiro num temporário, o
// processo nunca segura o arquivo em memória, e a publicação — um `rename`
// atômico — só acontece depois de o domínio ter aprovado com os fatos na mão.
type armazemDeArquivos interface {
	// Receber grava o conteúdo num temporário, medindo tamanho, tipo e hash.
	// Devolve erro quando o conteúdo passa do limite.
	Receber(conteudo io.Reader, limite int64) (ArquivoRecebido, error)
	// Publicar promove o temporário ao nome definitivo, de forma atômica.
	Publicar(recebido ArquivoRecebido, extensao string) (string, error)
	// Descartar apaga um temporário que não será publicado.
	Descartar(recebido ArquivoRecebido) error
	Abrir(caminho string) (io.ReadCloser, error)
	Remover(caminho string) error
}

// RepositorioConvite guarda os convites de quadro.
//
// Aceitar e Revogar recebem o ID e o instante, e não o agregado inteiro. A
// diferença não é de estilo: um `Atualizar(c *Convite)` grava o que foi lido
// antes, e entre a leitura e a escrita cabe outra transação inteira — duas abas
// clicando no mesmo link liam "pendente" as duas e gravavam as duas. Com a
// transição no comando, a condição vai para o WHERE e só uma das duas afeta
// linha; a outra recebe convite.ErrJaResolvido.
//
// O instante vem de fora, e não de now() no SQL, porque é o MESMO que o domínio
// usou para decidir: relógio do banco e relógio do processo não são o mesmo, e
// deixar a fronteira do vencimento depender de qual foi consultado tornaria o
// caso de borda irreprodutível.
type RepositorioConvite interface {
	Salvar(ctx context.Context, c *convite.Convite) error
	BuscarPorID(ctx context.Context, id string) (*convite.Convite, error)
	BuscarPorTokenHash(ctx context.Context, hash string) (*convite.Convite, error)
	// BuscarPendentePorEmail ignora convite aceito ou revogado. O VENCIDO ainda
	// volta: ele ocupa a vaga do índice de pendência, e é o domínio que decide
	// revogá-lo para liberar espaço a um convite novo.
	BuscarPendentePorEmail(ctx context.Context, boardID, email string) (*convite.Convite, error)
	ListarPendentes(ctx context.Context, boardID string) ([]convite.Convite, error)
	// Aceitar e Revogar devolvem convite.ErrJaResolvido quando nenhuma linha
	// muda — outra requisição chegou primeiro.
	Aceitar(ctx context.Context, id string, em time.Time) error
	Revogar(ctx context.Context, id string, em time.Time) error
}

// RepositorioColuna guarda as colunas.
//
// Renomear e DefinirChave são separados pela mesma razão de RepositorioBoard:
// com o UPDATE largo, renomear uma coluna regravava a chave de ordenação lida
// antes, e uma reordenação simultânea era desfeita em silêncio.
type RepositorioColuna interface {
	Salvar(ctx context.Context, c *coluna.Coluna) error
	Renomear(ctx context.Context, id, titulo string, cores cor.Cor, em time.Time) error
	DefinirChave(ctx context.Context, id, chave string, em time.Time) error
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
	// ListarDaColuna devolve os cards de UMA coluna, em ordem de chave. É o que
	// o rebalanceamento precisa: as chaves em uso naquele contêiner, e nada
	// além.
	ListarDaColuna(ctx context.Context, colunaID string) ([]card.Card, error)
	Apagar(ctx context.Context, id string) error
	// UltimaChave devolve a maior chave em uso na coluna, ou vazio.
	UltimaChave(ctx context.Context, colunaID string) (string, error)
}
