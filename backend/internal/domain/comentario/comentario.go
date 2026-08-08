// Package comentario modela a conversa de um card.
//
// É o primeiro fluxo append-only do projeto: um comentário acontece e fica. Não
// tem posição, não se reordena, e a ordem é a do tempo — por isso nada aqui
// lembra o pacote ordem.
package comentario

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// TamanhoMaximoTexto é o limite de caracteres de um comentário.
//
// Menor que o da descrição do card (5000) de propósito: a descrição é o
// documento do card, e o comentário é um recado. Um limite folgado demais
// convidaria a escrever ali o que pertence à descrição.
const TamanhoMaximoTexto = 2000

var (
	// ErrTextoObrigatorio é retornado quando o texto está vazio ou só com espaços.
	ErrTextoObrigatorio = errors.New("o comentário não pode ficar vazio")
	// ErrTextoLongo é retornado quando o texto passa de TamanhoMaximoTexto caracteres.
	ErrTextoLongo = errors.New("o comentário é longo demais")
	// ErrNaoEncontrado é retornado quando o comentário não existe — ou quando
	// quem pergunta não participa do quadro dele.
	ErrNaoEncontrado = errors.New("comentário não encontrado")
	// ErrNaoEhAutor é retornado quando alguém tenta mexer no comentário de
	// outra pessoa.
	//
	// Editar é diferente de apagar de propósito: qualquer um pode apagar o
	// próprio, o dono do quadro pode apagar o de qualquer um (é dele a
	// responsabilidade pelo que fica no quadro), mas EDITAR é só do autor —
	// ninguém pode pôr palavras na boca de outra pessoa.
	ErrNaoEhAutor = errors.New("só o autor pode editar o próprio comentário")
)

// Comentario é uma mensagem de alguém num card.
type Comentario struct {
	ID       string
	CardID   string
	AutorID  string
	Texto    string
	CriadoEm time.Time
	// EditadoEm é nil enquanto o comentário nunca foi editado. É o que deixa a
	// tela dizer "editado" sem comparar datas que sempre diferem.
	EditadoEm *time.Time
}

// Novo cria um comentário validado.
func Novo(id, cardID, autorID, texto string) (*Comentario, error) {
	texto, err := validar(texto)
	if err != nil {
		return nil, err
	}
	return &Comentario{
		ID: id, CardID: cardID, AutorID: autorID, Texto: texto,
		CriadoEm: time.Now(),
	}, nil
}

// Editar troca o texto e marca quando isso aconteceu.
func (c *Comentario) Editar(texto string) error {
	texto, err := validar(texto)
	if err != nil {
		return err
	}
	c.Texto = texto
	agora := time.Now()
	c.EditadoEm = &agora
	return nil
}

// EhAutor informa se o usuário escreveu este comentário.
func (c *Comentario) EhAutor(usuarioID string) bool {
	return c.AutorID == usuarioID
}

// validar apara e confere o texto.
//
// O texto é aparado nas pontas, ao contrário da descrição do card: um
// comentário que é só espaço não é comentário, e sobra em branco no fim vem de
// copiar e colar, não de intenção.
func validar(texto string) (string, error) {
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return "", ErrTextoObrigatorio
	}
	if utf8.RuneCountInString(texto) > TamanhoMaximoTexto {
		return "", ErrTextoLongo
	}
	return texto, nil
}
