package board

import (
	"context"
	"time"

	dboard "stacktrack/internal/domain/board"
	"stacktrack/internal/domain/card"
	"stacktrack/internal/domain/coluna"
	"stacktrack/internal/domain/cor"
	"stacktrack/internal/domain/etiqueta"
	dpublicacao "stacktrack/internal/domain/publicacao"
	"stacktrack/internal/pkg/token"
)

// PublicacaoUseCase cuida do link público de um quadro: criar, revogar,
// consultar — e servir o quadro a quem chega por ele.
//
// É um usecase SEPARADO do QuadroUseCase, e a separação é o ponto. Ver é a
// única coisa que o caminho público faz, e ele não tem como fazer outra: este
// tipo não recebe repositório de comentário, de anexo, de responsável nem de
// evento, então nenhuma alteração futura consegue, por descuido, pendurar aqui
// uma escrita ou um dado de pessoa. A garantia é a lista de dependências, não a
// disciplina de quem mexer depois.
type PublicacaoUseCase struct {
	publicacoes RepositorioPublicacao
	membros     RepositorioMembro
	boards      RepositorioBoard
	colunas     RepositorioColuna
	cards       RepositorioCard
	etiquetas   repositorioEtiqueta
	checklists  repositorioChecklist
}

// NovoPublicacaoUseCase cria uma instância de PublicacaoUseCase com as
// dependências injetadas.
func NovoPublicacaoUseCase(
	publicacoes RepositorioPublicacao,
	membros RepositorioMembro,
	boards RepositorioBoard,
	colunas RepositorioColuna,
	cards RepositorioCard,
	etiquetas repositorioEtiqueta,
	checklists repositorioChecklist,
) *PublicacaoUseCase {
	return &PublicacaoUseCase{
		publicacoes: publicacoes, membros: membros, boards: boards,
		colunas: colunas, cards: cards, etiquetas: etiquetas, checklists: checklists,
	}
}

// Atual devolve a publicação do quadro, ou (nil, nil) quando ele não está
// publicado. Exige papel de administração: o token é o segredo do link, e
// entregá-lo a um leitor seria deixá-lo publicar o quadro por conta própria,
// repassando o que recebeu.
func (uc *PublicacaoUseCase) Atual(ctx context.Context, boardID, usuarioID string) (*dpublicacao.Publicacao, error) {
	if _, err := acessoDeAdministracao(ctx, uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}
	return uc.publicacoes.BuscarPorBoard(ctx, boardID)
}

