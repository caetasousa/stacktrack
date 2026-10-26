//go:build integracao

// A entrada pelo domínio do email e os tetos do plano gratuito, com os
// repositórios e a unidade de trabalho de verdade.
//
// Como isto quebra — cada teste abaixo cobre um item:
//
//	DM1  a colega do mesmo domínio não encontra o quadro aberto, ou entra com
//	    outro papel, ou entra sem o evento que avisa quem está no quadro;
//	DM2  outro domínio, um subdomínio ou conta sem email confirmado veem ou
//	    entram no quadro;
//	DM3  o dono abre um domínio de email pessoal, ou abre sem ter confirmado o
//	    próprio email;
//	DM4  quem o dono tirou volta sozinho pelo domínio — e o convite aceito, que é
//	    decisão do dono, não o deixa voltar;
//	DM5  fechar a porta não esconde o quadro de quem não entrou, ou tira quem já
//	    tinha entrado;
//	DM6  o quadro no teto aceita mais uma pessoa, pelo domínio ou por convite;
//	DM7  duas entradas simultâneas da mesma pessoa criam dois vínculos ou dois
//	    eventos;
//	DM8  a conta no teto de quadros cria mais um.
package repository_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"stacktrack/internal/adapter/repository"
	dboard "stacktrack/internal/domain/board"
	"stacktrack/internal/domain/dominio"
	"stacktrack/internal/domain/membro"
	ucboard "stacktrack/internal/usecase/board"

	"github.com/google/uuid"
)

// ambienteDeDominio é uma dona com email confirmado de uma empresa de mentira
// (um domínio novo por teste, para os testes não se enxergarem) e o quadro dela.
type ambienteDeDominio struct {
	membros *ucboard.MembroUseCase
	quadros *ucboard.QuadroUseCase
	empresa string
	dona    string
	boardID string
}

func novoAmbienteDeDominio(t *testing.T) *ambienteDeDominio {
	t.Helper()
	a := &ambienteDeDominio{empresa: "empresa-" + uuid.NewString()[:8] + ".com.br"}

	usuarios := repository.NovoUsuarioPostgres(pool)
	boards := repository.NovoBoardPostgres(pool)
	membros := repository.NovoMembroPostgres(pool)
	responsaveis := repository.NovoResponsavelPostgres(pool)
	unidade := novaUnidade(3 * time.Second)
	registro := repository.NovoEventoPostgres(pool)

	a.membros = ucboard.NovoMembroUseCase(membros, repository.NovoConvitePostgres(pool), usuarios, boards, responsaveis)
	a.membros.ComDominios(repository.NovoDominioPostgres(pool))
	a.quadros = ucboard.NovoQuadroUseCase(boards, membros,
		repository.NovoColunaPostgres(pool), repository.NovoCardPostgres(pool),
		repository.NovoEtiquetaPostgres(pool), repository.NovoChecklistPostgres(pool),
		repository.NovoAnexoPostgres(pool), responsaveis, repository.NovoComentarioPostgres(pool), nil)
	for _, uc := range []interface {
		ComEscritaAtomica(ucboard.EscritaAtomica)
		ComRegistro(ucboard.RegistroDeEventos)
	}{a.membros, a.quadros} {
		uc.ComEscritaAtomica(unidade)
		uc.ComRegistro(registro)
	}

	a.dona = contaComEmail(t, "Ana", "ana-"+uuid.NewString()[:8]+"@"+a.empresa, true)
	b, err := a.quadros.Criar(context.Background(), a.dona, "Vendas")
	if err != nil {
		t.Fatalf("criar o quadro: %v", err)
	}
	a.boardID = b.ID
	return a
}

