// Etiquetas, checklists e anexos. Todos passam pelo mesmo caminho de
// autorização — card → coluna → quadro —, e é isso que estes testes cobrem,
// além do que só o armazém de arquivos traz de novo.
package usecase_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	danexo "stacktrack/internal/domain/anexo"
	dchecklist "stacktrack/internal/domain/checklist"
	dcor "stacktrack/internal/domain/cor"
	detiqueta "stacktrack/internal/domain/etiqueta"
	"stacktrack/internal/domain/membro"
	ucboard "stacktrack/internal/usecase/board"
	"stacktrack/test/repository/memoria"
)

type extras struct {
	*quadro
	armazem     *memoria.Armazem
	exclusoes   *memoria.Exclusoes
	etiquetaUC  *ucboard.EtiquetaUseCase
	checklistUC *ucboard.ChecklistUseCase
	anexoUC     *ucboard.AnexoUseCase
}

func novoExtras() *extras {
	q := novoQuadro()
	// O MESMO armazém do quadro, e não um segundo: apagar um card limpa o
	// volume, e com dois armazéns o teste anexaria num e conferiria o outro —
	// verde sem provar nada.
	armazem := q.armazem

	// O outbox das exclusões é ligado aos três usecases que apagam algo com
	// anexo pendurado. Sem ele, o arquivo simplesmente ficaria no disco sem
	// registro — seguro, e invisível para o teste.
	exclusoes := memoria.NovasExclusoes()
	q.card.ComExclusoes(exclusoes)
	q.coluna.ComExclusoes(exclusoes)
	q.quadros.ComExclusoes(exclusoes)

	return &extras{
		quadro:      q,
		armazem:     armazem,
		exclusoes:   exclusoes,
		etiquetaUC:  ucboard.NovoEtiquetaUseCase(q.membros, q.colunas, q.cards, q.etiquetas),
		checklistUC: ucboard.NovoChecklistUseCase(q.membros, q.colunas, q.cards, q.checklists),
		anexoUC:     ucboard.NovoAnexoUseCase(q.membros, q.colunas, q.cards, q.anexos, armazem),
	}
}

// montar cria quadro, coluna e card de uma vez, devolvendo os ids.
func (e *extras) montar(t *testing.T, dono string) (boardID, cardID string) {
	t.Helper()
	boardID = e.criarQuadro(t, dono, "Estudos")
	colunaID := e.criarColuna(t, boardID, dono, "A fazer")
	return boardID, e.criarCard(t, colunaID, dono, "Tarefa")
}

// --- etiquetas ------------------------------------------------------------

func TestAplicarERemoverEtiquetaNoCard(t *testing.T) {
	e := novoExtras()
	boardID, cardID := e.montar(t, "ana")
	etq, err := e.etiquetaUC.Criar(context.Background(), boardID, "ana", "Urgente", detiqueta.CorVermelho)
	if err != nil {
		t.Fatalf("erro ao criar etiqueta: %v", err)
	}

	if err := e.etiquetaUC.Aplicar(context.Background(), cardID, etq.ID, "ana"); err != nil {
		t.Fatalf("erro ao aplicar: %v", err)
	}

	detalhe, err := e.card.Detalhar(context.Background(), cardID, "ana")
	if err != nil {
		t.Fatalf("erro ao detalhar: %v", err)
	}
	if len(detalhe.Etiquetas) != 1 || detalhe.Etiquetas[0].Nome != "Urgente" {
		t.Fatalf("etiquetas do card = %+v", detalhe.Etiquetas)
	}

	if err := e.etiquetaUC.Remover(context.Background(), cardID, etq.ID, "ana"); err != nil {
		t.Fatalf("erro ao remover: %v", err)
	}
	detalhe, _ = e.card.Detalhar(context.Background(), cardID, "ana")
	if len(detalhe.Etiquetas) != 0 {
		t.Errorf("a etiqueta devia ter saído do card: %+v", detalhe.Etiquetas)
	}
}

