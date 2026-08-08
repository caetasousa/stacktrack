package board

import (
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
	boards     repositorioBoard
	membros    repositorioMembro
	colunas    repositorioColuna
	cards      repositorioCard
	etiquetas  repositorioEtiqueta
	checklists repositorioChecklist
	anexos     repositorioAnexo
}

// NovoQuadroUseCase cria uma instância de QuadroUseCase com as dependências injetadas.
func NovoQuadroUseCase(
	boards repositorioBoard,
	membros repositorioMembro,
	colunas repositorioColuna,
	cards repositorioCard,
	etiquetas repositorioEtiqueta,
	checklists repositorioChecklist,
	anexos repositorioAnexo,
) *QuadroUseCase {
	return &QuadroUseCase{
		boards: boards, membros: membros, colunas: colunas, cards: cards,
		etiquetas: etiquetas, checklists: checklists, anexos: anexos,
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
func (uc *QuadroUseCase) Criar(usuarioID, titulo string) (*dboard.Board, error) {
	b, err := dboard.Novo(uuid.NewString(), titulo)
	if err != nil {
		return nil, err
	}
	if err := uc.boards.Salvar(b); err != nil {
		return nil, err
	}

	vinculo, err := membro.Novo(b.ID, usuarioID, membro.PapelDono)
	if err != nil {
		return nil, err
	}
	if err := uc.membros.Salvar(vinculo); err != nil {
		return nil, err
	}
	return b, nil
}

// Listar devolve os quadros de que o usuário participa, com o papel dele em
// cada um. Nunca devolve quadro de terceiro: a consulta parte do vínculo.
func (uc *QuadroUseCase) Listar(usuarioID string) ([]Resumo, error) {
	return uc.boards.ListarDoUsuario(usuarioID)
}

// Detalhar devolve o quadro com colunas e cards, em ordem de posição. Retorna
// dboard.ErrNaoEncontrado se o quadro não existir ou se o usuário não
// participar dele.
func (uc *QuadroUseCase) Detalhar(boardID, usuarioID string) (*Detalhado, error) {
	vinculo, err := acesso(uc.membros, boardID, usuarioID)
	if err != nil {
		return nil, err
	}

	b, err := uc.boards.BuscarPorID(boardID)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, dboard.ErrNaoEncontrado
	}

	listaColunas, err := uc.colunas.ListarDoBoard(boardID)
	if err != nil {
		return nil, err
	}
	listaCards, err := uc.cards.ListarDoBoard(boardID)
	if err != nil {
		return nil, err
	}

	// Os três resumos vêm do banco numa consulta cada, e não card a card: a
	// tela do quadro mostra selo de etiqueta, "2/5" de checklist e contagem de
	// anexos em TODO card, e uma consulta por card seria um N+1 que piora
	// justamente nos quadros grandes.
	etiquetasPorCard, err := uc.etiquetas.EtiquetasDoBoardPorCard(boardID)
	if err != nil {
		return nil, err
	}
	progressoPorCard, err := uc.checklists.ProgressoDoBoard(boardID)
	if err != nil {
		return nil, err
	}
	anexosPorCard, err := uc.anexos.ContarPorCardDoBoard(boardID)
	if err != nil {
		return nil, err
	}
	etiquetasDoBoard, err := uc.etiquetas.ListarDoBoard(boardID)
	if err != nil {
		return nil, err
	}

	return &Detalhado{
		Board:     *b,
		Papel:     vinculo.Papel,
		Colunas:   agrupar(listaColunas, listaCards, etiquetasPorCard, progressoPorCard, anexosPorCard),
		Etiquetas: etiquetasDoBoard,
	}, nil
}

// PodeVer informa se o usuário participa do quadro. É a pergunta que o
// handshake do WebSocket faz antes de aceitar a conexão — sem ela, qualquer
// pessoa autenticada assinaria a sala de qualquer quadro e leria tudo que
// acontece nele em tempo real.
//
// Devolve bool, e não erro, porque quem chama só precisa decidir entre aceitar
// e recusar: distinguir "não existe" de "não participa" seria justamente a
// informação que o 404 do resto da API esconde.
func (uc *QuadroUseCase) PodeVer(boardID, usuarioID string) bool {
	_, err := acesso(uc.membros, boardID, usuarioID)
	return err == nil
}

// DefinirFundo troca o fundo do quadro. Exige papel de administração.
func (uc *QuadroUseCase) DefinirFundo(boardID, usuarioID, fundo string) (*dboard.Board, error) {
	if _, err := acessoDeAdministracao(uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}

	b, err := uc.boards.BuscarPorID(boardID)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, dboard.ErrNaoEncontrado
	}
	if err := b.DefinirFundo(fundo); err != nil {
		return nil, err
	}
	if err := uc.boards.Atualizar(b); err != nil {
		return nil, err
	}
	uc.publicar(evento.QuadroAlterado, boardID, usuarioID, nil)
	return b, nil
}

// Renomear troca o título do quadro. Exige papel de administração (dono).
func (uc *QuadroUseCase) Renomear(boardID, usuarioID, titulo string) (*dboard.Board, error) {
	if _, err := acessoDeAdministracao(uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}

	b, err := uc.boards.BuscarPorID(boardID)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, dboard.ErrNaoEncontrado
	}
	if err := b.Renomear(titulo); err != nil {
		return nil, err
	}
	if err := uc.boards.Atualizar(b); err != nil {
		return nil, err
	}
	uc.publicar(evento.QuadroAlterado, boardID, usuarioID, nil)
	return b, nil
}

// Apagar remove o quadro. Exige papel de administração (dono). Colunas, cards
// e vínculos vão junto, pelo ON DELETE CASCADE do schema.
func (uc *QuadroUseCase) Apagar(boardID, usuarioID string) error {
	if _, err := acessoDeAdministracao(uc.membros, boardID, usuarioID); err != nil {
		return err
	}
	return uc.boards.Apagar(boardID)
}

// agrupar distribui os cards nas colunas a que pertencem, preservando a ordem
// em que cada lista chegou do repositório (ambas por posição), e pendura em
// cada card o resumo do que ele carrega.
func agrupar(
	colunas []coluna.Coluna,
	cards []card.Card,
	etiquetasPorCard map[string][]string,
	progressoPorCard map[string]Progresso,
	anexosPorCard map[string]int,
) []ColunaComCards {
	porColuna := make(map[string][]CardNoQuadro, len(colunas))
	for _, c := range cards {
		etiquetas := etiquetasPorCard[c.ID]
		if etiquetas == nil {
			etiquetas = []string{}
		}
		porColuna[c.ColunaID] = append(porColuna[c.ColunaID], CardNoQuadro{
			Card:      c,
			Etiquetas: etiquetas,
			Checklist: progressoPorCard[c.ID],
			QtdAnexos: anexosPorCard[c.ID],
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