// contaComEmail cria uma conta com o email exato, confirmada ou não.
func contaComEmail(t *testing.T, nome, email string, confirmada bool) string {
	t.Helper()
	id := uuid.NewString()
	confirmadaEm := any(nil)
	if confirmada {
		confirmadaEm = time.Now()
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO usuarios (id, nome, email, senha_hash, email_confirmado_em, criado_em, atualizado_em)
		 VALUES ($1, $2, $3, 'hash', $4, now(), now())`,
		id, nome, email, confirmadaEm); err != nil {
		t.Fatalf("conta %s: %v", email, err)
	}
	return id
}

// colega cria uma conta confirmada do domínio da empresa.
func (a *ambienteDeDominio) colega(t *testing.T, nome string) string {
	t.Helper()
	return contaComEmail(t, nome, nome+"-"+uuid.NewString()[:8]+"@"+a.empresa, true)
}

func (a *ambienteDeDominio) abrir(t *testing.T, papel membro.Papel) {
	t.Helper()
	if _, err := a.membros.AbrirAoDominio(context.Background(), a.boardID, a.dona, papel); err != nil {
		t.Fatalf("abrir ao domínio: %v", err)
	}
}

// enxerga diz se o quadro aparece na lista "da sua empresa" da conta.
func (a *ambienteDeDominio) enxerga(t *testing.T, usuarioID string) bool {
	t.Helper()
	quadros, err := a.membros.QuadrosDoMeuDominio(context.Background(), usuarioID)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	for _, q := range quadros {
		if q.ID == a.boardID {
			return true
		}
	}
	return false
}

func (a *ambienteDeDominio) papelDe(t *testing.T, usuarioID string) membro.Papel {
	t.Helper()
	m, err := repository.NovoMembroPostgres(pool).Buscar(context.Background(), a.boardID, usuarioID)
	if err != nil {
		t.Fatalf("buscar vínculo: %v", err)
	}
	if m == nil {
		return ""
	}
	return m.Papel
}

func eventosDoTipo(t *testing.T, boardID, tipo string) int {
	t.Helper()
	var total int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM board_events WHERE board_id = $1 AND tipo = $2`, boardID, tipo,
	).Scan(&total); err != nil {
		t.Fatalf("contar eventos: %v", err)
	}
	return total
}

func TestDM1AColegaDoDominioEncontraEEntraComOPapelEscolhido(t *testing.T) {
	a := novoAmbienteDeDominio(t)
	a.abrir(t, membro.PapelLeitor)
	bruno := a.colega(t, "bruno")

	if !a.enxerga(t, bruno) {
		t.Fatal("a colega do mesmo domínio não encontra o quadro aberto")
	}
	_, papel, err := a.membros.EntrarPeloDominio(context.Background(), a.boardID, bruno)
	if err != nil {
		t.Fatalf("entrar: %v", err)
	}
	if papel != membro.PapelLeitor || a.papelDe(t, bruno) != membro.PapelLeitor {
		t.Errorf("esperado leitor, veio %q (gravado: %q)", papel, a.papelDe(t, bruno))
	}
	if n := eventosDoTipo(t, a.boardID, "membro.entrou"); n != 1 {
		t.Errorf("esperado 1 evento membro.entrou, veio %d", n)
	}
	if a.enxerga(t, bruno) {
		t.Error("quem já entrou continua vendo o quadro na lista de quadros para entrar")
	}
}

func TestDM2OutroDominioSubdominioESemConfirmacaoNaoEntram(t *testing.T) {
	a := novoAmbienteDeDominio(t)
	a.abrir(t, membro.PapelEditor)
	ctx := context.Background()

	deFora := contaComEmail(t, "Carla", "carla-"+uuid.NewString()[:8]+"@outra-empresa.com.br", true)
	subdominio := contaComEmail(t, "Davi", "davi-"+uuid.NewString()[:8]+"@vendas."+a.empresa, true)
	semConfirmar := contaComEmail(t, "Eva", "eva-"+uuid.NewString()[:8]+"@"+a.empresa, false)

	for nome, id := range map[string]string{"outro domínio": deFora, "subdomínio": subdominio, "sem confirmar": semConfirmar} {
		if a.enxerga(t, id) {
			t.Errorf("%s: vê o quadro da empresa", nome)
		}
		if _, _, err := a.membros.EntrarPeloDominio(ctx, a.boardID, id); !errors.Is(err, dboard.ErrNaoEncontrado) {
			t.Errorf("%s: esperado ErrNaoEncontrado, veio %v", nome, err)
		}
		if a.papelDe(t, id) != "" {
			t.Errorf("%s: entrou no quadro", nome)
		}
	}
}

