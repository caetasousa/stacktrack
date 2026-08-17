package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"stacktrack/internal/domain/evento"
	ucboard "stacktrack/internal/usecase/board"

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
		`INSERT INTO board_events (board_id, card_id, tipo, payload, autor_id, criado_em)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING seq`,
		e.BoardID, vazioParaNulo(e.CardID), string(e.Tipo), corpo,
		vazioParaNulo(e.AutorID), e.OcorridoEm,
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

// DoCard devolve o histórico de um card, do mais recente para o mais antigo —
// é a ordem em que se lê histórico.
//
// O JOIN com usuarios é LEFT de propósito: `autor_id` não tem chave
// estrangeira, justamente para o histórico sobreviver à remoção de uma conta.
// Um INNER JOIN faria a linha inteira sumir nesse caso, que é o oposto do que
// um histórico serve para fazer.
func (r *EventoPostgres) DoCard(ctx context.Context, cardID string, limite int) ([]ucboard.Atividade, error) {
	linhas, err := r.pool.Query(ctx,
		`SELECT e.seq, e.tipo, e.payload, e.autor_id, u.nome, u.email, e.criado_em
		   FROM board_events e
		   LEFT JOIN usuarios u ON u.id = e.autor_id
		  WHERE e.card_id = $1
		  ORDER BY e.seq DESC
		  LIMIT $2`, cardID, limite,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	return lerAtividades(linhas)
}

// DoBoard devolve o histórico do quadro inteiro, do mais recente para o mais
// antigo — é o que a tela de auditoria lê.
//
// A consulta é montada com argumentos posicionais, e nunca por concatenação: os
// três filtros vêm da query string, e um deles interpolado no SQL seria injeção
// direta sobre o log de eventos de todos os quadros.
//
// O cursor é `seq < $n`, e não OFFSET. O log recebe escrita o tempo todo: com
// OFFSET, um evento novo entre a primeira página e a segunda empurraria uma
// linha para fora da janela e ela nunca seria lida — numa auditoria, uma linha
// pulada em silêncio é o pior defeito possível.
func (r *EventoPostgres) DoBoard(ctx context.Context, boardID string, filtro ucboard.FiltroDeAtividade) ([]ucboard.Atividade, error) {
	sql := `SELECT e.seq, e.tipo, e.payload, e.autor_id, u.nome, u.email, e.criado_em
	          FROM board_events e
	          LEFT JOIN usuarios u ON u.id = e.autor_id
	         WHERE e.board_id = $1`
	args := []any{boardID}

	if filtro.SoMovimentacoes {
		args = append(args, string(evento.CardMovido))
		sql += fmt.Sprintf(" AND e.tipo = $%d", len(args))
	}
	if filtro.AutorID != "" {
		args = append(args, filtro.AutorID)
		sql += fmt.Sprintf(" AND e.autor_id = $%d", len(args))
	}
	if filtro.AntesDe > 0 {
		args = append(args, filtro.AntesDe)
		sql += fmt.Sprintf(" AND e.seq < $%d", len(args))
	}
	args = append(args, filtro.Limite)
	sql += fmt.Sprintf(" ORDER BY e.seq DESC LIMIT $%d", len(args))

	linhas, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	return lerAtividades(linhas)
}

// UltimaMovimentacaoPorCard devolve, para cada card do quadro que já foi movido,
// a última movimentação dele — numa consulta só.
//
// DISTINCT ON é do PostgreSQL e é o que resolve isto sem subconsulta: com
// `ORDER BY card_id, seq DESC` ele fica com a PRIMEIRA linha de cada card, que
// nessa ordenação é a mais recente. A alternativa portável seria uma janela
// (ROW_NUMBER) num subselect — mais SQL para o mesmo plano.
func (r *EventoPostgres) UltimaMovimentacaoPorCard(ctx context.Context, boardID string) (map[string]ucboard.Movimentacao, error) {
	linhas, err := r.pool.Query(ctx,
		`SELECT DISTINCT ON (e.card_id) e.card_id, e.autor_id, u.nome, e.payload, e.criado_em
		   FROM board_events e
		   LEFT JOIN usuarios u ON u.id = e.autor_id
		  WHERE e.board_id = $1 AND e.tipo = $2 AND e.card_id IS NOT NULL
		  ORDER BY e.card_id, e.seq DESC`,
		boardID, string(evento.CardMovido),
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	porCard := make(map[string]ucboard.Movimentacao)
	for linhas.Next() {
		var cardID string
		var autor, nome *string
		var corpo []byte
		var quando time.Time
		if err := linhas.Scan(&cardID, &autor, &nome, &corpo, &quando); err != nil {
			return nil, err
		}
		m := ucboard.Movimentacao{
			AutorID:    valorOuVazio(autor),
			AutorNome:  valorOuVazio(nome),
			OcorridoEm: quando.Format(time.RFC3339),
		}
		// As colunas vêm do payload, e não de um JOIN com `colunas`: o evento
		// gravou o nome que a coluna tinha NA HORA. Resolver o id agora mostraria
		// o nome de hoje numa frase sobre ontem — e nada, se a coluna já tiver
		// sido apagada. É a mesma decisão documentada em DadosDoCard.
		if len(corpo) > 0 {
			var dados ucboard.DadosDoCard
			if err := json.Unmarshal(corpo, &dados); err == nil {
				m.DeColuna, m.ParaColuna = dados.DeColuna, dados.Coluna
			}
		}
		porCard[cardID] = m
	}
	return porCard, linhas.Err()
}

// lerAtividades converte as linhas do log em entradas de histórico. É a mesma
// leitura de DoCard e de DoBoard — duas cópias divergiriam no dia em que o log
// ganhasse uma coluna.
func lerAtividades(linhas pgx.Rows) ([]ucboard.Atividade, error) {
	lista := make([]ucboard.Atividade, 0)
	for linhas.Next() {
		var a ucboard.Atividade
		var tipo string
		var corpo []byte
		var autor, nome, email *string
		var quando time.Time
		if err := linhas.Scan(&a.Seq, &tipo, &corpo, &autor, &nome, &email, &quando); err != nil {
			return nil, err
		}
		a.Tipo = evento.Tipo(tipo)
		a.AutorID = valorOuVazio(autor)
		a.AutorNome = valorOuVazio(nome)
		a.AutorEmail = valorOuVazio(email)
		a.OcorridoEm = quando.Format(time.RFC3339)
		if len(corpo) > 0 {
			a.Dados = json.RawMessage(corpo)
		}
		lista = append(lista, a)
	}
	return lista, linhas.Err()
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
