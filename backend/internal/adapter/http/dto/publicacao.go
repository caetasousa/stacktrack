package dto

import "time"

// PublicacaoResponse é o estado do link público de um quadro, como o dono o vê.
//
// A URL vem montada pelo servidor, e não o token cru: o link é para copiar e
// mandar, e montá-lo no cliente espalharia por lá o conhecimento de onde a
// página pública mora. É o mesmo caminho do link de convite.
type PublicacaoResponse struct {
	Publicado bool   `json:"publicado"`
	URL       string `json:"url,omitempty"`
	// CriadoEm é nulo quando o quadro não está publicado. Vai junto para a tela
	// poder dizer desde quando o link existe — "publicado" sem data não deixa
	// ninguém perceber um link esquecido aberto há meses.
	CriadoEm *time.Time `json:"criadoEm,omitempty"`
}

// QuadroPublicoResponse é o quadro servido a quem chega pelo link, sem sessão.
//
// Espelha ucboard.QuadroPublico — inclusive nas ausências, que são o motivo de
// ele existir: sem responsáveis, sem comentários, sem anexos, sem histórico,
// sem membros e sem ids. Ver o comentário daquele tipo para o porquê de cada
// uma.
type QuadroPublicoResponse struct {
	Titulo       string                  `json:"titulo"`
	Fundo        string                  `json:"fundo"`
	AtualizadoEm time.Time               `json:"atualizadoEm"`
	Colunas      []ColunaPublicaResponse `json:"colunas"`
}

// ColunaPublicaResponse é uma etapa do fluxo com os cards dela, em ordem.
type ColunaPublicaResponse struct {
	Titulo string                `json:"titulo"`
	Cor    string                `json:"cor"`
	Cards  []CardPublicoResponse `json:"cards"`
}

// CardPublicoResponse é uma tarefa como quem acompanha de fora a enxerga.
type CardPublicoResponse struct {
	Titulo    string     `json:"titulo"`
	Descricao string     `json:"descricao"`
	Cor       string     `json:"cor"`
	Prazo     *time.Time `json:"prazo"`
	Vencido   bool       `json:"vencido"`
	// As etiquetas vêm resolvidas em nome e cor: não há lista de etiquetas do
	// quadro para cruzar, porque não há id nenhum nesta resposta.
	Etiquetas []EtiquetaPublicaResponse `json:"etiquetas"`
	Checklist ProgressoResponse         `json:"checklist"`
}

// EtiquetaPublicaResponse é o rótulo de um card: só o que se lê.
type EtiquetaPublicaResponse struct {
	Nome string `json:"nome"`
	Cor  string `json:"cor"`
}
