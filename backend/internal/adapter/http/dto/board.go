package dto

import "time"

// TituloRequest é o corpo de criar e renomear quadro e coluna — os três usam a
// mesma régua de título.
type TituloRequest struct {
	Titulo string `json:"titulo" validate:"required,max=120"`
}

// Validar checa o formato dos campos informados.
func (r TituloRequest) Validar() error {
	return validate.Struct(r)
}

// CardRequest é o corpo de criar e editar card. A descrição é opcional.
type CardRequest struct {
	Titulo    string `json:"titulo" validate:"required,max=200"`
	Descricao string `json:"descricao" validate:"max=5000"`
}

// Validar checa o formato dos campos informados.
func (r CardRequest) Validar() error {
	return validate.Struct(r)
}

// BoardResponse é um quadro na listagem. Papel é o papel de quem pediu — o
// frontend usa isso para decidir o que mostrar, mas quem decide o que PODE é
// sempre o servidor.
type BoardResponse struct {
	ID       string    `json:"id"`
	Titulo   string    `json:"titulo"`
	Papel    string    `json:"papel"`
	CriadoEm time.Time `json:"criadoEm"`
}

// ListaBoardsResponse envelopa a listagem num objeto em vez de devolver um
// array na raiz: assim cabe acrescentar paginação ou totais depois sem quebrar
// quem já consome.
type ListaBoardsResponse struct {
	Boards []BoardResponse `json:"boards"`
}

// CardResponse é um card. Version viaja para o cliente porque a partir da fase
// 6 ela volta no update, como prova de qual versão a pessoa estava vendo.
type CardResponse struct {
	ID        string  `json:"id"`
	ColunaID  string  `json:"colunaId"`
	Titulo    string  `json:"titulo"`
	Descricao string  `json:"descricao"`
	Posicao   float64 `json:"posicao"`
	Version   int     `json:"version"`
}

// ColunaResponse é uma coluna com os cards dela, já em ordem.
type ColunaResponse struct {
	ID      string         `json:"id"`
	BoardID string         `json:"boardId"`
	Titulo  string         `json:"titulo"`
	Posicao float64        `json:"posicao"`
	Cards   []CardResponse `json:"cards"`
}

// BoardDetalhadoResponse é o quadro inteiro: o que a tela do quadro precisa
// para renderizar, numa requisição só.
type BoardDetalhadoResponse struct {
	ID      string           `json:"id"`
	Titulo  string           `json:"titulo"`
	Papel   string           `json:"papel"`
	Colunas []ColunaResponse `json:"colunas"`
}
