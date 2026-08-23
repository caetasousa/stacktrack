package board

import (
	"context"
	"errors"
	"time"

	danexo "stacktrack/internal/domain/anexo"
	dboard "stacktrack/internal/domain/board"
	dcard "stacktrack/internal/domain/card"
	dchecklist "stacktrack/internal/domain/checklist"
	dcoluna "stacktrack/internal/domain/coluna"
	dcor "stacktrack/internal/domain/cor"
	detiqueta "stacktrack/internal/domain/etiqueta"
	"stacktrack/internal/domain/evento"
	"stacktrack/internal/domain/ordem"

	"github.com/google/uuid"
)

// CardUseCase reúne as operações sobre cards.
type CardUseCase struct {
	eventos
	exclusoes
	boards       RepositorioBoard
	membros      RepositorioMembro
	colunas      RepositorioColuna
	cards        RepositorioCard
	etiquetas    RepositorioEtiqueta
	checklists   RepositorioChecklist
	anexos       RepositorioAnexo
	responsaveis RepositorioResponsavel
	comentarios  RepositorioComentario
	// armazem existe só para a LIMPEZA do disco ao apagar. Ver limpeza.go.
	armazem armazemDeArquivos
	// instantaneo mantém o card, a autorização, a revisão do quadro e tudo que
	// o modal mostra sobre a mesma fotografia do banco. Sem ele, cada coleção
	// seria lida sob um instante diferente de READ COMMITTED.
	instantaneo InstantaneoConsistente
}

// NovoCardUseCase cria uma instância de CardUseCase com as dependências injetadas.
func NovoCardUseCase(
	boards RepositorioBoard,
	membros RepositorioMembro,
	colunas RepositorioColuna,
	cards RepositorioCard,
	etiquetas RepositorioEtiqueta,
	checklists RepositorioChecklist,
	anexos RepositorioAnexo,
	responsaveis RepositorioResponsavel,
	comentarios RepositorioComentario,
	armazem armazemDeArquivos,
) *CardUseCase {
	return &CardUseCase{
		boards: boards, membros: membros, colunas: colunas, cards: cards,
		etiquetas: etiquetas, checklists: checklists, anexos: anexos, responsaveis: responsaveis, comentarios: comentarios,
		armazem: armazem,
	}
}

// Detalhar devolve o card com tudo que pende dele — é o que o modal mostra.
// Qualquer membro pode ver; editar é que exige papel.
func (uc *CardUseCase) Detalhar(ctx context.Context, cardID, usuarioID string) (*CardDetalhado, error) {
	var (
		c            *dcard.Card
		col          *dcoluna.Coluna
		quadro       *dboard.Board
		responsaveis []Responsavel
		comentarios  []ComentarioComAutor
		etiquetas    []detiqueta.Etiqueta
		listas       []dchecklist.Checklist
		comItens     []ChecklistComItens
		anexos       []danexo.Anexo
	)

	montar := func(l Leitura) error {
		var err error
		if c, err = l.Cards.BuscarPorID(ctx, cardID); err != nil {
			return err
		}
		if c == nil {
			return dcard.ErrNaoEncontrado
		}
		if col, err = l.Colunas.BuscarPorID(ctx, c.ColunaID); err != nil {
			return err
		}
		if col == nil {
			return dcard.ErrNaoEncontrado
		}
		// O vínculo faz parte do mesmo snapshot. Checá-lo antes da transação
		// permitiria devolver o modal depois de uma remoção concorrente.
		if _, err = acesso(ctx, l.Membros, col.BoardID, usuarioID); err != nil {
			return traduzirParaCard(err)
		}
		if quadro, err = l.Boards.BuscarPorID(ctx, col.BoardID); err != nil {
			return err
		}
		if quadro == nil {
			return dcard.ErrNaoEncontrado
		}

		if responsaveis, err = l.Responsaveis.DoCard(ctx, cardID); err != nil {
			return err
		}
		if comentarios, err = l.Comentarios.ListarDoCard(ctx, cardID); err != nil {
			return err
		}
		if etiquetas, err = l.Etiquetas.EtiquetasDoCard(ctx, cardID); err != nil {
			return err
		}
		if listas, err = l.Checklists.ListarDoCard(ctx, cardID); err != nil {
			return err
		}
		if anexos, err = l.Anexos.ListarDoCard(ctx, cardID); err != nil {
			return err
		}

		comItens = make([]ChecklistComItens, 0, len(listas))
		for _, lista := range listas {
			itens, err := l.Checklists.ListarItens(ctx, lista.ID)
			if err != nil {
				return err
			}
			comItens = append(comItens, ChecklistComItens{Checklist: lista, Itens: itens})
		}
		return nil
	}

	if uc.instantaneo != nil {
		if err := uc.instantaneo.Executar(ctx, montar); err != nil {
			return nil, err
		}
	} else if err := montar(uc.leitura()); err != nil {
		return nil, err
	}

	return &CardDetalhado{
		Card: *c, BoardID: col.BoardID, Revisao: quadro.Revisao, Responsaveis: responsaveis,
		Etiquetas: etiquetas, Checklists: comItens, Anexos: anexos,
		Comentarios: comentarios,
	}, nil
}

