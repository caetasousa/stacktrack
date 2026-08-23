//go:build integracao

// Os testes de INTERLEAVING de A2: duas transações concorrentes disputando a
// mesma invariante.
//
// O que eles provam não é que o código funciona — os testes em memória já
// fazem isso. É que ele continua correto quando duas requisições chegam ao
// mesmo tempo, que é exatamente o caso em que READ COMMITTED, o isolamento
// padrão do Postgres, NÃO protege: cada transação enxerga um instantâneo
// consistente, e nada garante que as duas façam sentido juntas.
//
// Por isso todos usam o repositório e a UnidadeDeTrabalho de verdade, contra um
// Postgres de verdade. Um fake em memória com mutex passaria em todos eles e
// não provaria nada sobre o banco.
package repository_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"stacktrack/internal/adapter/repository"
	"stacktrack/internal/domain/convite"
	"stacktrack/internal/domain/evento"
	"stacktrack/internal/domain/membro"
	dusuario "stacktrack/internal/domain/usuario"
	ucboard "stacktrack/internal/usecase/board"

	"github.com/google/uuid"
)

// novaUnidade monta a UnidadeDeTrabalho real sobre o pool do pacote.
func novaUnidade(espera time.Duration) *repository.UnidadeDeTrabalho {
	return repository.NovaUnidadeDeTrabalho(pool, espera, 5*time.Second)
}

