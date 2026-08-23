package limite_test

import (
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"stacktrack/internal/pkg/limite"
)

// A reserva precisa fazer parte da decisao atomica. Sem isso, todas as
// goroutines consultam zero antes que qualquer uma registre a falha.
func TestRajadaConcorrenteNaoUltrapassaOTeto(t *testing.T) {
	const teto = 5
	contador := limite.NovoPorChave(teto, time.Minute)
	inicio := make(chan struct{})
	liberar := make(chan struct{})
	var tentaram sync.WaitGroup
	var terminaram sync.WaitGroup
	var permitidas atomic.Int64

	for i := 0; i < 100; i++ {
		tentaram.Add(1)
		terminaram.Add(1)
		go func() {
			defer terminaram.Done()
			<-inicio
			reserva, ok := contador.Reservar("mesma-chave")
			tentaram.Done()
			if !ok {
				return
			}
			permitidas.Add(1)
			<-liberar
			reserva.Cancelar()
		}()
	}
	close(inicio)
	tentaram.Wait()
	if got := permitidas.Load(); got != teto {
		t.Fatalf("permitidas = %d, esperado exatamente %d", got, teto)
	}
	close(liberar)
	terminaram.Wait()
}

func TestSomenteReservaConfirmadaConsomeCota(t *testing.T) {
	contador := limite.NovoPorChave(2, time.Minute)

	// Sucessos e falhas de infraestrutura cancelam a reserva.
	for i := 0; i < 20; i++ {
		reserva, ok := contador.Reservar("conta")
		if !ok {
			t.Fatal("reserva cancelada consumiu cota")
		}
		reserva.Cancelar()
	}

	for i := 0; i < 2; i++ {
		reserva, ok := contador.Reservar("conta")
		if !ok {
			t.Fatalf("falha %d foi bloqueada cedo demais", i)
		}
		reserva.Confirmar(httptest.NewRecorder())
	}
	if _, ok := contador.Reservar("conta"); ok {
		t.Error("terceira falha passou por um teto de duas")
	}
}