// Aplicar duas vezes é o mesmo que aplicar uma: o resultado pretendido já vale,
// e a segunda chamada não pode virar erro numa tela que só marca uma caixa.
func TestAplicarEtiquetaDuasVezesNaoEErro(t *testing.T) {
	e := novoExtras()
	boardID, cardID := e.montar(t, "ana")
	etq, _ := e.etiquetaUC.Criar(context.Background(), boardID, "ana", "Urgente", detiqueta.CorVermelho)

	e.etiquetaUC.Aplicar(context.Background(), cardID, etq.ID, "ana")
	if err := e.etiquetaUC.Aplicar(context.Background(), cardID, etq.ID, "ana"); err != nil {
		t.Fatalf("erro na segunda aplicação: %v", err)
	}

	detalhe, _ := e.card.Detalhar(context.Background(), cardID, "ana")
	if len(detalhe.Etiquetas) != 1 {
		t.Errorf("etiquetas = %d, esperado 1", len(detalhe.Etiquetas))
	}
}

// Sem essa checagem, quem participa de dois quadros usaria o id de uma etiqueta
// do quadro A num card do quadro B — e a etiqueta apareceria lá com nome e cor
// que ninguém do quadro B consegue editar.
func TestNaoDaParaAplicarEtiquetaDeOutroQuadro(t *testing.T) {
	e := novoExtras()
	_, cardDeA := e.montar(t, "ana")
	boardB := e.criarQuadro(t, "ana", "Outro quadro")
	etqDeB, _ := e.etiquetaUC.Criar(context.Background(), boardB, "ana", "De outro quadro", detiqueta.CorAzul)

	err := e.etiquetaUC.Aplicar(context.Background(), cardDeA, etqDeB.ID, "ana")

	if !errors.Is(err, detiqueta.ErrNaoEncontrada) {
		t.Errorf("erro = %v, esperado ErrNaoEncontrada", err)
	}
}

func TestLeitorNaoMexeEmEtiqueta(t *testing.T) {
	e := novoExtras()
	boardID, cardID := e.montar(t, "ana")
	etq, _ := e.etiquetaUC.Criar(context.Background(), boardID, "ana", "Urgente", detiqueta.CorVermelho)
	e.convidar(t, boardID, "bob", membro.PapelLeitor)

	// enxerga
	if _, err := e.etiquetaUC.Listar(context.Background(), boardID, "bob"); err != nil {
		t.Errorf("o leitor devia ver as etiquetas: %v", err)
	}
	// mas não mexe
	if _, err := e.etiquetaUC.Criar(context.Background(), boardID, "bob", "Nova", detiqueta.CorVerde); !errors.Is(err, membro.ErrSemPermissao) {
		t.Errorf("criar: erro = %v", err)
	}
	if err := e.etiquetaUC.Aplicar(context.Background(), cardID, etq.ID, "bob"); !errors.Is(err, membro.ErrSemPermissao) {
		t.Errorf("aplicar: erro = %v", err)
	}
}

func TestQuemNaoParticipaNaoDescobreAEtiqueta(t *testing.T) {
	e := novoExtras()
	boardID, _ := e.montar(t, "ana")
	etq, _ := e.etiquetaUC.Criar(context.Background(), boardID, "ana", "Urgente", detiqueta.CorVermelho)

	_, err := e.etiquetaUC.Editar(context.Background(), etq.ID, "bob", "Invadida", detiqueta.CorRoxo)

	if !errors.Is(err, detiqueta.ErrNaoEncontrada) {
		t.Errorf("erro = %v, esperado ErrNaoEncontrada", err)
	}
	if errors.Is(err, membro.ErrSemPermissao) {
		t.Error("'sem permissão' confirmaria que a etiqueta existe")
	}
}

// --- checklists -----------------------------------------------------------

func TestChecklistComItensEProgresso(t *testing.T) {
	e := novoExtras()
	boardID, cardID := e.montar(t, "ana")

	lista, err := e.checklistUC.Criar(context.Background(), cardID, "ana", "To-do List")
	if err != nil {
		t.Fatalf("erro ao criar checklist: %v", err)
	}
	primeiro, _ := e.checklistUC.CriarItem(context.Background(), lista.ID, "ana", "Escrever o teste")
	e.checklistUC.CriarItem(context.Background(), lista.ID, "ana", "Rodar o -race")

	concluido := true
	if _, err := e.checklistUC.EditarItem(context.Background(), primeiro.ID, "ana", nil, &concluido); err != nil {
		t.Fatalf("erro ao marcar: %v", err)
	}

	detalhado, _ := e.quadros.Detalhar(context.Background(), boardID, "ana")
	progresso := detalhado.Colunas[0].Cards[0].Checklist
	if progresso.Concluidos != 1 || progresso.Total != 2 {
		t.Errorf("progresso = %d/%d, esperado 1/2", progresso.Concluidos, progresso.Total)
	}
}

