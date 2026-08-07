// Package etiqueta modela as marcações coloridas do quadro — "Flagged",
// "Aguardando revisão", "Em produção". A etiqueta pertence ao QUADRO, e não ao
// card: a mesma marcação aparece em vários cards, e renomeá-la precisa valer
// para todos de uma vez.
package etiqueta

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// TamanhoMaximoNome é o limite de caracteres do nome da etiqueta.
const TamanhoMaximoNome = 60

// Cor é o nome de uma cor da paleta — e não um hex.
//
// O banco guarda 'vermelho', não '#F04438': a paleta é decisão de design e
// muda com o tema claro/escuro. Gravar o hex congelaria no dado uma escolha
// que pertence ao CSS.
type Cor string

const (
	CorCinza    Cor = "cinza"
	CorVermelho Cor = "vermelho"
	CorLaranja  Cor = "laranja"
	CorAmarelo  Cor = "amarelo"
	CorVerde    Cor = "verde"
	CorAzul     Cor = "azul"
	CorRoxo     Cor = "roxo"
	CorRosa     Cor = "rosa"
)

// Cores é a paleta inteira, na ordem em que a tela deve oferecê-la.
var Cores = []Cor{CorCinza, CorVermelho, CorLaranja, CorAmarelo, CorVerde, CorAzul, CorRoxo, CorRosa}

var (
	// ErrNomeObrigatorio é retornado quando o nome está vazio ou só com espaços.
	ErrNomeObrigatorio = errors.New("nome da etiqueta é obrigatório")
	// ErrNomeLongo é retornado quando o nome passa de TamanhoMaximoNome caracteres.
	ErrNomeLongo = errors.New("nome da etiqueta é longo demais")
	// ErrCorInvalida é retornado quando a cor não está na paleta.
	ErrCorInvalida = errors.New("cor de etiqueta inválida")
	// ErrNaoEncontrada é retornado quando a etiqueta não existe — ou quando
	// quem pergunta não participa do quadro dela.
	ErrNaoEncontrada = errors.New("etiqueta não encontrada")
)

// CorValida informa se a cor pertence à paleta.
func CorValida(c Cor) bool {
	for _, conhecida := range Cores {
		if c == conhecida {
			return true
		}
	}
	return false
}

// Etiqueta é uma marcação nomeada e colorida do quadro.
type Etiqueta struct {
	ID       string
	BoardID  string
	Nome     string
	Cor      Cor
	Posicao  float64
	CriadoEm time.Time
}

// Nova cria uma etiqueta. O nome pode ser vazio no Trello, mas aqui não: uma
// etiqueta só de cor obriga quem lê a decorar o que cada cor significa.
func Nova(id, boardID, nome string, cor Cor, posicao float64) (*Etiqueta, error) {
	nome, err := validarNome(nome)
	if err != nil {
		return nil, err
	}
	if !CorValida(cor) {
		return nil, ErrCorInvalida
	}

	return &Etiqueta{
		ID:       id,
		BoardID:  boardID,
		Nome:     nome,
		Cor:      cor,
		Posicao:  posicao,
		CriadoEm: time.Now(),
	}, nil
}

// Editar troca nome e cor da etiqueta, valendo para todos os cards que a usam.
func (e *Etiqueta) Editar(nome string, cor Cor) error {
	nome, err := validarNome(nome)
	if err != nil {
		return err
	}
	if !CorValida(cor) {
		return ErrCorInvalida
	}
	e.Nome, e.Cor = nome, cor
	return nil
}

func validarNome(nome string) (string, error) {
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return "", ErrNomeObrigatorio
	}
	if utf8.RuneCountInString(nome) > TamanhoMaximoNome {
		return "", ErrNomeLongo
	}
	return nome, nil
}
