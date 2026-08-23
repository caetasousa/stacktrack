// Os tetos de ocupação do tempo real: por conta, global e de handshake.
package realtime_test

import (
	"sync"
	"testing"

	"stacktrack/internal/adapter/realtime/hub"
)

// O teto por CONTA existe porque o teto global sozinho é um recurso que a
// primeira conta a chegar consome inteiro: bastaria uma pessoa abrir cem abas
// para o quadro de todo mundo parar de atualizar.
func TestTetoPorContaRecusaAConexaoExcedente(t *testing.T) {
	l := hub.NovosLimites(2, 0)

	if _, motivo := l.Reservar("ana"); motivo != "" {
		t.Fatalf("primeira conexão recusada: %s", motivo)
	}
	if _, motivo := l.Reservar("ana"); motivo != "" {
		t.Fatalf("segunda conexão recusada: %s", motivo)
	}
	if _, motivo := l.Reservar("ana"); motivo != hub.MotivoContaCheia {
		t.Errorf("motivo = %q, esperado conta cheia", motivo)
	}

	// E OUTRA conta não é afetada: o teto é por conta, não do servidor.
	if _, motivo := l.Reservar("bob"); motivo != "" {
		t.Errorf("bob foi recusado pelo teto da ana: %s", motivo)
	}
}

func TestTetoGlobalRecusaQualquerConta(t *testing.T) {
	l := hub.NovosLimites(0, 2)

	l.Reservar("ana")
	l.Reservar("bob")
	if _, motivo := l.Reservar("carla"); motivo != hub.MotivoServidorCheio {
		t.Errorf("motivo = %q, esperado servidor cheio", motivo)
	}
}

// Liberar devolve a vaga — senão o teto seria um balde que só enche, e o
// servidor pararia de aceitar tempo real depois de algumas horas de uso normal.
func TestLiberarDevolveAVaga(t *testing.T) {
	l := hub.NovosLimites(1, 0)

	liberar, motivo := l.Reservar("ana")
	if motivo != "" {
		t.Fatalf("recusada: %s", motivo)
	}
	if _, motivo := l.Reservar("ana"); motivo == "" {
		t.Fatal("a segunda devia ter sido recusada")
	}

	liberar()
	if _, motivo := l.Reservar("ana"); motivo != "" {
		t.Errorf("depois de liberar, a conexão devia ser aceita: %s", motivo)
	}
}

// Liberar DUAS VEZES não pode abrir uma vaga que não existe.
//
// Acontece de verdade: a conexão sai por dois caminhos ao mesmo tempo — o defer
// do handler e o fechamento pelo hub — e um contador que anda para trás
// transforma o teto em ficção.
func TestLiberarDuasVezesNaoAbreVagaFantasma(t *testing.T) {
	l := hub.NovosLimites(0, 1)

	liberar, _ := l.Reservar("ana")
	liberar()
	liberar()

	if l.Abertas() != 0 {
		t.Fatalf("abertas = %d, esperado 0", l.Abertas())
	}
	if _, motivo := l.Reservar("ana"); motivo != "" {
		t.Fatalf("recusada: %s", motivo)
	}
	if _, motivo := l.Reservar("bob"); motivo != hub.MotivoServidorCheio {
		t.Errorf("motivo = %q: o contador andou para trás e abriu vaga fantasma", motivo)
	}
}

// Teto zero desliga o limite — é o que os testes e o desenvolvimento querem.
// Produção não pode ficar assim, e config.Validar recusa subir com zero.
func TestTetoZeroDesligaOLimite(t *testing.T) {
	l := hub.NovosLimites(0, 0)
	for i := 0; i < 500; i++ {
		if _, motivo := l.Reservar("ana"); motivo != "" {
			t.Fatalf("conexão %d recusada com teto desligado: %s", i, motivo)
		}
	}
}

// Reservar e liberar em paralelo, sob -race: é o cenário real, com uma
// goroutine por conexão.
func TestReservarELiberarEmParaleloNaoCorrompeOContador(t *testing.T) {
	l := hub.NovosLimites(0, 0)

	var espera sync.WaitGroup
	for i := 0; i < 100; i++ {
		espera.Add(1)
		go func() {
			defer espera.Done()
			liberar, motivo := l.Reservar("ana")
			if motivo == "" {
				liberar()
			}
		}()
	}
	espera.Wait()

	if l.Abertas() != 0 {
		t.Errorf("abertas = %d, esperado 0", l.Abertas())
	}
	if l.AbertasDaConta("ana") != 0 {
		t.Errorf("abertas da ana = %d, esperado 0", l.AbertasDaConta("ana"))
	}
}
