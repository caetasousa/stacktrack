package board

import (
	"context"
	dcard "stacktrack/internal/domain/card"
	dcoluna "stacktrack/internal/domain/coluna"
	"stacktrack/internal/domain/evento"
	"stacktrack/internal/domain/ordem"
)

// Vizinhos identifica onde um item foi solto: entre quem fica acima e quem fica
// abaixo dele. Vazio significa "ponta" — sem anterior é o topo, sem próximo é
// o fim.
//
// A API recebe os VIZINHOS, e não a posição já calculada, embora a posição
// pudesse ser calculada no cliente. Três razões:
//
//  1. o cliente pode estar com uma cópia velha do quadro, e calcular a média
//     entre posições que já mudaram coloca o item no lugar errado;
//  2. só onde os valores reais estão dá para calcular a chave entre os
//     vizinhos de verdade;
//  3. posição vinda do cliente é entrada do usuário: um float qualquer
//     embaralharia a ordem de um quadro inteiro.
//
// O cliente continua movendo o card na tela na hora — ele só não decide o
// número.
type Vizinhos struct {
	AnteriorID string
	ProximoID  string
}

// MoverCard leva o card para uma coluna e uma posição entre os vizinhos
// informados. Exige papel de edição.
//
// A coluna de destino precisa ser do MESMO quadro: sem essa checagem, quem
// participa de dois quadros arrastaria um card de um para o outro informando o
// id da coluna de destino — e o card sumiria da vista de quem não participa do
// quadro de origem.
func (uc *CardUseCase) Mover(ctx context.Context, cardID, usuarioID, colunaDestinoID string, vizinhos Vizinhos) (*dcard.Card, error) {
	c, boardID, err := uc.carregarComAcessoDeEdicao(ctx, cardID, usuarioID)
	if err != nil {
		return nil, err
	}

	origem, err := uc.colunas.BuscarPorID(ctx, c.ColunaID)
	if err != nil {
		return nil, err
	}
	if origem == nil {
		return nil, dcard.ErrNaoEncontrado
	}

	destino := origem
	if colunaDestinoID != "" && colunaDestinoID != c.ColunaID {
		destino, err = uc.colunas.BuscarPorID(ctx, colunaDestinoID)
		if err != nil {
			return nil, err
		}
		if destino == nil || destino.BoardID != origem.BoardID {
			return nil, dcoluna.ErrNaoEncontrada
		}
	}

	// A CHAVE é o que manda na ordem. Ela vem PRIMEIRO de propósito: é a que
	// pode recusar o movimento por motivo real (vizinho inválido).
	chave, err := uc.chaveEntreCards(ctx, destino.ID, vizinhos)
	if err != nil {
		return nil, err
	}
	c.Mover(destino.ID, chave)
	// A coluna de ORIGEM entra no evento porque ela se perde no próprio Mover:
	// depois dele, o card só sabe onde está. Sem isto o histórico diria "moveu
	// este card", que é a metade inútil da informação — e essa era exatamente a
	// armadilha anotada no plano da fase 11.
	if err := uc.escreverEPublicarNoCard(ctx, evento.CardMovido, boardID, c.ID, usuarioID,
		DadosDoCard{
			CardID: c.ID, Titulo: c.Titulo,
			DeColuna: origem.Titulo, Coluna: destino.Titulo,
			ColunaID: c.ColunaID, Version: c.Version,
		},
		uc.escrita(), func(e Escrita) error { return e.Cards.Atualizar(ctx, c) }); err != nil {
		return nil, err
	}
	return c, nil
}