// Publicar liga o link público do quadro e devolve a publicação. Exige papel de
// administração (dono).
//
// É idempotente: publicar um quadro já publicado devolve o MESMO token, sem
// gerar outro. Sem isso, o dono que abrisse a tela de compartilhamento duas
// vezes invalidaria em silêncio o link que já tinha mandado para as pessoas.
func (uc *PublicacaoUseCase) Publicar(ctx context.Context, boardID, usuarioID string) (*dpublicacao.Publicacao, error) {
	if _, err := acessoDeAdministracao(ctx, uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}

	existente, err := uc.publicacoes.BuscarPorBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	if existente != nil {
		return existente, nil
	}

	// crypto/rand com 256 bits, o mesmo token de sessão: esta URL é a
	// credencial inteira de quem a possui, e um gerador previsível deixaria
	// adivinhar o quadro dos outros a partir do próprio.
	t, err := token.Gerar()
	if err != nil {
		return nil, err
	}
	p, err := dpublicacao.Nova(boardID, t, usuarioID)
	if err != nil {
		return nil, err
	}
	if err := uc.publicacoes.Salvar(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// Revogar desliga o link público. Exige papel de administração (dono).
//
// O link morre na hora, e voltar a publicar gera um token NOVO: o endereço
// antigo não ressuscita. É o que separa revogar de esconder — quem guardou a
// URL num histórico de conversa não volta a entrar.
func (uc *PublicacaoUseCase) Revogar(ctx context.Context, boardID, usuarioID string) error {
	if _, err := acessoDeAdministracao(ctx, uc.membros, boardID, usuarioID); err != nil {
		return err
	}
	return uc.publicacoes.Remover(ctx, boardID)
}

// Ver devolve o quadro a quem chega pelo link público, SEM sessão nenhuma.
//
// O token é a autorização inteira: não há usuário para conferir papel, e é por
// isso que o que sai daqui é uma projeção própria (QuadroPublico) e não o
// Detalhado da tela autenticada. Retorna publicacao.ErrNaoEncontrada para token
// desconhecido, revogado ou de quadro apagado.
func (uc *PublicacaoUseCase) Ver(ctx context.Context, tokenDoLink string) (*QuadroPublico, error) {
	if tokenDoLink == "" {
		return nil, dpublicacao.ErrNaoEncontrada
	}
	p, err := uc.publicacoes.BuscarPorToken(ctx, tokenDoLink)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, dpublicacao.ErrNaoEncontrada
	}

	b, err := uc.boards.BuscarPorID(ctx, p.BoardID)
	if err != nil {
		return nil, err
	}
	if b == nil {
		// Publicação sem quadro só acontece com o CASCADE fora do ar. Responder
		// "não encontrada" é o certo: para quem tem o link, o quadro apagado e o
		// link inválido são a mesma coisa.
		return nil, dpublicacao.ErrNaoEncontrada
	}

	listaColunas, err := uc.colunas.ListarDoBoard(ctx, p.BoardID)
	if err != nil {
		return nil, err
	}
	listaCards, err := uc.cards.ListarDoBoard(ctx, p.BoardID)
	if err != nil {
		return nil, err
	}
	etiquetasPorCard, err := uc.etiquetas.EtiquetasDoBoardPorCard(ctx, p.BoardID)
	if err != nil {
		return nil, err
	}
	etiquetasDoBoard, err := uc.etiquetas.ListarDoBoard(ctx, p.BoardID)
	if err != nil {
		return nil, err
	}
	progressoPorCard, err := uc.checklists.ProgressoDoBoard(ctx, p.BoardID)
	if err != nil {
		return nil, err
	}

	return montarQuadroPublico(b, listaColunas, listaCards, etiquetasDoBoard, etiquetasPorCard, progressoPorCard), nil
}

// QuadroPublico é o quadro como ele sai para a internet.
//
// É um tipo à parte, e não o Detalhado com campos apagados na saída, porque o
// que importa aqui é o que NÃO existe. Reaproveitar o Detalhado deixaria cada
// campo novo dele — e cada campo é acrescentado pensando na tela autenticada —
// escapar por esta rota até alguém reparar.
//
// Fora de propósito, e cada ausência com um motivo:
//
//   - responsáveis, autores e membros: são NOMES de pessoas que não escolheram
//     ser publicadas. Quem decide abrir o quadro é o dono; o nome do colega não
//     é dele para publicar.
//   - comentários: a conversa é onde se discute o que ainda não está decidido.
//   - anexos: arquivo é conteúdo que ninguém revisou pensando em plateia — e
//     baixá-lo exigiria uma rota de download sem sessão.
//   - histórico: quem moveu o quê e quando é vigilância sobre a equipe.
//   - ids do quadro: esta resposta não precisa endereçar nada, e um id que não
//     sai não é um id que alguém tenta em outra rota.
//   - version dos cards: só serve para escrever, e por aqui não se escreve.
type QuadroPublico struct {
	Titulo string
	Fundo  string
	// AtualizadoEm é o que deixa a página dizer de quando é o que está na tela.
	// Sem isso, um quadro parado e um quadro recém-mexido são indistinguíveis
	// para quem só olha.
	AtualizadoEm time.Time
	Colunas      []ColunaPublica
}

// ColunaPublica é uma etapa do fluxo, com os cards que estão nela.
type ColunaPublica struct {
	Titulo string
	Cor    cor.Cor
	Cards  []CardPublico
}

// CardPublico é uma tarefa como ela aparece para quem acompanha de fora.
type CardPublico struct {
	Titulo    string
	Descricao string
	Cor       cor.Cor
	Prazo     *time.Time
	// Vencido vem calculado aqui pelo mesmo motivo da tela autenticada: o
	// relógio do navegador de quem visita pode estar errado.
	Vencido bool
	// Etiquetas vêm RESOLVIDAS em nome e cor, e não como ids a cruzar com uma
	// lista à parte. Repetir o nome em cada card custa alguns bytes e apaga uma
	// categoria inteira de identificador da resposta.
	Etiquetas []EtiquetaPublica
	Checklist Progresso
}

// EtiquetaPublica é o rótulo de um card, sem id: só o que se lê na tela.
type EtiquetaPublica struct {
	Nome string
	Cor  cor.Cor
}

// montarQuadroPublico distribui os cards nas colunas e resolve as etiquetas de
// cada um, preservando a ordem em que os repositórios devolveram — as duas
// listas já vêm ordenadas por chave.
func montarQuadroPublico(
	b *dboard.Board,
	colunas []coluna.Coluna,
	cards []card.Card,
	etiquetasDoBoard []etiqueta.Etiqueta,
	etiquetasPorCard map[string][]string,
	progressoPorCard map[string]Progresso,
) *QuadroPublico {
	// O nome e a cor de cada etiqueta, por id, para resolver as do card sem
	// varrer a lista inteira uma vez por card.
	porID := make(map[string]EtiquetaPublica, len(etiquetasDoBoard))
	for _, e := range etiquetasDoBoard {
		porID[e.ID] = EtiquetaPublica{Nome: e.Nome, Cor: e.Cor}
	}

	agora := time.Now()
	porColuna := make(map[string][]CardPublico, len(colunas))
	for _, c := range cards {
		etiquetas := make([]EtiquetaPublica, 0, len(etiquetasPorCard[c.ID]))
		for _, id := range etiquetasPorCard[c.ID] {
			if e, conhecida := porID[id]; conhecida {
				etiquetas = append(etiquetas, e)
			}
		}
		porColuna[c.ColunaID] = append(porColuna[c.ColunaID], CardPublico{
			Titulo:    c.Titulo,
			Descricao: c.Descricao,
			Cor:       c.Cor,
			Prazo:     c.Prazo,
			Vencido:   c.Vencido(agora),
			Etiquetas: etiquetas,
			Checklist: progressoPorCard[c.ID],
		})
	}

	publicas := make([]ColunaPublica, 0, len(colunas))
	for _, col := range colunas {
		daColuna := porColuna[col.ID]
		if daColuna == nil {
			// Slice vazia, e não nil: coluna sem card precisa sair como [] no
			// JSON, senão a tela teria de tratar os dois casos.
			daColuna = []CardPublico{}
		}
		publicas = append(publicas, ColunaPublica{Titulo: col.Titulo, Cor: col.Cor, Cards: daColuna})
	}

	return &QuadroPublico{
		Titulo:       b.Titulo,
		Fundo:        b.FundoEfetivo(),
		AtualizadoEm: b.AtualizadoEm,
		Colunas:      publicas,
	}
}
