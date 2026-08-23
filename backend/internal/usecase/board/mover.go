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

// chaveNoFimDaColunaSobLock é o caminho barato da criação. Como a maior chave
// é lida sob o lock e ChaveEntre sempre devolve algo estritamente maior, a
// candidata não pode colidir com nenhuma chave existente. Só varremos e
// redistribuímos a coluna quando a maior chave legada é inválida ou esgotou o
// limite; fazer ListarDaColuna em toda criação tornaria acrescentar N cards um
// trabalho quadrático.
func (uc *CardUseCase) chaveNoFimDaColunaSobLock(ctx context.Context, e Escrita, colunaID string) (string, error) {
	for tentativa := 0; tentativa < 2; tentativa++ {
		ultima, err := e.Cards.UltimaChave(ctx, colunaID)
		if err != nil {
			return "", err
		}
		chave, err := ordem.ChaveEntre(ultima, "")
		if err == nil {
			return chave, nil
		}
		if !precisaRebalancear(err) || tentativa > 0 {
			return "", err
		}
		if err := rebalancearCards(ctx, e, colunaID); err != nil {
			return "", err
		}
	}
	return "", errChaveOcupada
}

func (uc *ColunaUseCase) chaveNoFimDoQuadroSobLock(ctx context.Context, e Escrita, boardID string) (string, error) {
	for tentativa := 0; tentativa < 2; tentativa++ {
		ultima, err := e.Colunas.UltimaChave(ctx, boardID)
		if err != nil {
			return "", err
		}
		chave, err := ordem.ChaveEntre(ultima, "")
		if err == nil {
			return chave, nil
		}
		if !precisaRebalancear(err) || tentativa > 0 {
			return "", err
		}
		if err := rebalancearColunas(ctx, e, boardID); err != nil {
			return "", err
		}
	}
	return "", errChaveOcupada
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

	// A chave é calculada DENTRO da transação, e não antes dela. É a diferença
	// que faz o cálculo enxergar o estado que vale agora: duas pessoas soltando
	// um card no mesmo ponto ao mesmo tempo liam os mesmos vizinhos fora da
	// transação e calculavam a partir de um estado que a outra já tinha mudado.
	// Sob o lock do quadro, a segunda vê o resultado da primeira.
	//
	// A coluna de ORIGEM entra no evento porque ela se perde no próprio Mover:
	// depois dele, o card só sabe onde está. Sem isto o histórico diria "moveu
	// este card", que é a metade inútil da informação.
	dados := &DadosDoCard{CardID: c.ID, Titulo: c.Titulo}
	if err := uc.escreverEPublicarNoCard(ctx, evento.CardMovido, boardID, c.ID, usuarioID, dados,
		uc.escrita(), func(e Escrita) error {
			if err := revalidarEdicao(ctx, e, boardID, usuarioID); err != nil {
				return err
			}

			// Origem e destino são resolvidos de novo sob o lock. O card pode ter
			// sido movido e a coluna de destino apagada enquanto esta requisição
			// esperava; usar os agregados externos produziria evento mentiroso ou
			// violação de FK.
			atual, err := e.Cards.BuscarPorID(ctx, c.ID)
			if err != nil {
				return err
			}
			if atual == nil {
				return dcard.ErrNaoEncontrado
			}
			origem, err := e.Colunas.BuscarPorID(ctx, atual.ColunaID)
			if err != nil {
				return err
			}
			if origem == nil || origem.BoardID != boardID {
				return dcard.ErrNaoEncontrado
			}
			destino := origem
			if colunaDestinoID != "" && colunaDestinoID != origem.ID {
				destino, err = e.Colunas.BuscarPorID(ctx, colunaDestinoID)
				if err != nil {
					return err
				}
				if destino == nil || destino.BoardID != boardID {
					return dcoluna.ErrNaoEncontrada
				}
			}

			chave, err := uc.chaveDeDestino(ctx, e, destino.ID, vizinhos, atual.ID)
			if err != nil {
				return erroDeOrdenacaoDoCard(err)
			}

			// O rebalanceamento também atualiza a versão do card movido. Usar a
			// cópia carregada antes da transação faria o UPDATE otimista conflitar
			// com uma mudança feita por esta mesma transação e desfaria o reparo.
			// Recarregar aqui também cobre o tempo em que a requisição esperou pelo
			// lock do quadro.
			atual, err = e.Cards.BuscarPorID(ctx, c.ID)
			if err != nil {
				return err
			}
			if atual == nil {
				return dcard.ErrNaoEncontrado
			}
			dados.Titulo = atual.Titulo
			dados.DeColuna = origem.Titulo
			dados.Coluna = destino.Titulo
			dados.ColunaID = destino.ID
			atual.Mover(destino.ID, chave)
			if err := e.Cards.Atualizar(ctx, atual); err != nil {
				return err
			}
			c = atual
			return nil
		}); err != nil {
		return nil, err
	}
	return c, nil
}

