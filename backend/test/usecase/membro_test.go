// Convites e participação. O que estes testes mais exercitam é quem pode
// convidar, quem pode aceitar e o que impede o quadro de ficar sem dono.
package usecase_test

import (
	"context"
	"errors"
	"testing"

	dconvite "stacktrack/internal/domain/convite"
	"stacktrack/internal/domain/membro"
	"stacktrack/internal/domain/usuario"
	ucauth "stacktrack/internal/usecase/auth"
	ucboard "stacktrack/internal/usecase/board"
	"stacktrack/test/repository/memoria"
)

type colaboracao struct {
	*quadro
	usuarios *memoria.Usuarios
	convites *memoria.Convites
	membroUC *ucboard.MembroUseCase
	cadastro *ucauth.CadastrarUseCase
}

func novaColaboracao(t *testing.T) *colaboracao {
	t.Helper()
	q := novoQuadro()
	usuarios := memoria.NovosUsuarios()
	convites := memoria.NovosConvites()
	q.membros.LigarUsuarios(usuarios)
	q.responsaveis.LigarUsuarios(usuarios)

	return &colaboracao{
		quadro:   q,
		usuarios: usuarios,
		convites: convites,
		membroUC: ucboard.NovoMembroUseCase(q.membros, convites, usuarios, q.boards, q.responsaveis),
		cadastro: ucauth.NovoCadastrarUseCase(usuarios, memoria.NovasSessoes(), &memoria.Hasher{}),
	}
}

// conta cria um usuário de verdade pelo usecase de cadastro e devolve o id.
func (c *colaboracao) conta(t *testing.T, nome, email string) string {
	t.Helper()
	out, err := c.cadastro.Executar(context.Background(), ucauth.CadastroInput{Nome: nome, Email: email, Senha: "senha-boa-123"})
	if err != nil {
		t.Fatalf("cadastro de %s falhou: %v", nome, err)
	}
	return out.UsuarioID
}

func TestConvidarQuemJaTemContaAdicionaNaHora(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	bob := c.conta(t, "Bob", "bob@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")

	resultado, err := c.membroUC.Convidar(context.Background(), boardID, ana, "bob@exemplo.com", membro.PapelEditor)
	if err != nil {
		t.Fatalf("erro ao convidar: %v", err)
	}

	if !resultado.Adicionado {
		t.Fatal("quem já tem conta entra direto, sem passar por convite")
	}
	if resultado.Token != "" {
		t.Error("não faz sentido gerar link para quem já entrou")
	}
	if c.convites.Quantidade() != 0 {
		t.Error("nenhum convite devia ter sido criado")
	}

	vinculo, _ := c.membros.Buscar(context.Background(), boardID, bob)
	if vinculo == nil || vinculo.Papel != membro.PapelEditor {
		t.Errorf("vínculo = %+v, esperado editor", vinculo)
	}
}

// O email é a chave: quem se cadastrou com outra caixa é a mesma pessoa.
func TestConvidarReconheceContaComOutraCaixaNoEmail(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	c.conta(t, "Bob", "bob@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")

	resultado, err := c.membroUC.Convidar(context.Background(), boardID, ana, "  BOB@Exemplo.com ", membro.PapelLeitor)
	if err != nil {
		t.Fatalf("erro ao convidar: %v", err)
	}
	if !resultado.Adicionado {
		t.Error("devia ter reconhecido a conta existente")
	}
}

func TestConvidarQuemNaoTemContaGeraLink(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")

	resultado, err := c.membroUC.Convidar(context.Background(), boardID, ana, "novo@exemplo.com", membro.PapelEditor)
	if err != nil {
		t.Fatalf("erro ao convidar: %v", err)
	}

	if resultado.Adicionado {
		t.Fatal("não há conta para adicionar")
	}
	if resultado.Token == "" {
		t.Fatal("o convite pendente precisa devolver o token para montar o link")
	}
	if resultado.Convite.Papel != membro.PapelEditor {
		t.Errorf("papel do convite = %q", resultado.Convite.Papel)
	}
}

// O banco guarda só o hash: quem tiver o dump não consegue usar os convites.
func TestConvitePersisteApenasOHashDoToken(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")

	resultado, _ := c.membroUC.Convidar(context.Background(), boardID, ana, "novo@exemplo.com", membro.PapelEditor)

	if achado, _ := c.convites.BuscarPorTokenHash(context.Background(), resultado.Token); achado != nil {
		t.Error("o convite foi encontrado pelo token puro — ele está sendo persistido")
	}
}

