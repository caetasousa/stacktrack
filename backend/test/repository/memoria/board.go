package memoria

import (
	"context"
	"sort"

	"stacktrack/internal/domain/board"
	"stacktrack/internal/domain/card"
	"stacktrack/internal/domain/coluna"
	"stacktrack/internal/domain/membro"
	ucboard "stacktrack/internal/usecase/board"
)

// Boards guarda quadros em memória.
type Boards struct {
	porID map[string]*board.Board
	// membros é a mesma instância usada pelos usecases: a listagem parte do
	// vínculo, exatamente como o SQL do repositório de verdade.
	membros     *Membros
	ErroForcado error
}

// NovosBoards cria o repositório em memória, ligado aos vínculos informados.
func NovosBoards(membros *Membros) *Boards {
	return &Boards{porID: make(map[string]*board.Board), membros: membros}
}

func (r *Boards) Salvar(ctx context.Context, b *board.Board) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	copia := *b
	r.porID[b.ID] = &copia
	return nil
}

func (r *Boards) Atualizar(ctx context.Context, b *board.Board) error {
	return r.Salvar(ctx, b)
}

func (r *Boards) BuscarPorID(ctx context.Context, id string) (*board.Board, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	b, ok := r.porID[id]
	if !ok {
		return nil, nil
	}
	copia := *b
	return &copia, nil
}

// Apagar remove o quadro e imita o ON DELETE CASCADE do schema, levando junto
// os vínculos — sem isso o teste de "apagar quadro" passaria deixando membros
// órfãos que o Postgres nunca deixaria existir.
func (r *Boards) Apagar(ctx context.Context, id string) error {
	delete(r.porID, id)
	r.membros.apagarDoBoard(id)
	return nil
}

func (r *Boards) ListarDoUsuario(ctx context.Context, usuarioID string) ([]ucboard.Resumo, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	resumos := make([]ucboard.Resumo, 0)
	for _, m := range r.membros.porBoardEUsuario {
		if m.UsuarioID != usuarioID {
			continue
		}
		if b, ok := r.porID[m.BoardID]; ok {
			resumos = append(resumos, ucboard.Resumo{Board: *b, Papel: m.Papel})
		}
	}
	sort.Slice(resumos, func(i, j int) bool {
		return resumos[i].Board.CriadoEm.After(resumos[j].Board.CriadoEm)
	})
	return resumos, nil
}

// Membros guarda os vínculos entre pessoas e quadros.
type Membros struct {
	porBoardEUsuario map[string]*membro.Membro
	// usuarios existe só para Participantes conseguir imitar o JOIN do SQL;
	// é opcional, e os testes que não olham nome nem email podem ignorá-lo.
	usuarios    *Usuarios
	ErroForcado error
}

// NovosMembros cria o repositório em memória vazio.
func NovosMembros() *Membros {
	return &Membros{porBoardEUsuario: make(map[string]*membro.Membro)}
}

func chaveMembro(boardID, usuarioID string) string { return boardID + "|" + usuarioID }

func (r *Membros) Salvar(ctx context.Context, m *membro.Membro) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	copia := *m
	r.porBoardEUsuario[chaveMembro(m.BoardID, m.UsuarioID)] = &copia
	return nil
}

func (r *Membros) Buscar(ctx context.Context, boardID, usuarioID string) (*membro.Membro, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	m, ok := r.porBoardEUsuario[chaveMembro(boardID, usuarioID)]
	if !ok {
		return nil, nil
	}
	copia := *m
	return &copia, nil
}

func (r *Membros) Atualizar(ctx context.Context, m *membro.Membro) error {
	return r.Salvar(ctx, m)
}

func (r *Membros) Remover(ctx context.Context, boardID, usuarioID string) error {
	delete(r.porBoardEUsuario, chaveMembro(boardID, usuarioID))
	return nil
}

func (r *Membros) Todos(ctx context.Context, boardID string) ([]membro.Membro, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	lista := make([]membro.Membro, 0)
	for _, m := range r.porBoardEUsuario {
		if m.BoardID == boardID {
			lista = append(lista, *m)
		}
	}
	return lista, nil
}

// Participantes imita o JOIN com usuarios do repositório de verdade — por isso
// precisa enxergar as contas.
func (r *Membros) Participantes(ctx context.Context, boardID string) ([]ucboard.Participante, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	lista := make([]ucboard.Participante, 0)
	for _, m := range r.porBoardEUsuario {
		if m.BoardID != boardID {
			continue
		}
		p := ucboard.Participante{UsuarioID: m.UsuarioID, Papel: m.Papel, CriadoEm: m.CriadoEm}
		if r.usuarios != nil {
			if u, _ := r.usuarios.BuscarPorID(ctx, m.UsuarioID); u != nil {
				p.Nome, p.Email = u.Nome, u.Email
			}
		}
		lista = append(lista, p)
	}
	// dono primeiro, depois por ordem de entrada — igual ao ORDER BY do SQL
	sort.Slice(lista, func(i, j int) bool {
		donoI, donoJ := lista[i].Papel == membro.PapelDono, lista[j].Papel == membro.PapelDono
		if donoI != donoJ {
			return donoI
		}
		return lista[i].CriadoEm.Before(lista[j].CriadoEm)
	})
	return lista, nil
}