// contaDeTeste cria um usuário real e devolve o id e o email.
func contaDeTeste(t *testing.T, nome string) (string, string) {
	t.Helper()
	id := uuid.NewString()
	email := id + "@teste.dev"
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO usuarios (id, nome, email, senha_hash, criado_em, atualizado_em)
		 VALUES ($1, $2, $3, 'hash', now(), now())`, id, nome, email); err != nil {
		t.Fatalf("usuário: %v", err)
	}
	return id, email
}

// eventoDe monta um evento mínimo para a unidade de trabalho gravar.
func eventoDe(boardID string) evento.Evento {
	return evento.Novo(evento.MembroRemovido, boardID, "", nil)
}

// emParalelo dispara as duas funções ao mesmo tempo e devolve os dois erros.
//
// A barreira (WaitGroup de largada) é o que faz as duas realmente competirem:
// sem ela, a primeira goroutine costuma terminar antes de a segunda começar, e
// o teste passaria sem nunca ter havido concorrência.
func emParalelo(a, b func() error) (errA, errB error) {
	var largada, fim sync.WaitGroup
	largada.Add(1)
	fim.Add(2)

	go func() {
		defer fim.Done()
		largada.Wait()
		errA = a()
	}()
	go func() {
		defer fim.Done()
		largada.Wait()
		errB = b()
	}()

	largada.Done()
	fim.Wait()
	return errA, errB
}

// ---------------------------------------------------------------------------
// Invariante 1: o quadro nunca fica sem dono
// ---------------------------------------------------------------------------

// Dois donos, cada um removendo o outro, ao mesmo tempo.
//
// Sem o lock do quadro as duas transações leem "há dois donos", as duas
// concluem "posso remover", as duas comitam — e o quadro fica órfão: ninguém
// para convidar, ninguém para apagá-lo. Nenhuma constraint pega isso, porque a
// regra é sobre o CONJUNTO de linhas.
func TestRemocaoConcorrenteNaoDeixaOQuadroSemDono(t *testing.T) {
	ctx := context.Background()
	boardID, _, ana := cenario(t)
	bruno, _ := contaDeTeste(t, "Bruno")

	membros := repository.NovoMembroPostgres(pool)
	segundoDono, _ := membro.Novo(boardID, bruno, membro.PapelDono)
	if err := membros.Salvar(ctx, segundoDono); err != nil {
		t.Fatalf("segundo dono: %v", err)
	}

	u := novaUnidade(3 * time.Second)
	remover := func(alvo string) func() error {
		return func() error {
			_, err := u.Escrever(ctx, eventoDe(boardID), func(e ucboard.Escrita) error {
				todos, err := e.Membros.Todos(ctx, boardID)
				if err != nil {
					return err
				}
				if err := membro.ValidarRemocao(todos, alvo); err != nil {
					return err
				}
				return e.Membros.Remover(ctx, boardID, alvo)
			})
			return err
		}
	}

	erroA, erroB := emParalelo(remover(ana), remover(bruno))

	// Uma das duas TEM de falhar: remover os dois donos é a única combinação
	// que a regra proíbe.
	if erroA == nil && erroB == nil {
		t.Error("as duas remoções passaram — o quadro pode ter ficado sem dono")
	}

	restantes, err := membros.Todos(ctx, boardID)
	if err != nil {
		t.Fatalf("listar membros: %v", err)
	}
	donos := 0
	for _, m := range restantes {
		if m.Papel == membro.PapelDono {
			donos++
		}
	}
	if donos == 0 {
		t.Fatalf("o quadro ficou sem dono; erros = (%v, %v)", erroA, erroB)
	}
}

// O mesmo, pelo caminho da troca de papel: dois donos se rebaixando a leitor.
func TestRebaixamentoConcorrenteNaoDeixaOQuadroSemDono(t *testing.T) {
	ctx := context.Background()
	boardID, _, ana := cenario(t)
	bruno, _ := contaDeTeste(t, "Bruno")

	membros := repository.NovoMembroPostgres(pool)
	segundoDono, _ := membro.Novo(boardID, bruno, membro.PapelDono)
	if err := membros.Salvar(ctx, segundoDono); err != nil {
		t.Fatalf("segundo dono: %v", err)
	}

	u := novaUnidade(3 * time.Second)
	rebaixar := func(alvo string) func() error {
		return func() error {
			_, err := u.Escrever(ctx, eventoDe(boardID), func(e ucboard.Escrita) error {
				todos, err := e.Membros.Todos(ctx, boardID)
				if err != nil {
					return err
				}
				if err := membro.ValidarTrocaDePapel(todos, alvo, membro.PapelLeitor); err != nil {
					return err
				}
				vinculo, err := e.Membros.Buscar(ctx, boardID, alvo)
				if err != nil {
					return err
				}
				if err := vinculo.DefinirPapel(membro.PapelLeitor); err != nil {
					return err
				}
				return e.Membros.Atualizar(ctx, vinculo)
			})
			return err
		}
	}

	emParalelo(rebaixar(ana), rebaixar(bruno))

	restantes, _ := membros.Todos(ctx, boardID)
	donos := 0
	for _, m := range restantes {
		if m.Papel == membro.PapelDono {
			donos++
		}
	}
	if donos == 0 {
		t.Error("os dois donos viraram leitor ao mesmo tempo: o quadro ficou órfão")
	}
}

// ---------------------------------------------------------------------------
// Invariante 2: um convite tem UM resultado terminal
// ---------------------------------------------------------------------------

// conviteDeTeste cria um convite pendente e devolve o agregado.
func conviteDeTeste(t *testing.T, boardID, criadoPor, email string) *convite.Convite {
	t.Helper()
	c, err := convite.Novo(uuid.NewString(), boardID, email, membro.PapelEditor, uuid.NewString(), criadoPor)
	if err != nil {
		t.Fatalf("convite: %v", err)
	}
	if err := repository.NovoConvitePostgres(pool).Salvar(context.Background(), c); err != nil {
		t.Fatalf("salvar convite: %v", err)
	}
	return c
}

// Duas abas clicando no mesmo link ao mesmo tempo.
//
// Sem o UPDATE condicional, as duas leem "pendente", as duas criam o vínculo e
// as duas gravam a aceitação — dois eventos "entrou no quadro" para a mesma
// pessoa, e o convite consumido duas vezes.
func TestAceitarDuasVezesEmParaleloSoValeUma(t *testing.T) {
	ctx := context.Background()
	boardID, _, ana := cenario(t)
	convidado, email := contaDeTeste(t, "Convidado")
	c := conviteDeTeste(t, boardID, ana, email)

	u := novaUnidade(3 * time.Second)
	aceitar := func() error {
		_, err := u.Escrever(ctx, eventoDe(boardID), func(e ucboard.Escrita) error {
			if err := e.Convites.Aceitar(ctx, c.ID, time.Now()); err != nil {
				return err
			}
			existente, err := e.Membros.Buscar(ctx, boardID, convidado)
			if err != nil || existente != nil {
				return err
			}
			vinculo, err := membro.Novo(boardID, convidado, c.Papel)
			if err != nil {
				return err
			}
			return e.Membros.Salvar(ctx, vinculo)
		})
		return err
	}

	erroA, erroB := emParalelo(aceitar, aceitar)

	// Exatamente uma passa, e a outra recebe ErrJaResolvido.
	sucessos := 0
	for _, err := range []error{erroA, erroB} {
		switch {
		case err == nil:
			sucessos++
		case errors.Is(err, convite.ErrJaResolvido):
		default:
			t.Errorf("erro inesperado: %v", err)
		}
	}
	if sucessos != 1 {
		t.Errorf("sucessos = %d, esperado exatamente 1 (erros: %v, %v)", sucessos, erroA, erroB)
	}

	// E o efeito colateral que importa: UM vínculo, não dois.
	todos, _ := repository.NovoMembroPostgres(pool).Todos(ctx, boardID)
	vinculos := 0
	for _, m := range todos {
		if m.UsuarioID == convidado {
			vinculos++
		}
	}
	if vinculos != 1 {
		t.Errorf("vínculos do convidado = %d, esperado 1", vinculos)
	}
}

// Aceitar e revogar ao mesmo tempo: um dos dois vence, e o outro sabe disso.
//
// O caso que dói sem o UPDATE condicional: o dono revoga, a pessoa aceita, e as
// duas escritas passam — a pessoa entra num quadro cujo convite o dono acredita
// ter cancelado.
func TestAceitarERevogarEmParaleloTemUmVencedorSo(t *testing.T) {
	ctx := context.Background()
	boardID, _, ana := cenario(t)
	convidado, email := contaDeTeste(t, "Convidado")
	c := conviteDeTeste(t, boardID, ana, email)

	u := novaUnidade(3 * time.Second)
	aceitar := func() error {
		_, err := u.Escrever(ctx, eventoDe(boardID), func(e ucboard.Escrita) error {
			if err := e.Convites.Aceitar(ctx, c.ID, time.Now()); err != nil {
				return err
			}
			vinculo, err := membro.Novo(boardID, convidado, c.Papel)
			if err != nil {
				return err
			}
			return e.Membros.Salvar(ctx, vinculo)
		})
		return err
	}
	revogar := func() error {
		_, err := u.Escrever(ctx, eventoDe(boardID), func(e ucboard.Escrita) error {
			return e.Convites.Revogar(ctx, c.ID, time.Now())
		})
		return err
	}

	erroA, erroB := emParalelo(aceitar, revogar)
	if (erroA == nil) == (erroB == nil) {
		t.Fatalf("esperava exatamente um vencedor; erros = (%v, %v)", erroA, erroB)
	}

	// O estado final é coerente com quem venceu: se a aceitação passou, existe
	// vínculo e o convite está aceito; se a revogação passou, não existe
	// vínculo nenhum.
	final, err := repository.NovoConvitePostgres(pool).BuscarPorID(ctx, c.ID)
	if err != nil {
		t.Fatalf("buscar convite: %v", err)
	}
	if final.AceitoEm != nil && final.RevogadoEm != nil {
		t.Error("o convite ficou aceito E revogado ao mesmo tempo")
	}

	vinculo, _ := repository.NovoMembroPostgres(pool).Buscar(ctx, boardID, convidado)
	if final.AceitoEm != nil && vinculo == nil {
		t.Error("o convite consta aceito, mas a participação não existe")
	}
	if final.RevogadoEm != nil && vinculo != nil {
		t.Error("o convite foi revogado e a pessoa entrou assim mesmo")
	}
}

// Convidar o mesmo email duas vezes ao mesmo tempo: o índice único parcial
// decide, e o perdedor recebe um erro de DOMÍNIO — não a violação crua.
func TestConvidarOMesmoEmailEmParaleloCriaUmConviteSo(t *testing.T) {
	ctx := context.Background()
	boardID, _, ana := cenario(t)
	_, email := contaDeTeste(t, "Convidado")

	u := novaUnidade(3 * time.Second)
	convidar := func() error {
		_, err := u.Escrever(ctx, eventoDe(boardID), func(e ucboard.Escrita) error {
			pendente, err := e.Convites.BuscarPendentePorEmail(ctx, boardID, email)
			if err != nil {
				return err
			}
			if pendente != nil {
				return convite.ErrJaConvidado
			}
			c, err := convite.Novo(uuid.NewString(), boardID, email, membro.PapelEditor, uuid.NewString(), ana)
			if err != nil {
				return err
			}
			return e.Convites.Salvar(ctx, c)
		})
		return err
	}

	erroA, erroB := emParalelo(convidar, convidar)

	sucessos := 0
	for _, err := range []error{erroA, erroB} {
		switch {
		case err == nil:
			sucessos++
		case errors.Is(err, convite.ErrJaConvidado):
		default:
			t.Errorf("erro inesperado: %v", err)
		}
	}
	if sucessos != 1 {
		t.Errorf("sucessos = %d, esperado 1 (erros: %v, %v)", sucessos, erroA, erroB)
	}

	pendentes, _ := repository.NovoConvitePostgres(pool).ListarPendentes(ctx, boardID)
	iguais := 0
	for _, c := range pendentes {
		if c.Email == dusuario.NormalizarEmail(email) {
			iguais++
		}
	}
	if iguais != 1 {
		t.Errorf("convites pendentes para o mesmo email = %d, esperado 1", iguais)
	}
}

// Criações também precisam calcular a chave depois de adquirir o lock. Só
// colocar o INSERT na UoW não basta: se a chave nasceu antes, todas as
// requisições de uma coluna vazia levam para dentro um dos mesmos cinco
// valores possíveis.
func TestCriarCardsEmParaleloNaoRepeteChave(t *testing.T) {
	ctx := context.Background()
	_, colunaID, ana := cenario(t)
	membros := repository.NovoMembroPostgres(pool)
	colunas := repository.NovoColunaPostgres(pool)
	cards := repository.NovoCardPostgres(pool)
	uc := ucboard.NovoCardUseCase(repository.NovoBoardPostgres(pool), membros, colunas, cards, nil, nil, nil, nil, nil, nil)
	uc.ComEscritaAtomica(novaUnidade(10 * time.Second))

	const quantidade = 24
	var largada, fim sync.WaitGroup
	largada.Add(1)
	fim.Add(quantidade)
	erros := make(chan error, quantidade)
	for i := 0; i < quantidade; i++ {
		go func() {
			defer fim.Done()
			largada.Wait()
			_, err := uc.Criar(ctx, colunaID, ana, "Card concorrente", "", "")
			erros <- err
		}()
	}
	largada.Done()
	fim.Wait()
	close(erros)
	for err := range erros {
		if err != nil {
			t.Fatalf("criar card: %v", err)
		}
	}

	criados, err := cards.ListarDaColuna(ctx, colunaID)
	if err != nil {
		t.Fatalf("listar cards: %v", err)
	}
	vistas := make(map[string]struct{}, len(criados))
	for _, card := range criados {
		if _, repetida := vistas[card.Chave]; repetida {
			t.Fatalf("chave de card repetida: %q", card.Chave)
		}
		vistas[card.Chave] = struct{}{}
	}
}

func TestCriarColunasEmParaleloNaoRepeteChave(t *testing.T) {
	ctx := context.Background()
	boardID, colunaInicial, ana := cenario(t)
	membros := repository.NovoMembroPostgres(pool)
	colunas := repository.NovoColunaPostgres(pool)
	if err := colunas.Apagar(ctx, colunaInicial); err != nil {
		t.Fatalf("limpar coluna inicial: %v", err)
	}
	uc := ucboard.NovoColunaUseCase(membros, colunas, nil, nil)
	uc.ComEscritaAtomica(novaUnidade(10 * time.Second))

	const quantidade = 24
	var largada, fim sync.WaitGroup
	largada.Add(1)
	fim.Add(quantidade)
	erros := make(chan error, quantidade)
	for i := 0; i < quantidade; i++ {
		go func() {
			defer fim.Done()
			largada.Wait()
			_, err := uc.Criar(ctx, boardID, ana, "Coluna concorrente", "")
			erros <- err
		}()
	}
	largada.Done()
	fim.Wait()
	close(erros)
	for err := range erros {
		if err != nil {
			t.Fatalf("criar coluna: %v", err)
		}
	}

	criadas, err := colunas.ListarDoBoard(ctx, boardID)
	if err != nil {
		t.Fatalf("listar colunas: %v", err)
	}
	vistas := make(map[string]struct{}, len(criadas))
	for _, coluna := range criadas {
		if _, repetida := vistas[coluna.Chave]; repetida {
			t.Fatalf("chave de coluna repetida: %q", coluna.Chave)
		}
		vistas[coluna.Chave] = struct{}{}
	}
}

// Convite VENCIDO não pode bloquear um convite novo.
//
// Era o defeito do índice antigo (`WHERE aceito_em IS NULL`): o vencido
// continuava ocupando a vaga, e convidar de novo virava 500. Agora o domínio
// revoga o vencido — na mesma transação — antes de inserir.
func TestConviteVencidoNaoBloqueiaUmNovo(t *testing.T) {
	ctx := context.Background()
	boardID, _, ana := cenario(t)
	_, email := contaDeTeste(t, "Convidado")

	velho := conviteDeTeste(t, boardID, ana, email)
	if _, err := pool.Exec(ctx,
		`UPDATE convites_board SET expira_em = now() - interval '1 day' WHERE id = $1`, velho.ID); err != nil {
		t.Fatalf("vencer o convite: %v", err)
	}

	u := novaUnidade(3 * time.Second)
	agora := time.Now()
	_, err := u.Escrever(ctx, eventoDe(boardID), func(e ucboard.Escrita) error {
		pendente, err := e.Convites.BuscarPendentePorEmail(ctx, boardID, email)
		if err != nil {
			return err
		}
		if pendente != nil && !pendente.Pendente(agora) {
			if err := e.Convites.Revogar(ctx, pendente.ID, agora); err != nil {
				return err
			}
		}
		novo, err := convite.Novo(uuid.NewString(), boardID, email, membro.PapelEditor, uuid.NewString(), ana)
		if err != nil {
			return err
		}
		return e.Convites.Salvar(ctx, novo)
	})
	if err != nil {
		t.Fatalf("convidar de novo depois do vencimento devia funcionar: %v", err)
	}
}

// A invariante "falha do evento desfaz a mutação" é provada em outbox_test.go,
// pelos dois lados (mudança que falha e evento que falha). Não se repete aqui.

// ---------------------------------------------------------------------------
// Invariante 4: a espera pelo lock termina
// ---------------------------------------------------------------------------

// Sem lock_timeout, uma transação travada segurando o quadro faz cada nova
// mutação daquele quadro esperar para SEMPRE — uma conexão do pool por vez, até
// o pool acabar. Aí um quadro travado derruba a API inteira.
//
// O teste segura o lock numa transação própria e mede quanto a segunda espera.
func TestEsperaPeloLockTerminaNoPrazo(t *testing.T) {
	ctx := context.Background()
	boardID, _, _ := cenario(t)

	// Uma transação que trava a linha do quadro e não solta.
	seguradora, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer seguradora.Rollback(ctx) //nolint:errcheck
	if _, err := seguradora.Exec(ctx, `SELECT id FROM boards WHERE id = $1 FOR UPDATE`, boardID); err != nil {
		t.Fatalf("travar: %v", err)
	}

	u := novaUnidade(300 * time.Millisecond)
	inicio := time.Now()
	_, err = u.Escrever(ctx, eventoDe(boardID), func(e ucboard.Escrita) error {
		return nil
	})
	decorrido := time.Since(inicio)

	if !errors.Is(err, ucboard.ErrQuadroOcupado) {
		t.Fatalf("erro = %v, esperado ErrQuadroOcupado", err)
	}
	// Com folga generosa: o que se prova é que TERMINA, não a precisão do
	// relógio num container.
	if decorrido > 5*time.Second {
		t.Errorf("a espera levou %v: o lock_timeout não foi aplicado", decorrido)
	}
}

// Quadros DIFERENTES não se esperam. É o que torna o lock por quadro aceitável:
// ele serializa o agregado, e não a aplicação.
func TestQuadrosDiferentesNaoSeEsperam(t *testing.T) {
	ctx := context.Background()
	boardA, _, _ := cenario(t)
	boardB, _, _ := cenario(t)

	seguradora, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer seguradora.Rollback(ctx) //nolint:errcheck
	if _, err := seguradora.Exec(ctx, `SELECT id FROM boards WHERE id = $1 FOR UPDATE`, boardA); err != nil {
		t.Fatalf("travar: %v", err)
	}

	u := novaUnidade(300 * time.Millisecond)
	if _, err := u.Escrever(ctx, eventoDe(boardB), func(e ucboard.Escrita) error {
		return nil
	}); err != nil {
		t.Errorf("escrever no quadro B enquanto A está travado: %v", err)
	}
}