func TestSoODonoConvida(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	bob := c.conta(t, "Bob", "bob@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")
	c.convidar(t, boardID, bob, membro.PapelEditor)

	_, err := c.membroUC.Convidar(context.Background(), boardID, bob, "outro@exemplo.com", membro.PapelLeitor)

	if !errors.Is(err, membro.ErrSemPermissao) {
		t.Errorf("erro = %v, esperado ErrSemPermissao — editor não convida", err)
	}
}

func TestQuemNaoParticipaNaoDescobreOQuadroAoConvidar(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	bob := c.conta(t, "Bob", "bob@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")

	_, err := c.membroUC.Convidar(context.Background(), boardID, bob, "outro@exemplo.com", membro.PapelLeitor)

	if errors.Is(err, membro.ErrSemPermissao) {
		t.Error("responder 'sem permissão' confirmaria que o quadro existe")
	}
}

func TestNaoDaParaConvidarQuemJaParticipaNemASiMesmo(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	bob := c.conta(t, "Bob", "bob@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")
	c.convidar(t, boardID, bob, membro.PapelEditor)

	if _, err := c.membroUC.Convidar(context.Background(), boardID, ana, "bob@exemplo.com", membro.PapelLeitor); !errors.Is(err, dconvite.ErrJaEMembro) {
		t.Errorf("erro = %v, esperado ErrJaEMembro", err)
	}
	if _, err := c.membroUC.Convidar(context.Background(), boardID, ana, "ana@exemplo.com", membro.PapelLeitor); !errors.Is(err, dconvite.ErrNaoConvidaODono) {
		t.Errorf("erro = %v, esperado ErrNaoConvidaODono", err)
	}
}

func TestNaoDaParaConvidarDuasVezesOMesmoEmail(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")
	c.membroUC.Convidar(context.Background(), boardID, ana, "novo@exemplo.com", membro.PapelEditor)

	_, err := c.membroUC.Convidar(context.Background(), boardID, ana, "novo@exemplo.com", membro.PapelLeitor)

	if !errors.Is(err, dconvite.ErrJaConvidado) {
		t.Errorf("erro = %v, esperado ErrJaConvidado", err)
	}
}

func TestAceitarConviteViraParticipacao(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")
	resultado, _ := c.membroUC.Convidar(context.Background(), boardID, ana, "novo@exemplo.com", membro.PapelEditor)

	// a pessoa cria a conta depois de receber o link
	novo := c.conta(t, "Novo", "novo@exemplo.com")

	b, papel, err := c.membroUC.Aceitar(context.Background(), resultado.Token, novo)
	if err != nil {
		t.Fatalf("erro ao aceitar: %v", err)
	}
	if b.ID != boardID {
		t.Errorf("aceitar devia devolver o quadro do convite")
	}
	// O papel volta junto para a tela saber o que oferecer sem uma segunda ida
	// à API logo depois de aceitar.
	if papel != membro.PapelEditor {
		t.Errorf("papel = %q, esperado o do convite", papel)
	}

	vinculo, _ := c.membros.Buscar(context.Background(), boardID, novo)
	if vinculo == nil || vinculo.Papel != membro.PapelEditor {
		t.Errorf("vínculo = %+v, esperado editor", vinculo)
	}
	if _, err := c.quadros.Detalhar(context.Background(), boardID, novo); err != nil {
		t.Errorf("quem aceitou devia enxergar o quadro: %v", err)
	}
}

// O link é a credencial, mas não basta: sem amarrar ao email, um link
// encaminhado colocaria qualquer pessoa dentro do quadro.
func TestOutraContaNaoAceitaOConviteAlheio(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	intruso := c.conta(t, "Intruso", "intruso@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")
	resultado, _ := c.membroUC.Convidar(context.Background(), boardID, ana, "novo@exemplo.com", membro.PapelEditor)

	_, _, err := c.membroUC.Aceitar(context.Background(), resultado.Token, intruso)

	if !errors.Is(err, dconvite.ErrOutroDestinatario) {
		t.Errorf("erro = %v, esperado ErrOutroDestinatario", err)
	}
	if vinculo, _ := c.membros.Buscar(context.Background(), boardID, intruso); vinculo != nil {
		t.Error("o intruso não podia ter entrado no quadro")
	}
}