func TestDM3EmailPessoalOuNaoConfirmadoNaoAbreODominio(t *testing.T) {
	a := novoAmbienteDeDominio(t)
	ctx := context.Background()

	pessoal := contaComEmail(t, "Fabi", "fabi-"+uuid.NewString()[:8]+"@gmail.com", true)
	b, err := a.quadros.Criar(ctx, pessoal, "Pessoal")
	if err != nil {
		t.Fatalf("criar: %v", err)
	}
	if _, err := a.membros.AbrirAoDominio(ctx, b.ID, pessoal, membro.PapelEditor); !errors.Is(err, dominio.ErrDominioPessoal) {
		t.Errorf("gmail: esperado ErrDominioPessoal, veio %v", err)
	}

	pendente := contaComEmail(t, "Gil", "gil-"+uuid.NewString()[:8]+"@"+a.empresa, false)
	b2, err := a.quadros.Criar(ctx, pendente, "Sem confirmar")
	if err != nil {
		t.Fatalf("criar: %v", err)
	}
	if _, err := a.membros.AbrirAoDominio(ctx, b2.ID, pendente, membro.PapelEditor); !errors.Is(err, dominio.ErrEmailNaoConfirmado) {
		t.Errorf("sem confirmar: esperado ErrEmailNaoConfirmado, veio %v", err)
	}
	for _, id := range []string{b.ID, b2.ID} {
		if d, _, _ := repository.NovoDominioPostgres(pool).DoQuadro(ctx, id); d != "" {
			t.Errorf("a recusa gravou o domínio %q", d)
		}
	}
}