// ComInstantaneo liga a leitura consistente do modal. A mesma implementação é
// compartilhada com o detalhe do quadro no wiring de produção.
func (uc *CardUseCase) ComInstantaneo(i InstantaneoConsistente) {
	uc.instantaneo = i
}

// leitura monta o caminho sem transação usado pelos testes de regra. Em
// produção ComInstantaneo substitui todos estes repositórios pelos ligados à
// mesma transação REPEATABLE READ.
func (uc *CardUseCase) leitura() Leitura {
	return Leitura{
		Boards: uc.boards, Membros: uc.membros, Colunas: uc.colunas, Cards: uc.cards,
		Etiquetas: uc.etiquetas, Checklists: uc.checklists, Anexos: uc.anexos,
		Responsaveis: uc.responsaveis, Comentarios: uc.comentarios,
	}
}

// DefinirPrazo marca ou limpa a data de entrega do card. Exige papel de edição.
func (uc *CardUseCase) DefinirPrazo(ctx context.Context, cardID, usuarioID string, prazo *time.Time) (*dcard.Card, error) {
	c, boardID, err := uc.carregarComAcessoDeEdicao(ctx, cardID, usuarioID)
	if err != nil {
		return nil, err
	}
	c.DefinirPrazo(prazo)
	if err := uc.escreverEPublicarNoCard(ctx, evento.CardAlterado, boardID, c.ID, usuarioID,
		DadosDoCard{CardID: c.ID, Titulo: c.Titulo, ColunaID: c.ColunaID, Version: c.Version},
		uc.escrita(), func(e Escrita) error {
			if err := revalidarEdicao(ctx, e, boardID, usuarioID); err != nil {
				return err
			}
			return e.Cards.Atualizar(ctx, c)
		}); err != nil {
		return nil, err
	}
	return c, nil
}