// Marcar a caixa não pode exigir reenviar o texto, e renomear não pode
// desmarcar sem querer.
func TestEditarItemMexeSoNoQueFoiInformado(t *testing.T) {
	e := novoExtras()
	_, cardID := e.montar(t, "ana")
	lista, _ := e.checklistUC.Criar(context.Background(), cardID, "ana", "To-do List")
	item, _ := e.checklistUC.CriarItem(context.Background(), lista.ID, "ana", "Texto original")

	marcado := true
	depoisDeMarcar, _ := e.checklistUC.EditarItem(context.Background(), item.ID, "ana", nil, &marcado)
	if depoisDeMarcar.Texto != "Texto original" || !depoisDeMarcar.Concluido {
		t.Fatalf("item = %+v", depoisDeMarcar)
	}

	novoTexto := "Texto novo"
	depoisDeRenomear, _ := e.checklistUC.EditarItem(context.Background(), item.ID, "ana", &novoTexto, nil)
	if depoisDeRenomear.Texto != "Texto novo" || !depoisDeRenomear.Concluido {
		t.Errorf("item = %+v — renomear não podia ter desmarcado", depoisDeRenomear)
	}
}

func TestApagarChecklistLevaOsItens(t *testing.T) {
	e := novoExtras()
	boardID, cardID := e.montar(t, "ana")
	lista, _ := e.checklistUC.Criar(context.Background(), cardID, "ana", "To-do List")
	e.checklistUC.CriarItem(context.Background(), lista.ID, "ana", "Item")

	if err := e.checklistUC.Apagar(context.Background(), lista.ID, "ana"); err != nil {
		t.Fatalf("erro ao apagar: %v", err)
	}

	detalhado, _ := e.quadros.Detalhar(context.Background(), boardID, "ana")
	if p := detalhado.Colunas[0].Cards[0].Checklist; p.Total != 0 {
		t.Errorf("progresso = %d/%d, esperado zerado", p.Concluidos, p.Total)
	}
}

func TestChecklistDeCardAlheioNaoEAlcancavel(t *testing.T) {
	e := novoExtras()
	_, cardID := e.montar(t, "ana")
	lista, _ := e.checklistUC.Criar(context.Background(), cardID, "ana", "To-do List")
	item, _ := e.checklistUC.CriarItem(context.Background(), lista.ID, "ana", "Segredo")

	if _, err := e.checklistUC.Criar(context.Background(), cardID, "bob", "Invasora"); !errors.Is(err, dchecklist.ErrNaoEncontrada) {
		if !errors.Is(err, errCardNaoEncontrado()) {
			t.Errorf("criar: erro = %v", err)
		}
	}
	if _, err := e.checklistUC.Renomear(context.Background(), lista.ID, "bob", "Invadida"); !errors.Is(err, dchecklist.ErrNaoEncontrada) {
		t.Errorf("renomear: erro = %v", err)
	}
	marcado := true
	if _, err := e.checklistUC.EditarItem(context.Background(), item.ID, "bob", nil, &marcado); !errors.Is(err, dchecklist.ErrItemNaoEncontrado) {
		t.Errorf("marcar item: erro = %v", err)
	}
}

// --- anexos ---------------------------------------------------------------

func TestAnexarArquivoGuardaConteudoERegistra(t *testing.T) {
	e := novoExtras()
	_, cardID := e.montar(t, "ana")
	conteudo := "id,nome\n1,ana\n"

	a, err := e.anexoUC.AnexarArquivo(context.Background(), cardID, "ana", "relatorio.csv", strings.NewReader(conteudo))
	if err != nil {
		t.Fatalf("erro ao anexar: %v", err)
	}

	if a.Tipo != danexo.TipoArquivo || a.Nome != "relatorio.csv" {
		t.Errorf("anexo = %+v", a)
	}
	if e.armazem.Quantidade() != 1 {
		t.Errorf("arquivos no armazém = %d, esperado 1", e.armazem.Quantidade())
	}

	baixado, err := e.anexoUC.Baixar(context.Background(), a.ID, "ana")
	if err != nil {
		t.Fatalf("erro ao baixar: %v", err)
	}
	defer baixado.Leitura.Close()
	dados, _ := io.ReadAll(baixado.Leitura)
	if string(dados) != conteudo {
		t.Errorf("conteúdo baixado = %q", dados)
	}
}