// chaveEntreCards resolve os vizinhos em chaves reais e calcula a do meio.
//
// Vizinho ausente conta como ponta: sem anterior o card vai para o começo, sem
// próximo vai para o fim, e sem nenhum dos dois vai para o fim da coluna.
func (uc *CardUseCase) chaveEntreCards(ctx context.Context, colunaID string, vizinhos Vizinhos) (string, error) {
	anterior, err := uc.chaveDoCard(ctx, vizinhos.AnteriorID, colunaID)
	if err != nil {
		return "", err
	}
	proximo, err := uc.chaveDoCard(ctx, vizinhos.ProximoID, colunaID)
	if err != nil {
		return "", err
	}

	// SEM VIZINHO NENHUM significa "solto no espaço vazio abaixo dos cards", e
	// isso quer dizer NO FIM — não no meio de uma lista vazia. É a mesma regra
	// que posicaoEntreCards aplica, e esquecê-la aqui fazia o card pousar num
	// lugar que ninguém pediu, dependendo do sorteio da chave.
	if anterior == "" && proximo == "" {
		anterior, err = uc.cards.UltimaChave(ctx, colunaID)
		if err != nil {
			return "", err
		}
	}
	return ordem.ChaveEntre(anterior, proximo)
}

// chaveDoCard devolve a chave do vizinho, conferindo que ele é DA COLUNA de
// destino.
//
// A conferência não é zelo: um id de card de outra coluna produziria uma chave
// que não significa nada ali, e o item pousaria em lugar nenhum.
func (uc *CardUseCase) chaveDoCard(ctx context.Context, cardID, colunaID string) (string, error) {
	if cardID == "" {
		return "", nil
	}
	c, err := uc.cards.BuscarPorID(ctx, cardID)
	if err != nil {
		return "", err
	}
	if c == nil || c.ColunaID != colunaID {
		return "", dcard.ErrNaoEncontrado
	}
	return c.Chave, nil
}

// Mover reposiciona a coluna dentro do quadro. Exige papel de edição.
func (uc *ColunaUseCase) Mover(ctx context.Context, colunaID, usuarioID string, vizinhos Vizinhos) (*dcoluna.Coluna, error) {
	c, err := uc.carregarComAcessoDeEdicao(ctx, colunaID, usuarioID)
	if err != nil {
		return nil, err
	}

	chave, err := uc.chaveEntreColunas(ctx, c.BoardID, vizinhos)
	if err != nil {
		return nil, err
	}

	c.MoverPara(chave)
	if err := uc.escreverEPublicar(ctx, evento.ColunaMovida, c.BoardID, usuarioID,
		DadosDaColuna{ColunaID: c.ID, Titulo: c.Titulo},
		uc.escrita(), func(e Escrita) error { return e.Colunas.Atualizar(ctx, c) }); err != nil {
		return nil, err
	}
	return c, nil
}

// chaveEntreColunas faz para colunas o que chaveEntreCards faz para cards.
func (uc *ColunaUseCase) chaveEntreColunas(ctx context.Context, boardID string, vizinhos Vizinhos) (string, error) {
	anterior, err := uc.chaveDaColuna(ctx, vizinhos.AnteriorID, boardID)
	if err != nil {
		return "", err
	}
	proximo, err := uc.chaveDaColuna(ctx, vizinhos.ProximoID, boardID)
	if err != nil {
		return "", err
	}
	// Sem vizinha nenhuma é NO FIM — ver chaveEntreCards.
	if anterior == "" && proximo == "" {
		anterior, err = uc.colunas.UltimaChave(ctx, boardID)
		if err != nil {
			return "", err
		}
	}
	return ordem.ChaveEntre(anterior, proximo)
}

// chaveDaColuna devolve a chave da vizinha, conferindo que ela é DO MESMO
// quadro — mesma razão de chaveDoCard.
func (uc *ColunaUseCase) chaveDaColuna(ctx context.Context, colunaID, boardID string) (string, error) {
	if colunaID == "" {
		return "", nil
	}
	c, err := uc.colunas.BuscarPorID(ctx, colunaID)
	if err != nil {
		return "", err
	}
	if c == nil || c.BoardID != boardID {
		return "", dcoluna.ErrNaoEncontrada
	}
	return c.Chave, nil
}
