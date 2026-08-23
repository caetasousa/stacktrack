package hub

import (
	"sync"
)

// Limites é o teto de conexões simultâneas do processo e de cada conta.
//
// Por que os DOIS, e não só o global: o teto global sozinho é um recurso que a
// primeira conta a chegar consome inteiro. Bastaria uma pessoa abrir cem abas —
// ou um script com a sessão dela — para o quadro de todo mundo parar de
// atualizar, sem nada parecendo errado do lado do servidor. O teto por conta é
// o que impede um cliente de monopolizar; o global é o que protege a memória do
// processo.
//
// Não é rate limit: rate limit conta requisições por janela, e uma conexão de
// tempo real é UMA requisição que dura horas. O que se conta aqui é ocupação
// simultânea.
type Limites struct {
	mu sync.Mutex
	// porConta é quantas conexões cada conta tem abertas agora.
	porConta map[string]int
	// abertas é o total no processo.
	abertas int

	tetoPorConta int
	tetoGlobal   int
}

// NovosLimites cria o contador. Teto não positivo desliga aquele limite —
// desligar é o que os testes e o ambiente de desenvolvimento querem, e não um
// estado que produção deva alcançar (config.Validar cobre isso).
func NovosLimites(porConta, global int) *Limites {
	return &Limites{
		porConta:     make(map[string]int),
		tetoPorConta: porConta,
		tetoGlobal:   global,
	}
}

// Motivo diz por que uma conexão foi recusada. String vazia significa aceita.
type Motivo string

const (
	// MotivoContaCheia é a conta que já tem conexões demais abertas.
	MotivoContaCheia Motivo = "muitas conexões desta conta"
	// MotivoServidorCheio é o teto global do processo.
	MotivoServidorCheio Motivo = "servidor sem capacidade de tempo real agora"
)

// Reservar tenta ocupar uma vaga. Devolve a função de liberação e o motivo da
// recusa.
//
// A liberação é devolvida em vez de haver um `Liberar(usuarioID)` público
// porque assim é impossível liberar sem ter reservado: o chamador só tem a
// função quando a reserva deu certo, e um `defer` a devolve em qualquer saída,
// inclusive no panic. Um par reservar/liberar solto sempre acaba com um caminho
// de erro que esquece o segundo.
func (l *Limites) Reservar(usuarioID string) (func(), Motivo) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.tetoGlobal > 0 && l.abertas >= l.tetoGlobal {
		return nil, MotivoServidorCheio
	}
	if l.tetoPorConta > 0 && l.porConta[usuarioID] >= l.tetoPorConta {
		return nil, MotivoContaCheia
	}

	l.abertas++
	l.porConta[usuarioID]++

	var umaVez sync.Once
	return func() {
		// Once porque a conexão pode sair por dois caminhos ao mesmo tempo (o
		// defer do handler e o fechamento pelo hub), e decrementar duas vezes
		// faria o contador andar para trás — abrindo vagas que não existem.
		umaVez.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.abertas--
			if l.porConta[usuarioID]--; l.porConta[usuarioID] <= 0 {
				// Sem isto o mapa cresceria para sempre, guardando uma entrada
				// por conta que já conectou uma vez.
				delete(l.porConta, usuarioID)
			}
		})
	}, ""
}

// Abertas informa quantas conexões existem agora no processo. É o número que a
// métrica de A8 vai publicar.
func (l *Limites) Abertas() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.abertas
}

// AbertasDaConta informa quantas conexões uma conta tem agora.
func (l *Limites) AbertasDaConta(usuarioID string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.porConta[usuarioID]
}
