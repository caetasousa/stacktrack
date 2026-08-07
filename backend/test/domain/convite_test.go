package domain_test

import (
	"errors"
	"testing"
	"time"

	"stacktrack/internal/domain/convite"
	"stacktrack/internal/domain/membro"
)

func novoConvite(t *testing.T, email string, papel membro.Papel) *convite.Convite {
	t.Helper()
	c, err := convite.Novo("c-1", "b-1", email, papel, "hash", "ana")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	return c
}

func TestNovoConviteNormalizaOEmail(t *testing.T) {
	c := novoConvite(t, "  Bob@Exemplo.COM ", membro.PapelEditor)

	// Sem normalizar, quem foi convidado como "Bob@Exemplo.COM" e se cadastrou
	// como "bob@exemplo.com" não seria reconhecido ao aceitar.
	if c.Email != "bob@exemplo.com" {
		t.Errorf("email = %q, esperado normalizado", c.Email)
	}
}

func TestNovoConviteRecusaEmailVazioEPapelInvalido(t *testing.T) {
	if _, err := convite.Novo("c-1", "b-1", "   ", membro.PapelEditor, "hash", "ana"); !errors.Is(err, convite.ErrEmailObrigatorio) {
		t.Errorf("erro = %v, esperado ErrEmailObrigatorio", err)
	}
	if _, err := convite.Novo("c-1", "b-1", "bob@exemplo.com", membro.Papel("chefe"), "hash", "ana"); !errors.Is(err, membro.ErrPapelInvalido) {
		t.Errorf("erro = %v, esperado ErrPapelInvalido", err)
	}
}

func TestConviteNasceValidoPorSeteDias(t *testing.T) {
	c := novoConvite(t, "bob@exemplo.com", membro.PapelEditor)

	if !c.Pendente(c.CriadoEm.Add(6 * 24 * time.Hour)) {
		t.Error("no sexto dia o convite ainda devia valer")
	}
	if c.Pendente(c.CriadoEm.Add(convite.TTL + time.Minute)) {
		t.Error("passado o TTL o convite não vale mais")
	}
}

// Um convite vale uma vez só: sem isso, o link vazado continuaria funcionando
// depois de a pessoa certa já ter entrado.
func TestConviteAceitoNaoAceitaDeNovo(t *testing.T) {
	c := novoConvite(t, "bob@exemplo.com", membro.PapelEditor)

	if err := c.Aceitar(time.Now()); err != nil {
		t.Fatalf("primeira aceitação falhou: %v", err)
	}
	if c.AceitoEm == nil {
		t.Fatal("o convite devia ter registrado quando foi aceito")
	}

	if err := c.Aceitar(time.Now()); !errors.Is(err, convite.ErrInvalido) {
		t.Errorf("erro = %v, esperado ErrInvalido na segunda aceitação", err)
	}
}

func TestConviteVencidoNaoPodeSerAceito(t *testing.T) {
	c := novoConvite(t, "bob@exemplo.com", membro.PapelEditor)
	c.ExpiraEm = time.Now().Add(-time.Minute)

	if err := c.Aceitar(time.Now()); !errors.Is(err, convite.ErrInvalido) {
		t.Errorf("erro = %v, esperado ErrInvalido", err)
	}
	if c.AceitoEm != nil {
		t.Error("convite vencido não podia ter sido marcado como aceito")
	}
}

// --- regra do último dono -------------------------------------------------

func quadroCom(papeis ...membro.Papel) []membro.Membro {
	lista := make([]membro.Membro, 0, len(papeis))
	for i, papel := range papeis {
		lista = append(lista, membro.Membro{
			BoardID:   "b-1",
			UsuarioID: string(rune('a' + i)),
			Papel:     papel,
		})
	}
	return lista
}

// Quadro sem dono fica órfão: ninguém pode mais convidar, renomear nem
// apagá-lo, e não existe administrador do sistema para consertar.
func TestNaoDaParaRemoverOUltimoDono(t *testing.T) {
	todos := quadroCom(membro.PapelDono, membro.PapelEditor)

	if err := membro.ValidarRemocao(todos, "a"); !errors.Is(err, membro.ErrSemDono) {
		t.Errorf("erro = %v, esperado ErrSemDono", err)
	}
	if err := membro.ValidarRemocao(todos, "b"); err != nil {
		t.Errorf("remover o editor devia ser permitido: %v", err)
	}
}

func TestComDoisDonosQualquerUmPodeSair(t *testing.T) {
	todos := quadroCom(membro.PapelDono, membro.PapelDono)

	if err := membro.ValidarRemocao(todos, "a"); err != nil {
		t.Errorf("com dois donos, um pode sair: %v", err)
	}
}

// Rebaixar o último dono deixa o quadro órfão do mesmo jeito que removê-lo.
func TestNaoDaParaRebaixarOUltimoDono(t *testing.T) {
	todos := quadroCom(membro.PapelDono, membro.PapelLeitor)

	if err := membro.ValidarTrocaDePapel(todos, "a", membro.PapelEditor); !errors.Is(err, membro.ErrSemDono) {
		t.Errorf("erro = %v, esperado ErrSemDono", err)
	}
	// Continuar dono não é rebaixar: a troca para o mesmo papel é inofensiva.
	if err := membro.ValidarTrocaDePapel(todos, "a", membro.PapelDono); err != nil {
		t.Errorf("manter o papel devia ser permitido: %v", err)
	}
	if err := membro.ValidarTrocaDePapel(todos, "b", membro.PapelDono); err != nil {
		t.Errorf("promover o leitor devia ser permitido: %v", err)
	}
}

func TestOperacoesSobreQuemNaoParticipa(t *testing.T) {
	todos := quadroCom(membro.PapelDono)

	if err := membro.ValidarRemocao(todos, "estranho"); !errors.Is(err, membro.ErrNaoEMembro) {
		t.Errorf("erro = %v, esperado ErrNaoEMembro", err)
	}
	if err := membro.ValidarTrocaDePapel(todos, "estranho", membro.PapelEditor); !errors.Is(err, membro.ErrNaoEMembro) {
		t.Errorf("erro = %v, esperado ErrNaoEMembro", err)
	}
}

func TestTrocaParaPapelInvalidoERecusada(t *testing.T) {
	todos := quadroCom(membro.PapelDono, membro.PapelEditor)

	if err := membro.ValidarTrocaDePapel(todos, "b", membro.Papel("chefe")); !errors.Is(err, membro.ErrPapelInvalido) {
		t.Errorf("erro = %v, esperado ErrPapelInvalido", err)
	}
}
