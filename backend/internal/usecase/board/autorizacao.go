package board

import (
	dboard "stacktrack/internal/domain/board"
	"stacktrack/internal/domain/membro"
)

// acesso resolve o vínculo entre o usuário e o quadro.
//
// Quem não participa recebe dboard.ErrNaoEncontrado — 404, e NÃO 403. A
// diferença importa: responder "proibido" confirmaria que o quadro existe, e
// isso basta para varrer ids e mapear o que os outros têm. Só quem já enxerga
// o quadro pode receber 403, porque para essa pessoa a existência dele não é
// segredo nenhum.
func acesso(membros repositorioMembro, boardID, usuarioID string) (*membro.Membro, error) {
	m, err := membros.Buscar(boardID, usuarioID)
	if err != nil {
		return nil, err
	}
	if m == nil || !m.PodeVer() {
		return nil, dboard.ErrNaoEncontrado
	}
	return m, nil
}

// acessoDeEdicao é o acesso exigido para mexer no conteúdo do quadro (colunas
// e cards). Leitor recebe membro.ErrSemPermissao — 403, porque ele enxerga o
// quadro e a recusa não revela nada que ele já não saiba.
func acessoDeEdicao(membros repositorioMembro, boardID, usuarioID string) (*membro.Membro, error) {
	m, err := acesso(membros, boardID, usuarioID)
	if err != nil {
		return nil, err
	}
	if !m.PodeEditar() {
		return nil, membro.ErrSemPermissao
	}
	return m, nil
}

// acessoDeAdministracao é o acesso exigido para renomear e apagar o quadro.
func acessoDeAdministracao(membros repositorioMembro, boardID, usuarioID string) (*membro.Membro, error) {
	m, err := acesso(membros, boardID, usuarioID)
	if err != nil {
		return nil, err
	}
	if !m.PodeAdministrar() {
		return nil, membro.ErrSemPermissao
	}
	return m, nil
}
