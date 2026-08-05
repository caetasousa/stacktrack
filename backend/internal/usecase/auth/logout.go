package auth

import "kanbango/internal/pkg/token"

// LogoutUseCase encerra a sessão atual.
type LogoutUseCase struct {
	sessoes repositorioSessao
}

// NovoLogoutUseCase cria uma instância de LogoutUseCase com as dependências injetadas.
func NovoLogoutUseCase(sessoes repositorioSessao) *LogoutUseCase {
	return &LogoutUseCase{sessoes: sessoes}
}

// Executar apaga a sessão correspondente ao token informado. Encerrar uma
// sessão que já não existe não é erro: o resultado pretendido — nenhuma sessão
// ativa com esse token — já está valendo.
//
// A sessão é apagada do BANCO, não só o cookie do navegador. Limpar apenas o
// cookie deixaria o token válido no servidor, e quem tivesse uma cópia dele
// continuaria autenticado depois de a pessoa ter clicado em "sair".
func (uc *LogoutUseCase) Executar(tokenPuro string) error {
	return uc.sessoes.Remover(token.Hash(tokenPuro))
}