// chaveDeDestino calcula a chave do card na coluna de destino, redistribuindo o
// contêiner quando não houver espaço ou quando houver chave repetida.
//
// A repetição é procurada ANTES de calcular: é ela que fecha o espaço para o
// próximo movimento, e detectá-la só quando o cálculo falhasse significaria
// esperar alguém tentar soltar um card exatamente entre as duas iguais para
// então descobrir que não dá.
func (uc *CardUseCase) chaveDeDestino(ctx context.Context, e Escrita, colunaID string, vizinhos Vizinhos, ignorarID string) (string, error) {
	if repetida, err := uc.colunaTemChaveRepetida(ctx, e, colunaID); err != nil {
		return "", err
	} else if repetida {
		if err := rebalancearCards(ctx, e, colunaID); err != nil {
			return "", err
		}
	}

	var ultimoErro error
	for tentativa := 0; tentativa < tentativasDeChave; tentativa++ {
		chave, err := uc.chaveLivreEntreCardsDe(ctx, e, colunaID, vizinhos, ignorarID)
		if err == nil {
			return chave, nil
		}
		ultimoErro = err
		if !precisaRebalancear(err) && !errors.Is(err, errChaveOcupada) {
			// Vizinho de outra coluna, por exemplo: repetir daria o mesmo erro.
			return "", err
		}
		if err := rebalancearCards(ctx, e, colunaID); err != nil {
			return "", err
		}
	}

	// Uma candidata pode cair numa chave que já existe quando os vizinhos
	// informados não são adjacentes. Depois das tentativas baratas, redistribuir
	// fecha esse estado e dá ao cálculo uma régua nova; então tentamos uma última
	// vez. Tudo continua sob o mesmo lock.
	if errors.Is(ultimoErro, errChaveOcupada) {
		if err := rebalancearCards(ctx, e, colunaID); err != nil {
			return "", err
		}
		return uc.chaveLivreEntreCardsDe(ctx, e, colunaID, vizinhos, ignorarID)
	}
	return "", ultimoErro
}

// chaveLivreEntreCardsDe calcula uma candidata e a confere contra todas as
// chaves em uso. Ao colidir, usa a própria colisão como novo limite inferior:
// cada tentativa avança estritamente dentro do intervalo, logo depois de no
// máximo uma passagem pela lista encontra uma chave livre sem depender de
// sorte.
func (uc *CardUseCase) chaveLivreEntreCardsDe(ctx context.Context, e Escrita, colunaID string, vizinhos Vizinhos, ignorarID string) (string, error) {
	anterior, err := uc.chaveDoCardDe(ctx, e, vizinhos.AnteriorID, colunaID)
	if err != nil {
		return "", err
	}
	proximo, err := uc.chaveDoCardDe(ctx, e, vizinhos.ProximoID, colunaID)
	if err != nil {
		return "", err
	}
	if anterior == "" && proximo == "" {
		anterior, err = e.Cards.UltimaChave(ctx, colunaID)
		if err != nil {
			return "", err
		}
	}

	cards, err := e.Cards.ListarDaColuna(ctx, colunaID)
	if err != nil {
		return "", err
	}
	emUso := make(map[string]struct{}, len(cards))
	for _, c := range cards {
		if c.ID != ignorarID {
			emUso[c.Chave] = struct{}{}
		}
	}
	for tentativa := 0; tentativa <= len(emUso); tentativa++ {
		chave, err := ordem.ChaveEntre(anterior, proximo)
		if err != nil {
			return "", err
		}
		if _, ocupada := emUso[chave]; !ocupada {
			return chave, nil
		}
		anterior = chave
	}
	return "", errChaveOcupada
}

// colunaTemChaveRepetida lê as chaves em uso na coluna e procura empate.
func (uc *CardUseCase) colunaTemChaveRepetida(ctx context.Context, e Escrita, colunaID string) (bool, error) {
	cards, err := e.Cards.ListarDaColuna(ctx, colunaID)
	if err != nil {
		return false, err
	}
	chaves := make([]string, 0, len(cards))
	for _, c := range cards {
		chaves = append(chaves, c.Chave)
	}
	return chavesRepetidas(chaves), nil
}

