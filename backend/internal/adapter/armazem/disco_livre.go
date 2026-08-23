package armazem

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
)

// EspacoEmDisco descreve quanto sobra no volume dos anexos.
type EspacoEmDisco struct {
	LivreBytes  uint64
	TotalBytes  uint64
	LivrePorCem float64
}

// Guarda é o porteiro do disco: responde se ainda dá para aceitar escrita.
//
// O que ele evita: disco cheio não degrada, ele QUEBRA. O upload falha no meio,
// o Postgres para de aceitar escrita (o WAL não tem para onde ir), o log de
// acesso para de ser gravado — e nada disso avisa antes. O porteiro transforma
// "vai quebrar daqui a pouco" em "recuso escrita agora, e a leitura continua".
//
// Recusar ESCRITA e manter LEITURA é a decisão central. Um quadro que não
// aceita card novo mas mostra o que já existe é um sistema degradado; um que
// não abre é um sistema fora do ar. A diferença importa justamente na hora em
// que alguém precisa consultar o que está lá para decidir o que apagar.
type Guarda struct {
	caminho string
	// minimoBytes e minimoPorCem são os dois pisos, e vale o MAIS
	// CONSERVADOR dos dois. Só o percentual falharia num volume de 2 TB (1%
	// ainda são 20 GB, mas 1% de um volume de 10 GB são 100 MB); só o absoluto
	// falharia num volume pequeno, onde 2 GB podem ser metade do disco.
	minimoBytes  uint64
	minimoPorCem float64

	mu       sync.RWMutex
	ultima   EspacoEmDisco
	erro     error
	medidoEm time.Time
	// validade evita medir o disco a cada requisição: statfs é barato, mas não
	// de graça, e o espaço livre não muda de forma relevante entre duas
	// requisições consecutivas.
	validade time.Duration
}

// NovaGuarda cria o porteiro sobre o diretório informado.
func NovaGuarda(caminho string, minimoBytes uint64, minimoPorCem float64, validade time.Duration) *Guarda {
	if validade <= 0 {
		validade = 10 * time.Second
	}
	return &Guarda{
		caminho:      caminho,
		minimoBytes:  minimoBytes,
		minimoPorCem: minimoPorCem,
		validade:     validade,
	}
}

// Medir devolve o espaço livre, usando a última medição enquanto ela for
// recente.
func (g *Guarda) Medir(ctx context.Context) (EspacoEmDisco, error) {
	g.mu.RLock()
	if time.Since(g.medidoEm) < g.validade {
		espaco, err := g.ultima, g.erro
		g.mu.RUnlock()
		return espaco, err
	}
	g.mu.RUnlock()

	uso, err := disk.UsageWithContext(ctx, g.caminho)

	g.mu.Lock()
	defer g.mu.Unlock()
	g.medidoEm = time.Now()
	if err != nil {
		g.erro = err
		return g.ultima, err
	}
	g.erro = nil
	g.ultima = EspacoEmDisco{
		LivreBytes: uso.Free,
		TotalBytes: uso.Total,
		// 100 - UsedPercent, e não Free/Total: num filesystem ext4 há blocos
		// reservados para o root, e os dois cálculos divergem em alguns por
		// cento. UsedPercent é o que o `df` mostra, que é o número com que
		// alguém vai comparar ao investigar.
		LivrePorCem: 100 - uso.UsedPercent,
	}
	return g.ultima, nil
}

// PodeEscrever informa se ainda há margem para aceitar escrita.
//
// Erro ao MEDIR não bloqueia: um statfs que falha é um problema de observação,
// não de espaço, e transformar "não consegui olhar" em "recuso tudo" derrubaria
// o produto por causa do instrumento. O erro volta junto para quem quiser
// registrá-lo.
func (g *Guarda) PodeEscrever(ctx context.Context) (bool, EspacoEmDisco, error) {
	espaco, err := g.Medir(ctx)
	if err != nil {
		return true, espaco, err
	}
	if g.minimoBytes > 0 && espaco.LivreBytes < g.minimoBytes {
		return false, espaco, nil
	}
	if g.minimoPorCem > 0 && espaco.LivrePorCem < g.minimoPorCem {
		return false, espaco, nil
	}
	return true, espaco, nil
}

// ErrSemEspacoEmDisco é o erro que a borda traduz em 507.
var ErrSemEspacoEmDisco = fmt.Errorf("sem espaço em disco para aceitar novos dados")