// Criar acrescenta um card no fim da coluna. Exige papel de edição no quadro
// da coluna.
func (uc *CardUseCase) Criar(ctx context.Context, colunaID, usuarioID, titulo, descricao string, cores dcor.Cor) (*dcard.Card, error) {
	col, err := uc.colunas.BuscarPorID(ctx, colunaID)
	if err != nil {
		return nil, err
	}
	if col == nil {
		return nil, dcoluna.ErrNaoEncontrada
	}
	if _, err := acessoDeEdicao(ctx, uc.membros, col.BoardID, usuarioID); err != nil {
		return nil, traduzirParaColuna(err)
	}

	// O id pode nascer antes do lock; a chave, não. Duas criações concorrentes
	// numa coluna vazia calculavam a mesma posição a partir do mesmo estado e o
	// schema ainda não tem uma UNIQUE que pudesse impedir o empate. O cálculo e
	// o INSERT agora acontecem na mesma unidade de trabalho, sob o lock do
	// quadro, e a segunda criação enxerga a primeira.
	cardID := uuid.NewString()
	rascunho, err := dcard.Novo(cardID, colunaID, titulo, descricao, cores, ordem.ChaveInicial)
	if err != nil {
		return nil, err
	}
	var c *dcard.Card
	dados := &DadosDoCard{CardID: cardID, Titulo: rascunho.Titulo, Coluna: col.Titulo, ColunaID: colunaID, Version: rascunho.Version}
	if err := uc.escreverEPublicarNoCard(ctx, evento.CardCriado, col.BoardID, cardID, usuarioID,
		dados,
		uc.escrita(), func(e Escrita) error {
			if err := revalidarEdicao(ctx, e, col.BoardID, usuarioID); err != nil {
				return err
			}
			// A coluna pode ter sido apagada enquanto esta requisição esperava o
			// lock. Conferi-la aqui evita transformar esse caso esperado numa
			// violação crua de FK.
			atual, err := e.Colunas.BuscarPorID(ctx, colunaID)
			if err != nil {
				return err
			}
			if atual == nil || atual.BoardID != col.BoardID {
				return dcoluna.ErrNaoEncontrada
			}
			dados.Coluna = atual.Titulo

			chave, err := uc.chaveNoFimDaColunaSobLock(ctx, e, colunaID)
			if err != nil {
				return erroDeOrdenacaoDoCard(err)
			}
			c, err = dcard.Novo(cardID, colunaID, rascunho.Titulo, rascunho.Descricao, rascunho.Cor, chave)
			if err != nil {
				return err
			}
			return e.Cards.Salvar(ctx, c)
		}); err != nil {
		return nil, err
	}
	return c, nil
}

// Editar troca título, descrição e cor do card, incrementando a versão. Exige
// papel de edição.
//
// Devolve ErrConflito (409) quando versaoVista não é a versão atual — alguém
// gravou entre a leitura e esta escrita. Recusar é a decisão: sobrescrever
// apagaria o trabalho da outra pessoa sem ninguém ficar sabendo.
func (uc *CardUseCase) Editar(ctx context.Context, cardID, usuarioID, titulo, descricao string, cores dcor.Cor, versaoVista int) (*dcard.Card, error) {
	c, boardID, err := uc.carregarComAcessoDeEdicao(ctx, cardID, usuarioID)
	if err != nil {
		return nil, err
	}
	// A conferência aqui pega o caso que o WHERE do SQL não pega: quem abriu o
	// card há cinco minutos e só agora salvou. O banco, sozinho, só percebe
	// duas escritas simultâneas — e o conflito que incomoda de verdade é o
	// lento.
	//
	// Zero significa "não confira", e é o que o arraste manda.
	if versaoVista != 0 && c.Version != versaoVista {
		return nil, dcard.ErrConflito
	}
	// O título de ANTES é lido aqui, enquanto ainda existe: depois do Editar ele
	// se perde, e sem ele o histórico não consegue dizer "renomeou de X para Y".
	tituloAnterior := c.Titulo
	if err := c.Editar(titulo, descricao, cores); err != nil {
		return nil, err
	}
	dados := DadosDoCard{CardID: c.ID, Titulo: c.Titulo, ColunaID: c.ColunaID, Version: c.Version}
	if tituloAnterior != c.Titulo {
		dados.TituloAnterior = tituloAnterior
	}
	if err := uc.escreverEPublicarNoCard(ctx, evento.CardAlterado, boardID, c.ID, usuarioID, dados,
		uc.escrita(), func(e Escrita) error {
			if err := revalidarEdicao(ctx, e, boardID, usuarioID); err != nil {
				return err
			}
			return e.Cards.Atualizar(ctx, c)
		}); err != nil {
		return nil, err
	}
	return c, nil
}

