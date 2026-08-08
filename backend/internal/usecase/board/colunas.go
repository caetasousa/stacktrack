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
	membros RepositorioMembro
	colunas RepositorioColuna
}

// NovoColunaUseCase cria uma instância de ColunaUseCase com as dependências injetadas.
func NovoColunaUseCase(membros RepositorioMembro, colunas RepositorioColuna) *ColunaUseCase {
	return &ColunaUseCase{membros: membros, colunas: colunas}
}

// Criar acrescenta uma coluna no fim do quadro. Exige papel de edição.
func (uc *ColunaUseCase) Criar(ctx context.Context, boardID, usuarioID, titulo string, cores dcor.Cor) (*dcoluna.Coluna, error) {
	if _, err := acessoDeEdicao(ctx, uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}

	ultima, err := uc.colunas.UltimaPosicao(ctx, boardID)
	if err != nil {
		return nil, err
	}

	ultimaChave, err := uc.colunas.UltimaChave(ctx, boardID)
	if err != nil {
		return nil, err
	}
	chave, err := ordem.ChaveEntre(ultimaChave, "")
	if err != nil {
		return nil, err
	}

	c, err := dcoluna.Nova(uuid.NewString(), boardID, titulo, cores, dcoluna.PosicaoNoFim(ultima), chave)
	if err != nil {
		return nil, err
	}
	if err := uc.escreverEPublicar(ctx, evento.ColunaCriada, boardID, usuarioID,
		DadosDaColuna{ColunaID: c.ID, Titulo: c.Titulo},
		uc.escrita(), func(e Escrita) error { return e.Colunas.Salvar(ctx, c) }); err != nil {
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
	dados := DadosDaColuna{ColunaID: c.ID, Titulo: c.Titulo}
	if tituloAnterior != c.Titulo {
		dados.TituloAnterior = tituloAnterior
	}
	if err := uc.escreverEPublicar(ctx, evento.ColunaAlterada, c.BoardID, usuarioID, dados,
		uc.escrita(), func(e Escrita) error { return e.Colunas.Atualizar(ctx, c) }); err != nil {
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
	return uc.escreverEPublicar(ctx, evento.ColunaApagada, c.BoardID, usuarioID,
		DadosDaColuna{ColunaID: colunaID, Titulo: c.Titulo},
		uc.escrita(), func(e Escrita) error { return e.Colunas.Apagar(ctx, colunaID) })
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
