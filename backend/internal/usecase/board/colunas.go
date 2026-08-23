package board

import (
	"context"

	dcoluna "stacktrack/internal/domain/coluna"
	dcor "stacktrack/internal/domain/cor"
	"stacktrack/internal/domain/evento"
	"stacktrack/internal/domain/ordem"

	"github.com/google/uuid"
)

// ColunaUseCase reúne as operações sobre colunas.
type ColunaUseCase struct {
	eventos
	exclusoes
	membros RepositorioMembro
	colunas RepositorioColuna
	// anexos e armazem existem só para a LIMPEZA do disco ao apagar: o cascata
	// do schema leva as linhas de anexo junto e não toca no volume.
	anexos  RepositorioAnexo
	armazem armazemDeArquivos
}

// NovoColunaUseCase cria uma instância de ColunaUseCase com as dependências injetadas.
func NovoColunaUseCase(
	membros RepositorioMembro,
	colunas RepositorioColuna,
	anexos RepositorioAnexo,
	armazem armazemDeArquivos,
) *ColunaUseCase {
	return &ColunaUseCase{membros: membros, colunas: colunas, anexos: anexos, armazem: armazem}
}

// Criar acrescenta uma coluna no fim do quadro. Exige papel de edição.
func (uc *ColunaUseCase) Criar(ctx context.Context, boardID, usuarioID, titulo string, cores dcor.Cor) (*dcoluna.Coluna, error) {
	if _, err := acessoDeEdicao(ctx, uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}

	// Assim como no card, só o id nasce fora da transação. A posição é lida e
	// gravada sob o lock do quadro para que duas criações no mesmo fim não
	// partam do mesmo estado.
	colunaID := uuid.NewString()
	rascunho, err := dcoluna.Nova(colunaID, boardID, titulo, cores, ordem.ChaveInicial)
	if err != nil {
		return nil, err
	}
	var c *dcoluna.Coluna
	if err := uc.escreverEPublicar(ctx, evento.ColunaCriada, boardID, usuarioID,
		DadosDaColuna{ColunaID: colunaID, Titulo: rascunho.Titulo, Cor: string(rascunho.Cor)},
		uc.escrita(), func(e Escrita) error {
			if err := revalidarEdicao(ctx, e, boardID, usuarioID); err != nil {
				return err
			}
			chave, err := uc.chaveNoFimDoQuadroSobLock(ctx, e, boardID)
			if err != nil {
				return erroDeOrdenacaoDaColuna(err)
			}
			c, err = dcoluna.Nova(colunaID, boardID, rascunho.Titulo, rascunho.Cor, chave)
			if err != nil {
				return err
			}
			return e.Colunas.Salvar(ctx, c)
		}); err != nil {
		return nil, err
	}
	return c, nil
}

// Renomear troca o título e a cor da coluna. Exige papel de edição no quadro
// dela.
func (uc *ColunaUseCase) Renomear(ctx context.Context, colunaID, usuarioID, titulo string, cores dcor.Cor) (*dcoluna.Coluna, error) {
	c, err := uc.carregarComAcessoDeEdicao(ctx, colunaID, usuarioID)
	if err != nil {
		return nil, err
	}
	tituloAnterior := c.Titulo
	if err := c.Renomear(titulo); err != nil {
		return nil, err
	}
	if err := c.DefinirCor(cores); err != nil {
		return nil, err
	}
	dados := &DadosDaColuna{ColunaID: c.ID, Titulo: c.Titulo, Cor: string(c.Cor)}
	if tituloAnterior != c.Titulo {
		dados.TituloAnterior = tituloAnterior
	}
	if err := uc.escreverEPublicar(ctx, evento.ColunaAlterada, c.BoardID, usuarioID, dados,
		uc.escrita(), func(e Escrita) error {
			if err := revalidarEdicao(ctx, e, c.BoardID, usuarioID); err != nil {
				return err
			}
			atual, err := e.Colunas.BuscarPorID(ctx, c.ID)
			if err != nil {
				return err
			}
			if atual == nil || atual.BoardID != c.BoardID {
				return dcoluna.ErrNaoEncontrada
			}
			dados.TituloAnterior = ""
			if atual.Titulo != c.Titulo {
				dados.TituloAnterior = atual.Titulo
			}
			if err := atual.Renomear(c.Titulo); err != nil {
				return err
			}
			if err := atual.DefinirCor(c.Cor); err != nil {
				return err
			}
			dados.Titulo, dados.Cor = atual.Titulo, string(atual.Cor)
			if err := e.Colunas.Renomear(ctx, atual.ID, atual.Titulo, atual.Cor, atual.AtualizadoEm); err != nil {
				return err
			}
			c = atual
			return nil
		}); err != nil {
		return nil, err
	}
	return c, nil
}

// Apagar remove a coluna e, por cascata, os cards dela. Exige papel de edição.
func (uc *ColunaUseCase) Apagar(ctx context.Context, colunaID, usuarioID string) error {
	c, err := uc.carregarComAcessoDeEdicao(ctx, colunaID, usuarioID)
	if err != nil {
		return err
	}
	var orfaos []string
	dados := &DadosDaColuna{ColunaID: colunaID, Titulo: c.Titulo, Cor: string(c.Cor)}
	if err := uc.escreverEPublicar(ctx, evento.ColunaApagada, c.BoardID, usuarioID,
		dados,
		uc.escrita(), func(e Escrita) error {
			if err := revalidarEdicao(ctx, e, c.BoardID, usuarioID); err != nil {
				return err
			}
			atual, err := e.Colunas.BuscarPorID(ctx, colunaID)
			if err != nil {
				return err
			}
			if atual == nil || atual.BoardID != c.BoardID {
				return dcoluna.ErrNaoEncontrada
			}
			dados.Titulo, dados.Cor = atual.Titulo, string(atual.Cor)
			orfaos, err = e.Anexos.CaminhosDeArquivoDaColuna(ctx, colunaID)
			if err != nil {
				return err
			}
			if err := registrarExclusaoDeArquivos(ctx, e, c.BoardID, orfaos); err != nil {
				return err
			}
			return e.Colunas.Apagar(ctx, colunaID)
		}); err != nil {
		return err
	}
	return nil
}

// carregarComAcessoDeEdicao busca a coluna e confere o acesso ao quadro DELA —
// e não a um quadro informado por quem chamou. Aceitar o boardID da requisição
// deixaria alguém apagar a coluna de um quadro alheio informando o id de um
// quadro próprio.
func (uc *ColunaUseCase) carregarComAcessoDeEdicao(ctx context.Context, colunaID, usuarioID string) (*dcoluna.Coluna, error) {
	c, err := uc.colunas.BuscarPorID(ctx, colunaID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, dcoluna.ErrNaoEncontrada
	}
	if _, err := acessoDeEdicao(ctx, uc.membros, c.BoardID, usuarioID); err != nil {
		// Quem não participa do quadro recebe "coluna não encontrada" pelo
		// mesmo motivo do quadro: confirmar a existência já é informação.
		return nil, traduzirParaColuna(err)
	}
	return c, nil
}
