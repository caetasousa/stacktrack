// Package card modela a tarefa que caminha pelo quadro. Um card pertence a uma
// coluna, e mudar de coluna é como ele "anda" pelo fluxo.
package card

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"stacktrack/internal/domain/cor"
)

const (
	// TamanhoMaximoTitulo é o limite de caracteres do título do card.
	TamanhoMaximoTitulo = 200
	// TamanhoMaximoDescricao é o limite de caracteres da descrição.
	TamanhoMaximoDescricao = 5000
)

// ErrConflito é devolvido quando o card mudou entre a leitura e a escrita —
// outra pessoa gravou primeiro. Não é erro de quem escreveu: é a informação de
// que a base em que ele decidiu já não vale.
//
// Existe no domínio, e não no repositório, porque a REGRA é do domínio: "não
// sobrescreva o trabalho de outra pessoa em silêncio". O SQL é só onde ela é
// aplicada.
var ErrConflito = errors.New("o card foi alterado por outra pessoa")

var (
	// ErrTituloObrigatorio é retornado quando o título está vazio ou só com espaços.
	ErrTituloObrigatorio = errors.New("título do card é obrigatório")
	// ErrTituloLongo é retornado quando o título passa de TamanhoMaximoTitulo caracteres.
	ErrTituloLongo = errors.New("título do card é longo demais")
	// ErrDescricaoLonga é retornado quando a descrição passa de TamanhoMaximoDescricao caracteres.
	ErrDescricaoLonga = errors.New("descrição do card é longa demais")
	// ErrNaoEncontrado é retornado quando o card não existe — ou quando quem
	// pergunta não participa do quadro dele.
	ErrNaoEncontrado = errors.New("card não encontrado")
	// ErrJaArquivado é retornado ao arquivar um card que já está arquivado.
	ErrJaArquivado = errors.New("o card já está arquivado")
	// ErrNaoArquivado é retornado ao desarquivar um card que não está arquivado.
	ErrNaoArquivado = errors.New("o card não está arquivado")
)

// Card é uma tarefa dentro de uma coluna.
//
// Version conta as edições, e é o contador do bloqueio otimista. Quem grava
// leva a versão que leu; se ela já não for a atual, a escrita é recusada com
// ErrConflito em vez de sobrescrever o trabalho alheio em silêncio. A regra é
// aplicada em dois lugares: no usecase (que pega o conflito lento, de quem
// abriu o card há cinco minutos) e no WHERE do UPDATE (que pega o simultâneo).
type Card struct {
	ID        string
	ColunaID  string
	Titulo    string
	Descricao string
	// Chave é a ordenação. Textual, e não numérica: entre duas chaves sempre
	// cabe outra, então arrastar para o mesmo ponto nunca esgota — o que a
	// posição em double precision fazia em 52 inserções.
	Chave   string
	Version int
	// Cor é opcional: card sem cor usa o visual padrão. Vira uma tarja na
	// lateral — as etiquetas já ocupam o topo do card.
	Cor cor.Cor
	// Prazo é nil quando o card não tem data de entrega — o caso normal.
	Prazo *time.Time
	// ArquivadoEm é nil enquanto o card está no quadro.
	//
	// É o instante, e não um booleano: responde tudo o que um booleano
	// responderia e ainda permite ordenar a tela de arquivados pelo mais
	// recente. O card arquivado guarda a Chave e a ColunaID que tinha — é o que
	// faz desarquivar devolvê-lo ao mesmo lugar, e não ao fim da coluna.
	ArquivadoEm  *time.Time
	CriadoEm     time.Time
	AtualizadoEm time.Time
}

// Arquivado informa se o card saiu do quadro.
func (c *Card) Arquivado() bool { return c.ArquivadoEm != nil }

// Arquivar tira o card do quadro sem apagá-lo. Erra com ErrJaArquivado se ele
// já estiver fora — o que acontece quando duas pessoas arquivam o mesmo card, e
// dizer isso é melhor que fingir que a segunda também arquivou.
func (c *Card) Arquivar() error {
	if c.Arquivado() {
		return ErrJaArquivado
	}
	agora := time.Now()
	c.ArquivadoEm = &agora
	c.Version++
	c.AtualizadoEm = agora
	return nil
}

// Desarquivar devolve o card ao quadro, na coluna e na posição em que estava.
// Erra com ErrNaoArquivado se ele não estiver arquivado.
func (c *Card) Desarquivar() error {
	if !c.Arquivado() {
		return ErrNaoArquivado
	}
	c.ArquivadoEm = nil
	c.Version++
	c.AtualizadoEm = time.Now()
	return nil
}

// DefinirPrazo marca a data de entrega do card. Passar nil limpa o prazo.
//
// Prazo no passado é aceito de propósito: registrar que algo venceu ontem é
// informação legítima, e recusar obrigaria a pessoa a mentir a data para
// conseguir cadastrar o atraso.
func (c *Card) DefinirPrazo(prazo *time.Time) {
	c.Prazo = prazo
	c.Version++
	c.AtualizadoEm = time.Now()
}

// Mover leva o card para uma coluna e uma chave de ordem. A coluna pode ser a
// mesma — reordenar dentro dela é o caso mais comum.
//
// A chave vem pronta: quem a calcula é quem enxerga os vizinhos (ver
// usecase/board e o pacote ordem). O card não tem como saber ao lado de quem
// foi solto.
func (c *Card) Mover(colunaID, chave string) {
	c.ColunaID = colunaID
	c.Chave = chave
	c.Version++
	c.AtualizadoEm = time.Now()
}

// Vencido informa se o prazo já passou em relação a agora. Card sem prazo
// nunca vence.
func (c *Card) Vencido(agora time.Time) bool {
	return c.Prazo != nil && agora.After(*c.Prazo)
}

// Novo cria um card na chave de ordem informada. A descrição é opcional; o
// título, não.
func Novo(id, colunaID, titulo, descricao string, cores cor.Cor, chave string) (*Card, error) {
	titulo, descricao, err := validar(titulo, descricao)
	if err != nil {
		return nil, err
	}
	if !cor.ValidaOuVazia(cores) {
		return nil, cor.ErrInvalida
	}

	agora := time.Now()
	return &Card{
		ID:           id,
		ColunaID:     colunaID,
		Titulo:       titulo,
		Descricao:    descricao,
		Cor:          cores,
		Chave:        chave,
		Version:      1,
		CriadoEm:     agora,
		AtualizadoEm: agora,
	}, nil
}

// Editar troca título, descrição e cor, incrementando a versão do card.
func (c *Card) Editar(titulo, descricao string, cores cor.Cor) error {
	titulo, descricao, err := validar(titulo, descricao)
	if err != nil {
		return err
	}
	if !cor.ValidaOuVazia(cores) {
		return cor.ErrInvalida
	}
	c.Titulo = titulo
	c.Descricao = descricao
	c.Cor = cores
	c.Version++
	c.AtualizadoEm = time.Now()
	return nil
}

func validar(titulo, descricao string) (string, string, error) {
	titulo = strings.TrimSpace(titulo)
	if titulo == "" {
		return "", "", ErrTituloObrigatorio
	}
	if utf8.RuneCountInString(titulo) > TamanhoMaximoTitulo {
		return "", "", ErrTituloLongo
	}
	// A descrição não é aparada: espaço e quebra de linha podem ser
	// intencionais num texto livre, ao contrário de um título.
	if utf8.RuneCountInString(descricao) > TamanhoMaximoDescricao {
		return "", "", ErrDescricaoLonga
	}
	return titulo, descricao, nil
}
