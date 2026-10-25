package dto

import "time"

// NotificacaoResponse é um aviso no sino.
//
// Traz o que a tela precisa para montar a frase ("Ana pôs você como
// responsável por Migrar o banco") e para levar a pessoa ao lugar certo — o
// quadro, o card ou os convites. A frase em si é montada na tela, como a do
// histórico: a API diz o que aconteceu, e a redação é apresentação.
type NotificacaoResponse struct {
	ID       string `json:"id"`
	Tipo     string `json:"tipo"`
	QuadroID string `json:"quadroId"`
	Quadro   string `json:"quadro"`
	CardID   string `json:"cardId,omitempty"`
	Card     string `json:"card,omitempty"`
	// Autor é vazio no aviso de prazo, que não tem pessoa por trás.
	Autor string `json:"autor,omitempty"`
	// Trecho é o começo do comentário, na menção.
	Trecho string `json:"trecho,omitempty"`
	// Prazo é a data que motivou o aviso de prazo.
	Prazo    *time.Time `json:"prazo,omitempty"`
	CriadaEm time.Time  `json:"criadaEm"`
	Lida     bool       `json:"lida"`
	// Situacao diz se o aviso ainda vale: "vigente", "encerrada" (a
	// atribuição desfeita, o prazo trocado) ou o desfecho do convite —
	// "aceito", "recusado", "revogado", "vencido".
	Situacao string `json:"situacao"`
}

// NotificacoesResponse é uma página do sino, com o número de não lidas da
// MESMA leitura: a lista e o número nunca descrevem instantes diferentes.
type NotificacoesResponse struct {
	Notificacoes []NotificacaoResponse `json:"notificacoes"`
	NaoLidas     int                   `json:"naoLidas"`
	TemMais      bool                  `json:"temMais"`
}
