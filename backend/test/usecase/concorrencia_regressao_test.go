package usecase_test

import (
	"context"
	"errors"
	"testing"

	danexo "stacktrack/internal/domain/anexo"
	dboard "stacktrack/internal/domain/board"
	dcoluna "stacktrack/internal/domain/coluna"
	dconvite "stacktrack/internal/domain/convite"
	"stacktrack/internal/domain/evento"
	"stacktrack/internal/domain/membro"
	dusuario "stacktrack/internal/domain/usuario"
	ucboard "stacktrack/internal/usecase/board"
	"stacktrack/test/repository/memoria"
)

// escritaInterceptada executa uma mudança concorrente exatamente depois das
// validações otimistas do usecase e antes da função transacional. É o ponto de
// interleaving que um teste puramente sequencial não alcança.
type escritaInterceptada struct {
	base  *escritaAtomicaFalsa
	antes func()
}

func (f *escritaInterceptada) Escrever(ctx context.Context, e evento.Evento, mudanca func(ucboard.Escrita) error) (evento.Evento, error) {
	if f.antes != nil {
		antes := f.antes
		f.antes = nil
		antes()
	}
	return f.base.Escrever(ctx, e, mudanca)
}

func (f *escritaInterceptada) ExcluirQuadro(ctx context.Context, boardID string, mudanca func(ucboard.Escrita) error) error {
	return f.base.ExcluirQuadro(ctx, boardID, mudanca)
}

func escritaDaColaboracao(c *colaboracao) ucboard.Escrita {
	return ucboard.Escrita{
		Boards: c.boards, Membros: c.membros, Colunas: c.colunas, Cards: c.cards,
		Usuarios: c.usuarios,
		Convites: c.convites, Etiquetas: c.etiquetas, Checklists: c.checklists,
		Anexos: c.anexos, Responsaveis: c.responsaveis, Comentarios: c.comentarios,
	}
}

func escritaDoQuadro(q *quadro) ucboard.Escrita {
	return ucboard.Escrita{
		Boards: q.boards, Membros: q.membros, Colunas: q.colunas, Cards: q.cards,
		Etiquetas: q.etiquetas, Checklists: q.checklists, Anexos: q.anexos,
		Responsaveis: q.responsaveis, Comentarios: q.comentarios,
		Publicacoes: q.publicacoes,
	}
}

func TestMutacaoRevalidaPapelDoAutorSobOLock(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	colunaID := q.criarColuna(t, boardID, "ana", "A fazer")
	cardID := q.criarCard(t, colunaID, "ana", "Antes")

	atomica := &escritaInterceptada{
		base: &escritaAtomicaFalsa{repos: escritaDoQuadro(q)},
		antes: func() {
			vinculo, _ := membro.Novo(boardID, "ana", membro.PapelLeitor)
			_ = q.membros.Salvar(context.Background(), vinculo)
		},
	}
	q.card.ComEscritaAtomica(atomica)

	_, err := q.card.Editar(context.Background(), cardID, "ana", "Depois", "", "", 1)
	if !errors.Is(err, membro.ErrSemPermissao) {
		t.Fatalf("erro = %v, esperado ErrSemPermissao depois do rebaixamento concorrente", err)
	}
	relido, _ := q.cards.BuscarPorID(context.Background(), cardID)
	if relido.Titulo != "Antes" {
		t.Fatalf("a escrita stale passou depois do rebaixamento: título = %q", relido.Titulo)
	}
}