func TestConviteValeUmaVezSo(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")
	resultado, _ := c.membroUC.Convidar(context.Background(), boardID, ana, "novo@exemplo.com", membro.PapelEditor)
	novo := c.conta(t, "Novo", "novo@exemplo.com")

	if _, _, err := c.membroUC.Aceitar(context.Background(), resultado.Token, novo); err != nil {
		t.Fatalf("primeira aceitação falhou: %v", err)
	}

	// A pessoa já é membro; o link, porém, não vale de novo para ninguém.
	if _, _, err := c.membroUC.Aceitar(context.Background(), resultado.Token, novo); !errors.Is(err, dconvite.ErrInvalido) {
		t.Errorf("erro = %v, esperado ErrInvalido", err)
	}
}

func TestConviteVencidoNaoEAceito(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")
	resultado, _ := c.membroUC.Convidar(context.Background(), boardID, ana, "novo@exemplo.com", membro.PapelEditor)
	c.convites.Vencer(resultado.Convite.ID)
	novo := c.conta(t, "Novo", "novo@exemplo.com")

	if _, _, err := c.membroUC.Aceitar(context.Background(), resultado.Token, novo); !errors.Is(err, dconvite.ErrInvalido) {
		t.Errorf("erro = %v, esperado ErrInvalido", err)
	}
}

// Token desconhecido e convite vencido respondem igual: distinguir ajudaria
// quem estivesse testando links.
func TestTokenDesconhecidoRespondeComoConviteInvalido(t *testing.T) {
	c := novaColaboracao(t)
	novo := c.conta(t, "Novo", "novo@exemplo.com")

	if _, err := c.membroUC.DetalharConvite(context.Background(), "token-inventado"); !errors.Is(err, dconvite.ErrInvalido) {
		t.Errorf("detalhar: erro = %v", err)
	}
	if _, _, err := c.membroUC.Aceitar(context.Background(), "token-inventado", novo); !errors.Is(err, dconvite.ErrInvalido) {
		t.Errorf("aceitar: erro = %v", err)
	}
}

func TestDetalharConviteNaoExigeSessao(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")
	resultado, _ := c.membroUC.Convidar(context.Background(), boardID, ana, "novo@exemplo.com", membro.PapelEditor)

	detalhe, err := c.membroUC.DetalharConvite(context.Background(), resultado.Token)
	if err != nil {
		t.Fatalf("erro ao detalhar: %v", err)
	}

	if detalhe.TituloQuadro != "Estudos" || detalhe.Email != "novo@exemplo.com" {
		t.Errorf("detalhe = %+v", detalhe)
	}
	if detalhe.ConvidadoPor != "Ana" {
		t.Errorf("convidadoPor = %q, esperado o nome de quem convidou", detalhe.ConvidadoPor)
	}
}

func TestListarMembrosTrazTodosComNomeEEmail(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	bob := c.conta(t, "Bob", "bob@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")
	c.membroUC.Convidar(context.Background(), boardID, ana, "bob@exemplo.com", membro.PapelLeitor)

	// Até o leitor enxerga a lista: saber com quem se divide um quadro é parte
	// de participar dele.
	participantes, err := c.membroUC.Listar(context.Background(), boardID, bob)
	if err != nil {
		t.Fatalf("erro ao listar: %v", err)
	}
	if len(participantes) != 2 {
		t.Fatalf("participantes = %d, esperado 2", len(participantes))
	}
	if participantes[0].Papel != membro.PapelDono || participantes[0].Nome != "Ana" {
		t.Errorf("o dono devia vir primeiro, veio %+v", participantes[0])
	}
	if participantes[1].Email != "bob@exemplo.com" {
		t.Errorf("segundo participante = %+v", participantes[1])
	}
}

// Para quem só participa, a lista de convites não é da conta: ela diz para
// quem o quadro foi oferecido.
func TestSoODonoVeOsConvitesPendentes(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	bob := c.conta(t, "Bob", "bob@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")
	c.convidar(t, boardID, bob, membro.PapelEditor)
	c.membroUC.Convidar(context.Background(), boardID, ana, "novo@exemplo.com", membro.PapelLeitor)

	doDono, err := c.membroUC.ListarConvites(context.Background(), boardID, ana)
	if err != nil || len(doDono) != 1 {
		t.Fatalf("o dono devia ver 1 convite: %d, %v", len(doDono), err)
	}
	if _, err := c.membroUC.ListarConvites(context.Background(), boardID, bob); !errors.Is(err, membro.ErrSemPermissao) {
		t.Errorf("erro = %v, esperado ErrSemPermissao", err)
	}
}

