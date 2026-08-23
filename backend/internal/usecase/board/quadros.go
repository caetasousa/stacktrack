package board

import (
	"context"

	dboard "stacktrack/internal/domain/board"
	"stacktrack/internal/domain/card"
	"stacktrack/internal/domain/coluna"
	detiqueta "stacktrack/internal/domain/etiqueta"
	"stacktrack/internal/domain/evento"
	"stacktrack/internal/domain/membro"

	"github.com/google/uuid"
)

// QuadroUseCase reúne as operações sobre o quadro em si. As cinco compartilham
// as mesmas dependências e a mesma checagem de acesso — separá-las em cinco
// structs só multiplicaria construtor e fiação, sem separar responsabilidade
// nenhuma.
type QuadroUseCase struct {
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
	// instantaneo é a leitura consistente que monta o snapshot do quadro. Entra
	// por ComInstantaneo, e não pelo construtor, pela mesma razão do publicador
	// de eventos: sem ele o usecase funciona lendo pelos repositórios de
	// sempre, que é o que os testes de regra querem.
	instantaneo InstantaneoConsistente
	// publicacoes serve APENAS para dizer se o quadro tem link público ligado.
	// Entra por ComPublicacoes, e não pelo construtor, pela mesma razão do
	// publicador de eventos: era um décimo primeiro parâmetro posicional em
	// todas as chamadas, inclusive nas dos testes que não têm nada com isso.
	// Quem publica e quem revoga é o PublicacaoUseCase — daqui não se escreve.
	publicacoes RepositorioPublicacao
	// atividades serve APENAS para o selo de "quem moveu por último" em cada
	// card. Entra por ComAtividades pela mesma razão de publicacoes: é leitura
	// acessória, e um parâmetro posicional a mais em todas as chamadas — testes
	// inclusive — só para isso não se paga.
	atividades RepositorioAtividade
}

// ComPublicacoes liga a consulta do link público. Sem ela ligada, todo quadro
// se descreve como não publicado — o que é o correto para os testes de regra,
// que constroem o usecase sem essa porta.
func (uc *QuadroUseCase) ComPublicacoes(publicacoes RepositorioPublicacao) {
	uc.publicacoes = publicacoes
}

// ComAtividades liga a consulta de quem moveu cada card por último. Sem ela, os
// cards vêm sem esse selo — ausência, e não informação errada.
func (uc *QuadroUseCase) ComAtividades(atividades RepositorioAtividade) {
	uc.atividades = atividades
}

// NovoQuadroUseCase cria uma instância de QuadroUseCase com as dependências injetadas.
func NovoQuadroUseCase(
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
) *QuadroUseCase {
	return &QuadroUseCase{
		boards: boards, membros: membros, colunas: colunas, cards: cards,
		etiquetas: etiquetas, checklists: checklists, anexos: anexos,
		responsaveis: responsaveis, comentarios: comentarios, armazem: armazem,
	}
}

// Criar cria um quadro e vincula quem criou como dono no mesmo commit. Retorna
// os erros de validação de título do domínio.
func (uc *QuadroUseCase) Criar(ctx context.Context, usuarioID, titulo string) (*dboard.Board, error) {
	b, err := dboard.Novo(uuid.NewString(), titulo)
	if err != nil {
		return nil, err
	}
	vinculo, err := membro.Novo(b.ID, usuarioID, membro.PapelDono)
	if err != nil {
		return nil, err
	}
	if err := uc.escreverEPublicar(ctx, evento.QuadroCriado, b.ID, usuarioID,
		DadosDoQuadro{Titulo: b.Titulo}, uc.escrita(), func(e Escrita) error {
			if err := e.Boards.Salvar(ctx, b); err != nil {
				return err
			}
			return e.Membros.Salvar(ctx, vinculo)
		}); err != nil {
		return nil, err
	}
	return b, nil
}

// Listar devolve os quadros de que o usuário participa, com o papel dele em
// cada um. Nunca devolve quadro de terceiro: a consulta parte do vínculo.
func (uc *QuadroUseCase) Listar(ctx context.Context, usuarioID string) ([]Resumo, error) {
	return uc.boards.ListarDoUsuario(ctx, usuarioID)
}

