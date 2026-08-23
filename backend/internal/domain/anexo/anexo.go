// Package anexo modela o que fica pendurado num card: um arquivo enviado ou um
// link. Os dois no mesmo domínio porque, para quem usa, são a mesma coisa — uma
// referência. O que muda é onde o conteúdo mora.
package anexo

import (
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// TamanhoMaximoNome é o limite de caracteres do nome exibido.
	TamanhoMaximoNome = 255
	// TamanhoMaximoArquivo é o teto de bytes de um arquivo enviado (10 MiB).
	//
	// Existe porque upload sem teto é negação de serviço barata: bastam alguns
	// arquivos gigantes para encher o disco do container e derrubar tudo, banco
	// incluído.
	TamanhoMaximoArquivo = 10 << 20

	// MaximoDeAnexosPorCard é quantos anexos um card comporta.
	//
	// Vinte: um card é uma unidade de trabalho, não um repositório de arquivos.
	// O teto existe porque a listagem do card carrega TODOS os anexos dele numa
	// consulta, e porque um card com mil anexos torna o modal inutilizável
	// muito antes de ser um problema de armazenamento.
	MaximoDeAnexosPorCard = 20

	// MaximoDeBytesPorBoard é quanto o conjunto de arquivos de um quadro pode
	// ocupar (1 GiB).
	//
	// É o teto que de fato protege o disco. O limite por arquivo não protege
	// nada sozinho: cem arquivos de 10 MiB cabem no limite individual e enchem
	// um gigabyte. É também o número que torna previsível o tamanho de um
	// backup por quadro.
	MaximoDeBytesPorBoard = 1 << 30
)

// Tipo distingue de onde vem o conteúdo do anexo.
type Tipo string

const (
	// TipoArquivo é um arquivo enviado, guardado no volume de anexos.
	TipoArquivo Tipo = "arquivo"
	// TipoLink é uma URL — o "Committed to Repo" do template do Trello.
	TipoLink Tipo = "link"
)

var (
	// ErrNomeObrigatorio é retornado quando o nome exibido está vazio.
	ErrNomeObrigatorio = errors.New("nome do anexo é obrigatório")
	// ErrNomeLongo é retornado quando o nome passa do limite.
	ErrNomeLongo = errors.New("nome do anexo é longo demais")
	// ErrURLInvalida é retornado quando a URL não é http(s) absoluta.
	ErrURLInvalida = errors.New("o link precisa começar com http:// ou https://")
	// ErrArquivoVazio é retornado quando o arquivo enviado não tem conteúdo.
	ErrArquivoVazio = errors.New("o arquivo está vazio")
	// ErrArquivoGrande é retornado quando o arquivo passa de TamanhoMaximoArquivo.
	ErrArquivoGrande = errors.New("o arquivo passa de 10 MB")
	// ErrTipoNaoPermitido é retornado quando o tipo do arquivo não está na lista.
	ErrTipoNaoPermitido = errors.New("tipo de arquivo não permitido")
	// ErrAnexosDemaisNoCard é retornado quando o card já tem
	// MaximoDeAnexosPorCard anexos.
	ErrAnexosDemaisNoCard = errors.New("este card já tem o número máximo de anexos")
	// ErrCotaDoQuadroExcedida é retornado quando os arquivos do quadro já
	// ocupam MaximoDeBytesPorBoard.
	ErrCotaDoQuadroExcedida = errors.New("o quadro atingiu o limite de armazenamento de anexos")
	// ErrNaoEncontrado é retornado quando o anexo não existe — ou quando quem
	// pergunta não participa do quadro dele.
	ErrNaoEncontrado = errors.New("anexo não encontrado")
)

// tiposPermitidos é a lista do que pode ser enviado.
//
// Lista de permissão, e não de bloqueio: com bloqueio, todo formato perigoso
// novo entra até alguém lembrar de proibi-lo. Falta de propósito o text/html —
// um HTML servido da nossa origem executaria script na nossa origem.
var tiposPermitidos = map[string]bool{
	"image/png":        true,
	"image/jpeg":       true,
	"image/gif":        true,
	"image/webp":       true,
	"image/svg+xml":    false, // SVG é XML com <script> dentro: mesmo risco do HTML
	"application/pdf":  true,
	"text/plain":       true,
	"text/csv":         true,
	"text/markdown":    true,
	"application/zip":  true,
	"application/json": true,
}

// TipoPermitido informa se o MIME informado pode ser enviado.
func TipoPermitido(mime string) bool {
	// O navegador manda "text/plain; charset=utf-8"; só o tipo interessa.
	if i := strings.Index(mime, ";"); i >= 0 {
		mime = mime[:i]
	}
	return tiposPermitidos[strings.ToLower(strings.TrimSpace(mime))]
}

// Anexo é um arquivo ou link pendurado num card.
type Anexo struct {
	ID     string
	CardID string
	Tipo   Tipo
	Nome   string
	// URL vem preenchida só para TipoLink.
	URL string
	// Caminho é o nome do arquivo dentro do diretório de anexos — nunca o nome
	// original, que vem de quem envia. Preenchido só para TipoArquivo.
	Caminho   string
	Tamanho   int64
	MIME      string
	CriadoPor string
	CriadoEm  time.Time
}

// NovoLink cria um anexo de link. Aceita só http e https: um `javascript:` ou
// `data:` guardado aqui viraria execução de script na tela de quem clicasse.
func NovoLink(id, cardID, nome, endereco, criadoPor string) (*Anexo, error) {
	endereco = strings.TrimSpace(endereco)
	u, err := url.Parse(endereco)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, ErrURLInvalida
	}

	// Sem nome informado, o domínio do link é um rótulo melhor que a URL inteira.
	if strings.TrimSpace(nome) == "" {
		nome = u.Host
	}
	nome, err = validarNome(nome)
	if err != nil {
		return nil, err
	}

	return &Anexo{
		ID: id, CardID: cardID, Tipo: TipoLink, Nome: nome, URL: endereco,
		CriadoPor: criadoPor, CriadoEm: time.Now(),
	}, nil
}

// NovoArquivo cria um anexo de arquivo. `caminho` é o nome gerado por quem
// gravou no disco; `nomeOriginal` é só o rótulo mostrado na tela.
func NovoArquivo(id, cardID, nomeOriginal, caminho, mime string, tamanho int64, criadoPor string) (*Anexo, error) {
	if tamanho <= 0 {
		return nil, ErrArquivoVazio
	}
	if tamanho > TamanhoMaximoArquivo {
		return nil, ErrArquivoGrande
	}
	if !TipoPermitido(mime) {
		return nil, ErrTipoNaoPermitido
	}

	// Só o nome-base: "../../etc/passwd" enviado como nome de arquivo não pode
	// sobreviver nem como rótulo, muito menos virar caminho.
	nome, err := validarNome(filepath.Base(nomeOriginal))
	if err != nil {
		return nil, err
	}

	return &Anexo{
		ID: id, CardID: cardID, Tipo: TipoArquivo, Nome: nome, Caminho: caminho,
		Tamanho: tamanho, MIME: mime, CriadoPor: criadoPor, CriadoEm: time.Now(),
	}, nil
}

func validarNome(nome string) (string, error) {
	nome = strings.TrimSpace(nome)
	if nome == "" || nome == "." || nome == "/" {
		return "", ErrNomeObrigatorio
	}
	if utf8.RuneCountInString(nome) > TamanhoMaximoNome {
		return "", ErrNomeLongo
	}
	return nome, nil
}
