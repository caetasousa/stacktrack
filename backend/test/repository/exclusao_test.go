//go:build integracao

// O outbox das exclusões de arquivo contra um PostgreSQL de verdade.
//
// O que só o banco responde: que a linha do outbox SOBREVIVE ao `ON DELETE
// CASCADE` que apaga o quadro. É a razão de `board_id` não ter chave
// estrangeira — uma FK levaria junto exatamente o registro que existe para
// sobreviver a ele, e o arquivo ficaria no disco sem ninguém sabendo por quê.
package repository_test

import (
	"context"
	"testing"
	"time"

	"stacktrack/internal/adapter/repository"
	ucboard "stacktrack/internal/usecase/board"
)

func TestOutboxDeExclusaoSobreviveAoCascadeDoQuadro(t *testing.T) {
	ctx := context.Background()
	boardID, _, _ := cenario(t)
	outbox := repository.NovoExclusaoDeArquivoPostgres(pool)

	if err := outbox.Registrar(ctx, boardID, []string{"a.bin", "b.bin"}, time.Now()); err != nil {
		t.Fatalf("registrar: %v", err)
	}

	// O quadro inteiro vai embora, com tudo que tem FK para ele.
	if _, err := pool.Exec(ctx, `DELETE FROM boards WHERE id = $1`, boardID); err != nil {
		t.Fatalf("apagar quadro: %v", err)
	}

	semProva, err := outbox.SemCobertura(ctx, 100)
	if err != nil {
		t.Fatalf("consultar: %v", err)
	}
	var doQuadro int
	for _, e := range semProva {
		if e.BoardID == boardID {
			doQuadro++
		}
	}
	if doQuadro != 2 {
		t.Errorf("exclusões sobreviventes = %d, esperado 2: o CASCADE levou o outbox junto", doQuadro)
	}
}

// Pendentes só devolve o que está COBERTO. É onde o fail-closed mora: sem
// prova, a linha nem aparece para o worker.
func TestPendentesSoDevolveOQueEstaCoberto(t *testing.T) {
	ctx := context.Background()
	boardID, _, _ := cenario(t)
	outbox := repository.NovoExclusaoDeArquivoPostgres(pool)

	if err := outbox.Registrar(ctx, boardID, []string{"x.bin", "y.bin"}, time.Now()); err != nil {
		t.Fatalf("registrar: %v", err)
	}

	pendentes, err := outbox.Pendentes(ctx, time.Now(), 100)
	if err != nil {
		t.Fatalf("pendentes: %v", err)
	}
	for _, p := range pendentes {
		if p.BoardID == boardID {
			t.Fatalf("uma exclusão sem cobertura apareceu como pendente: %+v", p)
		}
	}

	// Cobrindo UMA delas, ela — e só ela — aparece.
	semProva, err := outbox.SemCobertura(ctx, 100)
	if err != nil {
		t.Fatalf("sem cobertura: %v", err)
	}
	var alvo ucboard.ExclusaoDeArquivo
	for _, e := range semProva {
		if e.BoardID == boardID {
			alvo = e
			break
		}
	}
	if alvo.ID == 0 {
		t.Fatal("a exclusão registrada não apareceu como sem cobertura")
	}
	if err := outbox.MarcarCobertos(ctx, []int64{alvo.ID}, time.Now()); err != nil {
		t.Fatalf("marcar coberto: %v", err)
	}

	pendentes, err = outbox.Pendentes(ctx, time.Now(), 100)
	if err != nil {
		t.Fatalf("pendentes: %v", err)
	}
	var doQuadro int
	for _, p := range pendentes {
		if p.BoardID == boardID {
			doQuadro++
		}
	}
	if doQuadro != 1 {
		t.Errorf("pendentes do quadro = %d, esperado 1", doQuadro)
	}
}

// Marcar removido tira a linha da fila do worker, sem apagá-la: o histórico de
// "este arquivo saiu do disco em tal dia" é o que responde por ele depois.
func TestMarcarRemovidoTiraDaFilaSemApagarOHistorico(t *testing.T) {
	ctx := context.Background()
	boardID, _, _ := cenario(t)
	outbox := repository.NovoExclusaoDeArquivoPostgres(pool)

	if err := outbox.Registrar(ctx, boardID, []string{"z.bin"}, time.Now()); err != nil {
		t.Fatalf("registrar: %v", err)
	}
	semProva, _ := outbox.SemCobertura(ctx, 100)
	var id int64
	for _, e := range semProva {
		if e.BoardID == boardID {
			id = e.ID
		}
	}
	if err := outbox.MarcarCobertos(ctx, []int64{id}, time.Now()); err != nil {
		t.Fatalf("cobrir: %v", err)
	}
	if err := outbox.MarcarRemovido(ctx, id, time.Now()); err != nil {
		t.Fatalf("marcar removido: %v", err)
	}

	pendentes, _ := outbox.Pendentes(ctx, time.Now(), 100)
	for _, p := range pendentes {
		if p.ID == id {
			t.Error("uma exclusão já removida continua na fila do worker")
		}
	}

	var existe bool
	if err := pool.QueryRow(ctx,
		`SELECT removido_em IS NOT NULL FROM arquivo_exclusoes WHERE id = $1`, id).Scan(&existe); err != nil {
		t.Fatalf("consultar: %v", err)
	}
	if !existe {
		t.Error("a linha do histórico sumiu")
	}
}

// AdiarComErro conta a tentativa e CORTA a mensagem: erro de filesystem carrega
// caminho absoluto e não tem compromisso com o tamanho da coluna.
func TestAdiarComErroContaAtentativaECortaAMensagem(t *testing.T) {
	ctx := context.Background()
	boardID, _, _ := cenario(t)
	outbox := repository.NovoExclusaoDeArquivoPostgres(pool)

	if err := outbox.Registrar(ctx, boardID, []string{"w.bin"}, time.Now()); err != nil {
		t.Fatalf("registrar: %v", err)
	}
	semProva, _ := outbox.SemCobertura(ctx, 100)
	var id int64
	for _, e := range semProva {
		if e.BoardID == boardID {
			id = e.ID
		}
	}

	// Mensagem muito maior que a coluna (VARCHAR(500)).
	enorme := make([]byte, 2000)
	for i := range enorme {
		enorme[i] = 'e'
	}
	if err := outbox.AdiarComErro(ctx, id, string(enorme), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("adiar: %v", err)
	}

	var tentativas int
	var tamanho int
	if err := pool.QueryRow(ctx,
		`SELECT tentativas, length(ultimo_erro) FROM arquivo_exclusoes WHERE id = $1`, id,
	).Scan(&tentativas, &tamanho); err != nil {
		t.Fatalf("consultar: %v", err)
	}
	if tentativas != 1 {
		t.Errorf("tentativas = %d, esperado 1", tentativas)
	}
	if tamanho > 500 {
		t.Errorf("o erro foi gravado com %d caracteres: a coluna comporta 500", tamanho)
	}
}
