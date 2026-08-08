package memoria

import (
	"context"
	"sort"

	ucboard "stacktrack/internal/usecase/board"
)

// Responsaveis é o repositório de atribuições em memória.
//
// Guarda um conjunto por card, e não uma lista: é a forma que reproduz a chave
// primária composta do banco — atribuir duas vezes leva ao mesmo estado, sem o
// fake precisar de nenhuma checagem para isso.
type Responsaveis struct {
	// porCard[cardID] é o conjunto de usuários responsáveis por aquele card.
	porCard map[string]map[string]bool
	// usuarios resolve o id em nome de exibição, como o JOIN com usuarios faz
	// no repositório de verdade.
	usuarios *Usuarios
	// colunas e cards resolvem a que quadro um card pertence, como o JOIN do SQL.
	colunas     *Colunas
	cards       *Cards
	ErroForcado error
}

// NovosResponsaveis cria o repositório em memória vazio.
func NovosResponsaveis() *Responsaveis {
	return &Responsaveis{porCard: make(map[string]map[string]bool)}
}

// LigarQuadro dá acesso a colunas e cards, para as consultas por quadro.
func (r *Responsaveis) LigarQuadro(colunas *Colunas, cards *Cards) {
	r.colunas, r.cards = colunas, cards
}

// LigarUsuarios dá acesso às contas, para os responsáveis saírem com nome.
func (r *Responsaveis) LigarUsuarios(usuarios *Usuarios) { r.usuarios = usuarios }

func (r *Responsaveis) Atribuir(ctx context.Context, cardID, usuarioID string) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	if r.porCard[cardID] == nil {
		r.porCard[cardID] = make(map[string]bool)
	}
	r.porCard[cardID][usuarioID] = true
	return nil
}

func (r *Responsaveis) Remover(ctx context.Context, cardID, usuarioID string) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	delete(r.porCard[cardID], usuarioID)
	return nil
}

func (r *Responsaveis) RemoverDoBoard(ctx context.Context, boardID, usuarioID string) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	for cardID := range r.porCard {
		if r.boardDoCard(ctx, cardID) == boardID {
			delete(r.porCard[cardID], usuarioID)
		}
	}
	return nil
}

func (r *Responsaveis) DoCard(ctx context.Context, cardID string) ([]ucboard.Responsavel, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	return r.montar(ctx, r.porCard[cardID]), nil
}

func (r *Responsaveis) DoBoardPorCard(ctx context.Context, boardID string) (map[string][]ucboard.Responsavel, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	fora := make(map[string][]ucboard.Responsavel)
	for cardID, conjunto := range r.porCard {
		if len(conjunto) == 0 || r.boardDoCard(ctx, cardID) != boardID {
			continue
		}
		fora[cardID] = r.montar(ctx, conjunto)
	}
	return fora, nil
}

// montar converte o conjunto em lista ordenada por nome, como o ORDER BY do SQL
// — sem isso a ordem viria do mapa, que em Go é aleatória, e um teste que
// compara listas falharia de vez em quando.
func (r *Responsaveis) montar(ctx context.Context, conjunto map[string]bool) []ucboard.Responsavel {
	lista := make([]ucboard.Responsavel, 0, len(conjunto))
	for id := range conjunto {
		p := ucboard.Responsavel{UsuarioID: id}
		if r.usuarios != nil {
			if u, _ := r.usuarios.BuscarPorID(ctx, id); u != nil {
				p.Nome = u.Nome
			}
		}
		lista = append(lista, p)
	}
	sort.Slice(lista, func(i, j int) bool {
		if lista[i].Nome != lista[j].Nome {
			return lista[i].Nome < lista[j].Nome
		}
		return lista[i].UsuarioID < lista[j].UsuarioID
	})
	return lista
}

func (r *Responsaveis) boardDoCard(ctx context.Context, cardID string) string {
	if r.cards == nil || r.colunas == nil {
		return ""
	}
	c, _ := r.cards.BuscarPorID(ctx, cardID)
	if c == nil {
		return ""
	}
	col, _ := r.colunas.BuscarPorID(ctx, c.ColunaID)
	if col == nil {
		return ""
	}
	return col.BoardID
}
