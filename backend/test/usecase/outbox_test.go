// Testes do outbox transacional — a garantia de que a mudança e o evento que a
// descreve caem ou valem JUNTOS.
//
// Por que isso merece teste próprio: um evento faltando no log do quadro é
// INVISÍVEL. Quem reconecta pergunta "o que houve desde o 41?", recebe o que
// existe, e não tem como saber que houve uma mudança que nunca virou evento —
// a tela dele passa a discordar do banco em silêncio.
//
// Os testes daqui não usam Postgres: eles exercitam o contrato do usecase com a
// porta EscritaAtomica. Que a implementação de verdade comite tudo numa
// transação só é assunto do Postgres, e vive em test/repository (tag integracao).
package usecase_test

import (
	"context"
	"errors"
	"testing"

	detiqueta "stacktrack/internal/domain/etiqueta"
	"stacktrack/internal/domain/evento"
	ucboard "stacktrack/internal/usecase/board"
)

// escritaAtomicaFalsa registra o que passou por ela e permite forçar falha,
// para provar o que acontece quando a transação não fecha.
type escritaAtomicaFalsa struct {
	// registrados são os eventos que chegaram ao log — ou seja, os que
	// participaram de uma transação que deu certo.
	registrados []evento.Evento
	// repos é o que a mudança recebe. Aponta para os mesmos fakes em memória do
	// resto dos testes, então a "transação" escreve onde todo mundo lê.
	repos ucboard.Escrita
	// falhar simula a transação que não fecha.
	falhar error
	// mudancasAplicadas conta quantas vezes a mudança foi de fato executada.
	mudancasAplicadas int
	proximoSeq        int64
	proximaRevisao    int64
}

func (f *escritaAtomicaFalsa) Escrever(
	_ context.Context,
	e evento.Evento,
	mudanca func(ucboard.Escrita) error,
) (evento.Evento, error) {
	if f.falhar != nil {
		// A transação nem chega a valer: nem a mudança roda, nem o evento entra.
		return e, f.falhar
	}
	// A revisão é reservada ANTES da mudança, como a unidade de trabalho real
	// faz sob o lock — e o evento volta carimbado. Devolver só o seq era o
	// defeito que deixava o evento ao vivo sem revisão.
	proximaRevisao := f.proximaRevisao + 1
	e.Revisao = proximaRevisao

	if err := mudanca(f.repos); err != nil {
		// A mudança falhou: rollback, e o evento NÃO pode ser registrado.
		// A revisão reservada também volta atrás na UoW real.
		return e, err
	}
	f.proximaRevisao = proximaRevisao
	f.mudancasAplicadas++
	f.proximoSeq++
	e.Seq = f.proximoSeq
	f.registrados = append(f.registrados, e)
	return e, nil
}

func (f *escritaAtomicaFalsa) ExcluirQuadro(
	_ context.Context,
	_ string,
	mudanca func(ucboard.Escrita) error,
) error {
	if f.falhar != nil {
		return f.falhar
	}
	if err := mudanca(f.repos); err != nil {
		return err
	}
	f.mudancasAplicadas++
	return nil
}

// publicadorEspiao guarda o que foi entregue ao vivo.
type publicadorEspiao struct {
	entregues []evento.Evento
}

func (p *publicadorEspiao) Publicar(e evento.Evento) {
	p.entregues = append(p.entregues, e)
}

// comOutbox liga o outbox e o espião a um quadro já montado.
func comOutbox(q *quadro) (*escritaAtomicaFalsa, *publicadorEspiao) {
	// TODOS os repositórios, como a UnidadeDeTrabalho de verdade faz: desde A2
	// não há mais mutação de quadro fora da transação, e um campo faltando aqui
	// esconderia justamente isso.
	atomica := &escritaAtomicaFalsa{
		repos: ucboard.Escrita{
			Cards: q.cards, Colunas: q.colunas, Boards: q.boards,
			Membros: q.membros, Etiquetas: q.etiquetas, Checklists: q.checklists,
			Anexos: q.anexos, Responsaveis: q.responsaveis, Comentarios: q.comentarios,
			Publicacoes: q.publicacoes,
		},
	}
	espiao := &publicadorEspiao{}

	for _, uc := range []interface {
		ComPublicador(ucboard.Publicador)
		ComEscritaAtomica(ucboard.EscritaAtomica)
	}{q.quadros, q.coluna, q.card} {
		uc.ComPublicador(espiao)
		uc.ComEscritaAtomica(atomica)
	}
	return atomica, espiao
}

