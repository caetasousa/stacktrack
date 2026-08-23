package repository

import (
	"context"
	"time"

	ucboard "stacktrack/internal/usecase/board"
)

// ExclusaoDeArquivoPostgres é o outbox das exclusões de arquivo.
type ExclusaoDeArquivoPostgres struct {
	db consultante
}

// NovoExclusaoDeArquivoPostgres cria o repositório sobre a fonte informada.
func NovoExclusaoDeArquivoPostgres(pool Fonte) *ExclusaoDeArquivoPostgres {
	return &ExclusaoDeArquivoPostgres{db: pool}
}

// tamanhoMaximoDoErro é o teto do texto de falha guardado.
//
// A coluna é VARCHAR(500) e a mensagem vem do sistema de arquivos, que não tem
// compromisso com tamanho. Cortar aqui, no domínio do adaptador, evita que uma
// mensagem longa transforme uma falha de remoção numa falha de INSERT.
const tamanhoMaximoDoErro = 500

// Registrar grava as chaves físicas afetadas por uma exclusão.
//
// Chamado ANTES do DELETE que dispara o CASCADE e dentro da mesma transação:
// depois dele as linhas de `anexos` já não existem, e não há de onde tirar os
// caminhos. É a razão de este repositório aparecer em Escrita.
//
// `unnest` insere as N linhas num comando só: um INSERT por arquivo faria
// apagar um quadro com quinhentos anexos custar quinhentos round-trips dentro
// da transação que segura o lock do quadro.
func (r *ExclusaoDeArquivoPostgres) Registrar(ctx context.Context, boardID string, caminhos []string, em time.Time) error {
	if len(caminhos) == 0 {
		return nil
	}
	_, err := r.db.Exec(ctx,
		`INSERT INTO arquivo_exclusoes (caminho, board_id, excluido_em, tentativas, proxima_em)
		 SELECT c, $2, $3, 0, $3 FROM unnest($1::text[]) AS c`,
		caminhos, vazioParaNulo(boardID), em,
	)
	return err
}

const camposDaExclusao = `id, caminho, board_id, excluido_em, tentativas`

// Pendentes devolve exclusões COBERTAS, ainda no disco e na hora de tentar.
//
// `coberto_em IS NOT NULL` é a condição que sustenta tudo: sem prova de backup,
// a linha simplesmente não aparece para o worker. É assim que o fail-closed
// acontece por construção, e não por um `if` que alguém pode remover.
//
// `FOR UPDATE SKIP LOCKED` para o dia em que houver mais de um worker: cada um
// pega linhas diferentes em vez de esperar pelo outro. Com uma instância só ele
// não muda nada, e custa nada.
func (r *ExclusaoDeArquivoPostgres) Pendentes(ctx context.Context, agora time.Time, limite int) ([]ucboard.ExclusaoDeArquivo, error) {
	linhas, err := r.db.Query(ctx,
		`SELECT `+camposDaExclusao+`
		   FROM arquivo_exclusoes
		  WHERE removido_em IS NULL
		    AND coberto_em IS NOT NULL
		    AND (proxima_em IS NULL OR proxima_em <= $1)
		  ORDER BY proxima_em, id
		  LIMIT $2
		    FOR UPDATE SKIP LOCKED`, agora, limite,
	)
	if err != nil {
		return nil, err
	}
	return lerExclusoes(linhas)
}

// SemCobertura devolve exclusões que ainda não têm prova de backup.
//
// É o que A6 vai comprovar e o que A8 vai publicar como métrica: uma fila que só
// cresce significa que o backup parou de cobrir, e é isso que precisa virar
// alerta antes de o disco encher.
func (r *ExclusaoDeArquivoPostgres) SemCobertura(ctx context.Context, limite int) ([]ucboard.ExclusaoDeArquivo, error) {
	linhas, err := r.db.Query(ctx,
		`SELECT `+camposDaExclusao+`
		   FROM arquivo_exclusoes
		  WHERE coberto_em IS NULL AND removido_em IS NULL
		  ORDER BY excluido_em, id
		  LIMIT $1`, limite,
	)
	if err != nil {
		return nil, err
	}
	return lerExclusoes(linhas)
}

// MarcarCobertos marca como cobertos os IDs EXATOS comprovados pelo backup.
//
// Os ids vêm de uma lista literal, e não de um intervalo de tempo ou de um
// `id <= N`: o dump do banco e a cópia dos arquivos acontecem em instantes
// diferentes, e o relógio do host pode andar para trás. Só a lista do que o
// snapshot realmente levou responde sem margem.
func (r *ExclusaoDeArquivoPostgres) MarcarCobertos(ctx context.Context, ids []int64, em time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.db.Exec(ctx,
		`UPDATE arquivo_exclusoes SET coberto_em = $2
		  WHERE id = ANY($1::bigint[]) AND coberto_em IS NULL`, ids, em,
	)
	return err
}

// MarcarRemovido registra que os bytes saíram do disco.
func (r *ExclusaoDeArquivoPostgres) MarcarRemovido(ctx context.Context, id int64, em time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE arquivo_exclusoes SET removido_em = $2, ultimo_erro = NULL WHERE id = $1`, id, em,
	)
	return err
}

// AdiarComErro registra a falha e agenda a próxima tentativa.
func (r *ExclusaoDeArquivoPostgres) AdiarComErro(ctx context.Context, id int64, erro string, proximaEm time.Time) error {
	if len(erro) > tamanhoMaximoDoErro {
		erro = erro[:tamanhoMaximoDoErro]
	}
	_, err := r.db.Exec(ctx,
		`UPDATE arquivo_exclusoes
		    SET tentativas = tentativas + 1, ultimo_erro = $2, proxima_em = $3
		  WHERE id = $1`, id, erro, proximaEm,
	)
	return err
}

func lerExclusoes(linhas interface {
	Next() bool
	Scan(...any) error
	Close()
	Err() error
}) ([]ucboard.ExclusaoDeArquivo, error) {
	defer linhas.Close()

	exclusoes := make([]ucboard.ExclusaoDeArquivo, 0)
	for linhas.Next() {
		var e ucboard.ExclusaoDeArquivo
		var boardID *string
		if err := linhas.Scan(&e.ID, &e.Caminho, &boardID, &e.ExcluidoEm, &e.Tentativas); err != nil {
			return nil, err
		}
		e.BoardID = valorOuVazio(boardID)
		exclusoes = append(exclusoes, e)
	}
	return exclusoes, linhas.Err()
}