// chaveDoCardDe devolve a chave do vizinho, conferindo que ele é DA COLUNA de
// destino.
//
// A conferência não é zelo: um id de card de outra coluna produziria uma chave
// que não significa nada ali, e o item pousaria em lugar nenhum.
func (uc *CardUseCase) chaveDoCardDe(ctx context.Context, e Escrita, cardID, colunaID string) (string, error) {
	if cardID == "" {
		return "", nil
	}
	c, err := e.Cards.BuscarPorID(ctx, cardID)
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

	dados := &DadosDaColuna{ColunaID: c.ID, Titulo: c.Titulo}
	if err := uc.escreverEPublicar(ctx, evento.ColunaMovida, c.BoardID, usuarioID,
		dados,
		uc.escrita(), func(e Escrita) error {
			if err := revalidarEdicao(ctx, e, c.BoardID, usuarioID); err != nil {
				return err
			}
			chave, err := uc.chaveDeDestino(ctx, e, c.BoardID, vizinhos, c.ID)
			if err != nil {
				return erroDeOrdenacaoDaColuna(err)
			}
			atual, err := e.Colunas.BuscarPorID(ctx, c.ID)
			if err != nil {
				return err
			}
			if atual == nil {
				return dcoluna.ErrNaoEncontrada
			}
			if atual.BoardID != c.BoardID {
				return dcoluna.ErrNaoEncontrada
			}
			dados.Titulo = atual.Titulo
			atual.MoverPara(chave)
			if err := e.Colunas.DefinirChave(ctx, atual.ID, atual.Chave, atual.AtualizadoEm); err != nil {
				return err
			}
			c = atual
			return nil
		}); err != nil {
		return nil, err
	}
	return c, nil
}

// chaveDeDestino calcula a chave da coluna no quadro, redistribuindo quando não
// houver espaço ou quando houver chave repetida. É o gêmeo de
// CardUseCase.chaveDeDestino; ver lá o porquê de cada passo.
func (uc *ColunaUseCase) chaveDeDestino(ctx context.Context, e Escrita, boardID string, vizinhos Vizinhos, ignorarID string) (string, error) {
	if repetida, err := uc.quadroTemChaveRepetida(ctx, e, boardID); err != nil {
		return "", err
	} else if repetida {
		if err := rebalancearColunas(ctx, e, boardID); err != nil {
			return "", err
		}
	}

	var ultimoErro error
	for tentativa := 0; tentativa < tentativasDeChave; tentativa++ {
		chave, err := uc.chaveLivreEntreColunasDe(ctx, e, boardID, vizinhos, ignorarID)
		if err == nil {
			return chave, nil
		}
		ultimoErro = err
		if !precisaRebalancear(err) && !errors.Is(err, errChaveOcupada) {
			return "", err
		}
		if err := rebalancearColunas(ctx, e, boardID); err != nil {
			return "", err
		}
	}
	if errors.Is(ultimoErro, errChaveOcupada) {
		if err := rebalancearColunas(ctx, e, boardID); err != nil {
			return "", err
		}
		return uc.chaveLivreEntreColunasDe(ctx, e, boardID, vizinhos, ignorarID)
	}
	return "", ultimoErro
}

func (uc *ColunaUseCase) chaveLivreEntreColunasDe(ctx context.Context, e Escrita, boardID string, vizinhos Vizinhos, ignorarID string) (string, error) {
	anterior, err := uc.chaveDaColunaDe(ctx, e, vizinhos.AnteriorID, boardID)
	if err != nil {
		return "", err
	}
	proximo, err := uc.chaveDaColunaDe(ctx, e, vizinhos.ProximoID, boardID)
	if err != nil {
		return "", err
	}
	if anterior == "" && proximo == "" {
		anterior, err = e.Colunas.UltimaChave(ctx, boardID)
		if err != nil {
			return "", err
		}
	}

	colunas, err := e.Colunas.ListarDoBoard(ctx, boardID)
	if err != nil {
		return "", err
	}
	emUso := make(map[string]struct{}, len(colunas))
	for _, c := range colunas {
		if c.ID != ignorarID {
			emUso[c.Chave] = struct{}{}
		}
	}
	for tentativa := 0; tentativa <= len(emUso); tentativa++ {
		chave, err := ordem.ChaveEntre(anterior, proximo)
		if err != nil {
			return "", err
		}
		if _, ocupada := emUso[chave]; !ocupada {
			return chave, nil
		}
		anterior = chave
	}
	return "", errChaveOcupada
}

func (uc *ColunaUseCase) quadroTemChaveRepetida(ctx context.Context, e Escrita, boardID string) (bool, error) {
	colunas, err := e.Colunas.ListarDoBoard(ctx, boardID)
	if err != nil {
		return false, err
	}
	chaves := make([]string, 0, len(colunas))
	for _, c := range colunas {
		chaves = append(chaves, c.Chave)
	}
	return chavesRepetidas(chaves), nil
}

// chaveDaColunaDe devolve a chave da vizinha, conferindo que ela é DO MESMO
// quadro — mesma razão de chaveDoCardDe.
func (uc *ColunaUseCase) chaveDaColunaDe(ctx context.Context, e Escrita, colunaID, boardID string) (string, error) {
	if colunaID == "" {
		return "", nil
	}
	c, err := e.Colunas.BuscarPorID(ctx, colunaID)
	if err != nil {
		return "", err
	}
	if c == nil || c.BoardID != boardID {
		return "", dcoluna.ErrNaoEncontrada
	}
	return c.Chave, nil
}