// Se o armazém falhar, não pode sobrar linha apontando para arquivo que não
// existe — a tela mostraria anexo quebrado sem conserto pela interface.
func TestFalhaNoArmazemNaoDeixaAnexoRegistrado(t *testing.T) {
	e := novoExtras()
	_, cardID := e.montar(t, "ana")
	e.armazem.ErroAoGuardar = errors.New("disco cheio")

	_, err := e.anexoUC.AnexarArquivo(context.Background(), cardID, "ana", "a.txt", strings.NewReader("teste"))

	if err == nil {
		t.Fatal("a falha do armazém devia ter subido")
	}
	if e.anexos.Quantidade() != 0 {
		t.Error("nenhum anexo podia ter sido registrado")
	}
}

// E o inverso: se o banco falhar depois de gravar, o arquivo não pode ficar
// órfão ocupando disco para sempre.
func TestFalhaAoRegistrarApagaOArquivoGravado(t *testing.T) {
	e := novoExtras()
	_, cardID := e.montar(t, "ana")
	e.anexos.ErroForcado = errors.New("conexão recusada")

	_, err := e.anexoUC.AnexarArquivo(context.Background(), cardID, "ana", "a.txt", strings.NewReader("teste"))

	if err == nil {
		t.Fatal("a falha do banco devia ter subido")
	}
	if e.armazem.Quantidade() != 0 {
		t.Errorf("arquivos no armazém = %d — o gravado devia ter sido descartado", e.armazem.Quantidade())
	}
}

func TestApagarAnexoTiraDoBancoEDoArmazem(t *testing.T) {
	e := novoExtras()
	_, cardID := e.montar(t, "ana")
	a, _ := e.anexoUC.AnexarArquivo(context.Background(), cardID, "ana", "a.txt", strings.NewReader("teste"))

	if err := e.anexoUC.Apagar(context.Background(), a.ID, "ana"); err != nil {
		t.Fatalf("erro ao apagar: %v", err)
	}

	if e.anexos.Quantidade() != 0 || e.armazem.Quantidade() != 0 {
		t.Errorf("sobrou coisa: banco=%d armazem=%d", e.anexos.Quantidade(), e.armazem.Quantidade())
	}
}

// O arquivo não é servido por caminho adivinhável justamente para a checagem de
// participação valer também para ele.
func TestQuemNaoParticipaNaoBaixaOAnexo(t *testing.T) {
	e := novoExtras()
	_, cardID := e.montar(t, "ana")
	a, _ := e.anexoUC.AnexarArquivo(context.Background(), cardID, "ana", "segredo.txt", strings.NewReader("teste"))

	_, err := e.anexoUC.Baixar(context.Background(), a.ID, "bob")

	if !errors.Is(err, danexo.ErrNaoEncontrado) {
		t.Errorf("erro = %v, esperado ErrNaoEncontrado", err)
	}
}

func TestLeitorBaixaMasNaoAnexaNemApaga(t *testing.T) {
	e := novoExtras()
	boardID, cardID := e.montar(t, "ana")
	a, _ := e.anexoUC.AnexarArquivo(context.Background(), cardID, "ana", "a.txt", strings.NewReader("teste"))
	e.convidar(t, boardID, "bob", membro.PapelLeitor)

	if _, err := e.anexoUC.Baixar(context.Background(), a.ID, "bob"); err != nil {
		t.Errorf("o leitor devia poder baixar: %v", err)
	}
	if _, err := e.anexoUC.AnexarLink(context.Background(), cardID, "bob", "x", "https://exemplo.com"); !errors.Is(err, membro.ErrSemPermissao) {
		t.Errorf("anexar: erro = %v", err)
	}
	if err := e.anexoUC.Apagar(context.Background(), a.ID, "bob"); !errors.Is(err, membro.ErrSemPermissao) {
		t.Errorf("apagar: erro = %v", err)
	}
}