func TestCriarQuadroPassaPelaEscritaAtomica(t *testing.T) {
	q := novoQuadro()
	atomica, espiao := comOutbox(q)
	atomica.falhar = errors.New("commit falhou")

	if _, err := q.quadros.Criar(context.Background(), "ana", "Não pode sobrar"); err == nil {
		t.Fatal("criar quadro devia falhar quando a transação não fecha")
	}
	lista, err := q.boards.ListarDoUsuario(context.Background(), "ana")
	if err != nil {
		t.Fatalf("listar quadros: %v", err)
	}
	if len(lista) != 0 {
		t.Errorf("sobraram %d quadros depois da falha atômica", len(lista))
	}
	if len(espiao.entregues) != 0 {
		t.Errorf("publicou evento de criação numa transação que falhou: %v", tipos(espiao.entregues))
	}
}

func TestExcluirQuadroSoSinalizaDepoisDoCommitESemPersistirEventoTerminal(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Terminal")
	atomica, espiao := comOutbox(q)

	if err := q.quadros.Apagar(context.Background(), boardID, "ana"); err != nil {
		t.Fatalf("apagar: %v", err)
	}
	if len(atomica.registrados) != 0 {
		t.Fatalf("evento terminal tentou entrar no log apagado: %#v", atomica.registrados)
	}
	if len(espiao.entregues) != 1 || espiao.entregues[0].Tipo != evento.QuadroApagado {
		t.Fatalf("sinais ao vivo = %#v, esperado somente quadro.apagado", espiao.entregues)
	}
	if espiao.entregues[0].Seq != 0 || espiao.entregues[0].Revisao != 0 {
		t.Fatalf("sinal terminal fingiu ser persistido: %#v", espiao.entregues[0])
	}
}

func TestExcluirQuadroNaoSinalizaQuandoOCommitFalha(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Terminal")
	atomica, espiao := comOutbox(q)
	atomica.falhar = errors.New("commit falhou")

	if err := q.quadros.Apagar(context.Background(), boardID, "ana"); err == nil {
		t.Fatal("apagar deveria propagar a falha")
	}
	if len(espiao.entregues) != 0 {
		t.Fatalf("anunciou exclusão que não comitou: %#v", espiao.entregues)
	}
}

// O caminho feliz: mover um card grava o card e o evento na mesma passagem, e
// o evento entregue ao vivo carrega o seq que a transação atribuiu.
func TestMoverCardGravaDadoEEventoJuntos(t *testing.T) {
	q := novoQuadro()
	atomica, espiao := comOutbox(q)

	boardID := q.criarQuadro(t, "ana", "Estudos")
	colunaID := q.criarColuna(t, boardID, "ana", "A fazer")
	cardID := q.criarCard(t, colunaID, "ana", "Migração")

	if _, err := q.card.Mover(context.Background(), cardID, "ana", colunaID, ucboard.Vizinhos{}); err != nil {
		t.Fatalf("erro ao mover: %v", err)
	}

	if _, achou := ultimoDoTipo(atomica.registrados, evento.CardMovido); !achou {
		t.Errorf("card.movido não entrou no log; registrados = %v", tipos(atomica.registrados))
	}

	// Publicado COM seq: sem ele, quem reconecta não consegue retomar daqui.
	entregue, achou := ultimoDoTipo(espiao.entregues, evento.CardMovido)
	if !achou {
		t.Fatalf("card.movido não foi publicado; entregues = %v", tipos(espiao.entregues))
	}
	if entregue.Seq == 0 {
		t.Error("evento publicado sem seq: quem reconectar não tem de onde retomar")
	}
}

