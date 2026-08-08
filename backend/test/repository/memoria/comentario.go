package memoria

import (
	"context"
	"sort"

	"stacktrack/internal/domain/comentario"
	ucboard "stacktrack/internal/usecase/board"
)

// Comentarios é a conversa dos cards em memória.
type Comentarios struct {
	porID map[string]*comentario.Comentario
	// usuarios resolve o nome de quem escreveu, como o JOIN do SQL.
	usuarios *Usuarios
	// colunas e cards resolvem a que quadro um card pertence.
	colunas     *Colunas
	cards       *Cards
	ErroForcado error
}

// NovosComentarios cria o repositório em memória vazio.
func NovosComentarios() *Comentarios {
	return &Comentarios{porID: make(map[string]*comentario.Comentario)}
}

// LigarQuadro dá acesso a colunas e cards, para as consultas por quadro.
func (r *Comentarios) LigarQuadro(colunas *Colunas, cards *Cards) {
	r.colunas, r.cards = colunas, cards
}

// LigarUsuarios dá acesso às contas, para o comentário sair com o nome do autor.
func (r *Comentarios) LigarUsuarios(usuarios *Usuarios) { r.usuarios = usuarios }

func (r *Comentarios) Salvar(ctx context.Context, c *comentario.Comentario) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	copia := *c
	r.porID[c.ID] = &copia
	return nil
}

func (r *Comentarios) Atualizar(ctx context.Context, c *comentario.Comentario) error {
	return r.Salvar(ctx, c)
}

func (r *Comentarios) BuscarPorID(ctx context.Context, id string) (*comentario.Comentario, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	c, existe := r.porID[id]
	if !existe {
		return nil, nil
	}
	copia := *c
	return &copia, nil
}

func (r *Comentarios) Apagar(ctx context.Context, id string) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	delete(r.porID, id)
	return nil
}

func (r *Comentarios) ListarDoCard(ctx context.Context, cardID string) ([]ucboard.ComentarioComAutor, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	lista := make([]ucboard.ComentarioComAutor, 0)
	for _, c := range r.porID {
		if c.CardID != cardID {
			continue
		}
		item := ucboard.ComentarioComAutor{Comentario: *c}
		if r.usuarios != nil {
			if u, _ := r.usuarios.BuscarPorID(ctx, c.AutorID); u != nil {
				item.AutorNome = u.Nome
			}
		}
		lista = append(lista, item)
	}
	// Do mais antigo para o mais novo, como o ORDER BY do SQL. Sem isto a ordem
	// viria do mapa, que em Go é aleatória.
	sort.Slice(lista, func(i, j int) bool {
		if !lista[i].Comentario.CriadoEm.Equal(lista[j].Comentario.CriadoEm) {
			return lista[i].Comentario.CriadoEm.Before(lista[j].Comentario.CriadoEm)
		}
		return lista[i].Comentario.ID < lista[j].Comentario.ID
	})
	return lista, nil
}

func (r *Comentarios) ContarPorCardDoBoard(ctx context.Context, boardID string) (map[string]int, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	porCard := make(map[string]int)
	for _, c := range r.porID {
		if r.boardDoCard(ctx, c.CardID) == boardID {
			porCard[c.CardID]++
		}
	}
	return porCard, nil
}

func (r *Comentarios) boardDoCard(ctx context.Context, cardID string) string {
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
