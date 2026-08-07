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
	Fundo    string    `json:"fundo"`
	CriadoEm time.Time `json:"criadoEm"`
}

// ListaBoardsResponse envelopa a listagem num objeto em vez de devolver um
// array na raiz: assim cabe acrescentar paginação ou totais depois sem quebrar
// quem já consome.
type ListaBoardsResponse struct {
	Boards []BoardResponse `json:"boards"`
}

// CardResponse é um card na tela do quadro. Version viaja para o cliente
// porque a partir da fase 6 ela volta no update, como prova de qual versão a
// pessoa estava vendo.
//
// Etiquetas, Checklist e QtdAnexos são o RESUMO que o card mostra como selo —
// vêm juntos com o quadro para a tela não precisar abrir card por card.
type CardResponse struct {
	ID        string  `json:"id"`
	ColunaID  string  `json:"colunaId"`
	Titulo    string  `json:"titulo"`
	Descricao string  `json:"descricao"`
	Posicao   float64 `json:"posicao"`
	Version   int     `json:"version"`
	// Prazo é nulo quando o card não tem data de entrega.
	Prazo *time.Time `json:"prazo"`
	// Vencido é calculado pelo servidor: o relógio do navegador pode estar
	// errado, e um card vermelho por engano confunde mais do que ajuda.
	Vencido   bool              `json:"vencido"`
	Etiquetas []string          `json:"etiquetas"`
	Checklist ProgressoResponse `json:"checklist"`
	QtdAnexos int               `json:"qtdAnexos"`
}

// ProgressoResponse é o "2/5" de checklist mostrado no card.
type ProgressoResponse struct {
	Concluidos int `json:"concluidos"`
	Total      int `json:"total"`
}

// EtiquetaResponse é uma etiqueta do quadro. Cor é o NOME da cor na paleta
// ('vermelho'), não o hex: a paleta muda com o tema, e o hex é do CSS.
type EtiquetaResponse struct {
	ID      string  `json:"id"`
	Nome    string  `json:"nome"`
	Cor     string  `json:"cor"`
	Posicao float64 `json:"posicao"`
}

// ChecklistResponse é uma lista de verificação com as linhas dela.
type ChecklistResponse struct {
	ID      string                  `json:"id"`
	CardID  string                  `json:"cardId"`
	Titulo  string                  `json:"titulo"`
	Posicao float64                 `json:"posicao"`
	Itens   []ChecklistItemResponse `json:"itens"`
}

// ChecklistItemResponse é uma linha marcável.
type ChecklistItemResponse struct {
	ID          string  `json:"id"`
	ChecklistID string  `json:"checklistId"`
	Texto       string  `json:"texto"`
	Concluido   bool    `json:"concluido"`
	Posicao     float64 `json:"posicao"`
}

// AnexoResponse é um arquivo ou link pendurado no card.
//
// Arquivo não expõe o caminho no disco: o download passa por /anexos/{id}, que
// confere participação no quadro antes de entregar o conteúdo.
type AnexoResponse struct {
	ID       string    `json:"id"`
	CardID   string    `json:"cardId"`
	Tipo     string    `json:"tipo"`
	Nome     string    `json:"nome"`
	URL      string    `json:"url,omitempty"`
	Tamanho  int64     `json:"tamanho,omitempty"`
	MIME     string    `json:"mime,omitempty"`
	CriadoEm time.Time `json:"criadoEm"`
}

// CardDetalhadoResponse é o que o modal do card mostra.
type CardDetalhadoResponse struct {
	CardResponse
	BoardID         string              `json:"boardId"`
	EtiquetasDoCard []EtiquetaResponse  `json:"etiquetasDoCard"`
	Checklists      []ChecklistResponse `json:"checklists"`
	Anexos          []AnexoResponse     `json:"anexos"`
}

// PrazoRequest é o corpo de marcar ou limpar o prazo. Nulo limpa.
type PrazoRequest struct {
	Prazo *time.Time `json:"prazo"`
}

// Validar não tem o que checar: qualquer data é aceita, inclusive no passado —
// registrar que algo venceu ontem é informação legítima.
func (r PrazoRequest) Validar() error { return nil }

// EtiquetaRequest é o corpo de criar e editar etiqueta.
type EtiquetaRequest struct {
	Nome string `json:"nome" validate:"required,max=60"`
	Cor  string `json:"cor" validate:"required,oneof=cinza vermelho laranja amarelo verde azul roxo rosa"`
}

// Validar checa o formato dos campos informados.
func (r EtiquetaRequest) Validar() error { return validate.Struct(r) }

// ChecklistRequest é o corpo de criar e renomear checklist.
type ChecklistRequest struct {
	Titulo string `json:"titulo" validate:"required,max=120"`
}

// Validar checa o formato dos campos informados.
func (r ChecklistRequest) Validar() error { return validate.Struct(r) }

// ItemRequest é o corpo de criar um item de checklist.
type ItemRequest struct {
	Texto string `json:"texto" validate:"required,max=500"`
}

// Validar checa o formato dos campos informados.
func (r ItemRequest) Validar() error { return validate.Struct(r) }

// ItemPatchRequest edita um item. Os dois campos são opcionais: marcar uma
// caixa não pode exigir reenviar o texto, e renomear não pode desmarcar sem
// querer.
type ItemPatchRequest struct {
	Texto     *string `json:"texto" validate:"omitempty,max=500"`
	Concluido *bool   `json:"concluido"`
}

// Validar checa o formato dos campos informados.
func (r ItemPatchRequest) Validar() error { return validate.Struct(r) }

// LinkRequest é o corpo de anexar um link ao card.
type LinkRequest struct {
	URL  string `json:"url" validate:"required,url,max=2000"`
	Nome string `json:"nome" validate:"max=255"`
}

// Validar checa o formato dos campos informados.
func (r LinkRequest) Validar() error { return validate.Struct(r) }

// FundoRequest é o corpo de trocar o fundo do quadro.
type FundoRequest struct {
	Fundo string `json:"fundo" validate:"required,oneof=padrao ardosia oceano floresta ameixa brasa"`
}

// Validar checa o formato dos campos informados.
func (r FundoRequest) Validar() error { return validate.Struct(r) }

// ColunaResponse é uma coluna com os cards dela, já em ordem.
type ColunaResponse struct {
	ID      string         `json:"id"`
	BoardID string         `json:"boardId"`
	Titulo  string         `json:"titulo"`
	Posicao float64        `json:"posicao"`
	Cards   []CardResponse `json:"cards"`
}

// BoardDetalhadoResponse é o quadro inteiro: o que a tela do quadro precisa
// para renderizar, numa requisição só — inclusive as etiquetas disponíveis,
// porque o card só carrega os ids delas.
type BoardDetalhadoResponse struct {
	ID        string             `json:"id"`
	Titulo    string             `json:"titulo"`
	Papel     string             `json:"papel"`
	Fundo     string             `json:"fundo"`
	Colunas   []ColunaResponse   `json:"colunas"`
	Etiquetas []EtiquetaResponse `json:"etiquetas"`
}
