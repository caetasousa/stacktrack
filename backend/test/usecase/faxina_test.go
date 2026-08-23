// A faxina periódica: lotes, retomada e isolamento entre limpezas.
package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"stacktrack/internal/usecase/manutencao"
)

// faxineiroFalso apaga de um contador em memória, em lotes.
type faxineiroFalso struct {
	mu        sync.Mutex
	nome      string
	restantes int
	chamadas  int
	erro      error
}

type faxineiroAteOPrazo struct {
	nome     string
	chamadas atomic.Int64
}

func (f *faxineiroAteOPrazo) Nome() string { return f.nome }
func (f *faxineiroAteOPrazo) LimparLote(ctx context.Context, _ int) (int64, error) {
	f.chamadas.Add(1)
	<-ctx.Done()
	return 0, ctx.Err()
}

func (f *faxineiroFalso) Nome() string { return f.nome }

func (f *faxineiroFalso) LimparLote(_ context.Context, limite int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chamadas++
	if f.erro != nil {
		return 0, f.erro
	}
	apagadas := limite
	if f.restantes < limite {
		apagadas = f.restantes
	}
	f.restantes -= apagadas
	return int64(apagadas), nil
}

func (f *faxineiroFalso) estado() (restantes, chamadas int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.restantes, f.chamadas
}

// silencioso descarta o log para a saída do teste não virar ruído.
func silencioso() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// rodarUmaPassada dispara a faxina e a encerra depois do primeiro tique.
func rodarUmaPassada(t *testing.T, faxineiros ...manutencao.Faxineiro) {
	t.Helper()
	f := manutencao.Nova(10*time.Millisecond, 5*time.Second, silencioso(), faxineiros...)

	ctx, cancelar := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelar()

	pronto := make(chan struct{})
	go func() {
		f.Rodar(ctx)
		close(pronto)
	}()

	// Espera a passada terminar: a condição é o trabalho ter acabado.
	prazo := time.After(2 * time.Second)
	for {
		select {
		case <-prazo:
			cancelar()
			<-pronto
			return
		case <-time.After(5 * time.Millisecond):
			acabou := true
			for _, fx := range faxineiros {
				if falso, ok := fx.(*faxineiroFalso); ok {
					if restantes, chamadas := falso.estado(); restantes > 0 || chamadas == 0 {
						acabou = false
					}
				}
			}
			if acabou {
				cancelar()
				<-pronto
				return
			}
		}
	}
}

// A limpeza acontece EM LOTES, e não num DELETE só.
//
// Um DELETE que apaga cem mil linhas segura lock e infla o WAL por minutos, com
// o autovacuum ficando para trás. Em lotes, cada transação é curta e o trabalho
// é retomável.
func TestFaxinaApagaEmLotesAteAcabar(t *testing.T) {
	// Dois lotes e meio de trabalho.
	f := &faxineiroFalso{nome: "sessoes", restantes: manutencao.TamanhoDoLote*2 + 7}
	rodarUmaPassada(t, f)

	restantes, chamadas := f.estado()
	if restantes != 0 {
		t.Errorf("restaram %d linhas: a passada parou antes de acabar", restantes)
	}
	// Três lotes: dois cheios e um parcial, que é o que sinaliza o fim.
	if chamadas != 3 {
		t.Errorf("chamadas = %d, esperado 3 lotes", chamadas)
	}
}

// Nada a limpar não vira laço: uma passada, lote vazio, acabou.
func TestFaxinaComNadaALimparFazUmaChamadaSo(t *testing.T) {
	f := &faxineiroFalso{nome: "sessoes", restantes: 0}
	rodarUmaPassada(t, f)

	if _, chamadas := f.estado(); chamadas != 1 {
		t.Errorf("chamadas = %d, esperado 1", chamadas)
	}
}

// O erro de UMA limpeza não impede a outra: são independentes, e parar a
// segunda porque a primeira falhou transformaria um problema em dois.
func TestFalhaDeUmaLimpezaNaoImpedeAOutra(t *testing.T) {
	quebrada := &faxineiroFalso{nome: "sessoes", erro: errors.New("banco fora do ar")}
	boa := &faxineiroFalso{nome: "convites", restantes: 10}

	rodarUmaPassada(t, quebrada, boa)

	if restantes, chamadas := boa.estado(); chamadas == 0 || restantes != 0 {
		t.Errorf("a segunda limpeza não rodou (chamadas=%d, restantes=%d)", chamadas, restantes)
	}
}

// Um faxineiro que usa todo o proprio orcamento nao pode deixar os seguintes
// sem executar. Cada limpeza recebe uma fatia do prazo total da passada.
func TestFaxineiroLentoNaoCausaStarvationNosSeguintes(t *testing.T) {
	lento := &faxineiroAteOPrazo{nome: "sessoes"}
	rapido := &faxineiroFalso{nome: "convites", restantes: 1}
	f := manutencao.Nova(time.Millisecond, 60*time.Millisecond, silencioso(), lento, rapido)
	ctx, cancelar := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancelar()

	pronto := make(chan struct{})
	go func() {
		f.Rodar(ctx)
		close(pronto)
	}()

	limite := time.After(200 * time.Millisecond)
	for {
		if restantes, chamadas := rapido.estado(); chamadas > 0 && restantes == 0 {
			cancelar()
			<-pronto
			return
		}
		select {
		case <-limite:
			t.Fatal("o segundo faxineiro nao recebeu tempo depois do primeiro")
		case <-time.After(2 * time.Millisecond):
		}
	}
}

// Uma tabela muito atrasada NÃO faz a passada rodar sem fim: o teto de lotes a
// interrompe, e o resto fica para a próxima.
func TestPassadaMuitoLongaEhInterrompidaPeloTetoDeLotes(t *testing.T) {
	// Trabalho para muito mais lotes do que o teto permite numa passada.
	f := &faxineiroFalso{
		nome:      "sessoes",
		restantes: manutencao.TamanhoDoLote * (manutencao.MaxLotesPorPassada + 20),
	}

	fx := manutencao.Nova(10*time.Millisecond, 5*time.Second, silencioso(), f)
	ctx, cancelar := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancelar()
	fx.Rodar(ctx)

	_, chamadas := f.estado()
	if chamadas == 0 {
		t.Fatal("a faxina não rodou")
	}
	// Nenhuma passada pode ter feito mais lotes que o teto.
	if chamadas%manutencao.MaxLotesPorPassada != 0 && chamadas > manutencao.MaxLotesPorPassada {
		if chamadas/manutencao.MaxLotesPorPassada == 0 {
			t.Errorf("chamadas = %d: o teto de %d lotes por passada não foi respeitado",
				chamadas, manutencao.MaxLotesPorPassada)
		}
	}
}
