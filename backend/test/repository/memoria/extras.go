package memoria

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"

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

func (r *Etiquetas) Salvar(ctx context.Context, e *etiqueta.Etiqueta) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	copia := *e
	r.porID[e.ID] = &copia
	return nil
}

func (r *Etiquetas) Atualizar(ctx context.Context, e *etiqueta.Etiqueta) error {
	return r.Salvar(ctx, e)
}

func (r *Etiquetas) BuscarPorID(ctx context.Context, id string) (*etiqueta.Etiqueta, error) {
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

func (r *Etiquetas) ListarDoBoard(ctx context.Context, boardID string) ([]etiqueta.Etiqueta, error) {
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

func (r *Etiquetas) EtiquetasDoCard(ctx context.Context, cardID string) ([]etiqueta.Etiqueta, error) {
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

func (r *Etiquetas) EtiquetasDoBoardPorCard(ctx context.Context, boardID string) (map[string][]string, error) {
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

func (r *Etiquetas) Apagar(ctx context.Context, id string) error {
	delete(r.porID, id)
	for _, conjunto := range r.aplicadas {
		delete(conjunto, id)
	}
	return nil
}

func (r *Etiquetas) UltimaPosicao(ctx context.Context, boardID string) (float64, error) {
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

func (r *Etiquetas) Aplicar(ctx context.Context, cardID, etiquetaID string) error {
	if r.aplicadas[cardID] == nil {
		r.aplicadas[cardID] = make(map[string]bool)
	}
	r.aplicadas[cardID][etiquetaID] = true
	return nil
}

func (r *Etiquetas) Remover(ctx context.Context, cardID, etiquetaID string) error {
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

func (r *Checklists) Salvar(ctx context.Context, c *checklist.Checklist) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	copia := *c
	r.porID[c.ID] = &copia
	return nil
}

func (r *Checklists) Atualizar(ctx context.Context, c *checklist.Checklist) error {
	return r.Salvar(ctx, c)
}

func (r *Checklists) BuscarPorID(ctx context.Context, id string) (*checklist.Checklist, error) {
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

func (r *Checklists) ListarDoCard(ctx context.Context, cardID string) ([]checklist.Checklist, error) {
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
func (r *Checklists) Apagar(ctx context.Context, id string) error {
	delete(r.porID, id)
	for itemID, item := range r.itens {
		if item.ChecklistID == id {
			delete(r.itens, itemID)
		}
	}
	return nil
}

func (r *Checklists) UltimaPosicao(ctx context.Context, cardID string) (float64, error) {
	var ultima float64
	for _, c := range r.porID {
		if c.CardID == cardID && c.Posicao > ultima {
			ultima = c.Posicao
		}
	}
	return ultima, nil
}

func (r *Checklists) SalvarItem(ctx context.Context, i *checklist.Item) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	copia := *i
	r.itens[i.ID] = &copia
	return nil
}

// EditarItem e MarcarItem escrevem campo a campo, como o SQL estreito. Ver
// memoria.Boards.Renomear.
func (r *Checklists) EditarItem(ctx context.Context, id, texto string, em time.Time) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	i, ok := r.itens[id]
	if !ok {
		return nil
	}
	i.Texto, i.AtualizadoEm = texto, em
	return nil
}

func (r *Checklists) MarcarItem(ctx context.Context, id string, concluido bool, em time.Time) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	i, ok := r.itens[id]
	if !ok {
		return nil
	}
	i.Concluido, i.AtualizadoEm = concluido, em
	return nil
}

func (r *Checklists) BuscarItem(ctx context.Context, id string) (*checklist.Item, error) {
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

func (r *Checklists) ListarItens(ctx context.Context, checklistID string) ([]checklist.Item, error) {
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

func (r *Checklists) ApagarItem(ctx context.Context, id string) error {
	delete(r.itens, id)
	return nil
}

func (r *Checklists) UltimaPosicaoItem(ctx context.Context, checklistID string) (float64, error) {
	var ultima float64
	for _, i := range r.itens {
		if i.ChecklistID == checklistID && i.Posicao > ultima {
			ultima = i.Posicao
		}
	}
	return ultima, nil
}

func (r *Checklists) ProgressoDoBoard(ctx context.Context, boardID string) (map[string]ucboard.Progresso, error) {
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

// CaminhosDeArquivoDoCard, ...DaColuna e ...DoBoard reproduzem as consultas que
// o repositório de verdade usa para limpar o VOLUME ao apagar. Só TipoArquivo:
// link não ocupa disco.
func (r *Anexos) CaminhosDeArquivoDoCard(ctx context.Context, cardID string) ([]string, error) {
	return r.caminhos(func(a *anexo.Anexo) bool { return a.CardID == cardID }), nil
}

func (r *Anexos) CaminhosDeArquivoDaColuna(ctx context.Context, colunaID string) ([]string, error) {
	return r.caminhos(func(a *anexo.Anexo) bool {
		c, ok := r.cards.porID[a.CardID]
		return ok && c.ColunaID == colunaID
	}), nil
}

func (r *Anexos) CaminhosDeArquivoDoBoard(ctx context.Context, boardID string) ([]string, error) {
	return r.caminhos(func(a *anexo.Anexo) bool {
		c, ok := r.cards.porID[a.CardID]
		if !ok {
			return false
		}
		col, ok := r.colunas.porID[c.ColunaID]
		return ok && col.BoardID == boardID
	}), nil
}

func (r *Anexos) caminhos(pertence func(*anexo.Anexo) bool) []string {
	lista := make([]string, 0)
	for _, a := range r.porID {
		if a.Tipo == anexo.TipoArquivo && a.Caminho != "" && pertence(a) {
			lista = append(lista, a.Caminho)
		}
	}
	sort.Strings(lista)
	return lista
}

// NovosAnexos cria o repositório em memória vazio.
func NovosAnexos() *Anexos {
	return &Anexos{porID: make(map[string]*anexo.Anexo)}
}

// LigarQuadro dá acesso a colunas e cards, para a contagem por quadro.
func (r *Anexos) LigarQuadro(colunas *Colunas, cards *Cards) {
	r.colunas, r.cards = colunas, cards
}

func (r *Anexos) Salvar(ctx context.Context, a *anexo.Anexo) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	copia := *a
	r.porID[a.ID] = &copia
	return nil
}

func (r *Anexos) BuscarPorID(ctx context.Context, id string) (*anexo.Anexo, error) {
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

func (r *Anexos) ListarDoCard(ctx context.Context, cardID string) ([]anexo.Anexo, error) {
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

func (r *Anexos) Apagar(ctx context.Context, id string) (bool, error) {
	if r.ErroForcado != nil {
		return false, r.ErroForcado
	}
	if _, existe := r.porID[id]; !existe {
		return false, nil
	}
	delete(r.porID, id)
	return true, nil
}

// ContarDoCard e BytesDoBoard alimentam as cotas de anexo.
func (r *Anexos) ContarDoCard(ctx context.Context, cardID string) (int, error) {
	if r.ErroForcado != nil {
		return 0, r.ErroForcado
	}
	total := 0
	for _, a := range r.porID {
		if a.CardID == cardID {
			total++
		}
	}
	return total, nil
}

// BytesDoBoard soma só os ARQUIVOS: link não ocupa disco e não entra na conta.
func (r *Anexos) BytesDoBoard(ctx context.Context, boardID string) (int64, error) {
	if r.ErroForcado != nil {
		return 0, r.ErroForcado
	}
	var total int64
	for _, a := range r.porID {
		if a.Tipo != anexo.TipoArquivo {
			continue
		}
		if r.boardDoCard(a.CardID) == boardID {
			total += a.Tamanho
		}
	}
	return total, nil
}

func (r *Anexos) ContarPorCardDoBoard(ctx context.Context, boardID string) (map[string]int, error) {
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
	// ErroAoGuardar, quando definido, faz a RECEPÇÃO falhar — é como o teste
	// prova que uma falha no armazém não deixa linha órfã no banco.
	ErroAoGuardar error
	// ErroAoPublicar falha na promoção do temporário ao nome definitivo. É o
	// outro lado: prova que uma publicação que falha não deixa o anexo gravado.
	ErroAoPublicar error
}

// NovoArmazem cria o armazém em memória vazio.
func NovoArmazem() *Armazem {
	return &Armazem{arquivos: make(map[string][]byte)}
}

// ErrArquivoInexistente é devolvido ao abrir um caminho que não foi gravado.
var ErrArquivoInexistente = errors.New("arquivo não encontrado no armazém")

// recebidoEmMemoria espelha o que o armazém de disco mede durante a leitura.
type recebidoEmMemoria struct {
	dados []byte
	tipo  string
	hash  string
}

func (r *recebidoEmMemoria) Bytes() int64   { return int64(len(r.dados)) }
func (r *recebidoEmMemoria) Tipo() string   { return r.tipo }
func (r *recebidoEmMemoria) Digest() string { return r.hash }

// Receber lê o conteúdo medindo tamanho, tipo e hash — como o disco faz.
//
// O TETO é aplicado durante a leitura, com um byte de folga, pelo mesmo motivo
// do armazém real: confiar no tamanho declarado seria confiar em quem envia.
// Reproduzir isso aqui é o que faz o fake servir de teste — um Receber que
// aceitasse tudo esconderia justamente o caminho que o limite existe para
// cobrir.
func (a *Armazem) Receber(conteudo io.Reader, limite int64) (ucboard.ArquivoRecebido, error) {
	if a.ErroAoGuardar != nil {
		return nil, a.ErroAoGuardar
	}
	dados, err := io.ReadAll(io.LimitReader(conteudo, limite+1))
	if err != nil {
		return nil, err
	}
	if int64(len(dados)) > limite {
		return nil, ucboard.ErrArquivoAcimaDoLimite
	}
	soma := sha256.Sum256(dados)
	tipo := "application/octet-stream"
	if len(dados) > 0 {
		tipo = http.DetectContentType(dados)
	}
	return &recebidoEmMemoria{dados: dados, tipo: tipo, hash: hex.EncodeToString(soma[:])}, nil
}

// Publicar dá nome definitivo ao que foi recebido.
func (a *Armazem) Publicar(recebido ucboard.ArquivoRecebido, extensao string) (string, error) {
	interno, ok := recebido.(*recebidoEmMemoria)
	if !ok {
		return "", errors.New("recebido de outro armazém")
	}
	if a.ErroAoPublicar != nil {
		return "", a.ErroAoPublicar
	}
	a.proximo++
	nome := "arquivo-" + strconv.Itoa(a.proximo) + extensao
	a.arquivos[nome] = interno.dados
	return nome, nil
}

// Descartar joga fora o que não será publicado. Em memória não há temporário a
// apagar — o que importa é que nada tenha sido publicado.
func (a *Armazem) Descartar(ucboard.ArquivoRecebido) error { return nil }

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

// Exclusoes é o outbox das exclusões de arquivo em memória.
//
// Ele reproduz a regra que sustenta o fail-closed: `Pendentes` só devolve o que
// foi MARCADO COMO COBERTO. Um fake que devolvesse tudo esconderia justamente o
// que a porta de cobertura existe para garantir — e o teste passaria enquanto
// produção apagaria arquivo sem backup.
type Exclusoes struct {
	registradas []ucboard.ExclusaoDeArquivo
	cobertas    map[int64]bool
	removidas   map[int64]bool
	erros       map[int64]string
	proximoID   int64
	ErroForcado error
}

// NovasExclusoes cria o outbox em memória vazio.
func NovasExclusoes() *Exclusoes {
	return &Exclusoes{cobertas: map[int64]bool{}, removidas: map[int64]bool{}, erros: map[int64]string{}}
}

func (r *Exclusoes) Registrar(_ context.Context, boardID string, caminhos []string, em time.Time) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	for _, caminho := range caminhos {
		r.proximoID++
		r.registradas = append(r.registradas, ucboard.ExclusaoDeArquivo{
			ID: r.proximoID, Caminho: caminho, BoardID: boardID, ExcluidoEm: em,
		})
	}
	return nil
}

func (r *Exclusoes) Pendentes(_ context.Context, _ time.Time, limite int) ([]ucboard.ExclusaoDeArquivo, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	lista := make([]ucboard.ExclusaoDeArquivo, 0)
	for _, e := range r.registradas {
		if !r.cobertas[e.ID] || r.removidas[e.ID] {
			continue
		}
		lista = append(lista, e)
		if len(lista) == limite {
			break
		}
	}
	return lista, nil
}

func (r *Exclusoes) SemCobertura(_ context.Context, limite int) ([]ucboard.ExclusaoDeArquivo, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	lista := make([]ucboard.ExclusaoDeArquivo, 0)
	for _, e := range r.registradas {
		if r.cobertas[e.ID] || r.removidas[e.ID] {
			continue
		}
		lista = append(lista, e)
		if len(lista) == limite {
			break
		}
	}
	return lista, nil
}

func (r *Exclusoes) MarcarCobertos(_ context.Context, ids []int64, _ time.Time) error {
	for _, id := range ids {
		r.cobertas[id] = true
	}
	return nil
}

func (r *Exclusoes) MarcarRemovido(_ context.Context, id int64, _ time.Time) error {
	r.removidas[id] = true
	return nil
}

func (r *Exclusoes) AdiarComErro(_ context.Context, id int64, erro string, _ time.Time) error {
	r.erros[id] = erro
	// A tentativa é contada no agregado, como o UPDATE do Postgres faz.
	for i := range r.registradas {
		if r.registradas[i].ID == id {
			r.registradas[i].Tentativas++
		}
	}
	return nil
}

// Registradas informa quantas exclusões entraram no outbox — atalho de teste.
func (r *Exclusoes) Registradas() int { return len(r.registradas) }

// Caminhos devolve as chaves físicas registradas, na ordem.
func (r *Exclusoes) Caminhos() []string {
	lista := make([]string, 0, len(r.registradas))
	for _, e := range r.registradas {
		lista = append(lista, e.Caminho)
	}
	return lista
}

// Removidas informa quantos arquivos o coletor já deu por removidos.
func (r *Exclusoes) Removidas() int { return len(r.removidas) }

// ErroDe devolve a última falha registrada para uma exclusão.
func (r *Exclusoes) ErroDe(id int64) string { return r.erros[id] }