// O teto é aplicado DURANTE a leitura, e é isso que o torna confiável.
//
// Antes, o tamanho vinha do `Content-Length` do multipart — declarado por quem
// envia. O teste antigo aproveitava isso: mandava um byte dizendo que eram
// gigabytes, e passava. Isso provava a checagem e não protegia de nada, porque
// quem ataca declara o tamanho que quiser.
//
// Agora a fonte é um fluxo SEM FIM. Só termina se o limite for aplicado byte a
// byte: se ele não fosse, o teste rodaria para sempre.
func TestArquivoAcimaDoLimiteEhCortadoDuranteALeitura(t *testing.T) {
	e := novoExtras()
	_, cardID := e.montar(t, "ana")

	_, err := e.anexoUC.AnexarArquivo(context.Background(), cardID, "ana", "grande.bin", fluxoSemFim{})

	if !errors.Is(err, danexo.ErrArquivoGrande) {
		t.Fatalf("erro = %v, esperado ErrArquivoGrande", err)
	}
	// Nada publicado: o que foi recebido é descartado.
	if e.armazem.Quantidade() != 0 {
		t.Error("um arquivo acima do limite acabou publicado")
	}
}

// fluxoSemFim produz bytes indefinidamente, como um cliente que não para de
// enviar.
type fluxoSemFim struct{}

func (fluxoSemFim) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

// Arquivo vazio é recusado, e o motivo é próprio: "vazio" não é "grande demais"
// nem "tipo não permitido", e mandar a pessoa procurar o problema errado custa
// tempo dela.
func TestArquivoVazioEhRecusado(t *testing.T) {
	e := novoExtras()
	_, cardID := e.montar(t, "ana")

	_, err := e.anexoUC.AnexarArquivo(context.Background(), cardID, "ana", "vazio.txt", strings.NewReader(""))
	if !errors.Is(err, danexo.ErrArquivoVazio) {
		t.Errorf("erro = %v, esperado ErrArquivoVazio", err)
	}
	if e.armazem.Quantidade() != 0 {
		t.Error("um arquivo vazio acabou publicado")
	}
}

// O TIPO vem do CONTEÚDO, não do que o cliente declara.
//
// O Content-Type do multipart é escolhido por quem envia. Aceitá-lo faria a
// lista de permissão do domínio deixar de significar o que ela diz: bastaria
// mandar um HTML anunciando-se como PNG.
func TestTipoEhDeduzidoDoConteudoENaoDoNome(t *testing.T) {
	e := novoExtras()
	_, cardID := e.montar(t, "ana")

	// Extensão de imagem, conteúdo de HTML.
	_, err := e.anexoUC.AnexarArquivo(context.Background(), cardID, "ana", "inocente.png",
		strings.NewReader("<!DOCTYPE html><html><body>oi</body></html>"))

	if !errors.Is(err, danexo.ErrTipoNaoPermitido) {
		t.Errorf("erro = %v, esperado ErrTipoNaoPermitido", err)
	}
	if e.armazem.Quantidade() != 0 {
		t.Error("um HTML disfarçado de PNG acabou publicado")
	}
}

func TestAnexoDeLinkNaoTocaNoArmazem(t *testing.T) {
	e := novoExtras()
	_, cardID := e.montar(t, "ana")

	a, err := e.anexoUC.AnexarLink(context.Background(), cardID, "ana", "PR 12", "https://github.com/org/repo/pull/12")
	if err != nil {
		t.Fatalf("erro ao anexar link: %v", err)
	}

	if a.Tipo != danexo.TipoLink || a.Caminho != "" {
		t.Errorf("anexo = %+v", a)
	}
	if e.armazem.Quantidade() != 0 {
		t.Error("link não gera arquivo")
	}
	// E baixar um link não faz sentido: não há conteúdo nosso para entregar.
	if _, err := e.anexoUC.Baixar(context.Background(), a.ID, "ana"); !errors.Is(err, danexo.ErrNaoEncontrado) {
		t.Errorf("baixar link: erro = %v", err)
	}
}

// errCardNaoEncontrado evita importar o pacote card só para uma comparação.
func errCardNaoEncontrado() error {
	_, err := novoExtras().checklistUC.Criar(context.Background(), "card-que-nao-existe", "ana", "x")
	return err
}

