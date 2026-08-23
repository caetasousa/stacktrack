// O comando de manutenção que repara duplicidade histórica de chave de
// ordenação.
//
// Ele existe por causa de uma regra e de um contract. A regra é a do CLAUDE.md:
// migration não escreve dado, porque todo backfill precisa decidir com que valor
// as linhas antigas ficam, e essa decisão é do domínio — em SQL ela vira uma
// segunda fonte da verdade, sem teste e sem conserto, já que migration aplicada
// não se corrige. O contract é o `UNIQUE (coluna_id, chave)` /
// `UNIQUE (board_id, chave)` que fecha A2, e que o `CREATE UNIQUE INDEX` recusa
// criar enquanto houver duplicidade herdada.
//
// A alternativa seria pedir a alguém que arraste manualmente um item de cada
// contêiner afetado — o rebalanceamento automático da aplicação resolveria o
// resto. Isso não é rollout reproduzível: depende de uma pessoa achar todos os
// contêineres, não deixa relatório e não dá para rodar num pipeline.

package board

import (
	"context"
	"fmt"
	"sort"

	"stacktrack/internal/domain/evento"
)

// Duplicidade descreve um contêiner com chave de ordenação repetida.
//
// Contêiner é a coluna, para cards, e o quadro, para colunas — a mesma
// fronteira em que a unicidade vai valer.
type Duplicidade struct {
	BoardID string
	// ColunaID vazio significa que a duplicidade é entre COLUNAS do quadro.
	ColunaID string
	Chave    string
	// Ocorrencias é quantas linhas compartilham a chave.
	Ocorrencias int
}

// EhDeColunas informa se a duplicidade é entre colunas do quadro.
func (d Duplicidade) EhDeColunas() bool { return d.ColunaID == "" }

// Contêiner devolve o identificador do contêiner afetado, para o relatório.
func (d Duplicidade) Container() string {
	if d.EhDeColunas() {
		return "colunas do quadro " + d.BoardID
	}
	return "cards da coluna " + d.ColunaID
}

// LeitorDeDuplicidades encontra as chaves repetidas que ainda existem.
//
// É porta própria, e não um método do repositório de cards, porque a pergunta
// atravessa dois agregados e não pertence a nenhum deles: ela é sobre o estado
// do BANCO antes de um aperto de schema.
type LeitorDeDuplicidades interface {
	DuplicidadesDeChave(ctx context.Context) ([]Duplicidade, error)
}

// RelatorioDeReparo é o que o comando devolve para ser conferido.
//
// O relatório não é enfeite: ele é a evidência de que a pré-condição do
// contract foi satisfeita. Sem ele, "rodei o comando" não é verificável — e o
// `CREATE UNIQUE INDEX` do deploy seguinte falharia no meio da janela de
// manutenção, que é o pior momento para descobrir.
type RelatorioDeReparo struct {
	// Encontradas são as duplicidades vistas ANTES do reparo.
	Encontradas []Duplicidade
	// QuadrosReparados são os quadros que tiveram alguma chave reescrita.
	QuadrosReparados []string
	// Restantes são as duplicidades que AINDA existem depois do reparo.
	//
	// Deve vir vazia. Vindo linha, o contract não pode ser aplicado — e o
	// comando falhou de forma visível em vez de dizer que deu certo.
	Restantes []Duplicidade
}

// Limpo informa se o banco está pronto para o contract.
func (r RelatorioDeReparo) Limpo() bool { return len(r.Restantes) == 0 }

// ReparoDeOrdenacao repara duplicidade de chave sob o lock de cada quadro.
type ReparoDeOrdenacao struct {
	duplicidades LeitorDeDuplicidades
	atomica      EscritaAtomica
	pub          Publicador
}

// NovoReparoDeOrdenacao cria o comando.
//
// A escrita atômica é OBRIGATÓRIA aqui, ao contrário do resto do pacote, onde
// ela é opcional e o usecase cai num caminho sem transação quando falta. Reparar
// sem o lock do quadro é justamente o que não pode acontecer: a redistribuição
// lê as chaves, calcula as novas e reescreve todas — sem serialização, uma
// mutação concorrente entra no meio e o resultado é outro conjunto de
// duplicidades.
func NovoReparoDeOrdenacao(duplicidades LeitorDeDuplicidades, atomica EscritaAtomica) *ReparoDeOrdenacao {
	return &ReparoDeOrdenacao{duplicidades: duplicidades, atomica: atomica}
}

// ComPublicador liga a entrega ao vivo do evento de reparo.
//
// Sem ele o reparo continua correto no banco e INVISÍVEL para quem está com o
// quadro aberto: a pessoa segue vendo a ordem antiga até recarregar. Com ele, a
// aba reconcilia sozinha — que é o comportamento de qualquer outra mutação.
//
// Fica separado do construtor pela mesma razão dos demais usecases: os testes de
// regra não têm hub nenhum para ligar.
func (uc *ReparoDeOrdenacao) ComPublicador(p Publicador) {
	uc.pub = p
}

