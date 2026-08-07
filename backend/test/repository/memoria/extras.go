package memoria

import (
	"bytes"
	"errors"
	"io"
	"sort"
	"strconv"

	"stacktrack/internal/domain/anexo"
	"stacktrack/internal/domain/checklist"
	"stacktrack/internal/domain/etiqueta"
	ucboard "stacktrack/internal/usecase/board"
)

// Etiquetas guarda etiquetas e a aplicação delas nos cards.
type Etiquetas struct {
	porID map[string]*etiqueta.Etiqueta
	// aplicadas[cardID] é o conjunto de etiquetas daquele card.
	aplicadas map[string]map[string]bool
	// colunas e cards resolvem a que quadro um card pertence, como o JOIN do SQL.
	colunas     *Colunas
	cards       *Cards
	ErroForcado error
}

// NovasEtiquetas cria o repositório em memória vazio.
func NovasEtiquetas() *Etiquetas {
	return &Etiquetas{
		porID:     make(map[string]*etiqueta.Etiqueta),
		aplicadas: make(map[string]map[string]bool),
	}
}

// LigarQuadro dá acesso a colunas e cards, para as consultas por quadro.
func (r *Etiquetas) LigarQuadro(colunas *Colunas, cards *Cards) {
	r.colunas, r.cards = colunas, cards
}

func (r *Etiquetas) Salvar(e *etiqueta.Etiqueta) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	copia := *e
	r.porID[e.ID] = &copia
	return nil
}

func (r *Etiquetas) Atualizar(e *etiqueta.Etiqueta) error { return r.Salvar(e) }

func (r *Etiquetas) BuscarPorID(id string) (*etiqueta.Etiqueta, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	e, ok := r.porID[id]
	if !ok {
		return nil, nil
	}
	copia := *e
	return &copia, nil
}

func (r *Etiquetas) ListarDoBoard(boardID string) ([]etiqueta.Etiqueta, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	lista := make([]etiqueta.Etiqueta, 0)
	for _, e := range r.porID {
		if e.BoardID == boardID {
			lista = append(lista, *e)
		}
	}
	sort.Slice(lista, func(i, j int) bool {
		if lista[i].Posicao != lista[j].Posicao {
			return lista[i].Posicao < lista[j].Posicao
		}
		return lista[i].ID < lista[j].ID
	})
	return lista, nil
}

func (r *Etiquetas) EtiquetasDoCard(cardID string) ([]etiqueta.Etiqueta, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	lista := make([]etiqueta.Etiqueta, 0)
	for etiquetaID := range r.aplicadas[cardID] {
		if e, ok := r.porID[etiquetaID]; ok {
			lista = append(lista, *e)
		}
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].Posicao < lista[j].Posicao })
	return lista, nil
}

func (r *Etiquetas) EtiquetasDoBoardPorCard(boardID string) (map[string][]string, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	porCard := make(map[string][]string)
	for cardID, conjunto := range r.aplicadas {
		if r.boardDoCard(cardID) != boardID {
			continue
		}
		ids := make([]string, 0, len(conjunto))
		for etiquetaID := range conjunto {
			ids = append(ids, etiquetaID)
		}
		sort.Strings(ids)
		porCard[cardID] = ids
	}
	return porCard, nil
}

func (r *Etiquetas) Apagar(id string) error {
	delete(r.porID, id)
	for _, conjunto := range r.aplicadas {
		delete(conjunto, id)
	}
	return nil
}

func (r *Etiquetas) UltimaPosicao(boardID string) (float64, error) {
	if r.ErroForcado != nil {
		return 0, r.ErroForcado
	}
	var ultima float64
	for _, e := range r.porID {
		if e.BoardID == boardID && e.Posicao > ultima {
			ultima = e.Posicao
		}
	}
	return ultima, nil
}

func (r *Etiquetas) Aplicar(cardID, etiquetaID string) error {
	if r.aplicadas[cardID] == nil {
		r.aplicadas[cardID] = make(map[string]bool)
	}
	r.aplicadas[cardID][etiquetaID] = true
	return nil
}

