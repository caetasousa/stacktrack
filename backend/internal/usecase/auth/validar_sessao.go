package auth

import (
	"context"
	"time"

	"stacktrack/internal/pkg/token"
)

// ValidarSessaoUseCase valida um token de sessão e devolve a identidade do
// usuário autenticado.
type ValidarSessaoUseCase struct {
	sessoes RepositorioSessao
}

// NovoValidarSessaoUseCase cria uma instância de ValidarSessaoUseCase com as dependências injetadas.
func NovoValidarSessaoUseCase(sessoes RepositorioSessao) *ValidarSessaoUseCase {
	return &ValidarSessaoUseCase{sessoes: sessoes}
}

// Executar busca a sessão pelo hash do token informado e retorna a identidade
// do usuário autenticado. Retorna ErrSessaoInvalida se a sessão não existir ou
// já tiver expirado.
func (uc *ValidarSessaoUseCase) Executar(ctx context.Context, tokenPuro string) (*Identidade, error) {
	s, err := uc.sessoes.BuscarPorTokenHash(ctx, token.Hash(tokenPuro))
	if err != nil {
		return nil, err
	}
	if s == nil || s.Expirada(time.Now()) {
		return nil, ErrSessaoInvalida
	}
	return &Identidade{UsuarioID: s.UsuarioID}, nil
}
