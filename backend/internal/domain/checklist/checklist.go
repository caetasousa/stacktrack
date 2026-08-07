// Package checklist modela as listas de verificação de um card. Plural de
// propósito: no template de desenvolvimento do Trello cada card tem duas —
// "To-do List" e "Task Review" —, e juntá-las numa só perderia a separação
// entre fazer e conferir.
package checklist

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// TamanhoMaximoTitulo é o limite de caracteres do título da checklist.
	TamanhoMaximoTitulo = 120
	// TamanhoMaximoTexto é o limite de caracteres de um item.
	TamanhoMaximoTexto = 500
	// PassoPosicao é o intervalo deixado entre posições ao acrescentar no fim.
	PassoPosicao = 1024.0
)

var (
	// ErrTituloObrigatorio é retornado quando o título está vazio ou só com espaços.
	ErrTituloObrigatorio = errors.New("título da checklist é obrigatório")
	// ErrTituloLongo é retornado quando o título passa do limite.
	ErrTituloLongo = errors.New("título da checklist é longo demais")
	// ErrTextoObrigatorio é retornado quando o texto do item está vazio.
	ErrTextoObrigatorio = errors.New("texto do item é obrigatório")
	// ErrTextoLongo é retornado quando o texto do item passa do limite.
	ErrTextoLongo = errors.New("texto do item é longo demais")
	// ErrNaoEncontrada é retornado quando a checklist não existe — ou quando
	// quem pergunta não participa do quadro dela.
	ErrNaoEncontrada = errors.New("checklist não encontrada")
	// ErrItemNaoEncontrado é retornado quando o item não existe.
	ErrItemNaoEncontrado = errors.New("item não encontrado")
)

// Checklist é uma lista de verificação dentro de um card.
type Checklist struct {
	ID           string
	CardID       string
	Titulo       string
	Posicao      float64
	CriadoEm     time.Time
	AtualizadoEm time.Time
}

// Item é uma linha marcável de uma checklist.
type Item struct {
	ID           string
	ChecklistID  string
	Texto        string
	Concluido    bool
	Posicao      float64
	CriadoEm     time.Time
	AtualizadoEm time.Time
}

// Nova cria uma checklist na posição informada.
func Nova(id, cardID, titulo string, posicao float64) (*Checklist, error) {
	titulo, err := validarTitulo(titulo)
	if err != nil {
		return nil, err
	}

	agora := time.Now()
	return &Checklist{
		ID: id, CardID: cardID, Titulo: titulo, Posicao: posicao,
		CriadoEm: agora, AtualizadoEm: agora,
	}, nil
}

// Renomear troca o título da checklist.
func (c *Checklist) Renomear(titulo string) error {
	titulo, err := validarTitulo(titulo)
	if err != nil {
		return err
	}
	c.Titulo = titulo
	c.AtualizadoEm = time.Now()
	return nil
}

// NovoItem cria um item desmarcado. Nascer desmarcado é decisão do domínio, e
// é por isso que a coluna no banco não tem DEFAULT.
func NovoItem(id, checklistID, texto string, posicao float64) (*Item, error) {
	texto, err := validarTexto(texto)
	if err != nil {
		return nil, err
	}

	agora := time.Now()
	return &Item{
		ID: id, ChecklistID: checklistID, Texto: texto, Concluido: false,
		Posicao: posicao, CriadoEm: agora, AtualizadoEm: agora,
	}, nil
}

// Editar troca o texto do item.
func (i *Item) Editar(texto string) error {
	texto, err := validarTexto(texto)
	if err != nil {
		return err
	}
	i.Texto = texto
	i.AtualizadoEm = time.Now()
	return nil
}

// Marcar define se o item está concluído.
func (i *Item) Marcar(concluido bool) {
	i.Concluido = concluido
	i.AtualizadoEm = time.Now()
}

// Progresso conta quantos itens estão concluídos, para o "2/5" do card.
func Progresso(itens []Item) (concluidos, total int) {
	for _, item := range itens {
		if item.Concluido {
			concluidos++
		}
	}
	return concluidos, len(itens)
}

// PosicaoNoFim devolve a posição de um item acrescentado depois do último.
func PosicaoNoFim(ultimaPosicao float64) float64 {
	return ultimaPosicao + PassoPosicao
}

func validarTitulo(titulo string) (string, error) {
	titulo = strings.TrimSpace(titulo)
	if titulo == "" {
		return "", ErrTituloObrigatorio
	}
	if utf8.RuneCountInString(titulo) > TamanhoMaximoTitulo {
		return "", ErrTituloLongo
	}
	return titulo, nil
}

func validarTexto(texto string) (string, error) {
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return "", ErrTextoObrigatorio
	}
	if utf8.RuneCountInString(texto) > TamanhoMaximoTexto {
		return "", ErrTextoLongo
	}
	return texto, nil
}