func (r *Etiquetas) Remover(cardID, etiquetaID string) error {
	delete(r.aplicadas[cardID], etiquetaID)
	return nil
}

func (r *Etiquetas) boardDoCard(cardID string) string {
	if r.cards == nil || r.colunas == nil {
		return ""
	}
	c, ok := r.cards.porID[cardID]
	if !ok {
		return ""
	}
	col, ok := r.colunas.porID[c.ColunaID]
	if !ok {
		return ""
	}
	return col.BoardID
}

// Checklists guarda checklists e itens em memória.
type Checklists struct {
	porID       map[string]*checklist.Checklist
	itens       map[string]*checklist.Item
	colunas     *Colunas
	cards       *Cards
	ErroForcado error
}

// NovasChecklists cria o repositório em memória vazio.
func NovasChecklists() *Checklists {
	return &Checklists{
		porID: make(map[string]*checklist.Checklist),
		itens: make(map[string]*checklist.Item),
	}
}

// LigarQuadro dá acesso a colunas e cards, para o progresso por quadro.
func (r *Checklists) LigarQuadro(colunas *Colunas, cards *Cards) {
	r.colunas, r.cards = colunas, cards
}

func (r *Checklists) Salvar(c *checklist.Checklist) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	copia := *c
	r.porID[c.ID] = &copia
	return nil
}

func (r *Checklists) Atualizar(c *checklist.Checklist) error { return r.Salvar(c) }

func (r *Checklists) BuscarPorID(id string) (*checklist.Checklist, error) {
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

func (r *Checklists) ListarDoCard(cardID string) ([]checklist.Checklist, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	lista := make([]checklist.Checklist, 0)
	for _, c := range r.porID {
		if c.CardID == cardID {
			lista = append(lista, *c)
		}
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].Posicao < lista[j].Posicao })
	return lista, nil
}

// Apagar imita o cascata do schema levando os itens junto.
func (r *Checklists) Apagar(id string) error {
	delete(r.porID, id)
	for itemID, item := range r.itens {
		if item.ChecklistID == id {
			delete(r.itens, itemID)
		}
	}
	return nil
}

func (r *Checklists) UltimaPosicao(cardID string) (float64, error) {
	var ultima float64
	for _, c := range r.porID {
		if c.CardID == cardID && c.Posicao > ultima {
			ultima = c.Posicao
		}
	}
	return ultima, nil
}

func (r *Checklists) SalvarItem(i *checklist.Item) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	copia := *i
	r.itens[i.ID] = &copia
	return nil
}

func (r *Checklists) AtualizarItem(i *checklist.Item) error { return r.SalvarItem(i) }

func (r *Checklists) BuscarItem(id string) (*checklist.Item, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	i, ok := r.itens[id]
	if !ok {
		return nil, nil
	}
	copia := *i
	return &copia, nil
}

func (r *Checklists) ListarItens(checklistID string) ([]checklist.Item, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	lista := make([]checklist.Item, 0)
	for _, i := range r.itens {
		if i.ChecklistID == checklistID {
			lista = append(lista, *i)
		}
	}
	sort.Slice(lista, func(a, b int) bool { return lista[a].Posicao < lista[b].Posicao })
	return lista, nil
}

func (r *Checklists) ApagarItem(id string) error {
	delete(r.itens, id)
	return nil
}

func (r *Checklists) UltimaPosicaoItem(checklistID string) (float64, error) {
	var ultima float64
	for _, i := range r.itens {
		if i.ChecklistID == checklistID && i.Posicao > ultima {
			ultima = i.Posicao
		}
	}
	return ultima, nil
}

func (r *Checklists) ProgressoDoBoard(boardID string) (map[string]ucboard.Progresso, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	porCard := make(map[string]ucboard.Progresso)
	for _, item := range r.itens {
		lista, ok := r.porID[item.ChecklistID]
		if !ok || r.boardDoCard(lista.CardID) != boardID {
			continue
		}
		p := porCard[lista.CardID]
		p.Total++
		if item.Concluido {
			p.Concluidos++
		}
		porCard[lista.CardID] = p
	}
	return porCard, nil
}

