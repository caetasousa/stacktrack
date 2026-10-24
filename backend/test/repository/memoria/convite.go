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
			existente.AceitoEm == nil && existente.RevogadoEm == nil {
			return convite.ErrJaConvidado
		}
	}
	copia := *c
	r.porID[c.ID] = &copia
	return nil
}

// Aceitar e Revogar espelham os UPDATEs CONDICIONAIS do Postgres: a transição
// só acontece se o convite ainda estiver no estado de partida, e quem chega
// depois recebe convite.ErrJaResolvido.
//
// Reproduzir a condição aqui é o que faz este repositório servir de teste: um
// `Atualizar(c)` que gravasse o agregado inteiro esconderia justamente a
// corrida que o WHERE existe para resolver, e o teste passaria enquanto o
// Postgres reprovaria.
func (r *Convites) Aceitar(ctx context.Context, id string, em time.Time) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	c, ok := r.porID[id]
	if !ok || c.AceitoEm != nil || c.RevogadoEm != nil || em.After(c.ExpiraEm) {
		return convite.ErrJaResolvido
	}
	quando := em
	c.AceitoEm = &quando
	return nil
}

func (r *Convites) Revogar(ctx context.Context, id string, em time.Time) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	c, ok := r.porID[id]
	if !ok || c.AceitoEm != nil || c.RevogadoEm != nil {
		return convite.ErrJaResolvido
	}
	quando := em
	c.RevogadoEm = &quando
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
		if c.BoardID == boardID && c.Email == alvo && c.AceitoEm == nil && c.RevogadoEm == nil {
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
		if c.BoardID == boardID && c.AceitoEm == nil && c.RevogadoEm == nil {
			lista = append(lista, *c)
		}
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].CriadoEm.After(lista[j].CriadoEm) })
	return lista, nil
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

// ContarPendentes e Renovar existem só para este fake continuar satisfazendo a
// porta: ele é legado (ver "Testes" no CLAUDE.md), e teste novo de convite roda
// contra o Postgres de verdade, em test/repository/convite_email_test.go.

// ContarPendentes conta os convites sem aceite nem revogação do quadro.
func (r *Convites) ContarPendentes(ctx context.Context, boardID string) (int, error) {
	n := 0
	for _, c := range r.porID {
		if c.BoardID == boardID && c.AceitoEm == nil && c.RevogadoEm == nil {
			n++
		}
	}
	return n, nil
}

// Renovar troca o hash e a validade, se o convite continua sem aceite nem revogação.
func (r *Convites) Renovar(ctx context.Context, id, tokenHash string, expiraEm time.Time) error {
	c, ok := r.porID[id]
	if !ok || c.AceitoEm != nil || c.RevogadoEm != nil {
		return convite.ErrJaResolvido
	}
	c.TokenHash, c.ExpiraEm = tokenHash, expiraEm
	return nil
}

// Recusar e PendentesDoEmail existem só para o fake continuar satisfazendo a
// porta, pelo mesmo motivo de ContarPendentes e Renovar.

// Recusar marca recusado e revogado, se o convite continua pendente.
func (r *Convites) Recusar(ctx context.Context, id string, em time.Time) error {
	c, ok := r.porID[id]
	if !ok || c.AceitoEm != nil || c.RevogadoEm != nil {
		return convite.ErrJaResolvido
	}
	c.RecusadoEm, c.RevogadoEm = &em, &em
	return nil
}

// PendentesDoEmail devolve os convites em aberto e válidos para o email.
func (r *Convites) PendentesDoEmail(ctx context.Context, email string, agora time.Time) ([]convite.Convite, error) {
	var lista []convite.Convite
	for _, c := range r.porID {
		if c.Email == email && c.AceitoEm == nil && c.RevogadoEm == nil && c.ExpiraEm.After(agora) {
			lista = append(lista, *c)
		}
	}
	return lista, nil
}