// Detalhar devolve o quadro com colunas e cards, em ordem de posição. Retorna
// dboard.ErrNaoEncontrado se o quadro não existir ou se o usuário não
// participar dele.
func (uc *QuadroUseCase) Detalhar(ctx context.Context, boardID, usuarioID string) (*Detalhado, error) {
	// TUDO abaixo roda sobre UM ÚNICO instantâneo do banco.
	//
	// São dez consultas para montar um quadro. Sob READ COMMITTED cada uma
	// enxerga o banco no instante em que ela rodou, então uma escrita no meio da
	// sequência aparece para as seguintes e não para as anteriores — e o
	// snapshot devolvido descreve um estado que nunca existiu: card na lista de
	// cards e ausente da contagem de comentários, coluna que sumiu deixando
	// cards órfãos.
	//
	// Isso importa mais desde A3, porque o snapshot passou a sair CARIMBADO com
	// uma revisão. Um estado incoerente carimbado como coerente faz o cliente
	// aplicar os eventos seguintes por cima dele sem nunca descobrir que partiu
	// errado.
	var (
		vinculo             *membro.Membro
		b                   *dboard.Board
		listaColunas        []coluna.Coluna
		listaCards          []card.Card
		etiquetasPorCard    map[string][]string
		progressoPorCard    map[string]Progresso
		anexosPorCard       map[string]int
		responsaveisPorCard map[string][]Responsavel
		comentariosPorCard  map[string]int
		etiquetasDoBoard    []detiqueta.Etiqueta
		publicado           bool
	)

	montar := func(l Leitura) error {
		var err error
		// A autorização faz parte do snapshot. Lê-la antes da transação podia
		// combinar um papel antigo com uma revisão nova depois de uma remoção ou
		// rebaixamento concorrente.
		if vinculo, err = acesso(ctx, l.Membros, boardID, usuarioID); err != nil {
			return err
		}
		if b, err = l.Boards.BuscarPorID(ctx, boardID); err != nil {
			return err
		}
		if b == nil {
			return dboard.ErrNaoEncontrado
		}
		if listaColunas, err = l.Colunas.ListarDoBoard(ctx, boardID); err != nil {
			return err
		}
		if listaCards, err = l.Cards.ListarDoBoard(ctx, boardID); err != nil {
			return err
		}
		// Os resumos vêm do banco numa consulta cada, e não card a card: a tela
		// do quadro mostra selo de etiqueta, "2/5" de checklist e contagem de
		// anexos em TODO card, e uma consulta por card seria um N+1 que piora
		// justamente nos quadros grandes.
		if etiquetasPorCard, err = l.Etiquetas.EtiquetasDoBoardPorCard(ctx, boardID); err != nil {
			return err
		}
		if progressoPorCard, err = l.Checklists.ProgressoDoBoard(ctx, boardID); err != nil {
			return err
		}
		if anexosPorCard, err = l.Anexos.ContarPorCardDoBoard(ctx, boardID); err != nil {
			return err
		}
		if responsaveisPorCard, err = l.Responsaveis.DoBoardPorCard(ctx, boardID); err != nil {
			return err
		}
		if comentariosPorCard, err = l.Comentarios.ContarPorCardDoBoard(ctx, boardID); err != nil {
			return err
		}
		if etiquetasDoBoard, err = l.Etiquetas.ListarDoBoard(ctx, boardID); err != nil {
			return err
		}
		// O aviso de publicação pertence ao snapshot e à revisão tanto quanto
		// cards e colunas. Ler depois da transação podia devolver `Publico=true`
		// com uma revisão anterior ao evento de publicação (ou o inverso),
		// deixando a reconciliação confirmar uma combinação que nunca existiu.
		if uc.publicacoes != nil {
			p, err := l.Publicacoes.BuscarPorBoard(ctx, boardID)
			if err != nil {
				return err
			}
			publicado = p != nil
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

	// A movimentação fica fora do instantâneo: é um selo derivado do log, não
	// estado que o cliente aplica. A publicação, ao contrário, foi lida acima
	// porque agora possui evento e revisão próprios.
	movimentacoesPorCard, err := uc.ultimasMovimentacoes(ctx, boardID)
	if err != nil {
		return nil, err
	}

	return &Detalhado{
		Board: *b,
		// A revisão vem da MESMA leitura que montou o estado: é ela que o
		// cliente devolve ao WebSocket em `?revisao=N`, e um número lido fora do
		// instantâneo descreveria um estado diferente do que está sendo
		// entregue.
		Revisao:   b.Revisao,
		Papel:     vinculo.Papel,
		Colunas:   agrupar(listaColunas, listaCards, responsaveisPorCard, etiquetasPorCard, progressoPorCard, anexosPorCard, comentariosPorCard, movimentacoesPorCard),
		Publico:   publicado,
		Etiquetas: etiquetasDoBoard,
	}, nil
}

// ultimasMovimentacoes devolve, por card, quem o moveu por último. Sem a porta
// ligada devolve um mapa vazio — ver ComAtividades.
func (uc *QuadroUseCase) ultimasMovimentacoes(ctx context.Context, boardID string) (map[string]Movimentacao, error) {
	if uc.atividades == nil {
		return map[string]Movimentacao{}, nil
	}
	return uc.atividades.UltimaMovimentacaoPorCard(ctx, boardID)
}

// ComInstantaneo liga a leitura consistente do snapshot. Ver
// InstantaneoConsistente para o que ela evita.
func (uc *QuadroUseCase) ComInstantaneo(i InstantaneoConsistente) {
	uc.instantaneo = i
}

// leitura monta os repositórios do caminho NÃO transacional, para quando não há
// instantâneo ligado.
func (uc *QuadroUseCase) leitura() Leitura {
	return Leitura{
		Boards: uc.boards, Membros: uc.membros, Colunas: uc.colunas, Cards: uc.cards,
		Etiquetas: uc.etiquetas, Checklists: uc.checklists, Anexos: uc.anexos,
		Responsaveis: uc.responsaveis, Comentarios: uc.comentarios, Publicacoes: uc.publicacoes,
	}
}

// PodeVer informa se o usuário participa do quadro. É a pergunta que o
// handshake do WebSocket faz antes de aceitar a conexão — sem ela, qualquer
// pessoa autenticada assinaria a sala de qualquer quadro e leria tudo que
// acontece nele em tempo real.
//
// Devolve bool, e não erro, porque quem chama só precisa decidir entre aceitar
// e recusar: distinguir "não existe" de "não participa" seria justamente a
// informação que o 404 do resto da API esconde.
func (uc *QuadroUseCase) PodeVer(ctx context.Context, boardID, usuarioID string) bool {
	_, err := acesso(ctx, uc.membros, boardID, usuarioID)
	return err == nil
}

// DefinirFundo troca o fundo do quadro. Exige papel de administração.
func (uc *QuadroUseCase) DefinirFundo(ctx context.Context, boardID, usuarioID, fundo string) (*dboard.Board, error) {
	if _, err := acessoDeAdministracao(ctx, uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}

	b, err := uc.boards.BuscarPorID(ctx, boardID)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, dboard.ErrNaoEncontrado
	}
	if err := b.DefinirFundo(fundo); err != nil {
		return nil, err
	}
	if err := uc.escreverEPublicar(ctx, evento.QuadroFundo, boardID, usuarioID,
		DadosDoQuadro{Fundo: b.FundoEfetivo()},
		uc.escrita(), func(e Escrita) error {
			if err := revalidarAdministracao(ctx, e, boardID, usuarioID); err != nil {
				return err
			}
			return e.Boards.DefinirFundo(ctx, b.ID, b.Fundo, b.AtualizadoEm)
		}); err != nil {
		return nil, err
	}
	return b, nil
}

// Renomear troca o título do quadro. Exige papel de administração (dono).
func (uc *QuadroUseCase) Renomear(ctx context.Context, boardID, usuarioID, titulo string) (*dboard.Board, error) {
	if _, err := acessoDeAdministracao(ctx, uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}

	b, err := uc.boards.BuscarPorID(ctx, boardID)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, dboard.ErrNaoEncontrado
	}
	tituloAnterior := b.Titulo
	if err := b.Renomear(titulo); err != nil {
		return nil, err
	}
	if err := uc.escreverEPublicar(ctx, evento.QuadroRenomeado, boardID, usuarioID,
		DadosDoQuadro{Titulo: b.Titulo, TituloAnterior: tituloAnterior},
		uc.escrita(), func(e Escrita) error {
			if err := revalidarAdministracao(ctx, e, boardID, usuarioID); err != nil {
				return err
			}
			return e.Boards.Renomear(ctx, b.ID, b.Titulo, b.AtualizadoEm)
		}); err != nil {
		return nil, err
	}
	return b, nil
}

