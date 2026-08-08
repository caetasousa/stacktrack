package repository

import (
	"context"

	"stacktrack/internal/domain/evento"
	ucboard "stacktrack/internal/usecase/board"

	"github.com/jackc/pgx/v5/pgxpool"
)

// UnidadeDeTrabalho grava uma mudança e o evento que a descreve na MESMA
// transação. É a implementação do outbox transacional.
//
// O problema que ela resolve: sem transação comum, o card muda numa transação e
// o evento é gravado noutra, logo depois. Um processo que morra entre as duas
// deixa a mudança gravada e o evento não — e esse buraco é INVISÍVEL. O cliente
// que reconecta pergunta "o que houve desde o 41?", recebe do 42 em diante, e
// nunca fica sabendo que existiu uma mudança sem evento. A tela dele passa a
// discordar do banco em silêncio, que é o pior defeito possível aqui.
//
// Com a transação comum, os dois destinos são o mesmo commit: ou o card move e
// o evento existe, ou nenhum dos dois acontece.
type UnidadeDeTrabalho struct {
	pool    *pgxpool.Pool
	eventos *EventoPostgres
}

// NovaUnidadeDeTrabalho cria a unidade sobre o pool informado.
func NovaUnidadeDeTrabalho(pool *pgxpool.Pool) *UnidadeDeTrabalho {
	return &UnidadeDeTrabalho{pool: pool, eventos: NovoEventoPostgres(pool)}
}

// Escrever abre a transação, entrega à função os repositórios ligados a ela,
// grava o evento e comita. Devolve o seq atribuído ao evento.
//
// Nada é publicado aqui: publicar é entrega ao vivo, e anunciar antes do commit
// avisaria de uma mudança que ainda pode não acontecer. Quem publica é o
// usecase, depois desta função retornar sem erro.
func (u *UnidadeDeTrabalho) Escrever(
	ctx context.Context,
	e evento.Evento,
	mudanca func(ucboard.Escrita) error,
) (int64, error) {
	tx, err := u.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	// Rollback depois do commit não tem efeito, então o defer cobre só os
	// caminhos de erro — inclusive o panic, que sem isto deixaria a transação
	// aberta segurando conexão do pool.
	defer tx.Rollback(ctx) //nolint:errcheck // sem efeito depois do commit

	if err := mudanca(ucboard.Escrita{
		Cards:   &CardPostgres{db: tx},
		Colunas: &ColunaPostgres{db: tx},
		Boards:  &BoardPostgres{db: tx},
	}); err != nil {
		return 0, err
	}

	seq, err := u.eventos.RegistrarNaTransacao(ctx, tx, e)
	if err != nil {
		return 0, err
	}
	return seq, tx.Commit(ctx)
}
