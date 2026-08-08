package auth

import (
	"context"
	"stacktrack/internal/domain/usuario"

	"github.com/google/uuid"
)

// CadastrarUseCase cria uma conta nova e já abre a sessão dela.
type CadastrarUseCase struct {
	usuarios repositorioUsuario
	sessoes  repositorioSessao
	hasher   hasherSenha
}

// NovoCadastrarUseCase cria uma instância de CadastrarUseCase com as dependências injetadas.
func NovoCadastrarUseCase(usuarios repositorioUsuario, sessoes repositorioSessao, hasher hasherSenha) *CadastrarUseCase {
	return &CadastrarUseCase{usuarios: usuarios, sessoes: sessoes, hasher: hasher}
}

// Executar valida os dados, verifica duplicidade de email, persiste a conta e
// abre uma sessão para ela. Retorna usuario.ErrEmailEmUso quando o email já
// pertence a alguém, e os erros de validação do domínio (nome vazio, email
// inválido, senha curta) quando a entrada não presta.
//
// O cadastro já sai logado porque não há confirmação por email para esperar:
// obrigar a pessoa a digitar as mesmas credenciais na tela seguinte seria
// cerimônia sem nenhuma garantia em troca. Quando a confirmação entrar, é aqui
// que a decisão muda.
func (uc *CadastrarUseCase) Executar(ctx context.Context, input CadastroInput) (*SessaoAberta, error) {
	if err := usuario.ValidarSenha(input.Senha); err != nil {
		return nil, err
	}

	email := usuario.NormalizarEmail(input.Email)
	existente, err := uc.usuarios.BuscarPorEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if existente != nil {
		return nil, usuario.ErrEmailEmUso
	}

	hash, err := uc.hasher.Gerar(input.Senha)
	if err != nil {
		return nil, err
	}

	u, err := usuario.Novo(uuid.NewString(), input.Nome, email, hash)
	if err != nil {
		return nil, err
	}
	if err := uc.usuarios.Salvar(ctx, u); err != nil {
		return nil, err
	}

	return abrirSessao(ctx, uc.sessoes, u)
}
