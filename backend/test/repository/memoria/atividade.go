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
	// boardDe e cardDe guardam a que quadro e a que card cada linha pertence,
	// que no banco são colunas da própria tabela. Sem eles, a auditoria do
	// quadro não teria por onde filtrar — e o fake responderia a pergunta certa
	// pela razão errada.
	boardDe map[int]string
	cardDe  map[int]string
	// usuarios resolve o autor, como o LEFT JOIN com usuarios faz no repositório
	// de verdade.
	usuarios    *Usuarios
	proximoSeq  int64
	ErroForcado error
}

// NovasAtividades cria o log em memória vazio.
func NovasAtividades() *Atividades {
	return &Atividades{
		porCard: make(map[string][]int),
		boardDe: make(map[int]string),
		cardDe:  make(map[int]string),
	}
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
	indice := len(r.registradas) - 1
	r.boardDe[indice] = e.BoardID
	if e.CardID != "" {
		r.porCard[e.CardID] = append(r.porCard[e.CardID], indice)
		r.cardDe[indice] = e.CardID
	}
	return r.proximoSeq, nil
}

// DoBoard devolve o histórico do quadro, do mais recente para o mais antigo,
// aplicando o filtro — imitando o WHERE e o ORDER BY do SQL de verdade.
//
// ⚠️ Filtrar e ordenar IGUAL ao banco é obrigação do fake, não zelo. Com ele
// aplicando um critério e o SQL outro, todo teste passa validando um
// comportamento que produção não tem — foi assim que a ordenação por posição
// sobreviveu depois de o SQL já ordenar por chave.
func (r *Atividades) DoBoard(ctx context.Context, boardID string, filtro ucboard.FiltroDeAtividade) ([]ucboard.Atividade, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	lista := make([]ucboard.Atividade, 0)
	for i, a := range r.registradas {
		if r.boardDe[i] != boardID {
			continue
		}
		if filtro.SoMovimentacoes && a.Tipo != evento.CardMovido {
			continue
		}
		if filtro.AutorID != "" && a.AutorID != filtro.AutorID {
			continue
		}
		if filtro.AntesDe > 0 && a.Seq >= filtro.AntesDe {
			continue
		}
		lista = append(lista, r.comAutor(ctx, a))
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].Seq > lista[j].Seq })
	if filtro.Limite > 0 && len(lista) > filtro.Limite {
		lista = lista[:filtro.Limite]
	}
	return lista, nil
}

// UltimaMovimentacaoPorCard devolve, por card, a última vez que ele foi movido
// — o mesmo que o DISTINCT ON do repositório de verdade.
func (r *Atividades) UltimaMovimentacaoPorCard(ctx context.Context, boardID string) (map[string]ucboard.Movimentacao, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	ultimoSeq := make(map[string]int64)
	porCard := make(map[string]ucboard.Movimentacao)

	for i, a := range r.registradas {
		cardID := r.cardDe[i]
		if r.boardDe[i] != boardID || a.Tipo != evento.CardMovido || cardID == "" {
			continue
		}
		if a.Seq <= ultimoSeq[cardID] {
			continue
		}
		ultimoSeq[cardID] = a.Seq
		com := r.comAutor(ctx, a)
		m := ucboard.Movimentacao{
			AutorID: com.AutorID, AutorNome: com.AutorNome, OcorridoEm: com.OcorridoEm,
		}
		// O fake guarda os dados como struct; o repositório de verdade os lê
		// como JSON e desserializa. O resultado é o mesmo, e a asserção de tipo
		// falha em silêncio pelo mesmo caminho que um payload de outro formato
		// tomaria lá — sem colunas, e não com colunas erradas.
		if dados, ok := com.Dados.(ucboard.DadosDoCard); ok {
			m.DeColuna, m.ParaColuna = dados.DeColuna, dados.Coluna
		}
		porCard[cardID] = m
	}
	return porCard, nil
}

// comAutor resolve o nome de quem agiu, como o LEFT JOIN do SQL.
func (r *Atividades) comAutor(ctx context.Context, a ucboard.Atividade) ucboard.Atividade {
	if r.usuarios != nil && a.AutorID != "" {
		if u, _ := r.usuarios.BuscarPorID(ctx, a.AutorID); u != nil {
			a.AutorNome = u.Nome
		}
	}
	return a
}

// DoCard devolve o histórico do card, do mais recente para o mais antigo.
func (r *Atividades) DoCard(ctx context.Context, cardID string, limite int) ([]ucboard.Atividade, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	lista := make([]ucboard.Atividade, 0)
	for _, i := range r.porCard[cardID] {
		lista = append(lista, r.comAutor(ctx, r.registradas[i]))
	}
	// Do mais recente para o mais antigo, como o ORDER BY seq DESC do SQL.
	sort.Slice(lista, func(i, j int) bool { return lista[i].Seq > lista[j].Seq })
	if len(lista) > limite {
		lista = lista[:limite]
	}
	return lista, nil
}