// Apagar remove o card. Exige papel de edição.
func (uc *CardUseCase) Apagar(ctx context.Context, cardID, usuarioID string) error {
	c, boardID, err := uc.carregarComAcessoDeEdicao(ctx, cardID, usuarioID)
	if err != nil {
		return err
	}
	// O título vai no evento porque o card não estará mais lá para respondê-lo:
	// "apagou um card" é bem menos útil que "apagou Migração", e depois do
	// DELETE não há de onde tirar o nome.
	var orfaos []string
	dados := &DadosDoCard{CardID: cardID, Titulo: c.Titulo}
	if err := uc.escreverEPublicarNoCard(ctx, evento.CardApagado, boardID, cardID, usuarioID,
		dados,
		uc.escrita(), func(e Escrita) error {
			if err := revalidarEdicao(ctx, e, boardID, usuarioID); err != nil {
				return err
			}
			atual, err := e.Cards.BuscarPorID(ctx, cardID)
			if err != nil {
				return err
			}
			if atual == nil {
				return dcard.ErrNaoEncontrado
			}
			dados.Titulo = atual.Titulo
			orfaos, err = e.Anexos.CaminhosDeArquivoDoCard(ctx, cardID)
			if err != nil {
				return err
			}
			// O outbox é gravado ANTES do DELETE, na mesma transação: depois
			// dele as linhas de `anexos` já foram pelo CASCADE, e não há de
			// onde tirar as chaves físicas.
			if err := registrarExclusaoDeArquivos(ctx, e, boardID, orfaos); err != nil {
				return err
			}
			return e.Cards.Apagar(ctx, cardID)
		}); err != nil {
		return err
	}
	return nil
}

// carregarComAcessoDeEdicao percorre card → coluna → quadro para descobrir a
// quem pedir permissão. É o caminho que faz a autorização valer também para o
// card, que não guarda o quadro a que pertence.
// Devolve também o id do quadro: é a sala onde o evento correspondente será
// publicado, e o card sozinho não sabe a que quadro pertence.
func (uc *CardUseCase) carregarComAcessoDeEdicao(ctx context.Context, cardID, usuarioID string) (*dcard.Card, string, error) {
	c, err := uc.cards.BuscarPorID(ctx, cardID)
	if err != nil {
		return nil, "", err
	}
	if c == nil {
		return nil, "", dcard.ErrNaoEncontrado
	}
	col, err := uc.colunas.BuscarPorID(ctx, c.ColunaID)
	if err != nil {
		return nil, "", err
	}
	if col == nil {
		// Card sem coluna é inconsistência de dados, não "não encontrado":
		// o ON DELETE CASCADE deveria ter levado o card junto.
		return nil, "", dcard.ErrNaoEncontrado
	}
	if _, err := acessoDeEdicao(ctx, uc.membros, col.BoardID, usuarioID); err != nil {
		return nil, "", traduzirParaCard(err)
	}
	return c, col.BoardID, nil
}

// dcardNaoEncontrado é um atalho para os usecases que percorrem
// card → coluna → quadro e precisam parar no meio do caminho.
var dcardNaoEncontrado = dcard.ErrNaoEncontrado

// traduzirParaColuna converte "quadro não encontrado" em "coluna não
// encontrada": quem chamou pediu uma coluna, e a resposta não deve revelar que
// existe um quadro por trás dela.
func traduzirParaColuna(err error) error {
	if errors.Is(err, dboard.ErrNaoEncontrado) {
		return dcoluna.ErrNaoEncontrada
	}
	return err
}

// traduzirParaCard faz o mesmo para as rotas de card.
func traduzirParaCard(err error) error {
	if errors.Is(err, dboard.ErrNaoEncontrado) {
		return dcard.ErrNaoEncontrado
	}
	return err
}

// traduzirParaEtiqueta faz o mesmo para as rotas de etiqueta.
func traduzirParaEtiqueta(err error) error {
	if errors.Is(err, dboard.ErrNaoEncontrado) {
		return detiqueta.ErrNaoEncontrada
	}
	return err
}

// traduzirParaChecklist faz o mesmo para as rotas de checklist.
func traduzirParaChecklist(err error) error {
	if errors.Is(err, dboard.ErrNaoEncontrado) {
		return dchecklist.ErrNaoEncontrada
	}
	return err
}

// traduzirParaAnexo faz o mesmo para as rotas de anexo.
//
// Cobre também "card não encontrado" porque o caminho até o anexo passa pelo
// card: sem isso, quem pede um anexo de quadro alheio recebe de volta em que
// etapa da cadeia a busca parou, o que é informação sobre dado que ele não
// pode ver.
func traduzirParaAnexo(err error) error {
	if errors.Is(err, dboard.ErrNaoEncontrado) || errors.Is(err, dcard.ErrNaoEncontrado) {
		return danexo.ErrNaoEncontrado
	}
	return err
}
