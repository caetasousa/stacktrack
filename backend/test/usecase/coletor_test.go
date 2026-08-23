// O coletor de arquivos excluídos, e o fail-closed que ele garante.
//
// A regra que estes testes trancam: NENHUM byte sai do disco sem prova de que a
// exclusão está num backup externo. Enquanto A6 não existir, a porta de
// cobertura é CoberturaNegada e o coletor não remove nada — acumula e relata.
package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	ucboard "stacktrack/internal/usecase/board"
	"stacktrack/internal/usecase/manutencao"
	"stacktrack/test/repository/memoria"
)

// armazemQueConta apaga e registra o que foi apagado.
type armazemQueConta struct {
	removidos []string
	erro      error
}

func (a *armazemQueConta) Remover(caminho string) error {
	if a.erro != nil {
		return a.erro
	}
	a.removidos = append(a.removidos, caminho)
	return nil
}

// coberturaFixa cobre exatamente os ids informados — é o que A6 vai fazer com
// os manifests dos backups.
type coberturaFixa struct {
	cobre map[int64]bool
	erro  error
}

func (c coberturaFixa) Cobertos(_ context.Context, ids []int64) ([]int64, error) {
	if c.erro != nil {
		return nil, c.erro
	}
	var fora []int64
	for _, id := range ids {
		if c.cobre[id] {
			fora = append(fora, id)
		}
	}
	return fora, nil
}

// comExclusoes monta o outbox com N exclusões registradas.
func comExclusoes(t *testing.T, caminhos ...string) *memoria.Exclusoes {
	t.Helper()
	outbox := memoria.NovasExclusoes()
	if err := outbox.Registrar(context.Background(), "quadro-1", caminhos, time.Now()); err != nil {
		t.Fatalf("registrar: %v", err)
	}
	return outbox
}

// O FAIL-CLOSED, que é a razão de tudo isto existir.
//
// Com CoberturaNegada — o estado de produção até A6 —, o coletor não pode
// remover nem um arquivo, por mais antiga que seja a exclusão.
func TestSemCoberturaOColetorNaoRemoveNada(t *testing.T) {
	outbox := comExclusoes(t, "a.bin", "b.bin", "c.bin")
	armazem := &armazemQueConta{}

	coletor := manutencao.NovoColetor(outbox, ucboard.CoberturaNegada{}, armazem, silencioso())
	removidos, err := coletor.LimparLote(context.Background(), 100)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}

	if removidos != 0 {
		t.Errorf("removeu %d arquivos sem cobertura de backup", removidos)
	}
	if len(armazem.removidos) != 0 {
		t.Errorf("o armazém foi tocado: %v", armazem.removidos)
	}
	// E as exclusões continuam lá, esperando prova.
	if outbox.Removidas() != 0 {
		t.Error("uma exclusão foi dada por removida sem cobertura")
	}
}

// Com cobertura, e SÓ dos ids cobertos, os bytes saem.
//
// O teste cobre um de três de propósito: é o que prova que a decisão é por ID
// EXATO, e não "tudo o que é mais antigo que X". Timestamp e `max(id)` não
// servem de prova — o dump do banco e a cópia dos arquivos acontecem em
// instantes diferentes.
func TestRemoveApenasOsIdsComprovadamenteCobertos(t *testing.T) {
	outbox := comExclusoes(t, "a.bin", "b.bin", "c.bin")
	armazem := &armazemQueConta{}
	// Só o do meio.
	cobertura := coberturaFixa{cobre: map[int64]bool{2: true}}

	coletor := manutencao.NovoColetor(outbox, cobertura, armazem, silencioso())
	removidos, err := coletor.LimparLote(context.Background(), 100)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}

	if removidos != 1 {
		t.Fatalf("removidos = %d, esperado 1", removidos)
	}
	if len(armazem.removidos) != 1 || armazem.removidos[0] != "b.bin" {
		t.Errorf("removidos = %v, esperado apenas b.bin", armazem.removidos)
	}
}

// Falha ao CONSULTAR a cobertura não pode virar remoção: sem resposta, a
// resposta é não.
func TestFalhaAoConsultarACoberturaNaoRemoveNada(t *testing.T) {
	outbox := comExclusoes(t, "a.bin")
	armazem := &armazemQueConta{}
	cobertura := coberturaFixa{erro: errors.New("manifest indisponível")}

	coletor := manutencao.NovoColetor(outbox, cobertura, armazem, silencioso())
	if _, err := coletor.LimparLote(context.Background(), 100); err == nil {
		t.Error("a falha da porta de cobertura devia propagar")
	}
	if len(armazem.removidos) != 0 {
		t.Errorf("removeu apesar de não saber se estava coberto: %v", armazem.removidos)
	}
}

// Falha ao REMOVER adia com o erro registrado, e não marca como removido: um
// arquivo que continua no disco não pode sair do outbox.
func TestFalhaAoRemoverAdiaEmVezDeMarcar(t *testing.T) {
	outbox := comExclusoes(t, "a.bin")
	armazem := &armazemQueConta{erro: errors.New("volume somente leitura")}
	cobertura := coberturaFixa{cobre: map[int64]bool{1: true}}

	coletor := manutencao.NovoColetor(outbox, cobertura, armazem, silencioso())
	removidos, err := coletor.LimparLote(context.Background(), 100)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}

	if removidos != 0 {
		t.Errorf("removidos = %d, esperado 0", removidos)
	}
	if outbox.Removidas() != 0 {
		t.Error("uma exclusão que falhou foi marcada como removida")
	}
	if outbox.ErroDe(1) == "" {
		t.Error("a falha não foi registrada para quem for investigar")
	}
}

// Rodar duas vezes não remove duas vezes: o que já saiu do disco sai do outbox.
func TestColetorNaoRemoveDuasVezes(t *testing.T) {
	outbox := comExclusoes(t, "a.bin")
	armazem := &armazemQueConta{}
	cobertura := coberturaFixa{cobre: map[int64]bool{1: true}}

	coletor := manutencao.NovoColetor(outbox, cobertura, armazem, silencioso())
	if _, err := coletor.LimparLote(context.Background(), 100); err != nil {
		t.Fatalf("primeira passada: %v", err)
	}
	if _, err := coletor.LimparLote(context.Background(), 100); err != nil {
		t.Fatalf("segunda passada: %v", err)
	}

	if len(armazem.removidos) != 1 {
		t.Errorf("removeu %d vezes o mesmo arquivo: %v", len(armazem.removidos), armazem.removidos)
	}
}

// Sem nada no outbox, o coletor não faz nada — e não é erro.
func TestColetorComOutboxVazioNaoFazNada(t *testing.T) {
	outbox := memoria.NovasExclusoes()
	armazem := &armazemQueConta{}

	coletor := manutencao.NovoColetor(outbox, ucboard.CoberturaNegada{}, armazem, silencioso())
	removidos, err := coletor.LimparLote(context.Background(), 100)
	if err != nil || removidos != 0 {
		t.Errorf("removidos = %d, erro = %v", removidos, err)
	}
}