func (r *Checklists) boardDoCard(cardID string) string {
	if r.cards == nil || r.colunas == nil {
		return ""
	}
	c, ok := r.cards.porID[cardID]
	if !ok {
		return ""
	}
	col, ok := r.colunas.porID[c.ColunaID]
	if !ok {
		return ""
	}
	return col.BoardID
}

// Anexos guarda anexos em memória.
type Anexos struct {
	porID       map[string]*anexo.Anexo
	colunas     *Colunas
	cards       *Cards
	ErroForcado error
}

// NovosAnexos cria o repositório em memória vazio.
func NovosAnexos() *Anexos {
	return &Anexos{porID: make(map[string]*anexo.Anexo)}
}

// LigarQuadro dá acesso a colunas e cards, para a contagem por quadro.
func (r *Anexos) LigarQuadro(colunas *Colunas, cards *Cards) {
	r.colunas, r.cards = colunas, cards
}

func (r *Anexos) Salvar(a *anexo.Anexo) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	copia := *a
	r.porID[a.ID] = &copia
	return nil
}

func (r *Anexos) BuscarPorID(id string) (*anexo.Anexo, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	a, ok := r.porID[id]
	if !ok {
		return nil, nil
	}
	copia := *a
	return &copia, nil
}

func (r *Anexos) ListarDoCard(cardID string) ([]anexo.Anexo, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	lista := make([]anexo.Anexo, 0)
	for _, a := range r.porID {
		if a.CardID == cardID {
			lista = append(lista, *a)
		}
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].CriadoEm.After(lista[j].CriadoEm) })
	return lista, nil
}

func (r *Anexos) Apagar(id string) error {
	delete(r.porID, id)
	return nil
}

func (r *Anexos) ContarPorCardDoBoard(boardID string) (map[string]int, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	porCard := make(map[string]int)
	for _, a := range r.porID {
		if r.boardDoCard(a.CardID) == boardID {
			porCard[a.CardID]++
		}
	}
	return porCard, nil
}

// Quantidade informa quantos anexos existem — atalho para os testes.
func (r *Anexos) Quantidade() int { return len(r.porID) }

func (r *Anexos) boardDoCard(cardID string) string {
	if r.cards == nil || r.colunas == nil {
		return ""
	}
	c, ok := r.cards.porID[cardID]
	if !ok {
		return ""
	}
	col, ok := r.colunas.porID[c.ColunaID]
	if !ok {
		return ""
	}
	return col.BoardID
}

// Armazem guarda o conteúdo dos anexos em memória, no lugar do disco.
type Armazem struct {
	arquivos map[string][]byte
	proximo  int
	// ErroAoGuardar, quando definido, faz a gravação falhar — é como o teste
	// prova que uma falha no armazém não deixa linha órfã no banco.
	ErroAoGuardar error
}

// NovoArmazem cria o armazém em memória vazio.
func NovoArmazem() *Armazem {
	return &Armazem{arquivos: make(map[string][]byte)}
}

// ErrArquivoInexistente é devolvido ao abrir um caminho que não foi gravado.
var ErrArquivoInexistente = errors.New("arquivo não encontrado no armazém")

func (a *Armazem) Guardar(conteudo io.Reader, extensao string) (string, error) {
	if a.ErroAoGuardar != nil {
		return "", a.ErroAoGuardar
	}
	dados, err := io.ReadAll(conteudo)
	if err != nil {
		return "", err
	}
	a.proximo++
	nome := "arquivo-" + strconv.Itoa(a.proximo) + extensao
	a.arquivos[nome] = dados
	return nome, nil
}

func (a *Armazem) Abrir(caminho string) (io.ReadCloser, error) {
	dados, ok := a.arquivos[caminho]
	if !ok {
		return nil, ErrArquivoInexistente
	}
	return io.NopCloser(bytes.NewReader(dados)), nil
}

func (a *Armazem) Remover(caminho string) error {
	delete(a.arquivos, caminho)
	return nil
}

// Quantidade informa quantos arquivos estão guardados — atalho para os testes.
func (a *Armazem) Quantidade() int { return len(a.arquivos) }
