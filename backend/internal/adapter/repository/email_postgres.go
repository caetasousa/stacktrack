package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	demail "stacktrack/internal/domain/email"
	"stacktrack/internal/usecase/correio"

	"github.com/jackc/pgx/v5"
)

// RetencaoDeEmailEnviado é quanto a linha do outbox fica depois de criada — o
// registro de que houve envio, já sem a mensagem. Igual à dos convites
// terminais: os dois respondem juntos "quem convidou quem, e o email saiu?".
const RetencaoDeEmailEnviado = RetencaoDeConviteTerminal

// cifraDeEmail é o que o outbox precisa da cifra. Declarada aqui, no
// consumidor: o repositório não sabe que é AES-GCM.
type cifraDeEmail interface {
	Cifrar(claro []byte, contexto string) ([]byte, error)
	Decifrar(cifrado []byte, contexto string) ([]byte, error)
}

// EmailPostgres é o outbox de email. Serve às duas pontas: ao caso de uso que
// pede o envio (dentro da transação do convite) e ao worker que entrega.
//
// A mensagem é gravada cifrada, com o id do envio como contexto autenticado, e
// apagada assim que deixa de ser útil — no envio, no cancelamento e no
// vencimento.
type EmailPostgres struct {
	db    consultante
	cifra cifraDeEmail
}

// NovoEmailPostgres cria o repositório ligado ao pool.
func NovoEmailPostgres(pool Fonte, cifra cifraDeEmail) *EmailPostgres {
	return &EmailPostgres{db: pool, cifra: cifra}
}

const camposDoEnvio = `id, modelo, chave_deduplicacao, estado, tentativas, proxima_em, criado_em,
	enviado_em, expira_em, COALESCE(ultimo_erro, ''), autor_id, COALESCE(convite_id::text, ''),
	COALESCE(notificacao_id::text, '')`

// Enfileirar grava a intenção de envio com a mensagem cifrada.
//
// A chave de deduplicação já existente é tratada como sucesso, e por ON
// CONFLICT, não por erro capturado: dentro da transação do convite, uma
// violação de unicidade abortaria a transação inteira — e o convite com ela.
func (r *EmailPostgres) Enfileirar(ctx context.Context, e *demail.Envio, m demail.Mensagem) error {
	claro, err := json.Marshal(m)
	if err != nil {
		return err
	}
	cifrado, err := r.cifra.Cifrar(claro, e.ID)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx,
		`INSERT INTO email_outbox (id, modelo, chave_deduplicacao, payload_cifrado, estado,
		                           tentativas, proxima_em, criado_em, enviado_em, expira_em,
		                           ultimo_erro, autor_id, convite_id, notificacao_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, ''), $12, NULLIF($13, '')::uuid,
		         NULLIF($14, '')::uuid)
		 ON CONFLICT (chave_deduplicacao) DO NOTHING`,
		e.ID, e.Modelo, e.ChaveDeduplicacao, cifrado, string(e.Estado),
		e.Tentativas, e.ProximaEm, e.CriadoEm, e.EnviadoEm, e.ExpiraEm,
		e.UltimoErro, e.AutorID, e.ConviteID, e.NotificacaoID,
	)
	return err
}

// CancelarDoConvite tira da fila o que ainda não saiu de um convite, apagando
// a mensagem.
func (r *EmailPostgres) CancelarDoConvite(ctx context.Context, conviteID string, _ time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE email_outbox SET estado = $2, payload_cifrado = NULL
		  WHERE convite_id = $1 AND estado = $3`,
		conviteID, string(demail.EstadoCancelado), string(demail.EstadoPendente),
	)
	return err
}

// ContarDoAutorDesde conta os envios que a conta pediu a partir de `desde` —
// cancelados inclusive: convidar e revogar em laço não pode ser um jeito de
// contornar a cota.
//
// Os emails de notificação ficam de fora: eles têm teto próprio, por quem
// RECEBE (ContarDeNotificacaoDesde), e atribuir cards não pode gastar a cota de
// convites de quem atribui.
func (r *EmailPostgres) ContarDoAutorDesde(ctx context.Context, autorID string, desde time.Time) (int, error) {
	var n int
	err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM email_outbox
		  WHERE autor_id = $1 AND criado_em >= $2 AND notificacao_id IS NULL`,
		autorID, desde,
	).Scan(&n)
	return n, err
}

