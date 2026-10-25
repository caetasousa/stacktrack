package board

import (
	"context"
	"time"

	dcard "stacktrack/internal/domain/card"
	dcomentario "stacktrack/internal/domain/comentario"
	"stacktrack/internal/domain/evento"
	"stacktrack/internal/domain/notificacao"

	"github.com/google/uuid"
)

// ComentarioUseCase reúne a conversa dos cards.
type ComentarioUseCase struct {
	eventos
	notificacoes
	membros     RepositorioMembro
	colunas     RepositorioColuna
	cards       RepositorioCard
	comentarios RepositorioComentario
}

// NovoComentarioUseCase cria uma instância de ComentarioUseCase com as dependências injetadas.
func NovoComentarioUseCase(
	membros RepositorioMembro,
	colunas RepositorioColuna,
	cards RepositorioCard,
	comentarios RepositorioComentario,
) *ComentarioUseCase {
	return &ComentarioUseCase{membros: membros, colunas: colunas, cards: cards, comentarios: comentarios}
}

// Listar devolve a conversa do card. Qualquer membro pode ler — inclusive o
// leitor: acompanhar a discussão é ver, não editar.
func (uc *ComentarioUseCase) Listar(ctx context.Context, cardID, usuarioID string) ([]ComentarioComAutor, error) {
	if _, err := uc.boardComAcesso(ctx, cardID, usuarioID); err != nil {
		return nil, err
	}
	return uc.comentarios.ListarDoCard(ctx, cardID)
}

// Criar acrescenta um comentário ao card.
//
// Exige participação, e não papel de edição: comentar é conversar sobre o
// trabalho, não mexer nele. Um leitor que enxerga o quadro precisa poder
// responder — senão a revisão de quem só acompanha não tem para onde ir.
//
// Quem o texto chama pelo nome (ver dcomentario.Mencoes) é avisado na mesma
// transação. A lista de quem pode ser chamado é lida sob o lock do quadro: quem
// saiu um instante antes não é avisado de um comentário que não pode ler.
func (uc *ComentarioUseCase) Criar(ctx context.Context, cardID, usuarioID, texto string) (*dcomentario.Comentario, error) {
	boardID, err := uc.boardComAcesso(ctx, cardID, usuarioID)
	if err != nil {
		return nil, err
	}

	c, err := dcomentario.Novo(uuid.NewString(), cardID, usuarioID, texto)
	if err != nil {
		return nil, err
	}
	var avisados []string
	if err := uc.escreverEPublicarNoCard(ctx, evento.ComentarioCriado, boardID, cardID, usuarioID,
		DadosDoCard{CardID: cardID, Titulo: tituloDoCard(ctx, uc.cards, cardID)},
		uc.escrita(), func(e Escrita) error {
			if err := revalidarAcesso(ctx, e, boardID, usuarioID); err != nil {
				return err
			}
			if err := e.Comentarios.Salvar(ctx, c); err != nil {
				return err
			}
			mencionados, err := uc.mencionados(ctx, e, boardID, c)
			if err != nil {
				return err
			}
			avisados, err = uc.avisarMencionados(ctx, e, boardID, c, mencionados)
			return err
		}); err != nil {
		return nil, err
	}
	uc.depois(avisados...)
	return c, nil
}

// mencionados devolve quem o comentário chama, entre quem participa do quadro
// agora — menos o autor.
func (uc *ComentarioUseCase) mencionados(ctx context.Context, e Escrita, boardID string, c *dcomentario.Comentario) ([]string, error) {
	if !uc.avisando(e) {
		return nil, nil
	}
	participantes, err := e.Membros.Participantes(ctx, boardID)
	if err != nil {
		return nil, err
	}
	return dcomentario.Mencoes(c.Texto, mencionaveis(participantes, c.AutorID)), nil
}

// avisarMencionados grava o aviso de menção de cada pessoa. Devolve quem
// recebeu um aviso novo.
func (uc *ComentarioUseCase) avisarMencionados(ctx context.Context, e Escrita, boardID string, c *dcomentario.Comentario, ids []string) ([]string, error) {
	agora := time.Now()
	avisados := make([]string, 0, len(ids))
	for _, id := range ids {
		criada, err := uc.avisar(ctx, e,
			notificacao.NovaDeMencao(uuid.NewString(), id, boardID, c.CardID, c.ID, c.AutorID, agora))
		if err != nil {
			return nil, err
		}
		if criada {
			avisados = append(avisados, id)
		}
	}
	return avisados, nil
}

