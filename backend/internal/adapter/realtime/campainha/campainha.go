// Package campainha avisa as telas abertas de cada pessoa de que a lista de
// notificações dela mudou.
//
// É o irmão pequeno do hub. O hub tem salas por QUADRO e entrega eventos com
// conteúdo, em ordem, porque a tela do quadro reconcilia por revisão. A
// campainha tem salas por PESSOA e não entrega conteúdo nenhum: o toque só diz
// "olhe de novo", e a lista continua morando no banco. Por isso ela não precisa
// de fila, de ordem nem de reposição — dois toques seguidos valem o mesmo que
// um, e um toque perdido é corrigido pela próxima leitura.
package campainha

import (
	"sync"
)

// Ouvinte é uma tela aberta de uma pessoa.
type Ouvinte struct {
	// Toques é por onde a campainha avisa. Tem buffer de um e o envio não
	// bloqueia: um toque pendente já cobre os que chegarem antes de ele ser
	// lido, porque o que a tela faz com qualquer um deles — reler — é o mesmo.
	// Fechado é o sinal de que a campainha foi desligada.
	Toques    chan struct{}
	usuarioID string
}

// Campainha guarda quem está com uma tela aberta, por pessoa.
type Campainha struct {
	mu       sync.Mutex
	ouvintes map[string]map[*Ouvinte]struct{}
	fechada  bool
}

// Nova cria a campainha vazia.
func Nova() *Campainha {
	return &Campainha{ouvintes: make(map[string]map[*Ouvinte]struct{})}
}

// Assinar registra uma tela aberta da pessoa. Devolve nil se a campainha já foi
// desligada: quem chega durante o desligamento não entra numa sala que ninguém
// mais vai esvaziar.
func (c *Campainha) Assinar(usuarioID string) *Ouvinte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fechada {
		return nil
	}
	o := &Ouvinte{Toques: make(chan struct{}, 1), usuarioID: usuarioID}
	if c.ouvintes[usuarioID] == nil {
		c.ouvintes[usuarioID] = make(map[*Ouvinte]struct{})
	}
	c.ouvintes[usuarioID][o] = struct{}{}
	return o
}

// Cancelar tira a tela da sala. É idempotente: a conexão pode sair pelo fim da
// requisição e pelo desligamento ao mesmo tempo.
func (c *Campainha) Cancelar(o *Ouvinte) {
	if o == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	sala, existe := c.ouvintes[o.usuarioID]
	if !existe {
		return
	}
	if _, inscrito := sala[o]; !inscrito {
		return
	}
	delete(sala, o)
	close(o.Toques)
	// Sala vazia sai do mapa: sem isso ele cresceria para sempre, com uma
	// entrada por pessoa que abriu o sistema uma vez.
	if len(sala) == 0 {
		delete(c.ouvintes, o.usuarioID)
	}
}

// Tocar avisa todas as telas abertas das pessoas informadas, sem bloquear.
//
// Quem chama é o caso de uso, logo depois do commit, no caminho da requisição
// de outra pessoa: esperar aqui por uma tela lenta seria segurar a atribuição
// de alguém por causa da aba de outro. O `select` com `default` é o que garante
// que isso não acontece.
func (c *Campainha) Tocar(usuarioIDs ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, id := range usuarioIDs {
		for o := range c.ouvintes[id] {
			select {
			case o.Toques <- struct{}{}:
			default:
			}
		}
	}
}

// Conexoes informa quantas telas estão ouvindo agora.
func (c *Campainha) Conexoes() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	total := 0
	for _, sala := range c.ouvintes {
		total += len(sala)
	}
	return total
}

// Fechar desliga a campainha: fecha o canal de cada tela, o que encerra as
// conexões abertas, e recusa assinaturas novas.
//
// Chamado no desligamento gracioso, ANTES do Shutdown do servidor HTTP: uma
// resposta de SSE não termina sozinha, e o Shutdown esperaria o prazo inteiro
// por ela.
func (c *Campainha) Fechar() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fechada = true
	for _, sala := range c.ouvintes {
		for o := range sala {
			close(o.Toques)
		}
	}
	c.ouvintes = make(map[string]map[*Ouvinte]struct{})
}