func TestPublicarRevalidaDonoSobOLock(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	base := &escritaAtomicaFalsa{repos: escritaDoQuadro(q)}
	q.publicacao.ComEscritaAtomica(&escritaInterceptada{
		base: base,
		antes: func() {
			vinculo, _ := membro.Novo(boardID, "ana", membro.PapelLeitor)
			_ = q.membros.Salvar(context.Background(), vinculo)
		},
	})

	_, err := q.publicacao.Publicar(context.Background(), boardID, "ana")
	if !errors.Is(err, membro.ErrSemPermissao) {
		t.Fatalf("erro = %v, esperado ErrSemPermissao depois do rebaixamento concorrente", err)
	}
	if p, _ := q.publicacoes.BuscarPorBoard(context.Background(), boardID); p != nil {
		t.Fatal("ex-dono publicou o quadro enquanto esperava o lock")
	}
	if len(base.registrados) != 0 {
		t.Fatalf("a publicação recusada deixou evento: %#v", base.registrados)
	}
}

func TestRevogarPublicacaoRevalidaDonoSobOLock(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	token := q.publicar(t, boardID, "ana")
	base := &escritaAtomicaFalsa{repos: escritaDoQuadro(q)}
	q.publicacao.ComEscritaAtomica(&escritaInterceptada{
		base: base,
		antes: func() {
			vinculo, _ := membro.Novo(boardID, "ana", membro.PapelLeitor)
			_ = q.membros.Salvar(context.Background(), vinculo)
		},
	})

	err := q.publicacao.Revogar(context.Background(), boardID, "ana")
	if !errors.Is(err, membro.ErrSemPermissao) {
		t.Fatalf("erro = %v, esperado ErrSemPermissao depois do rebaixamento concorrente", err)
	}
	if p, _ := q.publicacoes.BuscarPorToken(context.Background(), token); p == nil {
		t.Fatal("ex-dono revogou o link enquanto esperava o lock")
	}
	if len(base.registrados) != 0 {
		t.Fatalf("a revogação recusada deixou evento: %#v", base.registrados)
	}
}

func TestMoverResolveOrigemEDestinoSobOLock(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	origemAntiga := q.criarColuna(t, boardID, "ana", "Origem antiga")
	origemAtual := q.criarColuna(t, boardID, "ana", "Origem atual")
	destino := q.criarColuna(t, boardID, "ana", "Destino antigo")
	cardID := q.criarCard(t, origemAntiga, "ana", "Tarefa")
	espiao := &publicadorEspiao{}

	atomica := &escritaInterceptada{
		base: &escritaAtomicaFalsa{repos: escritaDoQuadro(q)},
		antes: func() {
			card, _ := q.cards.BuscarPorID(context.Background(), cardID)
			card.Mover(origemAtual, "z")
			_ = q.cards.Atualizar(context.Background(), card)

			coluna, _ := q.colunas.BuscarPorID(context.Background(), destino)
			_ = coluna.Renomear("Destino atual")
			_ = q.colunas.Renomear(context.Background(), coluna.ID, coluna.Titulo, coluna.Cor, coluna.AtualizadoEm)
		},
	}
	q.card.ComEscritaAtomica(atomica)
	q.card.ComPublicador(espiao)

	if _, err := q.card.Mover(context.Background(), cardID, "ana", destino, ucboard.Vizinhos{}); err != nil {
		t.Fatalf("mover: %v", err)
	}
	if len(espiao.entregues) != 1 {
		t.Fatalf("eventos = %d, esperado um", len(espiao.entregues))
	}
	dados, ok := espiao.entregues[0].Dados.(ucboard.DadosDoCard)
	if !ok {
		t.Fatalf("payload = %T, esperado DadosDoCard", espiao.entregues[0].Dados)
	}
	if dados.DeColuna != "Origem atual" || dados.Coluna != "Destino atual" {
		t.Fatalf("evento stale: origem=%q destino=%q", dados.DeColuna, dados.Coluna)
	}
}