func TestDM4QuemODonoTirouSoVoltaPorConvite(t *testing.T) {
	a := novoAmbienteDeDominio(t)
	a.abrir(t, membro.PapelEditor)
	ctx := context.Background()
	bruno := a.colega(t, "bruno")

	if _, _, err := a.membros.EntrarPeloDominio(ctx, a.boardID, bruno); err != nil {
		t.Fatalf("entrar: %v", err)
	}
	if err := a.membros.Remover(ctx, a.boardID, a.dona, bruno); err != nil {
		t.Fatalf("remover: %v", err)
	}
	if a.enxerga(t, bruno) {
		t.Error("quem foi tirado continua vendo o quadro para entrar")
	}
	if _, _, err := a.membros.EntrarPeloDominio(ctx, a.boardID, bruno); !errors.Is(err, dboard.ErrNaoEncontrado) {
		t.Fatalf("quem foi tirado voltou sozinho: %v", err)
	}

	var email string
	if err := pool.QueryRow(ctx, `SELECT email FROM usuarios WHERE id = $1`, bruno).Scan(&email); err != nil {
		t.Fatalf("email: %v", err)
	}
	convite, err := a.membros.Convidar(ctx, a.boardID, a.dona, email, membro.PapelLeitor)
	if err != nil {
		t.Fatalf("convidar: %v", err)
	}
	if _, _, err := a.membros.Aceitar(ctx, convite.Token, bruno); err != nil {
		t.Fatalf("aceitar: %v", err)
	}
	if a.papelDe(t, bruno) != membro.PapelLeitor {
		t.Fatalf("o convite aceito não pôs a pessoa de volta")
	}
	// O convite é a decisão do dono: depois dele, o bloqueio não existe mais.
	var bloqueios int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM board_bloqueios WHERE board_id = $1 AND usuario_id = $2`,
		a.boardID, bruno).Scan(&bloqueios); err != nil {
		t.Fatalf("bloqueios: %v", err)
	}
	if bloqueios != 0 {
		t.Error("o convite aceito não desfez o bloqueio")
	}
}

func TestDM5FecharEscondeDeQuemNaoEntrouEMantemQuemEntrou(t *testing.T) {
	a := novoAmbienteDeDominio(t)
	a.abrir(t, membro.PapelEditor)
	ctx := context.Background()
	dentro := a.colega(t, "bruno")
	fora := a.colega(t, "carla")

	if _, _, err := a.membros.EntrarPeloDominio(ctx, a.boardID, dentro); err != nil {
		t.Fatalf("entrar: %v", err)
	}
	if err := a.membros.FecharAoDominio(ctx, a.boardID, a.dona); err != nil {
		t.Fatalf("fechar: %v", err)
	}
	if a.enxerga(t, fora) {
		t.Error("o quadro fechado continua na lista de quem não entrou")
	}
	if _, _, err := a.membros.EntrarPeloDominio(ctx, a.boardID, fora); !errors.Is(err, dboard.ErrNaoEncontrado) {
		t.Errorf("entrou num quadro fechado: %v", err)
	}
	if a.papelDe(t, dentro) != membro.PapelEditor {
		t.Error("fechar a porta tirou quem já tinha entrado")
	}
	if n := eventosDoTipo(t, a.boardID, "quadro.dominio_fechado"); n != 1 {
		t.Errorf("esperado 1 evento de fechamento, veio %d", n)
	}
	// Fechar de novo não grava outro evento.
	if err := a.membros.FecharAoDominio(ctx, a.boardID, a.dona); err != nil {
		t.Fatalf("fechar de novo: %v", err)
	}
	if n := eventosDoTipo(t, a.boardID, "quadro.dominio_fechado"); n != 1 {
		t.Errorf("fechar o que já estava fechado gravou evento: %d", n)
	}
}

func TestDM6OQuadroNoTetoNaoAceitaMaisNinguem(t *testing.T) {
	a := novoAmbienteDeDominio(t)
	a.abrir(t, membro.PapelEditor)
	ctx := context.Background()

	// A dona já ocupa uma vaga: o quadro fica com MaximoPorQuadro participantes.
	vinculos := repository.NovoMembroPostgres(pool)
	for i := 1; i < membro.MaximoPorQuadro; i++ {
		id := contaComEmail(t, fmt.Sprintf("p%d", i), fmt.Sprintf("p%d-%s@outra.dev", i, uuid.NewString()[:8]), true)
		m, _ := membro.Novo(a.boardID, id, membro.PapelLeitor)
		if err := vinculos.Salvar(ctx, m); err != nil {
			t.Fatalf("vínculo %d: %v", i, err)
		}
	}

	bruno := a.colega(t, "bruno")
	if _, _, err := a.membros.EntrarPeloDominio(ctx, a.boardID, bruno); !errors.Is(err, membro.ErrQuadroCheio) {
		t.Errorf("pelo domínio: esperado ErrQuadroCheio, veio %v", err)
	}

	carla := a.colega(t, "carla")
	var email string
	if err := pool.QueryRow(ctx, `SELECT email FROM usuarios WHERE id = $1`, carla).Scan(&email); err != nil {
		t.Fatalf("email: %v", err)
	}
	convite, err := a.membros.Convidar(ctx, a.boardID, a.dona, email, membro.PapelLeitor)
	if err != nil {
		t.Fatalf("convidar: %v", err)
	}
	if _, _, err := a.membros.Aceitar(ctx, convite.Token, carla); !errors.Is(err, membro.ErrQuadroCheio) {
		t.Errorf("por convite: esperado ErrQuadroCheio, veio %v", err)
	}
	todos, _ := vinculos.Todos(ctx, a.boardID)
	if len(todos) != membro.MaximoPorQuadro {
		t.Errorf("o quadro passou do teto: %d participantes", len(todos))
	}
}

func TestDM7DuasEntradasSimultaneasDaMesmaPessoaViramUma(t *testing.T) {
	a := novoAmbienteDeDominio(t)
	a.abrir(t, membro.PapelEditor)
	bruno := a.colega(t, "bruno")

	var grupo sync.WaitGroup
	erros := make([]error, 4)
	for i := range erros {
		grupo.Add(1)
		go func(i int) {
			defer grupo.Done()
			_, _, erros[i] = a.membros.EntrarPeloDominio(context.Background(), a.boardID, bruno)
		}(i)
	}
	grupo.Wait()

	for i, err := range erros {
		if err != nil {
			t.Errorf("entrada %d: %v", i, err)
		}
	}
	if n := eventosDoTipo(t, a.boardID, "membro.entrou"); n != 1 {
		t.Errorf("esperado 1 evento membro.entrou, veio %d", n)
	}
	if a.papelDe(t, bruno) != membro.PapelEditor {
		t.Error("a pessoa não ficou no quadro")
	}
}

func TestDM8AContaNoTetoDeQuadrosNaoCriaMaisUm(t *testing.T) {
	a := novoAmbienteDeDominio(t)
	ctx := context.Background()
	// A dona já tem o quadro do ambiente.
	for i := 1; i < dboard.MaximoDeQuadrosPorConta; i++ {
		if _, err := a.quadros.Criar(ctx, a.dona, fmt.Sprintf("Quadro %d", i)); err != nil {
			t.Fatalf("quadro %d: %v", i, err)
		}
	}
	if _, err := a.quadros.Criar(ctx, a.dona, "Um a mais"); !errors.Is(err, dboard.ErrQuadrosDemais) {
		t.Fatalf("esperado ErrQuadrosDemais, veio %v", err)
	}
	// Participar de quadros alheios não conta: outra pessoa continua criando.
	outra := a.colega(t, "bruno")
	if _, err := a.quadros.Criar(ctx, outra, "O primeiro dela"); err != nil {
		t.Errorf("o teto de uma conta valeu para outra: %v", err)
	}
}
