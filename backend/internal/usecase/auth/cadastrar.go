package auth

import (
	"context"
	"stacktrack/internal/domain/usuario"

	"github.com/google/uuid"
)

// CadastrarUseCase cria uma conta nova e já abre a sessão dela.
type CadastrarUseCase struct {
	usuarios RepositorioUsuario
	sessoes  RepositorioSessao
	hasher   HasherSenha
	unidade  UnidadeDeAutenticacao
}

// NovoCadastrarUseCase cria uma instância de CadastrarUseCase com as dependências injetadas.
func NovoCadastrarUseCase(usuarios RepositorioUsuario, sessoes RepositorioSessao, hasher HasherSenha) *CadastrarUseCase {
	return &CadastrarUseCase{usuarios: usuarios, sessoes: sessoes, hasher: hasher}
}

// ComUnidadeDeTrabalho liga a transação que grava conta e sessão juntas.
//
// Sem ela, o usecase continua funcionando com as duas escritas em sequência —
// que é o que os testes de regra querem, já que não há banco nenhum ali.
func (uc *CadastrarUseCase) ComUnidadeDeTrabalho(u UnidadeDeAutenticacao) {
	uc.unidade = u
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

	s, t, err := novaSessao(u)
	if err != nil {
		return nil, err
	}

	// Conta e sessão no MESMO commit. A checagem de email acima é conveniência
	// para dar uma mensagem boa; quem de fato decide a duplicidade é o UNIQUE do
	// banco, e entre a consulta e o INSERT cabe outro cadastro igual.
	//
	// Sem a unidade ligada (testes de regra), as duas escritas acontecem em
	// sequência — o comportamento anterior, preservado de propósito para que os
	// testes de domínio não precisem de transação.
	gravar := func(e EscritaDeAuth) error {
		if err := e.Usuarios.Salvar(ctx, u); err != nil {
			return err
		}
		return e.Sessoes.Salvar(ctx, s)
	}
	if uc.unidade != nil {
		if err := uc.unidade.Executar(ctx, gravar); err != nil {
			return nil, err
		}
	} else if err := gravar(EscritaDeAuth{Usuarios: uc.usuarios, Sessoes: uc.sessoes}); err != nil {
		return nil, err
	}

	return sessaoAberta(s, t, u), nil
}
