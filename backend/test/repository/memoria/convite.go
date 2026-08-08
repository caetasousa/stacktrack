package memoria

import (
	"context"
	"sort"
	"time"

	"stacktrack/internal/domain/convite"
	"stacktrack/internal/domain/usuario"
)

// Convites guarda convites de quadro em memória.
type Convites struct {
	porID       map[string]*convite.Convite
	ErroForcado error
}

// NovosConvites cria o repositório em memória vazio.
func NovosConvites() *Convites {
	return &Convites{porID: make(map[string]*convite.Convite)}
}

// Salvar grava o convite, devolvendo convite.ErrJaConvidado quando já existe um
// pendente para o mesmo email no mesmo quadro — o comportamento do índice único
// parcial no Postgres.
func (r *Convites) Salvar(ctx context.Context, c *convite.Convite) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	for _, existente := range r.porID {
		if existente.ID != c.ID &&
			existente.BoardID == c.BoardID &&
			existente.Email == c.Email &&
			existente.AceitoEm == nil {
			return convite.ErrJaConvidado
		}
	}
	copia := *c
	r.porID[c.ID] = &copia
	return nil
}

func (r *Convites) Atualizar(ctx context.Context, c *convite.Convite) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	copia := *c
	r.porID[c.ID] = &copia
	return nil
}

func (r *Convites) BuscarPorID(ctx context.Context, id string) (*convite.Convite, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	c, ok := r.porID[id]
	if !ok {
		return nil, nil
	}
	copia := *c
	return &copia, nil
}

func (r *Convites) BuscarPorTokenHash(ctx context.Context, hash string) (*convite.Convite, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	for _, c := range r.porID {
		if c.TokenHash == hash {
			copia := *c
			return &copia, nil
		}
	}
	return nil, nil
}

func (r *Convites) BuscarPendentePorEmail(ctx context.Context, boardID, email string) (*convite.Convite, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	alvo := usuario.NormalizarEmail(email)
	for _, c := range r.porID {
		if c.BoardID == boardID && c.Email == alvo && c.AceitoEm == nil {
			copia := *c
			return &copia, nil
		}
	}
	return nil, nil
}

func (r *Convites) ListarPendentes(ctx context.Context, boardID string) ([]convite.Convite, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	lista := make([]convite.Convite, 0)
	for _, c := range r.porID {
		if c.BoardID == boardID && c.AceitoEm == nil {
			lista = append(lista, *c)
		}
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].CriadoEm.After(lista[j].CriadoEm) })
	return lista, nil
}

func (r *Convites) Remover(ctx context.Context, id string) error {
	delete(r.porID, id)
	return nil
}

// Vencer empurra o convite para o passado, para os testes exercitarem o
// caminho do link expirado sem esperar sete dias.
func (r *Convites) Vencer(id string) {
	if c, ok := r.porID[id]; ok {
		c.ExpiraEm = time.Now().Add(-time.Minute)
	}
}

// Quantidade informa quantos convites existem — atalho para os testes.
func (r *Convites) Quantidade() int { return len(r.porID) }
