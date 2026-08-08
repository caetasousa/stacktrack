package memoria

import (
	"context"
	"sort"

	"stacktrack/internal/domain/evento"
	ucboard "stacktrack/internal/usecase/board"
)

// Atividades é o log de eventos em memória.
//
// Implementa as DUAS pontas da mesma tabela — a de escrita (RegistroDeEventos,
// que os usecases usam ao publicar) e a de leitura (RepositorioAtividade, que o
// histórico consulta). Juntá-las aqui é o que permite um teste publicar pelo
// usecase e depois perguntar "o que aconteceu com este card?", sem banco.
type Atividades struct {
	registradas []ucboard.Atividade
	porCard     map[string][]int
	// usuarios resolve o autor, como o LEFT JOIN com usuarios faz no repositório
	// de verdade.
	usuarios    *Usuarios
	proximoSeq  int64
	ErroForcado error
}

// NovasAtividades cria o log em memória vazio.
func NovasAtividades() *Atividades {
	return &Atividades{porCard: make(map[string][]int)}
}

// LigarUsuarios dá acesso às contas, para a atividade sair com o nome do autor.
func (r *Atividades) LigarUsuarios(usuarios *Usuarios) { r.usuarios = usuarios }

// Registrar guarda o evento e devolve o seq, como o BIGSERIAL do banco.
func (r *Atividades) Registrar(ctx context.Context, e evento.Evento) (int64, error) {
	if r.ErroForcado != nil {
		return 0, r.ErroForcado
	}
	r.proximoSeq++
	r.registradas = append(r.registradas, ucboard.Atividade{
		Seq: r.proximoSeq, Tipo: e.Tipo, AutorID: e.AutorID,
		Dados: e.Dados, OcorridoEm: e.OcorridoEm.Format("2006-01-02T15:04:05Z07:00"),
	})
	if e.CardID != "" {
		r.porCard[e.CardID] = append(r.porCard[e.CardID], len(r.registradas)-1)
	}
	return r.proximoSeq, nil
}

// DoCard devolve o histórico do card, do mais recente para o mais antigo.
func (r *Atividades) DoCard(ctx context.Context, cardID string, limite int) ([]ucboard.Atividade, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	lista := make([]ucboard.Atividade, 0)
	for _, i := range r.porCard[cardID] {
		a := r.registradas[i]
		if r.usuarios != nil && a.AutorID != "" {
			if u, _ := r.usuarios.BuscarPorID(ctx, a.AutorID); u != nil {
				a.AutorNome = u.Nome
			}
		}
		lista = append(lista, a)
	}
	// Do mais recente para o mais antigo, como o ORDER BY seq DESC do SQL.
	sort.Slice(lista, func(i, j int) bool { return lista[i].Seq > lista[j].Seq })
	if len(lista) > limite {
		lista = lista[:limite]
	}
	return lista, nil
}
