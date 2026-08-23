// Rebalanceamento das chaves de ordenação: quando o espaço entre duas chaves
// acaba, ou quando duas ficaram iguais.

package board

import (
	"context"
	"errors"

	dcard "stacktrack/internal/domain/card"
	dcoluna "stacktrack/internal/domain/coluna"
	"stacktrack/internal/domain/ordem"
)

// tentativasDeChave é quantas vezes o cálculo da chave é repetido antes de o
// movimento virar conflito.
//
// Poucas, e limitadas, de propósito. Cada repetição sorteia uma chave nova
// dentro do mesmo espaço (ver ordem.sorteioEntre), então a chance de colidir
// três vezes seguidas é desprezível — e se ela não for desprezível, o problema
// não é sorte ruim: é falta de espaço, e insistir só transformaria um conflito
// tratável num laço que segura o lock do quadro.
const tentativasDeChave = 3

// errChaveOcupada é a colisão improvável, mas possível, entre a candidata
// sorteada e outra chave já presente no intervalo informado.
var errChaveOcupada = errors.New("a chave de ordenação sorteada já está em uso")

// rebalancearCards reescreve as chaves de TODOS os cards da coluna, espaçadas.
//
// Roda dentro da transação, sob o lock do quadro, e lê a coluna ali dentro: as
// chaves precisam ser as que valem AGORA, não as de uma leitura anterior que
// outra transação já pode ter mudado.
//
// A reescrita passa pelo Atualizar comum, que incrementa a versão de cada card.
// É de propósito, e é o detalhe que evita um bug sutil: se a versão não subisse,
// alguém que tivesse lido um card antes do rebalanceamento gravaria a chave
// ANTIGA de volta, com o bloqueio otimista aprovando — e o card voltaria
// sozinho para o lugar de onde saiu. Com a versão nova, essa gravação recebe
// conflito e a pessoa recarrega.
func rebalancearCards(ctx context.Context, e Escrita, colunaID string) error {
	cards, err := e.Cards.ListarDaColuna(ctx, colunaID)
	if err != nil {
		return err
	}
	chaves, err := ordem.Redistribuir(len(cards))
	if err != nil {
		return err
	}
	for i := range cards {
		c := cards[i]
		// Mover para a MESMA coluna: o que muda é só a chave. Passar a coluna
		// atual, e não vazio, mantém o card onde está — e é o mesmo comando que
		// já incrementa a versão, que é justamente o efeito desejado aqui.
		c.Mover(c.ColunaID, chaves[i])
		if err := e.Cards.Atualizar(ctx, &c); err != nil {
			return err
		}
	}
	return nil
}

// rebalancearColunas faz o mesmo para as colunas de um quadro.
//
// Aqui o comando é o ESTREITO (DefinirChave), e não o Atualizar: coluna não tem
// versão otimista, e reescrever título e cor junto desfaria uma renomeação
// simultânea — exatamente o defeito que os comandos estreitos existem para
// evitar.
func rebalancearColunas(ctx context.Context, e Escrita, boardID string) error {
	colunas, err := e.Colunas.ListarDoBoard(ctx, boardID)
	if err != nil {
		return err
	}
	chaves, err := ordem.Redistribuir(len(colunas))
	if err != nil {
		return err
	}
	for i := range colunas {
		c := colunas[i]
		c.MoverPara(chaves[i])
		if err := e.Colunas.DefinirChave(ctx, c.ID, c.Chave, c.AtualizadoEm); err != nil {
			return err
		}
	}
	return nil
}

// chavesRepetidas informa se duas ou mais chaves da lista são iguais.
//
// Chave repetida não corrompe nada sozinha — a leitura desempata pelo id, e a
// ordem entre dois itens que ninguém ordenou é arbitrária de qualquer forma. O
// que ela quebra é o FUTURO: não existe chave entre duas iguais, então arrastar
// algo para o meio delas passa a ser impossível pela interface, e a pessoa fica
// sem saída.
func chavesRepetidas(chaves []string) bool {
	vistas := make(map[string]struct{}, len(chaves))
	for _, k := range chaves {
		if _, repetida := vistas[k]; repetida {
			return true
		}
		vistas[k] = struct{}{}
	}
	return false
}

// precisaRebalancear informa se o erro do cálculo da chave é dos que se
// resolvem redistribuindo o contêiner.
//
// ErrChaveLonga é "acabou o espaço": inserir centenas de vezes no mesmo ponto
// fez a chave crescer até não caber na coluna. ErrChaveInvalida vindo de um
// VIZINHO é dado velho, de antes da régua atual, e a redistribuição normaliza
// todo mundo de uma vez.
func precisaRebalancear(err error) bool {
	return errors.Is(err, ordem.ErrChaveLonga) || errors.Is(err, ordem.ErrChaveInvalida)
}

// erroDeOrdenacaoDoCard traduz a falha final para o erro que a borda entende.
func erroDeOrdenacaoDoCard(err error) error {
	if precisaRebalancear(err) || errors.Is(err, errChaveOcupada) {
		return dcard.ErrConflito
	}
	return err
}

// erroDeOrdenacaoDaColuna faz o mesmo para colunas.
func erroDeOrdenacaoDaColuna(err error) error {
	if precisaRebalancear(err) || errors.Is(err, errChaveOcupada) {
		return dcoluna.ErrConflito
	}
	return err
}