// O prazo passou despercebido pelos testes de unidade porque o fake em memória
// copia a struct inteira, enquanto o repositório de verdade lista coluna por
// coluna no SQL — e a coluna nova tinha ficado de fora do INSERT, do UPDATE e
// dos dois SELECT. Este teste não pega isso (só o Postgres pegaria), mas trava
// o comportamento que o usecase promete: o prazo sobrevive à releitura.
func TestPrazoSobreviveAReleituraDoCard(t *testing.T) {
	e := novoExtras()
	boardID, cardID := e.montar(t, "ana")
	prazo := time.Now().Add(48 * time.Hour)

	if _, err := e.card.DefinirPrazo(context.Background(), cardID, "ana", &prazo); err != nil {
		t.Fatalf("erro ao definir prazo: %v", err)
	}

	relido, err := e.card.Detalhar(context.Background(), cardID, "ana")
	if err != nil {
		t.Fatalf("erro ao detalhar: %v", err)
	}
	if relido.Card.Prazo == nil || !relido.Card.Prazo.Equal(prazo) {
		t.Fatalf("prazo relido = %v, esperado %v", relido.Card.Prazo, prazo)
	}

	// E o quadro precisa mostrar o mesmo, senão o selo do card mente.
	detalhado, _ := e.quadros.Detalhar(context.Background(), boardID, "ana")
	noQuadro := detalhado.Colunas[0].Cards[0].Card
	if noQuadro.Prazo == nil {
		t.Error("o card no quadro veio sem prazo")
	}

	if _, err := e.card.DefinirPrazo(context.Background(), cardID, "ana", nil); err != nil {
		t.Fatalf("erro ao limpar prazo: %v", err)
	}
	limpo, _ := e.card.Detalhar(context.Background(), cardID, "ana")
	if limpo.Card.Prazo != nil {
		t.Error("passar nil devia ter limpado o prazo")
	}
}

// A cor entrou em coluna e card depois que o prazo já tinha ensinado a lição:
// coluna nova no domínio que fica de fora do INSERT, do UPDATE ou dos SELECT do
// repositório passa despercebida, porque o fake em memória copia a struct
// inteira. Este teste trava a promessa do usecase; quem confere o SQL é a
// verificação contra o Postgres de verdade.
func TestCorSobreviveAReleituraDeColunaEDeCard(t *testing.T) {
	e := novoExtras()
	boardID := e.criarQuadro(t, "ana", "Estudos")

	col, err := e.coluna.Criar(context.Background(), boardID, "ana", "Start", dcor.Verde)
	if err != nil {
		t.Fatalf("erro ao criar coluna: %v", err)
	}
	card, err := e.card.Criar(context.Background(), col.ID, "ana", "Migrar o site", "", dcor.Azul)
	if err != nil {
		t.Fatalf("erro ao criar card: %v", err)
	}

	detalhado, _ := e.quadros.Detalhar(context.Background(), boardID, "ana")
	if detalhado.Colunas[0].Coluna.Cor != dcor.Verde {
		t.Errorf("cor da coluna = %q, esperado verde", detalhado.Colunas[0].Coluna.Cor)
	}
	if detalhado.Colunas[0].Cards[0].Card.Cor != dcor.Azul {
		t.Errorf("cor do card = %q, esperado azul", detalhado.Colunas[0].Cards[0].Card.Cor)
	}

	// e dá para tirar a cor voltando ao visual padrão
	if _, err := e.coluna.Renomear(context.Background(), col.ID, "ana", "Start", ""); err != nil {
		t.Fatalf("erro ao limpar a cor: %v", err)
	}
	semCor, _ := e.quadros.Detalhar(context.Background(), boardID, "ana")
	if semCor.Colunas[0].Coluna.Cor != "" {
		t.Errorf("cor = %q, esperado vazia", semCor.Colunas[0].Coluna.Cor)
	}
	_ = card
}

func TestCorForaDaPaletaERecusada(t *testing.T) {
	e := novoExtras()
	boardID := e.criarQuadro(t, "ana", "Estudos")

	if _, err := e.coluna.Criar(context.Background(), boardID, "ana", "Start", dcor.Cor("neon")); !errors.Is(err, dcor.ErrInvalida) {
		t.Errorf("coluna: erro = %v, esperado ErrInvalida", err)
	}

	col, _ := e.coluna.Criar(context.Background(), boardID, "ana", "Start", "")
	if _, err := e.card.Criar(context.Background(), col.ID, "ana", "Tarefa", "", dcor.Cor("neon")); !errors.Is(err, dcor.ErrInvalida) {
		t.Errorf("card: erro = %v, esperado ErrInvalida", err)
	}
}