// Apagar remove o quadro. Exige papel de administração (dono). Colunas, cards
// e vínculos vão junto, pelo ON DELETE CASCADE do schema.
func (uc *QuadroUseCase) Apagar(ctx context.Context, boardID, usuarioID string) error {
	if _, err := acessoDeAdministracao(ctx, uc.membros, boardID, usuarioID); err != nil {
		return err
	}
	var orfaos []string
	apagar := func(e Escrita) error {
		if err := revalidarAdministracao(ctx, e, boardID, usuarioID); err != nil {
			return err
		}
		var err error
		orfaos, err = e.Anexos.CaminhosDeArquivoDoBoard(ctx, boardID)
		if err != nil {
			return err
		}
		// O outbox guarda o board_id sem chave estrangeira justamente para esta
		// linha sobreviver ao CASCADE que está prestes a acontecer: uma FK
		// levaria junto o registro que existe para sobreviver a ele.
		if err := registrarExclusaoDeArquivos(ctx, e, boardID, orfaos); err != nil {
			return err
		}
		return e.Boards.Apagar(ctx, boardID)
	}
	if uc.atomica != nil {
		if err := uc.atomica.ExcluirQuadro(ctx, boardID, apagar); err != nil {
			return err
		}
	} else if err := apagar(uc.escrita()); err != nil {
		return err
	}
	// Exceção terminal do outbox: o próprio commit apagou `board_events` por
	// cascata e não há mais linha de board que aceite um novo evento pela FK.
	// Ainda assim as abas conectadas precisam sair, em vez de insistirem em GETs
	// que só podem responder 404. Por isso este sinal é efêmero e nasce somente
	// DEPOIS do commit bem-sucedido.
	uc.publicarEfemero(evento.QuadroApagado, boardID, usuarioID, nil)
	return nil
}

