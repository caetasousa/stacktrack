package usecase_test

import (
	"context"
	"errors"
	"testing"

	detiqueta "stacktrack/internal/domain/etiqueta"
	ucboard "stacktrack/internal/usecase/board"
	"stacktrack/test/repository/memoria"
)

// instantaneoDoModal entrega ao usecase repositórios diferentes dos que ele
// recebeu no construtor. Se qualquer pedaço do modal escapar da função
// Executar, o teste abaixo encontra os repositórios originais, todos
// configurados para falhar.
type instantaneoDoModal struct {
	leitura  ucboard.Leitura
	chamadas int
}

func (i *instantaneoDoModal) Executar(_ context.Context, montar func(ucboard.Leitura) error) error {
	i.chamadas++
	return montar(i.leitura)
}

func TestDetalharCardLeModalERevisaoNoMesmoInstantaneo(t *testing.T) {
	ctx := context.Background()
	snapshot := novoQuadro()
	boardID := snapshot.criarQuadro(t, "ana", "Estudos")
	colunaID := snapshot.criarColuna(t, boardID, "ana", "A fazer")
	cardID := snapshot.criarCard(t, colunaID, "ana", "Entregar relatório")

	etiquetaUC := ucboard.NovoEtiquetaUseCase(snapshot.membros, snapshot.colunas, snapshot.cards, snapshot.etiquetas)
	etiqueta, err := etiquetaUC.Criar(ctx, boardID, "ana", "Urgente", detiqueta.CorVermelho)
	if err != nil {
		t.Fatalf("criar etiqueta: %v", err)
	}
	if err := etiquetaUC.Aplicar(ctx, cardID, etiqueta.ID, "ana"); err != nil {
		t.Fatalf("aplicar etiqueta: %v", err)
	}

	checklistUC := ucboard.NovoChecklistUseCase(snapshot.membros, snapshot.colunas, snapshot.cards, snapshot.checklists)
	lista, err := checklistUC.Criar(ctx, cardID, "ana", "Antes de enviar")
	if err != nil {
		t.Fatalf("criar checklist: %v", err)
	}
	if _, err := checklistUC.CriarItem(ctx, lista.ID, "ana", "Revisar números"); err != nil {
		t.Fatalf("criar item: %v", err)
	}

	anexoUC := ucboard.NovoAnexoUseCase(snapshot.membros, snapshot.colunas, snapshot.cards, snapshot.anexos, snapshot.armazem)
	if _, err := anexoUC.AnexarLink(ctx, cardID, "ana", "Referência", "https://example.com"); err != nil {
		t.Fatalf("anexar link: %v", err)
	}

	responsavelUC := ucboard.NovoResponsavelUseCase(snapshot.membros, snapshot.colunas, snapshot.cards, snapshot.responsaveis)
	if err := responsavelUC.Atribuir(ctx, cardID, "ana", "ana"); err != nil {
		t.Fatalf("atribuir responsável: %v", err)
	}

	comentarioUC := ucboard.NovoComentarioUseCase(snapshot.membros, snapshot.colunas, snapshot.cards, snapshot.comentarios)
	if _, err := comentarioUC.Criar(ctx, cardID, "ana", "Tudo conferido"); err != nil {
		t.Fatalf("criar comentário: %v", err)
	}

	quadro, err := snapshot.boards.BuscarPorID(ctx, boardID)
	if err != nil || quadro == nil {
		t.Fatalf("buscar quadro do snapshot: %v", err)
	}
	quadro.Revisao = 37
	if err := snapshot.boards.Salvar(ctx, quadro); err != nil {
		t.Fatalf("carimbar revisão: %v", err)
	}

	// Estes são os repositórios do caminho fora do snapshot. Todos falham: um
	// único acesso a eles denuncia que parte do modal escapou da transação.
	falhaForaDoSnapshot := errors.New("leitura fora do snapshot")
	membros := memoria.NovosMembros()
	cards := memoria.NovosCards()
	colunas := memoria.NovasColunas(cards)
	cards.LigarColunas(colunas)
	boards := memoria.NovosBoards(membros)
	etiquetas := memoria.NovasEtiquetas()
	checklists := memoria.NovasChecklists()
	anexos := memoria.NovosAnexos()
	responsaveis := memoria.NovosResponsaveis()
	comentarios := memoria.NovosComentarios()
	boards.ErroForcado = falhaForaDoSnapshot
	membros.ErroForcado = falhaForaDoSnapshot
	colunas.ErroForcado = falhaForaDoSnapshot
	cards.ErroForcado = falhaForaDoSnapshot
	etiquetas.ErroForcado = falhaForaDoSnapshot
	checklists.ErroForcado = falhaForaDoSnapshot
	anexos.ErroForcado = falhaForaDoSnapshot
	responsaveis.ErroForcado = falhaForaDoSnapshot
	comentarios.ErroForcado = falhaForaDoSnapshot

	uc := ucboard.NovoCardUseCase(
		boards, membros, colunas, cards, etiquetas, checklists, anexos,
		responsaveis, comentarios, nil,
	)
	i := &instantaneoDoModal{leitura: ucboard.Leitura{
		Boards: snapshot.boards, Membros: snapshot.membros,
		Colunas: snapshot.colunas, Cards: snapshot.cards,
		Etiquetas: snapshot.etiquetas, Checklists: snapshot.checklists,
		Anexos: snapshot.anexos, Responsaveis: snapshot.responsaveis,
		Comentarios: snapshot.comentarios,
	}}
	uc.ComInstantaneo(i)

	detalhe, err := uc.Detalhar(ctx, cardID, "ana")
	if err != nil {
		t.Fatalf("detalhar: %v", err)
	}
	if i.chamadas != 1 {
		t.Fatalf("instantâneo executado %d vezes, esperado uma", i.chamadas)
	}
	if detalhe.BoardID != boardID || detalhe.Revisao != 37 {
		t.Fatalf("carimbo = (%q, %d), esperado (%q, 37)", detalhe.BoardID, detalhe.Revisao, boardID)
	}
	if len(detalhe.Responsaveis) != 1 || len(detalhe.Comentarios) != 1 || len(detalhe.Etiquetas) != 1 || len(detalhe.Anexos) != 1 {
		t.Fatalf("agregados incompletos: responsáveis=%d comentários=%d etiquetas=%d anexos=%d",
			len(detalhe.Responsaveis), len(detalhe.Comentarios), len(detalhe.Etiquetas), len(detalhe.Anexos))
	}
	if len(detalhe.Checklists) != 1 || len(detalhe.Checklists[0].Itens) != 1 {
		t.Fatalf("checklists incompletas: %+v", detalhe.Checklists)
	}
}
