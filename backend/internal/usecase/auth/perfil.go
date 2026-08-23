package auth

import (
	"context"
	"stacktrack/internal/domain/usuario"
)

// PerfilUseCase devolve os dados da conta autenticada.
type PerfilUseCase struct {
	usuarios BuscadorUsuario
}

// NovoPerfilUseCase cria uma instância de PerfilUseCase com as dependências injetadas.
func NovoPerfilUseCase(usuarios BuscadorUsuario) *PerfilUseCase {
	return &PerfilUseCase{usuarios: usuarios}
}

// Executar busca o usuário da identidade autenticada. Retorna
// ErrSessaoInvalida quando o usuário não existe mais — a conta foi apagada
// enquanto a sessão ainda estava viva, e uma sessão sem dono não autentica
// ninguém.
func (uc *PerfilUseCase) Executar(ctx context.Context, usuarioID string) (*usuario.Usuario, error) {
	u, err := uc.usuarios.BuscarPorID(ctx, usuarioID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrSessaoInvalida
	}
	return u, nil
}
