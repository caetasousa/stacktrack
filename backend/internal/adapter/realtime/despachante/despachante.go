// Package despachante entrega os eventos LENDO O LOG, e não a chamada em
// processo que os produziu.
//
// # O que ele conserta
//
// A publicação era direta: a mesma goroutine que comitava chamava o hub logo
// depois. Isso tem dois furos, e os dois são invisíveis de dentro.
//
// ORDEM. Duas mutações que comitam nas revisões 5 e 6 podem chamar o hub na
// ordem inversa — cada uma corre na sua própria goroutine, e nada as coordena
// depois do commit. O cliente não fica incorreto (ele reconcilia por snapshot),
// mas a ordem de entrega ao vivo não era garantida, e "às vezes chega trocado"
// é o tipo de comportamento que ninguém consegue reproduzir quando vira relato.
//
// PERDA. Se o processo morre entre o commit e a chamada ao hub, aquele evento
// não é entregue a ninguém. A recuperação existia — o replay por revisão na
// reconexão —, mas dependia de o cliente reconectar. E se a entrega falhasse
// com o processo VIVO (hub fechando, canal cheio), nada tentava de novo.
//
// # Como
//
// O banco passa a ser a fonte da verdade da entrega. Quem termina uma mutação
// apenas ACORDA o despachante para aquele quadro; ele lê `board_events` a
// partir do cursor daquele quadro, entrega em ordem de (revisão, índice), e só
// então avança o cursor. Um wake-up perdido é corrigido pelo polling curto: o
// pior caso deixa de ser "o evento sumiu" e passa a ser "o evento chegou um
// segundo depois".
package despachante

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"stacktrack/internal/domain/evento"
)

// Log é de onde os eventos são lidos.
type Log interface {
	DesdeRevisao(ctx context.Context, boardID string, revisao int64, limite int) ([]evento.Evento, error)
	RevisaoAtual(ctx context.Context, boardID string) (int64, error)
}

// Entrega é para onde os eventos vão — o hub.
type Entrega interface {
	Publicar(evento.Evento)
}

const (
	// porLeitura é quantos eventos são lidos de cada vez.
	//
	// Limitado porque um quadro muito ativo não pode fazer uma passada montar
	// milhares de eventos em memória. O que sobra fica para a passada seguinte,
	// que acontece no mesmo instante — o laço continua enquanto vier lote cheio.
	porLeitura = 200
	// filaDeAcordar é quantos quadros podem estar na fila de trabalho.
	//
	// Com buffer, e o envio é não bloqueante: acordar o despachante NUNCA pode
	// segurar quem acabou de comitar. Fila cheia significa que o polling vai
	// pegar aquele quadro em seguida — perder o aviso é aceitável, perder o
	// evento não.
	filaDeAcordar = 256
)

// Despachante lê o log e entrega os eventos em ordem, por quadro.
//
// Implementa a porta Publicador do usecase: quem comita chama `Publicar`, e o
// despachante IGNORA o evento recebido, usando-o apenas como aviso de que
// aquele quadro mudou. O que ele entrega é o que está gravado — que é a
// definição de "não perder publicação".
type Despachante struct {
	log      Log
	entrega  Entrega
	registro *slog.Logger

	// intervalo é o polling que corrige wake-up perdido.
	intervalo time.Duration

	acordar chan string

	mu sync.Mutex
	// cursor é a última revisão ENTREGUE de cada quadro.
	//
	// Só existe para quadro com alguém olhando: sem assinante não há a quem
	// entregar, e manter cursor de todo quadro que já teve uma conexão faria o
	// mapa crescer para sempre.
	cursor map[string]int64
	// atraso é quantas revisões faltam entregar no último passe de cada quadro.
	atraso map[string]int64
	// ultimoSucesso é quando a última entrega deu certo.
	ultimoSucesso time.Time
}

// Novo cria o despachante. intervalo não positivo cai num padrão curto.
func Novo(log Log, entrega Entrega, intervalo time.Duration, registro *slog.Logger) *Despachante {
	if intervalo <= 0 {
		intervalo = time.Second
	}
	return &Despachante{
		log: log, entrega: entrega, registro: registro, intervalo: intervalo,
		acordar: make(chan string, filaDeAcordar),
		cursor:  make(map[string]int64),
		atraso:  make(map[string]int64),
	}
}

// Observar começa a acompanhar um quadro, a partir da revisão ATUAL.
//
// Chamado quando a primeira conexão daquele quadro é aceita. A partir da
// revisão atual, e não do zero: a história anterior é assunto do replay do
// handshake, que entrega ao cliente exatamente o que ele perdeu. Republicá-la
// ao vivo seria mandar o quadro inteiro para quem acabou de baixá-lo.
func (d *Despachante) Observar(ctx context.Context, boardID string) {
	d.mu.Lock()
	_, jaObserva := d.cursor[boardID]
	d.mu.Unlock()
	if jaObserva {
		return
	}

	atual, err := d.log.RevisaoAtual(ctx, boardID)
	if err != nil {
		// Sem saber onde o quadro está, começar do zero republicaria tudo. É
		// melhor não observar: a conexão continua funcionando pelo replay, e a
		// próxima tenta de novo.
		d.registro.Warn("não foi possível começar a observar o quadro",
			slog.String("board", boardID), slog.String("erro", err.Error()))
		return
	}

	d.mu.Lock()
	if _, jaObserva := d.cursor[boardID]; !jaObserva {
		d.cursor[boardID] = atual
	}
	d.mu.Unlock()
}