// agrupar distribui os cards nas colunas a que pertencem, preservando a ordem
// em que cada lista chegou do repositório (ambas por posição), e pendura em
// cada card o resumo do que ele carrega.
func agrupar(
	colunas []coluna.Coluna,
	cards []card.Card,
	responsaveisPorCard map[string][]Responsavel,
	etiquetasPorCard map[string][]string,
	progressoPorCard map[string]Progresso,
	anexosPorCard map[string]int,
	comentariosPorCard map[string]int,
	movimentacoesPorCard map[string]Movimentacao,
) []ColunaComCards {
	porColuna := make(map[string][]CardNoQuadro, len(colunas))
	for _, c := range cards {
		etiquetas := etiquetasPorCard[c.ID]
		if etiquetas == nil {
			etiquetas = []string{}
		}
		responsaveis := responsaveisPorCard[c.ID]
		if responsaveis == nil {
			responsaveis = []Responsavel{}
		}
		// Ponteiro só quando existe: o card nunca movido não recebe selo nenhum.
		var movimentacao *Movimentacao
		if m, houve := movimentacoesPorCard[c.ID]; houve {
			movimentacao = &m
		}
		porColuna[c.ColunaID] = append(porColuna[c.ColunaID], CardNoQuadro{
			Card:               c,
			Responsaveis:       responsaveis,
			Etiquetas:          etiquetas,
			Checklist:          progressoPorCard[c.ID],
			QtdAnexos:          anexosPorCard[c.ID],
			QtdComentarios:     comentariosPorCard[c.ID],
			UltimaMovimentacao: movimentacao,
		})
	}

	resultado := make([]ColunaComCards, 0, len(colunas))
	for _, col := range colunas {
		cardsDaColuna := porColuna[col.ID]
		if cardsDaColuna == nil {
			// Slice vazia, e não nil: o JSON de uma coluna sem cards precisa
			// sair como [] e não null, senão o frontend teria de tratar os dois.
			cardsDaColuna = []CardNoQuadro{}
		}
		resultado = append(resultado, ColunaComCards{Coluna: col, Cards: cardsDaColuna})
	}
	return resultado
}
