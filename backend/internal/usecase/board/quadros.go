package board

import (
	"context"
	dboard "stacktrack/internal/domain/board"
	"stacktrack/internal/domain/card"
	"stacktrack/internal/domain/coluna"
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
	boards       RepositorioBoard
	membros      RepositorioMembro
	colunas      RepositorioColuna
	cards        RepositorioCard
	etiquetas    repositorioEtiqueta
	checklists   repositorioChecklist
	anexos       repositorioAnexo
	responsaveis RepositorioResponsavel
	comentarios  RepositorioComentario
	// armazem existe só para a LIMPEZA do disco ao apagar. Ver limpeza.go.
	armazem armazemDeArquivos
	// publicacoes serve APENAS para dizer se o quadro tem link público ligado.
	// Entra por ComPublicacoes, e não pelo construtor, pela mesma razão do
	// publicador de eventos: era um décimo primeiro parâmetro posicional em
	// todas as chamadas, inclusive nas dos testes que não têm nada com isso.
	// Quem publica e quem revoga é o PublicacaoUseCase — daqui não se escreve.
	publicacoes RepositorioPublicacao
}

// ComPublicacoes liga a consulta do link público. Sem ela ligada, todo quadro
// se descreve como não publicado — o que é o correto para os testes de regra,
// que constroem o usecase sem essa porta.
func (uc *QuadroUseCase) ComPublicacoes(publicacoes RepositorioPublicacao) {
	uc.publicacoes = publicacoes
}

// NovoQuadroUseCase cria uma instância de QuadroUseCase com as dependências injetadas.
func NovoQuadroUseCase(
	boards RepositorioBoard,
	membros RepositorioMembro,
	colunas RepositorioColuna,
	cards RepositorioCard,
	etiquetas repositorioEtiqueta,
	checklists repositorioChecklist,
	anexos repositorioAnexo,
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

// Criar cria um quadro e vincula quem criou como dono. Retorna os erros de
// validação de título do domínio.
//
// O vínculo é gravado logo depois do quadro, e não numa transação com ele: se
// a segunda gravação falhar, sobra um quadro sem dono nenhum — invisível para
// todo mundo, inclusive para quem o criou, porque toda leitura passa pelo
// vínculo. É lixo, não é risco, e a transação entra quando houver mais de uma
// escrita que precise andar junto (a fase 7, com o evento no outbox).
func (uc *QuadroUseCase) Criar(ctx context.Context, usuarioID, titulo string) (*dboard.Board, error) {
	b, err := dboard.Novo(uuid.NewString(), titulo)
	if err != nil {
		return nil, err
	}
	if err := uc.boards.Salvar(ctx, b); err != nil {
		return nil, err
	}

	vinculo, err := membro.Novo(b.ID, usuarioID, membro.PapelDono)
	if err != nil {
		return nil, err
	}
	if err := uc.membros.Salvar(ctx, vinculo); err != nil {
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
	vinculo, err := acesso(ctx, uc.membros, boardID, usuarioID)
	if err != nil {
		return nil, err
	}

	b, err := uc.boards.BuscarPorID(ctx, boardID)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, dboard.ErrNaoEncontrado
	}

	listaColunas, err := uc.colunas.ListarDoBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	listaCards, err := uc.cards.ListarDoBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}

	// Os três resumos vêm do banco numa consulta cada, e não card a card: a
	// tela do quadro mostra selo de etiqueta, "2/5" de checklist e contagem de
	// anexos em TODO card, e uma consulta por card seria um N+1 que piora
	// justamente nos quadros grandes.
	etiquetasPorCard, err := uc.etiquetas.EtiquetasDoBoardPorCard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	progressoPorCard, err := uc.checklists.ProgressoDoBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	anexosPorCard, err := uc.anexos.ContarPorCardDoBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	responsaveisPorCard, err := uc.responsaveis.DoBoardPorCard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	comentariosPorCard, err := uc.comentarios.ContarPorCardDoBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	etiquetasDoBoard, err := uc.etiquetas.ListarDoBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}

	publicado, err := uc.estaPublicado(ctx, boardID)
	if err != nil {
		return nil, err
	}

	return &Detalhado{
		Board:     *b,
		Papel:     vinculo.Papel,
		Colunas:   agrupar(listaColunas, listaCards, responsaveisPorCard, etiquetasPorCard, progressoPorCard, anexosPorCard, comentariosPorCard),
		Publico:   publicado,
		Etiquetas: etiquetasDoBoard,
	}, nil
}

// estaPublicado informa se o quadro tem link público ligado. Sem a porta ligada
// responde que não — ver ComPublicacoes.
func (uc *QuadroUseCase) estaPublicado(ctx context.Context, boardID string) (bool, error) {
	if uc.publicacoes == nil {
		return false, nil
	}
	p, err := uc.publicacoes.BuscarPorBoard(ctx, boardID)
	if err != nil {
		return false, err
	}
	return p != nil, nil
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
	if err := uc.escreverEPublicar(ctx, evento.QuadroAlterado, boardID, usuarioID, nil,
		uc.escrita(), func(e Escrita) error { return e.Boards.Atualizar(ctx, b) }); err != nil {
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
	if err := b.Renomear(titulo); err != nil {
		return nil, err
	}
	if err := uc.escreverEPublicar(ctx, evento.QuadroAlterado, boardID, usuarioID, nil,
		uc.escrita(), func(e Escrita) error { return e.Boards.Atualizar(ctx, b) }); err != nil {
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
	// Antes do DELETE, enquanto os anexos de todos os cards do quadro ainda
	// existem. Ver limpeza.go.
	orfaos, err := uc.anexos.CaminhosDeArquivoDoBoard(ctx, boardID)
	if err != nil {
		return err
	}
	if err := uc.boards.Apagar(ctx, boardID); err != nil {
		return err
	}
	descartarArquivos(ctx, uc.armazem, orfaos)
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
		porColuna[c.ColunaID] = append(porColuna[c.ColunaID], CardNoQuadro{
			Card:           c,
			Responsaveis:   responsaveis,
			Etiquetas:      etiquetas,
			Checklist:      progressoPorCard[c.ID],
			QtdAnexos:      anexosPorCard[c.ID],
			QtdComentarios: comentariosPorCard[c.ID],
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