// O que este arquivo existe para provar: se a transação não fecha, NADA sai.
//
// Nem o evento no log, nem — principalmente — a entrega ao vivo. Publicar aqui
// seria anunciar uma mudança que não aconteceu, e quem recebesse o evento
// recarregaria o quadro para encontrar o estado anterior.
func TestTransacaoQueFalhaNaoRegistraNemPublica(t *testing.T) {
	q := novoQuadro()
	atomica, espiao := comOutbox(q)

	boardID := q.criarQuadro(t, "ana", "Estudos")
	colunaID := q.criarColuna(t, boardID, "ana", "A fazer")
	cardID := q.criarCard(t, colunaID, "ana", "Migração")

	// A partir daqui, toda escrita atômica falha.
	atomica.registrados = nil
	espiao.entregues = nil
	atomica.falhar = errors.New("o banco caiu no meio do commit")

	if _, err := q.card.Mover(context.Background(), cardID, "ana", colunaID, ucboard.Vizinhos{}); err == nil {
		t.Fatal("mover devolveu sucesso com a transação falhando")
	}

	if len(atomica.registrados) != 0 {
		t.Errorf("registrou %d evento(s) numa transação que falhou", len(atomica.registrados))
	}
	if len(espiao.entregues) != 0 {
		t.Errorf("publicou %v numa transação que falhou — anunciou mudança que não houve",
			tipos(espiao.entregues))
	}
}

// A mudança precisa passar POR DENTRO da escrita atômica, e não ao lado dela.
//
// É o defeito que originou tudo isto: o dado ia por um caminho e o evento por
// outro, e a atomicidade anunciada na documentação não existia em lugar nenhum
// do código.
func TestMudancaEstruturalPassaPelaEscritaAtomica(t *testing.T) {
	q := novoQuadro()
	atomica, espiao := comOutbox(q)

	boardID := q.criarQuadro(t, "ana", "Estudos")
	colunaID := q.criarColuna(t, boardID, "ana", "A fazer")

	antes := atomica.mudancasAplicadas
	espiao.entregues = nil

	q.criarCard(t, colunaID, "ana", "Novo")

	if atomica.mudancasAplicadas == antes {
		t.Fatal("criar card não passou pela escrita atômica: o evento não está na transação do dado")
	}
	if len(espiao.entregues) == 0 {
		t.Error("nada foi publicado depois do commit")
	}
}

// Etiqueta, checklist e anexo PASSAM pelo caminho atômico — a decisão anterior
// era o contrário, e foi revista.
//
// O argumento antigo era que o evento deles é um aviso de "recarregue o
// quadro", e perder um não deixa buraco perceptível. Ele estava certo sobre o
// sintoma e errado sobre a garantia: com duas transações separadas existe
// também o caminho inverso — evento gravado sem a mudança — e um cliente que
// recebe "etiqueta aplicada", recarrega e não encontra etiqueta nenhuma não tem
// como se recuperar, porque o log afirma que aconteceu.
func TestEventoDeEtiquetaPassaPelaEscritaAtomica(t *testing.T) {
	e := novoExtras()
	atomica, _ := comOutbox(e.quadro)
	e.etiquetaUC.ComEscritaAtomica(atomica)

	boardID := e.quadro.criarQuadro(t, "ana", "Estudos")

	antes := atomica.mudancasAplicadas
	if _, err := e.etiquetaUC.Criar(context.Background(), boardID, "ana", "Urgente", detiqueta.CorVermelho); err != nil {
		t.Fatalf("erro ao criar etiqueta: %v", err)
	}

	if atomica.mudancasAplicadas == antes {
		t.Error("a etiqueta não passou pela escrita atômica: o evento não está na transação do dado")
	}
	if _, achou := ultimoDoTipo(atomica.registrados, evento.EtiquetaCriada); !achou {
		t.Error("o evento da etiqueta não foi registrado dentro da transação")
	}
}

// E o inverso, que é o que a atomicidade compra: transação que não fecha não
// deixa NEM a etiqueta NEM o evento.
func TestFalhaNaTransacaoNaoDeixaEtiquetaNemEvento(t *testing.T) {
	e := novoExtras()
	atomica, espiao := comOutbox(e.quadro)
	e.etiquetaUC.ComEscritaAtomica(atomica)

	boardID := e.quadro.criarQuadro(t, "ana", "Estudos")
	atomica.falhar = errors.New("commit falhou")

	if _, err := e.etiquetaUC.Criar(context.Background(), boardID, "ana", "Urgente", detiqueta.CorVermelho); err == nil {
		t.Fatal("criar etiqueta devia falhar quando a transação não fecha")
	}

	etiquetas, _ := e.quadro.etiquetas.ListarDoBoard(context.Background(), boardID)
	if len(etiquetas) != 0 {
		t.Errorf("etiquetas = %d, esperado nenhuma: a transação não fechou", len(etiquetas))
	}
	if _, achou := ultimoDoTipo(espiao.entregues, evento.EtiquetaCriada); achou {
		t.Error("um evento foi publicado ao vivo para uma mudança que não aconteceu")
	}
}

