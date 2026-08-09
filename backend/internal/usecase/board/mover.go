package board

import (
	"context"
	"errors"
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
	chave, err := uc.chaveEntreCards(ctx, vizinhos)
	if err != nil {
		return nil, err
	}
	// A posição é legado, e escrevê-la NÃO pode mais barrar nada.
	//
	// Enquanto o expand não terminar as duas colunas são gravadas — mas deixar o
	// esgotamento do float abortar o movimento manteria de pé exatamente a falha
	// que esta fase existe para remover: o 409 voltaria na 53ª inserção no mesmo
	// ponto, mesmo com a chave tendo espaço de sobra.
	posicao, err := uc.posicaoEntreCards(ctx, destino.ID, vizinhos)
	if err != nil {
		if !errors.Is(err, ordem.ErrSemEspaco) {
			return nil, err
		}
		// Sem espaço no float: a posição perde o sentido, e quem ordena já é a
		// chave. Fica a do próprio card, que o contract vai apagar.
		posicao = c.Posicao
	}

	c.Mover(destino.ID, posicao, chave)
	// A coluna de ORIGEM entra no evento porque ela se perde no próprio Mover:
	// depois dele, o card só sabe onde está. Sem isto o histórico diria "moveu
	// este card", que é a metade inútil da informação — e essa era exatamente a
	// armadilha anotada no plano da fase 11.
	if err := uc.escreverEPublicarNoCard(ctx, evento.CardMovido, boardID, c.ID, usuarioID,
		DadosDoCard{
			CardID: c.ID, Titulo: c.Titulo,
			DeColuna: origem.Titulo, Coluna: destino.Titulo,
			ColunaID: c.ColunaID, Posicao: c.Posicao, Version: c.Version,
		},
		uc.escrita(), func(e Escrita) error { return e.Cards.Atualizar(ctx, c) }); err != nil {
		return nil, err
	}
	return c, nil
}

// chaveEntreCards resolve os vizinhos em chaves reais e calcula a do meio.
//
// Vizinho sem chave — linha antiga, que o backfill ainda não alcançou — conta
// como ponta: é o comportamento seguro durante o expand, porque colocar o card
// entre uma chave e um vazio produziria uma ordem que não corresponde ao que a
// tela mostrava.
func (uc *CardUseCase) chaveEntreCards(ctx context.Context, vizinhos Vizinhos) (string, error) {
	anterior, err := uc.chaveDoCard(ctx, vizinhos.AnteriorID)
	if err != nil {
		return "", err
	}
	proximo, err := uc.chaveDoCard(ctx, vizinhos.ProximoID)
	if err != nil {
		return "", err
	}
	return ordem.ChaveEntre(anterior, proximo)
}

func (uc *CardUseCase) chaveDoCard(ctx context.Context, cardID string) (string, error) {
	if cardID == "" {
		return "", nil
	}
	c, err := uc.cards.BuscarPorID(ctx, cardID)
	if err != nil || c == nil {
		return "", err
	}
	return c.Chave, nil
}

// posicaoEntreCards resolve os vizinhos em posições reais e calcula o meio.
//
// Cada vizinho é conferido contra a coluna de destino: um id de card de outra
// coluna produziria uma posição que não faz sentido lá, e o item pousaria em
// lugar nenhum.
func (uc *CardUseCase) posicaoEntreCards(ctx context.Context, colunaID string, vizinhos Vizinhos) (float64, error) {
	anterior, err := uc.posicaoDoCardNaColuna(ctx, vizinhos.AnteriorID, colunaID)
	if err != nil {
		return 0, err
	}
	proximo, err := uc.posicaoDoCardNaColuna(ctx, vizinhos.ProximoID, colunaID)
	if err != nil {
		return 0, err
	}

	// Sem vizinho nenhum e coluna já povoada: o item vai para o fim, e não para
	// a posição inicial — soltar num espaço vazio abaixo dos cards significa
	// "no fim", não "no começo".
	if anterior == 0 && proximo == 0 {
		ultima, err := uc.cards.UltimaPosicao(ctx, colunaID)
		if err != nil {
			return 0, err
		}
		return ordem.NoFim(ultima), nil
	}
	return ordem.Entre(anterior, proximo)
}

