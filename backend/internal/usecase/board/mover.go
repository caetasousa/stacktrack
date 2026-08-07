package board

import (
	dcard "kanbango/internal/domain/card"
	dcoluna "kanbango/internal/domain/coluna"
	"kanbango/internal/domain/ordem"
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
//  2. o esgotamento da precisão (ordem.ErrSemEspaco) só pode ser detectado
//     onde os valores reais estão;
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
func (uc *CardUseCase) Mover(cardID, usuarioID, colunaDestinoID string, vizinhos Vizinhos) (*dcard.Card, error) {
	c, err := uc.carregarComAcessoDeEdicao(cardID, usuarioID)
	if err != nil {
		return nil, err
	}

	origem, err := uc.colunas.BuscarPorID(c.ColunaID)
	if err != nil {
		return nil, err
	}
	if origem == nil {
		return nil, dcard.ErrNaoEncontrado
	}

	destino := origem
	if colunaDestinoID != "" && colunaDestinoID != c.ColunaID {
		destino, err = uc.colunas.BuscarPorID(colunaDestinoID)
		if err != nil {
			return nil, err
		}
		if destino == nil || destino.BoardID != origem.BoardID {
			return nil, dcoluna.ErrNaoEncontrada
		}
	}

	posicao, err := uc.posicaoEntreCards(destino.ID, vizinhos)
	if err != nil {
		return nil, err
	}

	c.Mover(destino.ID, posicao)
	if err := uc.cards.Atualizar(c); err != nil {
		return nil, err
	}
	return c, nil
}

// posicaoEntreCards resolve os vizinhos em posições reais e calcula o meio.
//
// Cada vizinho é conferido contra a coluna de destino: um id de card de outra
// coluna produziria uma posição que não faz sentido lá, e o item pousaria em
// lugar nenhum.
func (uc *CardUseCase) posicaoEntreCards(colunaID string, vizinhos Vizinhos) (float64, error) {
	anterior, err := uc.posicaoDoCardNaColuna(vizinhos.AnteriorID, colunaID)
	if err != nil {
		return 0, err
	}
	proximo, err := uc.posicaoDoCardNaColuna(vizinhos.ProximoID, colunaID)
	if err != nil {
		return 0, err
	}

	// Sem vizinho nenhum e coluna já povoada: o item vai para o fim, e não para
	// a posição inicial — soltar num espaço vazio abaixo dos cards significa
	// "no fim", não "no começo".
	if anterior == 0 && proximo == 0 {
		ultima, err := uc.cards.UltimaPosicao(colunaID)
		if err != nil {
			return 0, err
		}
		return ordem.NoFim(ultima), nil
	}
	return ordem.Entre(anterior, proximo)
}

// posicaoDoCardNaColuna devolve a posição do card vizinho, ou 0 quando não há
// vizinho daquele lado.
func (uc *CardUseCase) posicaoDoCardNaColuna(cardID, colunaID string) (float64, error) {
	if cardID == "" {
		return 0, nil
	}
	vizinho, err := uc.cards.BuscarPorID(cardID)
	if err != nil {
		return 0, err
	}
	if vizinho == nil || vizinho.ColunaID != colunaID {
		return 0, dcard.ErrNaoEncontrado
	}
	return vizinho.Posicao, nil
}

// Mover reposiciona a coluna dentro do quadro. Exige papel de edição.
func (uc *ColunaUseCase) Mover(colunaID, usuarioID string, vizinhos Vizinhos) (*dcoluna.Coluna, error) {
	c, err := uc.carregarComAcessoDeEdicao(colunaID, usuarioID)
	if err != nil {
		return nil, err
	}

	anterior, err := uc.posicaoDaColunaNoBoard(vizinhos.AnteriorID, c.BoardID)
	if err != nil {
		return nil, err
	}
	proximo, err := uc.posicaoDaColunaNoBoard(vizinhos.ProximoID, c.BoardID)
	if err != nil {
		return nil, err
	}

	var posicao float64
	if anterior == 0 && proximo == 0 {
		ultima, err := uc.colunas.UltimaPosicao(c.BoardID)
		if err != nil {
			return nil, err
		}
		posicao = ordem.NoFim(ultima)
	} else {
		posicao, err = ordem.Entre(anterior, proximo)
		if err != nil {
			return nil, err
		}
	}

	c.MoverPara(posicao)
	if err := uc.colunas.Atualizar(c); err != nil {
		return nil, err
	}
	return c, nil
}

func (uc *ColunaUseCase) posicaoDaColunaNoBoard(colunaID, boardID string) (float64, error) {
	if colunaID == "" {
		return 0, nil
	}
	vizinha, err := uc.colunas.BuscarPorID(colunaID)
	if err != nil {
		return 0, err
	}
	if vizinha == nil || vizinha.BoardID != boardID {
		return 0, dcoluna.ErrNaoEncontrada
	}
	return vizinha.Posicao, nil
}