// Executar encontra as duplicidades, repara cada quadro afetado e reconfere.
//
// `origem` identifica o comando que rodou (ex.: "reparo-de-ordenacao") e vai
// para o payload do evento. Não é um usuário: nenhuma pessoa arrastou nada, e o
// autor do evento fica vazio de propósito — ver repararQuadro.
//
// O reparo é feito QUADRO A QUADRO, uma transação por quadro, e não tudo numa
// transação só. Duas razões:
//
//  1. o lock é da linha do quadro. Uma transação única seguraria o lock de
//     todos os quadros afetados ao mesmo tempo, parando escrita em cada um
//     deles durante a operação inteira;
//  2. uma falha no meio não desfaz o que já foi reparado. Cada quadro é
//     independente, e rodar o comando de novo termina o serviço — ele é
//     idempotente por construção, porque parte de uma consulta do estado atual.
//
// Dentro da transação, as chaves são LIDAS DE NOVO. É o que torna o comando
// seguro com a aplicação no ar: entre a consulta que listou as duplicidades e o
// lock do quadro cabe qualquer mutação, inclusive um rebalanceamento disparado
// pelo próprio uso normal — e aí não há mais o que reparar naquele contêiner.
func (uc *ReparoDeOrdenacao) Executar(ctx context.Context, origem string) (RelatorioDeReparo, error) {
	encontradas, err := uc.duplicidades.DuplicidadesDeChave(ctx)
	if err != nil {
		return RelatorioDeReparo{}, fmt.Errorf("consultar duplicidades: %w", err)
	}

	relatorio := RelatorioDeReparo{Encontradas: encontradas}
	if len(encontradas) == 0 {
		return relatorio, nil
	}

	for _, boardID := range quadrosAfetados(encontradas) {
		if err := uc.repararQuadro(ctx, boardID, origem, encontradas); err != nil {
			return relatorio, fmt.Errorf("reparar o quadro %s: %w", boardID, err)
		}
		relatorio.QuadrosReparados = append(relatorio.QuadrosReparados, boardID)
	}

	// A reconferência é o que transforma o relatório em evidência. Sem ela, o
	// comando afirmaria sucesso a partir da própria intenção.
	restantes, err := uc.duplicidades.DuplicidadesDeChave(ctx)
	if err != nil {
		return relatorio, fmt.Errorf("reconferir duplicidades: %w", err)
	}
	relatorio.Restantes = restantes
	return relatorio, nil
}

// repararQuadro redistribui, numa transação só, os contêineres afetados de um
// quadro.
func (uc *ReparoDeOrdenacao) repararQuadro(ctx context.Context, boardID, origem string, todas []Duplicidade) error {
	colunasAfetadas, precisaDeColunas := afetadosDoQuadro(todas, boardID)

	// O reparo é uma mutação do quadro como qualquer outra: ganha revisão e
	// evento no mesmo commit, e é entregue ao vivo DEPOIS do commit. É o que faz
	// quem está com o quadro aberto reconciliar em vez de continuar vendo a
	// ordem antiga — e o que deixa o reparo no log de auditoria, com autor.
	// O AUTOR vai vazio, e a origem vai no payload.
	//
	// `board_events.autor_id` é UUID e aponta para uma conta; uma execução de
	// manutenção não tem conta. Inventar um UUID mentiria sobre quem mexeu no
	// quadro — a única saída honesta é autor NULL com a origem registrada.
	carimbado, err := uc.atomica.Escrever(ctx,
		evento.Novo(evento.OrdenacaoReparada, boardID, "", DadosDaManutencao{
			Origem:     origem,
			Containers: len(colunasAfetadas) + contarSe(precisaDeColunas),
		}),
		func(e Escrita) error {
			if precisaDeColunas {
				if err := rebalancearColunas(ctx, e, boardID); err != nil {
					return err
				}
			}
			for _, colunaID := range colunasAfetadas {
				if err := rebalancearCards(ctx, e, colunaID); err != nil {
					return err
				}
			}
			return nil
		})
	if err != nil {
		return err
	}

	// Fora da transação, como em toda publicação do projeto: anunciar antes do
	// commit avisaria de uma mudança que o rollback ainda pode desfazer.
	if uc.pub != nil {
		uc.pub.Publicar(carimbado)
	}
	return nil
}

// quadrosAfetados devolve os ids dos quadros com alguma duplicidade, sem
// repetição e em ordem estável.
//
// Ordem estável para o relatório ser comparável entre duas execuções: um
// relatório que embaralha a lista a cada rodada não serve como evidência.
func quadrosAfetados(duplicidades []Duplicidade) []string {
	vistos := make(map[string]struct{}, len(duplicidades))
	ids := make([]string, 0, len(duplicidades))
	for _, d := range duplicidades {
		if _, repetido := vistos[d.BoardID]; repetido {
			continue
		}
		vistos[d.BoardID] = struct{}{}
		ids = append(ids, d.BoardID)
	}
	sort.Strings(ids)
	return ids
}

// contarSe devolve 1 quando a condição vale, para somar no payload.
func contarSe(condicao bool) int {
	if condicao {
		return 1
	}
	return 0
}

// afetadosDoQuadro separa, para um quadro, quais colunas têm card duplicado e
// se as próprias colunas se repetem.
func afetadosDoQuadro(duplicidades []Duplicidade, boardID string) (colunas []string, colunasDoQuadro bool) {
	vistas := make(map[string]struct{})
	for _, d := range duplicidades {
		if d.BoardID != boardID {
			continue
		}
		if d.EhDeColunas() {
			colunasDoQuadro = true
			continue
		}
		if _, repetida := vistas[d.ColunaID]; repetida {
			continue
		}
		vistas[d.ColunaID] = struct{}{}
		colunas = append(colunas, d.ColunaID)
	}
	sort.Strings(colunas)
	return colunas, colunasDoQuadro
}
