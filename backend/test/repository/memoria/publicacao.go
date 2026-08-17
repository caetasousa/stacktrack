package memoria

import (
	"context"

	"stacktrack/internal/domain/publicacao"
)

// Publicacoes guarda os links públicos de quadro em memória.
type Publicacoes struct {
	porBoard    map[string]*publicacao.Publicacao
	ErroForcado error
}

// NovasPublicacoes cria o repositório em memória vazio.
func NovasPublicacoes() *Publicacoes {
	return &Publicacoes{porBoard: make(map[string]*publicacao.Publicacao)}
}

func (r *Publicacoes) Salvar(ctx context.Context, p *publicacao.Publicacao) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	copia := *p
	r.porBoard[p.BoardID] = &copia
	return nil
}

func (r *Publicacoes) BuscarPorBoard(ctx context.Context, boardID string) (*publicacao.Publicacao, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	p, ok := r.porBoard[boardID]
	if !ok {
		return nil, nil
	}
	copia := *p
	return &copia, nil
}

// BuscarPorToken varre o mapa em vez de manter um índice: são poucos quadros
// num teste, e um segundo mapa a sincronizar seria uma fonte de divergência
// entre o que Salvar grava e o que a busca acha.
func (r *Publicacoes) BuscarPorToken(ctx context.Context, token string) (*publicacao.Publicacao, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	// Token vazio não casa com nada, nem por engano: sem esta guarda, um mapa
	// com uma publicação de token "" faria o caminho público abrir um quadro
	// para quem não apresentou nada.
	if token == "" {
		return nil, nil
	}
	for _, p := range r.porBoard {
		if p.Token == token {
			copia := *p
			return &copia, nil
		}
	}
	return nil, nil
}

func (r *Publicacoes) Remover(ctx context.Context, boardID string) error {
	delete(r.porBoard, boardID)
	return nil
}