// LigarUsuarios dá ao repositório de vínculos acesso às contas, para
// Participantes conseguir devolver nome e email.
func (r *Membros) LigarUsuarios(usuarios *Usuarios) { r.usuarios = usuarios }

func (r *Membros) apagarDoBoard(boardID string) {
	for chave, m := range r.porBoardEUsuario {
		if m.BoardID == boardID {
			delete(r.porBoardEUsuario, chave)
		}
	}
}

// Colunas guarda colunas em memória.
type Colunas struct {
	porID       map[string]*coluna.Coluna
	cards       *Cards
	ErroForcado error
}

// NovasColunas cria o repositório em memória, ligado aos cards informados.
func NovasColunas(cards *Cards) *Colunas {
	return &Colunas{porID: make(map[string]*coluna.Coluna), cards: cards}
}

func (r *Colunas) Salvar(ctx context.Context, c *coluna.Coluna) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	copia := *c
	r.porID[c.ID] = &copia
	return nil
}

func (r *Colunas) Atualizar(ctx context.Context, c *coluna.Coluna) error {
	return r.Salvar(ctx, c)
}

func (r *Colunas) BuscarPorID(ctx context.Context, id string) (*coluna.Coluna, error) {
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

func (r *Colunas) ListarDoBoard(ctx context.Context, boardID string) ([]coluna.Coluna, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	lista := make([]coluna.Coluna, 0)
	for _, c := range r.porID {
		if c.BoardID == boardID {
			lista = append(lista, *c)
		}
	}
	ordenarPorPosicao(lista, func(c coluna.Coluna) (string, float64, string) { return c.Chave, c.Posicao, c.ID })
	return lista, nil
}

// Apagar imita o cascata do schema levando os cards da coluna junto.
func (r *Colunas) Apagar(ctx context.Context, id string) error {
	delete(r.porID, id)
	r.cards.apagarDaColuna(id)
	return nil
}

func (r *Colunas) UltimaPosicao(ctx context.Context, boardID string) (float64, error) {
	if r.ErroForcado != nil {
		return 0, r.ErroForcado
	}
	var ultima float64
	for _, c := range r.porID {
		if c.BoardID == boardID && c.Posicao > ultima {
			ultima = c.Posicao
		}
	}
	return ultima, nil
}

// Quantidade informa quantas colunas existem — atalho para os testes.
func (r *Colunas) Quantidade() int { return len(r.porID) }

// Cards guarda cards em memória.
type Cards struct {
	porID map[string]*card.Card
	// colunas resolve a que quadro cada card pertence, como faz o JOIN do SQL.
	colunas     *Colunas
	ErroForcado error
}

// NovosCards cria o repositório em memória vazio.
func NovosCards() *Cards {
	return &Cards{porID: make(map[string]*card.Card)}
}

// LigarColunas fecha o ciclo entre cards e colunas, que se referenciam.
func (r *Cards) LigarColunas(colunas *Colunas) { r.colunas = colunas }

func (r *Cards) Salvar(ctx context.Context, c *card.Card) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	copia := *c
	r.porID[c.ID] = &copia
	return nil
}

// Atualizar repete o bloqueio otimista do Postgres.
//
// Se o fake apenas sobrescrevesse, todo teste de usecase passaria por um
// caminho que não existe em produção — e o 409 só apareceria com gente usando.
func (r *Cards) Atualizar(ctx context.Context, c *card.Card) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	atual, existe := r.porID[c.ID]
	if !existe || atual.Version != c.Version-1 {
		return card.ErrConflito
	}
	return r.Salvar(ctx, c)
}

