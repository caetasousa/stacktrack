// Package limite conta ocorrências por chave dentro de uma janela deslizante.
//
// Além das ocorrências confirmadas, o contador reserva uma vaga enquanto a
// operação protegida está em andamento. Isso é indispensável para limites
// seletivos: consultar o total e incrementa-lo somente depois do Argon2 ou do
// banco abre uma janela em que uma rajada inteira enxerga o mesmo total e passa.
package limite

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// PorChave é um teto de ocorrências por chave arbitrária dentro de uma janela.
// O estado é local ao processo, coerente com a topologia atual de uma réplica.
type PorChave struct {
	mu            sync.Mutex
	limite        int
	janela        time.Duration
	chaves        map[string]*estado
	agora         func() time.Time
	ultimaLimpeza time.Time
}

type estado struct {
	ocorrencias []time.Time
	emAndamento int
}

// Reserva ocupa atomicamente uma vaga antes do trabalho protegido. O chamador
// deve Confirmar quando a ocorrência deve contar ou Cancelar nos demais casos.
// Os dois métodos são idempotentes.
type Reserva struct {
	contador *PorChave
	chave    string
	umaVez   sync.Once
}

// NovoPorChave cria o contador. limite <= 0 devolve nil — um contador
// desligado, aceito por Reservar.
func NovoPorChave(limite int, janela time.Duration) *PorChave {
	if limite <= 0 || janela <= 0 {
		return nil
	}
	return &PorChave{
		limite: limite,
		janela: janela,
		chaves: make(map[string]*estado),
		agora:  time.Now,
	}
}

// Reservar confere e ocupa uma vaga em uma única seção crítica. Contar as
// reservas evita que cem pedidos simultâneos passem por um teto de cinco antes
// que a primeira verificação de senha termine.
func (l *PorChave) Reservar(chave string) (*Reserva, bool) {
	if l == nil {
		return &Reserva{}, true
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	agora := l.agora()
	l.limparChavesVencidas(agora)
	e := l.chaves[chave]
	if e == nil {
		e = &estado{}
		l.chaves[chave] = e
	}
	l.descartarVencidas(e, agora)
	if len(e.ocorrencias)+e.emAndamento >= l.limite {
		return nil, false
	}
	e.emAndamento++
	return &Reserva{contador: l, chave: chave}, true
}

// limparChavesVencidas impede que chaves controladas pelo cliente (emails e
// IPs) permaneçam para sempre no mapa depois que a janela termina. A varredura
// roda no máximo uma vez por minuto para não transformar cada reserva em O(n).
func (l *PorChave) limparChavesVencidas(agora time.Time) {
	intervalo := time.Minute
	if l.janela < intervalo {
		intervalo = l.janela
	}
	if !l.ultimaLimpeza.IsZero() && agora.Sub(l.ultimaLimpeza) < intervalo {
		return
	}
	for chave, e := range l.chaves {
		l.descartarVencidas(e, agora)
		if e.emAndamento == 0 && len(e.ocorrencias) == 0 {
			delete(l.chaves, chave)
		}
	}
	l.ultimaLimpeza = agora
}

// Confirmar transforma a reserva em ocorrência contabilizada e escreve os
// cabeçalhos usuais de rate limit antes de a resposta sair.
func (r *Reserva) Confirmar(w http.ResponseWriter) {
	if r == nil {
		return
	}
	r.umaVez.Do(func() {
		if r.contador == nil {
			return
		}
		l := r.contador
		l.mu.Lock()
		defer l.mu.Unlock()
		agora := l.agora()
		e := l.chaves[r.chave]
		if e == nil {
			return
		}
		l.descartarVencidas(e, agora)
		if e.emAndamento > 0 {
			e.emAndamento--
		}
		e.ocorrencias = append(e.ocorrencias, agora)
		if w != nil {
			restantes := l.limite - len(e.ocorrencias) - e.emAndamento
			if restantes < 0 {
				restantes = 0
			}
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(l.limite))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(restantes))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(agora.Add(l.janela).Unix(), 10))
		}
	})
}

// Cancelar devolve a vaga sem contabilizá-la. É usado em sucesso e em falha
// de infraestrutura: nenhuma das duas é tentativa inválida.
func (r *Reserva) Cancelar() {
	if r == nil {
		return
	}
	r.umaVez.Do(func() {
		if r.contador == nil {
			return
		}
		l := r.contador
		l.mu.Lock()
		defer l.mu.Unlock()
		e := l.chaves[r.chave]
		if e == nil {
			return
		}
		if e.emAndamento > 0 {
			e.emAndamento--
		}
		l.descartarVencidas(e, l.agora())
		if e.emAndamento == 0 && len(e.ocorrencias) == 0 {
			delete(l.chaves, r.chave)
		}
	})
}

func (l *PorChave) descartarVencidas(e *estado, agora time.Time) {
	limiar := agora.Add(-l.janela)
	primeira := 0
	for primeira < len(e.ocorrencias) && !e.ocorrencias[primeira].After(limiar) {
		primeira++
	}
	if primeira > 0 {
		copy(e.ocorrencias, e.ocorrencias[primeira:])
		e.ocorrencias = e.ocorrencias[:len(e.ocorrencias)-primeira]
	}
}

// SegundosDeEspera é o valor do cabeçalho Retry-After: quanto falta, no pior
// caso, para a janela virar.
func (l *PorChave) SegundosDeEspera() int {
	if l == nil {
		return 0
	}
	segundos := int(l.janela.Seconds())
	if segundos < 1 {
		return 1
	}
	return segundos
}
