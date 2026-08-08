package repository

import (
	"context"
	"encoding/json"

	"stacktrack/internal/domain/evento"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EventoPostgres é o log de eventos de cada quadro — o outbox.
//
// Tem duas portas de entrada, e a diferença entre elas é a garantia:
//
//   - RegistrarNaTransacao grava DENTRO da transação da mudança. Ou o card
//     move e o evento existe, ou nenhum dos dois. É o que os eventos
//     estruturais usam — via UnidadeDeTrabalho, em transacao.go —, porque um
//     buraco ali é invisível: o cliente que reconecta pediria "desde o 41",
//     receberia o 43 e nunca saberia que o 42 existiu.
//
//   - Registrar grava sozinho, fora de transação. Serve para os eventos que
//     são apenas um AVISO de "recarregue o quadro" (etiqueta, checklist,
//     anexo). Perder um deles não deixa buraco perceptível: qualquer evento
//     seguinte, ou a própria reconexão, manda a tela buscar tudo de novo.
type EventoPostgres struct {
	pool *pgxpool.Pool
}

// NovoEventoPostgres cria o repositório do log sobre o pool informado.
func NovoEventoPostgres(pool *pgxpool.Pool) *EventoPostgres {
	return &EventoPostgres{pool: pool}
}

// RegistrarNaTransacao grava o evento usando a transação recebida e devolve o
// seq atribuído pelo banco.
//
// O seq volta porque é ele que o cliente guarda para pedir o próximo intervalo
// — um evento entregue sem seq não pode ser retomado depois.
func (r *EventoPostgres) RegistrarNaTransacao(ctx context.Context, tx pgx.Tx, e evento.Evento) (int64, error) {
	corpo, err := json.Marshal(e.Dados)
	if err != nil {
		return 0, err
	}
	var seq int64
	err = tx.QueryRow(ctx,
		`INSERT INTO board_events (board_id, tipo, payload, autor_id, criado_em)
		 VALUES ($1, $2, $3, $4, $5) RETURNING seq`,
		e.BoardID, string(e.Tipo), corpo, vazioParaNulo(e.AutorID), e.OcorridoEm,
	).Scan(&seq)
	return seq, err
}

// Registrar grava o evento fora de transação.
func (r *EventoPostgres) Registrar(ctx context.Context, e evento.Evento) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // sem efeito depois do commit
	seq, err := r.RegistrarNaTransacao(ctx, tx, e)
	if err != nil {
		return 0, err
	}
	return seq, tx.Commit(ctx)
}

// Desde devolve os eventos do quadro posteriores ao seq informado, em ordem.
//
// O teto existe para o replay não virar uma máquina de negação de serviço: uma
// aba que ficou uma semana fechada pediria a história inteira do quadro, e o
// servidor a montaria inteira em memória para entregá-la. Quando o intervalo é
// grande demais, o cliente recarrega tudo — que é mais barato e sempre certo.
func (r *EventoPostgres) Desde(ctx context.Context, boardID string, seq int64, limite int) ([]evento.Evento, error) {
	linhas, err := r.pool.Query(ctx,
		`SELECT seq, tipo, payload, autor_id, criado_em
		   FROM board_events
		  WHERE board_id = $1 AND seq > $2
		  ORDER BY seq
		  LIMIT $3`, boardID, seq, limite,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	eventos := make([]evento.Evento, 0)
	for linhas.Next() {
		var e evento.Evento
		var tipo string
		var corpo []byte
		var autor *string
		if err := linhas.Scan(&e.Seq, &tipo, &corpo, &autor, &e.OcorridoEm); err != nil {
			return nil, err
		}
		e.Tipo = evento.Tipo(tipo)
		e.BoardID = boardID
		e.AutorID = valorOuVazio(autor)
		if len(corpo) > 0 {
			// O payload volta como JSON cru: quem o consome é o cliente, e
			// remontar o tipo Go aqui só para serializá-lo de novo seria
			// trabalho jogado fora.
			e.Dados = json.RawMessage(corpo)
		}
		eventos = append(eventos, e)
	}
	return eventos, linhas.Err()
}

// UltimoSeq informa em que ponto o quadro está agora.
//
// É o que o cliente recebe ao conectar pela primeira vez: sem ele, a primeira
// reconexão pediria "desde o 0" e receberia a história inteira do quadro para
// jogar fora.
func (r *EventoPostgres) UltimoSeq(ctx context.Context, boardID string) (int64, error) {
	var seq *int64
	err := r.pool.QueryRow(ctx,
		`SELECT MAX(seq) FROM board_events WHERE board_id = $1`, boardID,
	).Scan(&seq)
	if err != nil || seq == nil {
		return 0, err
	}
	return *seq, nil
}
