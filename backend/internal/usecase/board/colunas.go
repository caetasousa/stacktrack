package board

import (
	dcoluna "kanbango/internal/domain/coluna"

	"github.com/google/uuid"
)

// ColunaUseCase reúne as operações sobre colunas.
type ColunaUseCase struct {
	membros repositorioMembro
	colunas repositorioColuna
}

// NovoColunaUseCase cria uma instância de ColunaUseCase com as dependências injetadas.
func NovoColunaUseCase(membros repositorioMembro, colunas repositorioColuna) *ColunaUseCase {
	return &ColunaUseCase{membros: membros, colunas: colunas}
}

// Criar acrescenta uma coluna no fim do quadro. Exige papel de edição.
func (uc *ColunaUseCase) Criar(boardID, usuarioID, titulo string) (*dcoluna.Coluna, error) {
	if _, err := acessoDeEdicao(uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}

	ultima, err := uc.colunas.UltimaPosicao(boardID)
	if err != nil {
		return nil, err
	}

	c, err := dcoluna.Nova(uuid.NewString(), boardID, titulo, dcoluna.PosicaoNoFim(ultima))
	if err != nil {
		return nil, err
	}
	if err := uc.colunas.Salvar(c); err != nil {
		return nil, err
	}
	return c, nil
}

// Renomear troca o título da coluna. Exige papel de edição no quadro dela.
func (uc *ColunaUseCase) Renomear(colunaID, usuarioID, titulo string) (*dcoluna.Coluna, error) {
	c, err := uc.carregarComAcessoDeEdicao(colunaID, usuarioID)
	if err != nil {
		return nil, err
	}
	if err := c.Renomear(titulo); err != nil {
		return nil, err
	}
	if err := uc.colunas.Atualizar(c); err != nil {
		return nil, err
	}
	return c, nil
}

// Apagar remove a coluna e, por cascata, os cards dela. Exige papel de edição.
func (uc *ColunaUseCase) Apagar(colunaID, usuarioID string) error {
	if _, err := uc.carregarComAcessoDeEdicao(colunaID, usuarioID); err != nil {
		return err
	}
	return uc.colunas.Apagar(colunaID)
}

// carregarComAcessoDeEdicao busca a coluna e confere o acesso ao quadro DELA —
// e não a um quadro informado por quem chamou. Aceitar o boardID da requisição
// deixaria alguém apagar a coluna de um quadro alheio informando o id de um
// quadro próprio.
func (uc *ColunaUseCase) carregarComAcessoDeEdicao(colunaID, usuarioID string) (*dcoluna.Coluna, error) {
	c, err := uc.colunas.BuscarPorID(colunaID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, dcoluna.ErrNaoEncontrada
	}
	if _, err := acessoDeEdicao(uc.membros, c.BoardID, usuarioID); err != nil {
		// Quem não participa do quadro recebe "coluna não encontrada" pelo
		// mesmo motivo do quadro: confirmar a existência já é informação.
		return nil, traduzirParaColuna(err)
	}
	return c, nil
}