// ContarDeNotificacaoDesde conta os emails de notificação que a pessoa recebeu
// — ou ia receber — a partir de `desde`. Cancelados inclusive, pela mesma razão
// da cota de convites: atribuir e desatribuir em laço não pode contornar o teto.
func (r *EmailPostgres) ContarDeNotificacaoDesde(ctx context.Context, usuarioID string, desde time.Time) (int, error) {
	var n int
	err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM email_outbox e JOIN notificacoes n ON n.id = e.notificacao_id
		  WHERE n.usuario_id = $1 AND e.criado_em >= $2`,
		usuarioID, desde,
	).Scan(&n)
	return n, err
}

// Cancelar tira um envio da fila e apaga a mensagem. É o que o worker faz com o
// email de uma notificação que, na hora de sair, já não tem o que avisar.
func (r *EmailPostgres) Cancelar(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE email_outbox SET estado = $2, payload_cifrado = NULL WHERE id = $1 AND estado = $3`,
		id, string(demail.EstadoCancelado), string(demail.EstadoPendente),
	)
	return err
}

// DoConvite devolve os envios de um convite, do mais recente para o mais antigo.
func (r *EmailPostgres) DoConvite(ctx context.Context, conviteID string) ([]demail.Envio, error) {
	linhas, err := r.db.Query(ctx,
		`SELECT `+camposDoEnvio+` FROM email_outbox WHERE convite_id = $1 ORDER BY criado_em DESC`,
		conviteID,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	envios := make([]demail.Envio, 0)
	for linhas.Next() {
		e, err := lerEnvio(linhas)
		if err != nil {
			return nil, err
		}
		envios = append(envios, e)
	}
	return envios, linhas.Err()
}

// UltimosDosConvites devolve o envio mais recente de cada convite pedido.
func (r *EmailPostgres) UltimosDosConvites(ctx context.Context, conviteIDs []string) (map[string]demail.Envio, error) {
	ultimos := make(map[string]demail.Envio, len(conviteIDs))
	if len(conviteIDs) == 0 {
		return ultimos, nil
	}
	linhas, err := r.db.Query(ctx,
		`SELECT DISTINCT ON (convite_id) `+camposDoEnvio+`
		   FROM email_outbox
		  WHERE convite_id = ANY($1::uuid[])
		  ORDER BY convite_id, criado_em DESC`,
		conviteIDs,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	for linhas.Next() {
		e, err := lerEnvio(linhas)
		if err != nil {
			return nil, err
		}
		ultimos[e.ConviteID] = e
	}
	return ultimos, linhas.Err()
}

// Reservar pega até `limite` envios na hora e os empurra `reserva` para a
// frente, numa operação só. É a reserva que impede duas passadas — ou um
// worker reiniciado — de pegarem o mesmo envio; se o processo morrer depois de
// reservar, o envio volta à fila quando a reserva vence.
//
// SKIP LOCKED: duas reservas simultâneas dividem a fila em vez de esperar uma
// pela outra.
func (r *EmailPostgres) Reservar(ctx context.Context, agora time.Time, limite int, reserva time.Duration) ([]correio.Reservado, error) {
	linhas, err := r.db.Query(ctx,
		`UPDATE email_outbox SET proxima_em = $3
		  WHERE id IN (
		    SELECT id FROM email_outbox
		     WHERE estado = $4 AND proxima_em <= $1 AND expira_em > $1
		     ORDER BY proxima_em
		     LIMIT $2
		     FOR UPDATE SKIP LOCKED)
		 RETURNING `+camposDoEnvio+`, payload_cifrado`,
		agora, limite, agora.Add(reserva), string(demail.EstadoPendente),
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	reservados := make([]correio.Reservado, 0)
	for linhas.Next() {
		var e demail.Envio
		var estado string
		var cifrado []byte
		if err := linhas.Scan(&e.ID, &e.Modelo, &e.ChaveDeduplicacao, &estado, &e.Tentativas,
			&e.ProximaEm, &e.CriadoEm, &e.EnviadoEm, &e.ExpiraEm, &e.UltimoErro, &e.AutorID,
			&e.ConviteID, &e.NotificacaoID, &cifrado); err != nil {
			return nil, err
		}
		e.Estado = demail.Estado(estado)
		item := correio.Reservado{Envio: e}
		item.Mensagem, item.ErroDeLeitura = r.abrir(e.ID, cifrado)
		reservados = append(reservados, item)
	}
	return reservados, linhas.Err()
}

// abrir decifra a mensagem. Falhar aqui é definitivo — chave trocada ou dado
// adulterado não melhoram com o tempo —, e quem decide o que fazer é o worker.
func (r *EmailPostgres) abrir(id string, cifrado []byte) (demail.Mensagem, error) {
	var m demail.Mensagem
	if cifrado == nil {
		return m, errors.New("o envio está na fila sem a mensagem")
	}
	claro, err := r.cifra.Decifrar(cifrado, id)
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(claro, &m)
}

// MarcarEnviado registra a entrega e apaga a mensagem. Só vale para envio
// ainda pendente: um cancelamento que chegou durante a entrega continua
// valendo.
func (r *EmailPostgres) MarcarEnviado(ctx context.Context, id string, tentativas int, em time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE email_outbox
		    SET estado = $2, enviado_em = $3, tentativas = $4, payload_cifrado = NULL, ultimo_erro = NULL
		  WHERE id = $1 AND estado = $5`,
		id, string(demail.EstadoEnviado), em, tentativas, string(demail.EstadoPendente),
	)
	return err
}

// Adiar registra a falha passageira e marca a próxima tentativa.
func (r *EmailPostgres) Adiar(ctx context.Context, id string, tentativas int, proxima time.Time, erro string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE email_outbox SET tentativas = $2, proxima_em = $3, ultimo_erro = $4
		  WHERE id = $1 AND estado = $5`,
		id, tentativas, proxima, limitarErro(erro), string(demail.EstadoPendente),
	)
	return err
}

// MarcarFalhou desiste do envio. A mensagem fica até o vencimento — é o que
// permite reprocessar depois de corrigir a causa —, e a faxina a apaga então.
func (r *EmailPostgres) MarcarFalhou(ctx context.Context, id string, tentativas int, erro string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE email_outbox SET estado = $2, tentativas = $3, ultimo_erro = $4
		  WHERE id = $1 AND estado = $5`,
		id, string(demail.EstadoFalhou), tentativas, limitarErro(erro), string(demail.EstadoPendente),
	)
	return err
}

// Nome identifica esta limpeza na faxina.
func (r *EmailPostgres) Nome() string { return "email_outbox" }

// LimparLote apaga a mensagem dos envios vencidos — o pendente vira expirado —
// e remove as linhas mais velhas que a retenção. Devolve quantas linhas mexeu.
func (r *EmailPostgres) LimparLote(ctx context.Context, limite int) (int64, error) {
	vencidos, err := r.db.Exec(ctx,
		`UPDATE email_outbox
		    SET payload_cifrado = NULL,
		        estado = CASE WHEN estado = $2 THEN $3 ELSE estado END
		  WHERE ctid IN (
		    SELECT ctid FROM email_outbox
		     WHERE payload_cifrado IS NOT NULL AND expira_em < now()
		     LIMIT $1)`,
		limite, string(demail.EstadoPendente), string(demail.EstadoExpirado),
	)
	if err != nil {
		return 0, err
	}
	antigos, err := r.db.Exec(ctx,
		`DELETE FROM email_outbox
		  WHERE ctid IN (
		    SELECT ctid FROM email_outbox
		     WHERE criado_em < $1 AND payload_cifrado IS NULL
		     LIMIT $2)`,
		time.Now().Add(-RetencaoDeEmailEnviado), limite,
	)
	if err != nil {
		return vencidos.RowsAffected(), err
	}
	return vencidos.RowsAffected() + antigos.RowsAffected(), nil
}

func lerEnvio(linha pgx.Row) (demail.Envio, error) {
	var e demail.Envio
	var estado string
	err := linha.Scan(&e.ID, &e.Modelo, &e.ChaveDeduplicacao, &estado, &e.Tentativas,
		&e.ProximaEm, &e.CriadoEm, &e.EnviadoEm, &e.ExpiraEm, &e.UltimoErro, &e.AutorID, &e.ConviteID,
		&e.NotificacaoID)
	e.Estado = demail.Estado(estado)
	return e, err
}

// limitarErro cabe o erro na coluna. Quem chama já o mascarou.
func limitarErro(erro string) string {
	runas := []rune(erro)
	if len(runas) > 500 {
		return string(runas[:500])
	}
	return erro
}