func TestRevogarConviteInvalidaOLink(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")
	resultado, _ := c.membroUC.Convidar(context.Background(), boardID, ana, "novo@exemplo.com", membro.PapelEditor)

	if err := c.membroUC.RevogarConvite(context.Background(), resultado.Convite.ID, ana); err != nil {
		t.Fatalf("erro ao revogar: %v", err)
	}

	novo := c.conta(t, "Novo", "novo@exemplo.com")
	if _, _, err := c.membroUC.Aceitar(context.Background(), resultado.Token, novo); !errors.Is(err, dconvite.ErrInvalido) {
		t.Errorf("erro = %v — o link revogado não pode mais valer", err)
	}
}

func TestAlterarPapelEremoverExigemODono(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	bob := c.conta(t, "Bob", "bob@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")
	c.convidar(t, boardID, bob, membro.PapelEditor)

	if _, err := c.membroUC.AlterarPapel(context.Background(), boardID, bob, ana, membro.PapelLeitor); !errors.Is(err, membro.ErrSemPermissao) {
		t.Errorf("editor não troca papel: %v", err)
	}
	if err := c.membroUC.Remover(context.Background(), boardID, bob, ana); !errors.Is(err, membro.ErrSemPermissao) {
		t.Errorf("editor não remove ninguém: %v", err)
	}
}

func TestDonoPromoveERemove(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	bob := c.conta(t, "Bob", "bob@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")
	c.convidar(t, boardID, bob, membro.PapelLeitor)

	p, err := c.membroUC.AlterarPapel(context.Background(), boardID, ana, bob, membro.PapelEditor)
	if err != nil {
		t.Fatalf("erro ao promover: %v", err)
	}
	if p.Papel != membro.PapelEditor || p.Nome != "Bob" {
		t.Errorf("participante = %+v", p)
	}

	if err := c.membroUC.Remover(context.Background(), boardID, ana, bob); err != nil {
		t.Fatalf("erro ao remover: %v", err)
	}
	if vinculo, _ := c.membros.Buscar(context.Background(), boardID, bob); vinculo != nil {
		t.Error("o vínculo devia ter sumido")
	}
	// e o quadro some da vista de quem saiu
	if _, err := c.quadros.Detalhar(context.Background(), boardID, bob); err == nil {
		t.Error("quem foi removido não devia mais enxergar o quadro")
	}
}

// Um quadro sem dono fica órfão: ninguém pode convidar, renomear nem apagá-lo.
func TestODonoNaoConsegueSeRemoverSozinho(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")

	if err := c.membroUC.Remover(context.Background(), boardID, ana, ana); !errors.Is(err, membro.ErrSemDono) {
		t.Errorf("erro = %v, esperado ErrSemDono", err)
	}
	if _, err := c.membroUC.AlterarPapel(context.Background(), boardID, ana, ana, membro.PapelLeitor); !errors.Is(err, membro.ErrSemDono) {
		t.Errorf("erro = %v, esperado ErrSemDono ao se rebaixar", err)
	}
}

func TestComOutroDonoOPrimeiroPodeSair(t *testing.T) {
	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	bob := c.conta(t, "Bob", "bob@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")
	c.convidar(t, boardID, bob, membro.PapelDono)

	if err := c.membroUC.Remover(context.Background(), boardID, ana, ana); err != nil {
		t.Errorf("com dois donos, o primeiro pode sair: %v", err)
	}
}

// A conta e o quadro são coisas separadas: normalizar o email em um lugar só
// não bastaria se o outro lado guardasse a caixa original.
func TestEmailDoConviteSegueAMesmaReguaDaConta(t *testing.T) {
	if usuario.NormalizarEmail("  NOVO@Exemplo.com ") != "novo@exemplo.com" {
		t.Fatal("pré-condição do teste mudou")
	}

	c := novaColaboracao(t)
	ana := c.conta(t, "Ana", "ana@exemplo.com")
	boardID := c.criarQuadro(t, ana, "Estudos")

	resultado, err := c.membroUC.Convidar(context.Background(), boardID, ana, "  NOVO@Exemplo.com ", membro.PapelEditor)
	if err != nil {
		t.Fatalf("erro ao convidar: %v", err)
	}

	// cadastro com a caixa "errada" — mesma pessoa
	novo := c.conta(t, "Novo", "novo@EXEMPLO.com")
	if _, _, err := c.membroUC.Aceitar(context.Background(), resultado.Token, novo); err != nil {
		t.Errorf("o convite devia ser aceito: %v", err)
	}
}