// O evento ENTREGUE AO VIVO carrega a revisão, e não só o seq.
//
// Este teste existe por causa de um defeito real, encontrado num smoke test
// depois de a suíte inteira estar verde: a unidade de trabalho recebia o evento
// POR VALOR e devolvia só o seq. A revisão era gravada na cópia de dentro da
// transação — o log ficava correto — e o evento publicado saía com revisão
// zero. O cliente recebia um evento sem posição na sequência, não conseguia
// encaixá-lo, e caía em reconciliação a cada mudança, com o banco perfeitamente
// certo o tempo todo.
//
// Nenhum teste pegava isso porque todos olhavam para o que foi REGISTRADO. O
// que faltava era olhar para o que foi ENTREGUE.
func TestEventoPublicadoCarregaARevisao(t *testing.T) {
	q := novoQuadro()
	atomica, espiao := comOutbox(q)

	boardID := q.criarQuadro(t, "ana", "Estudos")
	colunaID := q.criarColuna(t, boardID, "ana", "A fazer")
	q.criarCard(t, colunaID, "ana", "Migração")

	entregue, achou := ultimoDoTipo(espiao.entregues, evento.CardCriado)
	if !achou {
		t.Fatal("nada foi publicado ao vivo")
	}
	if entregue.Revisao == 0 {
		t.Error("o evento entregue ao vivo saiu sem revisão: o cliente não consegue encaixá-lo na sequência")
	}
	if entregue.Seq == 0 {
		t.Error("o evento entregue ao vivo saiu sem seq")
	}

	// E os dois números batem com os do log: entregar um evento com posição
	// diferente da gravada faria o cliente confirmar uma revisão que não
	// corresponde ao que o banco tem.
	registrado, achou := ultimoDoTipo(atomica.registrados, evento.CardCriado)
	if !achou {
		t.Fatal("nada foi registrado")
	}
	if entregue.Revisao != registrado.Revisao || entregue.Seq != registrado.Seq {
		t.Errorf("entregue = (rev %d, seq %d), registrado = (rev %d, seq %d): os números divergem",
			entregue.Revisao, entregue.Seq, registrado.Revisao, registrado.Seq)
	}
}

// Toda mutação avança a revisão em exatamente um, e o grupo nasce completo.
func TestCadaMutacaoAvancaARevisaoUmaVez(t *testing.T) {
	q := novoQuadro()
	atomica, _ := comOutbox(q)

	boardID := q.criarQuadro(t, "ana", "Estudos")
	colunaID := q.criarColuna(t, boardID, "ana", "A fazer")
	q.criarCard(t, colunaID, "ana", "Um")
	q.criarCard(t, colunaID, "ana", "Dois")

	for i, e := range atomica.registrados {
		if e.Revisao != int64(i+1) {
			t.Fatalf("revisões registradas fora de sequência: o %dº evento tem revisão %d", i+1, e.Revisao)
		}
		// Uma mutação, um evento: o grupo é completo com um item só, e é isso
		// que autoriza o cliente a confirmar a revisão ao aplicá-lo.
		if e.Indice != 0 || e.Quantidade != 1 {
			t.Errorf("grupo do evento %d = (%d, %d), esperado (0, 1)", i+1, e.Indice, e.Quantidade)
		}
	}
}

// --- apoio ------------------------------------------------------------------

func ultimoDoTipo(eventos []evento.Evento, tipo evento.Tipo) (evento.Evento, bool) {
	for i := len(eventos) - 1; i >= 0; i-- {
		if eventos[i].Tipo == tipo {
			return eventos[i], true
		}
	}
	return evento.Evento{}, false
}

func tipos(eventos []evento.Evento) []evento.Tipo {
	fora := make([]evento.Tipo, 0, len(eventos))
	for _, e := range eventos {
		fora = append(fora, e.Tipo)
	}
	return fora
}