// posicaoDoCardNaColuna devolve a posição do card vizinho, ou 0 quando não há
// vizinho daquele lado.
func (uc *CardUseCase) posicaoDoCardNaColuna(ctx context.Context, cardID, colunaID string) (float64, error) {
	if cardID == "" {
		return 0, nil
	}
	vizinho, err := uc.cards.BuscarPorID(ctx, cardID)
	if err != nil {
		return 0, err
	}
	if vizinho == nil || vizinho.ColunaID != colunaID {
		return 0, dcard.ErrNaoEncontrado
	}
	return vizinho.Posicao, nil
}

// Mover reposiciona a coluna dentro do quadro. Exige papel de edição.
func (uc *ColunaUseCase) Mover(ctx context.Context, colunaID, usuarioID string, vizinhos Vizinhos) (*dcoluna.Coluna, error) {
	c, err := uc.carregarComAcessoDeEdicao(ctx, colunaID, usuarioID)
	if err != nil {
		return nil, err
	}

	anterior, err := uc.posicaoDaColunaNoBoard(ctx, vizinhos.AnteriorID, c.BoardID)
	if err != nil {
		return nil, err
	}
	proximo, err := uc.posicaoDaColunaNoBoard(ctx, vizinhos.ProximoID, c.BoardID)
	if err != nil {
		return nil, err
	}

	var posicao float64
	if anterior == 0 && proximo == 0 {
		ultima, err := uc.colunas.UltimaPosicao(ctx, c.BoardID)
		if err != nil {
			return nil, err
		}
		posicao = ordem.NoFim(ultima)
	} else {
		posicao, err = ordem.Entre(anterior, proximo)
		if err != nil {
			if !errors.Is(err, ordem.ErrSemEspaco) {
				return nil, err
			}
			// Legado sem espaço: quem ordena é a chave. Ver o comentário em
			// CardUseCase.Mover.
			posicao = c.Posicao
		}
	}

	chave, err := uc.chaveEntreColunas(ctx, vizinhos)
	if err != nil {
		return nil, err
	}

	c.MoverPara(posicao, chave)
	if err := uc.escreverEPublicar(ctx, evento.ColunaMovida, c.BoardID, usuarioID,
		DadosDaColuna{ColunaID: c.ID, Titulo: c.Titulo},
		uc.escrita(), func(e Escrita) error { return e.Colunas.Atualizar(ctx, c) }); err != nil {
		return nil, err
	}
	return c, nil
}

// chaveEntreColunas faz para colunas o que chaveEntreCards faz para cards.
func (uc *ColunaUseCase) chaveEntreColunas(ctx context.Context, vizinhos Vizinhos) (string, error) {
	anterior, err := uc.chaveDaColuna(ctx, vizinhos.AnteriorID)
	if err != nil {
		return "", err
	}
	proximo, err := uc.chaveDaColuna(ctx, vizinhos.ProximoID)
	if err != nil {
		return "", err
	}
	return ordem.ChaveEntre(anterior, proximo)
}

func (uc *ColunaUseCase) chaveDaColuna(ctx context.Context, colunaID string) (string, error) {
	if colunaID == "" {
		return "", nil
	}
	c, err := uc.colunas.BuscarPorID(ctx, colunaID)
	if err != nil || c == nil {
		return "", err
	}
	return c.Chave, nil
}

func (uc *ColunaUseCase) posicaoDaColunaNoBoard(ctx context.Context, colunaID, boardID string) (float64, error) {
	if colunaID == "" {
		return 0, nil
	}
	vizinha, err := uc.colunas.BuscarPorID(ctx, colunaID)
	if err != nil {
		return 0, err
	}
	if vizinha == nil || vizinha.BoardID != boardID {
		return 0, dcoluna.ErrNaoEncontrada
	}
	return vizinha.Posicao, nil
}
