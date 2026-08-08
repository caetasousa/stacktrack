package auth

import (
	"context"
	"stacktrack/internal/domain/usuario"
	"stacktrack/internal/pkg/token"
)

// LoginUseCase autentica um usuário e abre uma nova sessão.
type LoginUseCase struct {
	usuarios buscadorUsuario
	sessoes  repositorioSessao
	hasher   hasherSenha
	// hashDummy é verificado contra a senha informada quando o email não
	// existe, para equalizar o tempo de resposta — ver equalizarTempo.
	hashDummy string
}

// NovoLoginUseCase cria uma instância de LoginUseCase com as dependências injetadas.
//
// O hash dummy é calculado aqui, no boot, a partir de um valor aleatório: um
// hash fixo no código pareceria uma credencial esquecida no repositório, e
// gerá-lo a cada login custaria o mesmo Argon2id que se quer economizar.
func NovoLoginUseCase(usuarios buscadorUsuario, sessoes repositorioSessao, hasher hasherSenha) *LoginUseCase {
	uc := &LoginUseCase{usuarios: usuarios, sessoes: sessoes, hasher: hasher}
	if aleatorio, err := token.Gerar(); err == nil {
		uc.hashDummy, _ = hasher.Gerar(aleatorio)
	}
	return uc
}

// Executar valida as credenciais e, se corretas, cria uma nova sessão com
// validade de TTLSessao. Retorna ErrCredenciaisInvalidas tanto para email
// inexistente quanto para senha incorreta.
func (uc *LoginUseCase) Executar(ctx context.Context, input LoginInput) (*SessaoAberta, error) {
	u, err := uc.usuarios.BuscarPorEmail(ctx, usuario.NormalizarEmail(input.Email))
	if err != nil {
		return nil, err
	}
	if u == nil {
		uc.equalizarTempo(ctx, input.Senha)
		return nil, ErrCredenciaisInvalidas
	}

	ok, err := uc.hasher.Verificar(input.Senha, u.SenhaHash)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrCredenciaisInvalidas
	}

	return abrirSessao(ctx, uc.sessoes, u)
}

// equalizarTempo queima o mesmo custo de CPU que uma verificação real quando o
// email não existe.
//
// Sem isso, "email inexistente" responde na hora e "senha errada" demora os
// ~50ms do Argon2id — e essa diferença, medida de fora, vira um oráculo que
// diz quais emails têm conta aqui. A resposta já é genérica; o tempo também
// precisa ser.
func (uc *LoginUseCase) equalizarTempo(ctx context.Context, senha string) {
	if uc.hashDummy != "" {
		uc.hasher.Verificar(senha, uc.hashDummy)
		return
	}
	// Sem dummy (falha ao gerá-lo no boot), gerar um hash custa o equivalente.
	uc.hasher.Gerar(senha)
}