// Esquecer para de acompanhar um quadro. Chamado quando a sala esvazia.
func (d *Despachante) Esquecer(boardID string) {
	d.mu.Lock()
	delete(d.cursor, boardID)
	delete(d.atraso, boardID)
	d.mu.Unlock()
}

// Publicar é o aviso de que um quadro mudou.
//
// ⚠️ O EVENTO RECEBIDO É IGNORADO de propósito. Entregá-lo aqui recriaria o
// problema: seria a chamada em processo decidindo a ordem, e não o log. O que
// vale é o que está gravado.
//
// O envio é não bloqueante: acordar nunca pode segurar quem acabou de comitar.
// Fila cheia significa que o polling pega aquele quadro em seguida.
func (d *Despachante) Publicar(e evento.Evento) {
	if e.BoardID == "" {
		return
	}
	select {
	case d.acordar <- e.BoardID:
	default:
	}
}

// Rodar entrega até o contexto ser cancelado. Chamar numa goroutine própria.
func (d *Despachante) Rodar(ctx context.Context) {
	relogio := time.NewTicker(d.intervalo)
	defer relogio.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case boardID := <-d.acordar:
			d.entregarDoQuadro(ctx, boardID)
		case <-relogio.C:
			// O polling é a rede que pega o wake-up perdido — fila cheia,
			// processo reiniciado, entrega que falhou. Sem ele, o pior caso
			// voltaria a ser "o evento sumiu".
			for _, boardID := range d.quadrosObservados() {
				d.entregarDoQuadro(ctx, boardID)
			}
		}
	}
}

// EntregarAgora roda uma passada de entrega para um quadro, sem esperar o laço.
// Existe para os testes conseguirem observar o efeito sem depender de tempo.
func (d *Despachante) EntregarAgora(ctx context.Context, boardID string) {
	d.entregarDoQuadro(ctx, boardID)
}

// quadrosObservados devolve os quadros com alguém olhando, em ordem estável.
func (d *Despachante) quadrosObservados() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	ids := make([]string, 0, len(d.cursor))
	for id := range d.cursor {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// entregarDoQuadro publica o que houver depois do cursor, em ordem.
func (d *Despachante) entregarDoQuadro(ctx context.Context, boardID string) {
	for {
		d.mu.Lock()
		cursor, observado := d.cursor[boardID]
		d.mu.Unlock()
		if !observado {
			// A sala esvaziou entre o aviso e agora: não há a quem entregar.
			return
		}

		eventos, err := d.log.DesdeRevisao(ctx, boardID, cursor, porLeitura)
		if err != nil {
			// NÃO avança o cursor. A próxima passada tenta o mesmo intervalo —
			// é o que transforma uma falha de leitura em atraso em vez de
			// buraco.
			d.registro.Warn("falha ao ler o log para entrega",
				slog.String("board", boardID), slog.String("erro", err.Error()))
			return
		}
		if len(eventos) == 0 {
			d.marcarAtraso(boardID, 0)
			return
		}

		// Só GRUPOS COMPLETOS são entregues. Um grupo cortado faria o cliente
		// confirmar uma revisão que ele aplicou pela metade, e o que faltou
		// ficaria abaixo do cursor dele para sempre.
		completos := ateOFimDoGrupo(eventos)
		if len(completos) == 0 {
			// O primeiro grupo do intervalo é maior que o lote. Ler mais não
			// ajuda enquanto ele não estiver completo no banco; a próxima
			// passada resolve.
			return
		}

		for _, e := range completos {
			d.entrega.Publicar(e)
		}

		ultima := completos[len(completos)-1].Revisao
		d.mu.Lock()
		// Só avança se ninguém tiver esquecido o quadro no meio, e nunca para
		// trás.
		if atual, observado := d.cursor[boardID]; observado && ultima > atual {
			d.cursor[boardID] = ultima
		}
		d.ultimoSucesso = time.Now()
		d.mu.Unlock()

		// Lote cheio significa que pode haver mais: continua no mesmo passe.
		if len(eventos) < porLeitura {
			d.marcarAtraso(boardID, 0)
			return
		}
	}
}

func (d *Despachante) marcarAtraso(boardID string, revisoes int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, observado := d.cursor[boardID]; observado {
		d.atraso[boardID] = revisoes
	}
}

// ateOFimDoGrupo descarta o último grupo quando ele está incompleto.
//
// Um grupo está completo quando o número de eventos daquela revisão presentes
// na lista é igual à `quantidade` que eles declaram.
func ateOFimDoGrupo(eventos []evento.Evento) []evento.Evento {
	if len(eventos) == 0 {
		return eventos
	}
	ultimo := eventos[len(eventos)-1]
	if ultimo.Quantidade <= 1 || ultimo.Indice == ultimo.Quantidade-1 {
		return eventos
	}
	corte := len(eventos)
	for corte > 0 && eventos[corte-1].Revisao == ultimo.Revisao {
		corte--
	}
	return eventos[:corte]
}

// Estado é o retrato da entrega, para a métrica de A8.
type Estado struct {
	QuadrosObservados int
	AtrasoMaximo      int64
	UltimoSucesso     time.Time
}

// Estado devolve o retrato atual da entrega.
func (d *Despachante) Estado() Estado {
	d.mu.Lock()
	defer d.mu.Unlock()
	var maior int64
	for _, revisoes := range d.atraso {
		if revisoes > maior {
			maior = revisoes
		}
	}
	return Estado{
		QuadrosObservados: len(d.cursor),
		AtrasoMaximo:      maior,
		UltimoSucesso:     d.ultimoSucesso,
	}
}