func TestMoverRecusaDestinoApagadoEnquantoEsperavaOLock(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	origem := q.criarColuna(t, boardID, "ana", "Origem")
	destino := q.criarColuna(t, boardID, "ana", "Destino")
	cardID := q.criarCard(t, origem, "ana", "Tarefa")

	atomica := &escritaInterceptada{
		base:  &escritaAtomicaFalsa{repos: escritaDoQuadro(q)},
		antes: func() { _ = q.colunas.Apagar(context.Background(), destino) },
	}
	q.card.ComEscritaAtomica(atomica)

	_, err := q.card.Mover(context.Background(), cardID, "ana", destino, ucboard.Vizinhos{})
	if !errors.Is(err, dcoluna.ErrNaoEncontrada) {
		t.Fatalf("erro = %v, esperado destino não encontrado", err)
	}
	relido, _ := q.cards.BuscarPorID(context.Background(), cardID)
	if relido == nil || relido.ColunaID != origem {
		t.Fatalf("card saiu da origem apesar do destino apagado: %+v", relido)
	}
}

func TestAtribuicaoRevalidaMembroSobOLock(t *testing.T) {
	a := novaAtribuicao(t)
	ana := a.conta(t, "Ana", "ana@exemplo.com")
	bruno := a.conta(t, "Bruno", "bruno@exemplo.com")
	boardID, cardID := a.cenarioComCard(t, ana)
	a.convidar(t, boardID, bruno, membro.PapelEditor)

	atomica := &escritaInterceptada{
		base: &escritaAtomicaFalsa{repos: escritaDaColaboracao(a.colaboracao)},
		antes: func() {
			_ = a.membros.Remover(context.Background(), boardID, bruno)
		},
	}
	a.responsavelUC.ComEscritaAtomica(atomica)

	err := a.responsavelUC.Atribuir(context.Background(), cardID, bruno, ana)
	if !errors.Is(err, membro.ErrNaoEMembro) {
		t.Fatalf("erro = %v, esperado ErrNaoEMembro depois da remoção concorrente", err)
	}
	responsaveis, _ := a.responsaveis.DoCard(context.Background(), cardID)
	if len(responsaveis) != 0 {
		t.Fatalf("não membro ficou responsável: %#v", responsaveis)
	}
}

func TestDuasExclusoesDoMesmoAnexoSoPublicamUmEvento(t *testing.T) {
	e := novoExtras()
	_, cardID := e.montar(t, "ana")
	a, err := e.anexoUC.AnexarLink(context.Background(), cardID, "ana", "PR", "https://exemplo.com/pr")
	if err != nil {
		t.Fatalf("anexar: %v", err)
	}

	base := &escritaAtomicaFalsa{repos: escritaDoQuadro(e.quadro)}
	espiao := &publicadorEspiao{}

	// A requisição perdedora já encontrou o anexo fora da transação. Enquanto
	// ela espera o lock, a vencedora remove a linha e publica o único evento.
	vencedora := ucboard.NovoAnexoUseCase(e.membros, e.colunas, e.cards, e.anexos, e.armazem)
	vencedora.ComEscritaAtomica(base)
	vencedora.ComPublicador(espiao)
	var erroDaVencedora error

	perdedora := ucboard.NovoAnexoUseCase(e.membros, e.colunas, e.cards, e.anexos, e.armazem)
	perdedora.ComEscritaAtomica(&escritaInterceptada{
		base: base,
		antes: func() {
			erroDaVencedora = vencedora.Apagar(context.Background(), a.ID, "ana")
		},
	})
	perdedora.ComPublicador(espiao)

	erroDaPerdedora := perdedora.Apagar(context.Background(), a.ID, "ana")
	if erroDaVencedora != nil {
		t.Fatalf("exclusão vencedora: %v", erroDaVencedora)
	}
	if !errors.Is(erroDaPerdedora, danexo.ErrNaoEncontrado) {
		t.Fatalf("erro da perdedora = %v, esperado ErrNaoEncontrado", erroDaPerdedora)
	}
	if len(espiao.entregues) != 1 || espiao.entregues[0].Tipo != evento.AnexoRemovido {
		t.Fatalf("eventos publicados = %#v, esperado um anexo.removido", espiao.entregues)
	}
	if len(base.registrados) != 1 || base.registrados[0].Tipo != evento.AnexoRemovido {
		t.Fatalf("eventos persistidos = %#v, esperado um anexo.removido", base.registrados)
	}
}