// Editar troca o texto de um comentário. SÓ o autor.
//
// Nem o dono do quadro pode: apagar o que não serve é responsabilidade de quem
// administra, mas reescrever é pôr palavras na boca de outra pessoa.
func (uc *ComentarioUseCase) Editar(ctx context.Context, comentarioID, usuarioID, texto string) (*dcomentario.Comentario, error) {
	c, boardID, err := uc.carregar(ctx, comentarioID, usuarioID)
	if err != nil {
		return nil, err
	}
	if !c.EhAutor(usuarioID) {
		return nil, dcomentario.ErrNaoEhAutor
	}
	if err := c.Editar(texto); err != nil {
		return nil, err
	}
	// Editar e apagar NÃO entram no histórico do card: quem lê o histórico quer
	// saber o que aconteceu com o trabalho, e a conversa já está logo acima, com
	// a marca de "editado" em cada mensagem. O evento continua saindo ao vivo,
	// para a tela de quem está junto recarregar.
	//
	// As menções acompanham o texto novo: quem deixou de ser chamado perde o
	// aviso (e o email que ainda não saiu, pelo CASCADE), e quem passou a ser
	// chamado ganha um. Quem continua chamado não é avisado de novo.
	var tocados []string
	if err := uc.escreverEPublicarNoCard(ctx, evento.ComentarioEditado, boardID, c.CardID, usuarioID,
		DadosDoCard{CardID: c.CardID, Titulo: tituloDoCard(ctx, uc.cards, c.CardID)},
		uc.escrita(), func(e Escrita) error {
			if err := revalidarAcesso(ctx, e, boardID, usuarioID); err != nil {
				return err
			}
			if err := e.Comentarios.Atualizar(ctx, c); err != nil {
				return err
			}
			if !uc.avisando(e) {
				return nil
			}
			atuais, err := uc.mencionados(ctx, e, boardID, c)
			if err != nil {
				return err
			}
			antes, err := e.Notificacoes.MencionadosNoComentario(ctx, c.ID)
			if err != nil {
				return err
			}
			retirados := diferenca(antes, atuais)
			if err := e.Notificacoes.RemoverMencoes(ctx, c.ID, retirados); err != nil {
				return err
			}
			novos, err := uc.avisarMencionados(ctx, e, boardID, c, diferenca(atuais, antes))
			if err != nil {
				return err
			}
			tocados = append(retirados, novos...)
			return nil
		}); err != nil {
		return nil, err
	}
	uc.depois(tocados...)
	return c, nil
}

// Apagar remove o comentário. O autor apaga o próprio; quem administra o quadro
// apaga o de qualquer um, porque é dele a responsabilidade pelo que fica ali.
func (uc *ComentarioUseCase) Apagar(ctx context.Context, comentarioID, usuarioID string) error {
	c, boardID, err := uc.carregar(ctx, comentarioID, usuarioID)
	if err != nil {
		return err
	}

	ehAutor := c.EhAutor(usuarioID)
	if !ehAutor {
		if _, err := acessoDeAdministracao(ctx, uc.membros, boardID, usuarioID); err != nil {
			return err
		}
	}
	// Os avisos de menção vão junto pelo CASCADE. Quem os tinha é lido ANTES,
	// para o sino dessas pessoas saber que o número mudou.
	var tocados []string
	if err := uc.escreverEPublicarNoCard(ctx, evento.ComentarioApagado, boardID, c.CardID, usuarioID,
		DadosDoCard{CardID: c.CardID, Titulo: tituloDoCard(ctx, uc.cards, c.CardID)},
		uc.escrita(), func(e Escrita) error {
			var err error
			if ehAutor {
				err = revalidarAcesso(ctx, e, boardID, usuarioID)
			} else {
				err = revalidarAdministracao(ctx, e, boardID, usuarioID)
			}
			if err != nil {
				return err
			}
			if uc.avisando(e) {
				if tocados, err = e.Notificacoes.MencionadosNoComentario(ctx, comentarioID); err != nil {
					return err
				}
			}
			return e.Comentarios.Apagar(ctx, comentarioID)
		}); err != nil {
		return err
	}
	uc.depois(tocados...)
	return nil
}

// diferenca devolve o que está em `a` e não em `b`, na ordem de `a`.
func diferenca(a, b []string) []string {
	em := make(map[string]bool, len(b))
	for _, id := range b {
		em[id] = true
	}
	resto := make([]string, 0, len(a))
	for _, id := range a {
		if !em[id] {
			resto = append(resto, id)
		}
	}
	return resto
}

// carregar busca o comentário e confere que quem pede participa do quadro dele.
func (uc *ComentarioUseCase) carregar(ctx context.Context, comentarioID, usuarioID string) (*dcomentario.Comentario, string, error) {
	c, err := uc.comentarios.BuscarPorID(ctx, comentarioID)
	if err != nil {
		return nil, "", err
	}
	if c == nil {
		return nil, "", dcomentario.ErrNaoEncontrado
	}
	boardID, err := uc.boardComAcesso(ctx, c.CardID, usuarioID)
	if err != nil {
		// Quem não participa recebe "comentário não encontrado", e não "card não
		// encontrado": a resposta não deve revelar em que ponto da cadeia parou.
		return nil, "", dcomentario.ErrNaoEncontrado
	}
	return c, boardID, nil
}

// boardComAcesso percorre card → coluna → quadro e confere a participação.
func (uc *ComentarioUseCase) boardComAcesso(ctx context.Context, cardID, usuarioID string) (string, error) {
	c, err := uc.cards.BuscarPorID(ctx, cardID)
	if err != nil {
		return "", err
	}
	if c == nil {
		return "", dcard.ErrNaoEncontrado
	}
	col, err := uc.colunas.BuscarPorID(ctx, c.ColunaID)
	if err != nil {
		return "", err
	}
	if col == nil {
		return "", dcard.ErrNaoEncontrado
	}
	if _, err := acesso(ctx, uc.membros, col.BoardID, usuarioID); err != nil {
		return "", traduzirParaCard(err)
	}
	return col.BoardID, nil
}
