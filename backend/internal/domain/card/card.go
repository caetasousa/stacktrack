// Package card modela a tarefa que caminha pelo quadro. Um card pertence a uma
// coluna, e mudar de coluna é como ele "anda" pelo fluxo.
package card

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// TamanhoMaximoTitulo é o limite de caracteres do título do card.
	TamanhoMaximoTitulo = 200
	// TamanhoMaximoDescricao é o limite de caracteres da descrição.
	TamanhoMaximoDescricao = 5000
)

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
)

// Card é uma tarefa dentro de uma coluna.
//
// Version conta as edições. Ninguém a confere ainda: ela é o contador do
// bloqueio otimista da fase 6, quando duas pessoas editarem o mesmo card ao
// mesmo tempo e a segunda precisar receber 409 em vez de sobrescrever a
// primeira em silêncio.
type Card struct {
	ID        string
	ColunaID  string
	Titulo    string
	Descricao string
	Posicao   float64
	Version   int
	// Prazo é nil quando o card não tem data de entrega — o caso normal.
	Prazo        *time.Time
	CriadoEm     time.Time
	AtualizadoEm time.Time
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

// Mover leva o card para uma coluna e uma posição. A coluna pode ser a mesma —
// reordenar dentro dela é o caso mais comum.
//
// A posição vem pronta: quem a calcula é quem enxerga os vizinhos (ver
// usecase/board e o pacote ordem). O card não tem como saber ao lado de quem
// foi solto.
func (c *Card) Mover(colunaID string, posicao float64) {
	c.ColunaID = colunaID
	c.Posicao = posicao
	c.Version++
	c.AtualizadoEm = time.Now()
}

// Vencido informa se o prazo já passou em relação a agora. Card sem prazo
// nunca vence.
func (c *Card) Vencido(agora time.Time) bool {
	return c.Prazo != nil && agora.After(*c.Prazo)
}

// Novo cria um card na posição informada. A descrição é opcional; o título,
// não.
func Novo(id, colunaID, titulo, descricao string, posicao float64) (*Card, error) {
	titulo, descricao, err := validar(titulo, descricao)
	if err != nil {
		return nil, err
	}

	agora := time.Now()
	return &Card{
		ID:           id,
		ColunaID:     colunaID,
		Titulo:       titulo,
		Descricao:    descricao,
		Posicao:      posicao,
		Version:      1,
		CriadoEm:     agora,
		AtualizadoEm: agora,
	}, nil
}

// Editar troca título e descrição, incrementando a versão do card.
func (c *Card) Editar(titulo, descricao string) error {
	titulo, descricao, err := validar(titulo, descricao)
	if err != nil {
		return err
	}
	c.Titulo = titulo
	c.Descricao = descricao
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