func TestConviteRevalidaParticipacaoSobOLock(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")
	bruno := ""

	atomica := &escritaInterceptada{
		base: &escritaAtomicaFalsa{repos: escritaDaColaboracao(c)},
		antes: func() {
			// A primeira busca por email já ocorreu e respondeu "não existe".
			// A conta nasce e entra no quadro enquanto o convite espera o lock.
			bruno = c.conta(t, "Bruno", "bruno@exemplo.com")
			vinculo, _ := membro.Novo(boardID, bruno, membro.PapelEditor)
			_ = c.membros.Salvar(context.Background(), vinculo)
		},
	}
	c.membroUC.ComEscritaAtomica(atomica)

	_, err := c.membroUC.Convidar(context.Background(), boardID, ana, "bruno@exemplo.com", membro.PapelEditor)
	if !errors.Is(err, dconvite.ErrJaEMembro) {
		t.Fatalf("erro = %v, esperado ErrJaEMembro depois da entrada concorrente", err)
	}
	pendentes, _ := c.convites.ListarPendentes(context.Background(), boardID)
	if len(pendentes) != 0 {
		t.Fatalf("foi criado convite redundante: %#v", pendentes)
	}
}

// buscadorUsuarioLimitado representa o repositório ligado ao pool. As duas
// buscas otimistas anteriores à UoW são legítimas; uma terceira, já dentro da
// transação, denunciaria que o callback tentou adquirir outra conexão em vez
// de usar o repositório ligado à tx.
type buscadorUsuarioLimitado struct {
	base     *memoria.Usuarios
	limite   int
	chamadas int
}

func (b *buscadorUsuarioLimitado) BuscarPorID(ctx context.Context, id string) (*dusuario.Usuario, error) {
	b.chamadas++
	if b.chamadas > b.limite {
		return nil, errors.New("consulta externa durante a unidade de trabalho")
	}
	return b.base.BuscarPorID(ctx, id)
}

func (b *buscadorUsuarioLimitado) BuscarPorEmail(ctx context.Context, email string) (*dusuario.Usuario, error) {
	b.chamadas++
	if b.chamadas > b.limite {
		return nil, errors.New("consulta externa durante a unidade de trabalho")
	}
	return b.base.BuscarPorEmail(ctx, email)
}

func TestConvidarNaoAdquireSegundaConexaoDentroDaUoW(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	c.conta(t, "Bruno", "bruno@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")

	externo := &buscadorUsuarioLimitado{base: c.usuarios, limite: 2}
	uc := ucboard.NovoMembroUseCase(c.membros, c.convites, externo, c.boards, c.responsaveis)
	uc.ComEscritaAtomica(&escritaAtomicaFalsa{repos: escritaDaColaboracao(c)})

	if _, err := uc.Convidar(context.Background(), boardID, ana, "bruno@exemplo.com", membro.PapelEditor); err != nil {
		t.Fatalf("convidar: %v", err)
	}
	if externo.chamadas != 2 {
		t.Fatalf("consultas no repositório externo = %d, esperado somente as 2 anteriores à UoW", externo.chamadas)
	}
}

