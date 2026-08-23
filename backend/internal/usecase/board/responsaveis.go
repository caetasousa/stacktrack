package board

import (
	"context"

	dcard "stacktrack/internal/domain/card"
	"stacktrack/internal/domain/evento"
	dmembro "stacktrack/internal/domain/membro"
)

// ResponsavelUseCase resolve quem responde por cada card.
type ResponsavelUseCase struct {
	eventos
	membros      RepositorioMembro
	colunas      RepositorioColuna
	cards        RepositorioCard
	responsaveis RepositorioResponsavel
}

// NovoResponsavelUseCase cria uma instância de ResponsavelUseCase com as dependências injetadas.
func NovoResponsavelUseCase(
	membros RepositorioMembro,
	colunas RepositorioColuna,
	cards RepositorioCard,
	responsaveis RepositorioResponsavel,
) *ResponsavelUseCase {
	return &ResponsavelUseCase{membros: membros, colunas: colunas, cards: cards, responsaveis: responsaveis}
}

// Listar devolve quem responde pelo card. Qualquer membro pode ver.
func (uc *ResponsavelUseCase) Listar(ctx context.Context, cardID, usuarioID string) ([]Responsavel, error) {
	c, err := uc.cards.BuscarPorID(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, dcard.ErrNaoEncontrado
	}
	col, err := uc.colunas.BuscarPorID(ctx, c.ColunaID)
	if err != nil {
		return nil, err
	}
	if col == nil {
		return nil, dcard.ErrNaoEncontrado
	}
	if _, err := acesso(ctx, uc.membros, col.BoardID, usuarioID); err != nil {
		return nil, traduzirParaCard(err)
	}
	return uc.responsaveis.DoCard(ctx, cardID)
}

// Atribuir marca alguém como responsável pelo card. Exige papel de edição.
//
// Atribuir duas vezes não é erro: a chave primária composta já garante uma
// linha só, e o resultado pretendido — a pessoa responsável por aquele card —
// já vale.
func (uc *ResponsavelUseCase) Atribuir(ctx context.Context, cardID, alvoID, usuarioID string) error {
	boardID, err := uc.conferirAlvo(ctx, cardID, alvoID, usuarioID)
	if err != nil {
		return err
	}
	// O nome do alvo é resolvido pela lista de MEMBROS, e não pela lista de
	// responsáveis do card: no instante do payload a atribuição ainda não
	// aconteceu (ela acontece dentro da transação, logo abaixo), então buscá-la
	// ali devolveria vazio e o evento sairia sem dizer quem foi atribuído.
	return uc.escreverEPublicarNoCard(ctx, evento.ResponsavelAtribuido, boardID, cardID, usuarioID,
		DadosDoCard{CardID: cardID, Titulo: tituloDoCard(ctx, uc.cards, cardID), Alvo: uc.nomeNoQuadro(ctx, boardID, alvoID)},
		uc.escrita(), func(e Escrita) error {
			if err := revalidarEdicao(ctx, e, boardID, usuarioID); err != nil {
				return err
			}
			// A pessoa pode sair do quadro entre a validação acima e a aquisição do
			// lock. Revalidar com o repositório da transação faz a atribuição e a
			// participação observarem o mesmo estado serializado.
			vinculo, err := e.Membros.Buscar(ctx, boardID, alvoID)
			if err != nil {
				return err
			}
			if vinculo == nil {
				return dmembro.ErrNaoEMembro
			}
			return e.Responsaveis.Atribuir(ctx, cardID, alvoID)
		})
}

// Desatribuir tira a pessoa da responsabilidade do card. Exige papel de edição.
func (uc *ResponsavelUseCase) Desatribuir(ctx context.Context, cardID, alvoID, usuarioID string) error {
	boardID, err := uc.carregarComAcessoDeEdicao(ctx, cardID, usuarioID)
	if err != nil {
		return err
	}
	// O nome é resolvido ANTES da remoção: depois dela a pessoa já não está na
	// lista do card, e o evento sairia sem dizer quem saiu.
	nome := uc.nomeDe(ctx, cardID, alvoID)
	return uc.escreverEPublicarNoCard(ctx, evento.ResponsavelRemovido, boardID, cardID, usuarioID,
		DadosDoCard{CardID: cardID, Titulo: tituloDoCard(ctx, uc.cards, cardID), Alvo: nome},
		uc.escrita(), func(e Escrita) error {
			if err := revalidarEdicao(ctx, e, boardID, usuarioID); err != nil {
				return err
			}
			return e.Responsaveis.Remover(ctx, cardID, alvoID)
		})
}

// nomeDe resolve o nome de quem responde pelo card, para o payload do evento.
//
// Vem da lista do próprio card, que já traz nome resolvido — é a mesma consulta
// que a tela usa. Falha ou ausência viram string vazia: a frase encolhe, e o
// evento continua existindo.
func (uc *ResponsavelUseCase) nomeDe(ctx context.Context, cardID, alvoID string) string {
	lista, err := uc.responsaveis.DoCard(ctx, cardID)
	if err != nil {
		return ""
	}
	for _, r := range lista {
		if r.UsuarioID == alvoID {
			return r.Nome
		}
	}
	return ""
}

// nomeNoQuadro resolve o nome de quem PARTICIPA do quadro, para o payload de um
// evento gravado ANTES de a atribuição existir.
//
// Falha ou ausência viram string vazia: a frase encolhe, e o evento continua
// existindo — a mesma decisão de nomeDe e de tituloDoCard.
func (uc *ResponsavelUseCase) nomeNoQuadro(ctx context.Context, boardID, alvoID string) string {
	participantes, err := uc.membros.Participantes(ctx, boardID)
	if err != nil {
		return ""
	}
	for _, p := range participantes {
		if p.UsuarioID == alvoID {
			return p.Nome
		}
	}
	return ""
}

// conferirAlvo garante que quem vai ser atribuído PARTICIPA do quadro do card.
//
// É a regra que impede transformar a atribuição num vazamento: sem ela, bastaria
// o id de uma conta qualquer para pendurar o nome dela num quadro de que ela não
// faz parte — e essa pessoa apareceria como responsável por um trabalho que não
// pode nem abrir.
//
// A checagem é aqui, e não na migration: a chave estrangeira aponta para
// `usuarios`, porque "quem pode ser responsável" é regra de negócio e muda com
// ela, ao contrário da existência da conta.
func (uc *ResponsavelUseCase) conferirAlvo(ctx context.Context, cardID, alvoID, usuarioID string) (string, error) {
	boardID, err := uc.carregarComAcessoDeEdicao(ctx, cardID, usuarioID)
	if err != nil {
		return "", err
	}

	vinculo, err := uc.membros.Buscar(ctx, boardID, alvoID)
	if err != nil {
		return "", err
	}
	if vinculo == nil {
		return "", dmembro.ErrNaoEMembro
	}
	return boardID, nil
}

// carregarComAcessoDeEdicao percorre card → coluna → quadro e confere que quem
// pede pode editar. Devolve o id do quadro, que é a sala do evento.
func (uc *ResponsavelUseCase) carregarComAcessoDeEdicao(ctx context.Context, cardID, usuarioID string) (string, error) {
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
	if _, err := acessoDeEdicao(ctx, uc.membros, col.BoardID, usuarioID); err != nil {
		return "", traduzirParaCard(err)
	}
	return col.BoardID, nil
}