func (r *Cards) BuscarPorID(ctx context.Context, id string) (*card.Card, error) {
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

func (r *Cards) ListarDoBoard(ctx context.Context, boardID string) ([]card.Card, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	lista := make([]card.Card, 0)
	for _, c := range r.porID {
		col, ok := r.colunas.porID[c.ColunaID]
		if ok && col.BoardID == boardID {
			lista = append(lista, *c)
		}
	}
	ordenarPorPosicao(lista, func(c card.Card) (string, float64, string) { return c.Chave, c.Posicao, c.ID })
	return lista, nil
}

func (r *Cards) Apagar(ctx context.Context, id string) error {
	delete(r.porID, id)
	return nil
}

func (r *Cards) UltimaPosicao(ctx context.Context, colunaID string) (float64, error) {
	if r.ErroForcado != nil {
		return 0, r.ErroForcado
	}
	var ultima float64
	for _, c := range r.porID {
		if c.ColunaID == colunaID && c.Posicao > ultima {
			ultima = c.Posicao
		}
	}
	return ultima, nil
}

func (r *Cards) apagarDaColuna(colunaID string) {
	for id, c := range r.porID {
		if c.ColunaID == colunaID {
			delete(r.porID, id)
		}
	}
}

// Quantidade informa quantos cards existem — atalho para os testes.
func (r *Cards) Quantidade() int { return len(r.porID) }

// ordenarPorPosicao ordena por posição com desempate por id, igual ao
// ORDER BY posicao, id do repositório de verdade — sem o desempate a ordem de
// duas posições iguais varia entre execuções e o teste fica intermitente.
// ordenarPorPosicao reproduz o ORDER BY do repositório de verdade:
//
//	ORDER BY chave COLLATE "C" NULLS FIRST, posicao, id
//
// ⚠️ A ordem principal é a CHAVE, não a posição — e o nome desta função ficou
// do tempo em que era o contrário. O fake precisa ordenar igual ao banco: com
// ele ordenando por posição enquanto o SQL ordena por chave, todo teste de
// usecase validaria uma ordem que produção não produz. Foi assim que um defeito
// real passou — a posição legada esgotava e o card ia parar em outro lugar, e
// nenhum teste em memória percebia.
func ordenarPorPosicao[T any](lista []T, chave func(T) (string, float64, string)) {
	sort.Slice(lista, func(i, j int) bool {
		ki, pi, idi := chave(lista[i])
		kj, pj, idj := chave(lista[j])
		// NULLS FIRST: a linha que o backfill ainda não alcançou vem antes.
		if (ki == "") != (kj == "") {
			return ki == ""
		}
		if ki != kj {
			return ki < kj
		}
		if pi != pj {
			return pi < pj
		}
		return idi < idj
	})
}

// --- chave de ordenação (fase 9) -------------------------------------------

// UltimaChave devolve a maior chave em uso na coluna, ou vazio.
func (r *Cards) UltimaChave(ctx context.Context, colunaID string) (string, error) {
	if r.ErroForcado != nil {
		return "", r.ErroForcado
	}
	maior := ""
	for _, c := range r.porID {
		if c.ColunaID == colunaID && c.Chave > maior {
			maior = c.Chave
		}
	}
	return maior, nil
}

// SemChave devolve os cards que o backfill ainda não alcançou, na ordem que a
// POSIÇÃO ainda dita — é a ordem que o backfill precisa preservar.
func (r *Cards) SemChave(ctx context.Context, limite int) ([]card.Card, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	lista := make([]card.Card, 0)
	for _, c := range r.porID {
		if c.Chave == "" {
			lista = append(lista, *c)
		}
	}
	sort.Slice(lista, func(i, j int) bool {
		if lista[i].ColunaID != lista[j].ColunaID {
			return lista[i].ColunaID < lista[j].ColunaID
		}
		if lista[i].Posicao != lista[j].Posicao {
			return lista[i].Posicao < lista[j].Posicao
		}
		return lista[i].ID < lista[j].ID
	})
	if len(lista) > limite {
		lista = lista[:limite]
	}
	return lista, nil
}

// GravarChave grava só a chave, sem subir a version.
func (r *Cards) GravarChave(ctx context.Context, id, chave string) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	if c, existe := r.porID[id]; existe {
		c.Chave = chave
	}
	return nil
}

// UltimaChave devolve a maior chave em uso no quadro, ou vazio.
func (r *Colunas) UltimaChave(ctx context.Context, boardID string) (string, error) {
	if r.ErroForcado != nil {
		return "", r.ErroForcado
	}
	maior := ""
	for _, c := range r.porID {
		if c.BoardID == boardID && c.Chave > maior {
			maior = c.Chave
		}
	}
	return maior, nil
}

// SemChave devolve as colunas que o backfill ainda não alcançou.
func (r *Colunas) SemChave(ctx context.Context, limite int) ([]coluna.Coluna, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	lista := make([]coluna.Coluna, 0)
	for _, c := range r.porID {
		if c.Chave == "" {
			lista = append(lista, *c)
		}
	}
	sort.Slice(lista, func(i, j int) bool {
		if lista[i].BoardID != lista[j].BoardID {
			return lista[i].BoardID < lista[j].BoardID
		}
		if lista[i].Posicao != lista[j].Posicao {
			return lista[i].Posicao < lista[j].Posicao
		}
		return lista[i].ID < lista[j].ID
	})
	if len(lista) > limite {
		lista = lista[:limite]
	}
	return lista, nil
}

// GravarChave grava só a chave.
func (r *Colunas) GravarChave(ctx context.Context, id, chave string) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	if c, existe := r.porID[id]; existe {
		c.Chave = chave
	}
	return nil
}