func TestAceitarConviteRedundanteNaoPublicaMembroEntrou(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	bruno := c.conta(t, "Bruno", "bruno@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")
	convite, err := c.membroUC.Convidar(context.Background(), boardID, ana, "bruno@exemplo.com", membro.PapelEditor)
	if err != nil {
		t.Fatalf("convidar: %v", err)
	}

	// Outro caminho colocou Bruno no quadro antes de este link ser aberto.
	vinculo, _ := membro.Novo(boardID, bruno, membro.PapelLeitor)
	if err := c.membros.Salvar(context.Background(), vinculo); err != nil {
		t.Fatalf("preparar vínculo: %v", err)
	}

	base := &escritaAtomicaFalsa{repos: escritaDaColaboracao(c)}
	espiao := &publicadorEspiao{}
	c.membroUC.ComEscritaAtomica(base)
	c.membroUC.ComPublicador(espiao)

	_, papel, err := c.membroUC.Aceitar(context.Background(), convite.Token, bruno)
	if err != nil {
		t.Fatalf("aceitar convite redundante: %v", err)
	}
	if papel != membro.PapelLeitor {
		t.Fatalf("papel devolvido = %q, esperado papel atual leitor", papel)
	}
	for _, e := range espiao.entregues {
		if e.Tipo == evento.MembroEntrou {
			t.Fatal("publicou membro.entrou sem criar uma participação")
		}
	}
	if len(espiao.entregues) != 1 || espiao.entregues[0].Tipo != evento.ConviteRevogado {
		t.Fatalf("eventos = %#v, esperado somente convite.revogado", espiao.entregues)
	}
	resolvido, _ := c.convites.BuscarPorID(context.Background(), convite.Convite.ID)
	if resolvido == nil || resolvido.RevogadoEm == nil {
		t.Fatal("o convite redundante continuou pendente")
	}
}

type instantaneoInterceptado struct {
	leitura ucboard.Leitura
	antes   func()
	depois  func()
}

func (i *instantaneoInterceptado) Executar(_ context.Context, montar func(ucboard.Leitura) error) error {
	if i.antes != nil {
		i.antes()
	}
	err := montar(i.leitura)
	if i.depois != nil {
		i.depois()
	}
	return err
}

func TestAutorizacaoDoDetalhePertenceAoMesmoSnapshot(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	q.criarColuna(t, boardID, "ana", "A fazer")

	q.quadros.ComInstantaneo(&instantaneoInterceptado{
		leitura: ucboard.Leitura{
			Boards: q.boards, Membros: q.membros, Colunas: q.colunas, Cards: q.cards,
			Etiquetas: q.etiquetas, Checklists: q.checklists, Anexos: q.anexos,
			Responsaveis: q.responsaveis, Comentarios: q.comentarios, Publicacoes: q.publicacoes,
		},
		antes: func() { _ = q.membros.Remover(context.Background(), boardID, "ana") },
	})

	_, err := q.quadros.Detalhar(context.Background(), boardID, "ana")
	if !errors.Is(err, dboard.ErrNaoEncontrado) {
		t.Fatalf("erro = %v, esperado autorização negada pelo snapshot", err)
	}
}

func TestEstadoPublicoDoDetalhePertenceAoMesmoSnapshot(t *testing.T) {
	q := novoQuadro()
	boardID := q.criarQuadro(t, "ana", "Estudos")
	var erroAoPublicar error

	q.quadros.ComInstantaneo(&instantaneoInterceptado{
		leitura: ucboard.Leitura{
			Boards: q.boards, Membros: q.membros, Colunas: q.colunas, Cards: q.cards,
			Etiquetas: q.etiquetas, Checklists: q.checklists, Anexos: q.anexos,
			Responsaveis: q.responsaveis, Comentarios: q.comentarios, Publicacoes: q.publicacoes,
		},
		// A publicação comita depois de o snapshot ser montado, mas antes de
		// Detalhar devolver. O resultado precisa continuar descrevendo o estado
		// e a revisão do snapshot, não misturar o aviso novo à revisão antiga.
		depois: func() {
			_, erroAoPublicar = q.publicacao.Publicar(context.Background(), boardID, "ana")
		},
	})

	detalhe, err := q.quadros.Detalhar(context.Background(), boardID, "ana")
	if err != nil {
		t.Fatalf("detalhar: %v", err)
	}
	if erroAoPublicar != nil {
		t.Fatalf("publicação concorrente: %v", erroAoPublicar)
	}
	if detalhe.Publico {
		t.Fatal("snapshot privado saiu marcado como público por uma mudança posterior")
	}
	if p, _ := q.publicacoes.BuscarPorBoard(context.Background(), boardID); p == nil {
		t.Fatal("o cenário não publicou depois do snapshot")
	}
}
