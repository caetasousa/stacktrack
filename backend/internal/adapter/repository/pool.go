package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolComEspera é o pool com TETO DE ESPERA por conexão livre.
//
// O que ele resolve: `pgxpool` não expõe um prazo de aquisição separado do
// contexto de quem chama. Na prática isso fazia a espera por conexão herdar o
// orçamento inteiro da requisição — dez segundos. Sob saturação, cada
// requisição nova fica viva esses dez segundos segurando goroutine e memória
// para, no fim, ser atendida muito depois de quem pediu ter desistido; ou pior,
// ser atendida e devolver uma resposta que ninguém mais está esperando.
//
// Recusar rápido é melhor: o cliente repete, e a fila não vira um acúmulo que
// só cresce. Dois segundos é o teto do plano — acima disso a pessoa já
// abandonou a tela.
//
// ⚠️ O prazo vale SÓ para a aquisição. Depois de obtida a conexão, o comando
// roda com o contexto original: cortar a consulta em dois segundos seria outro
// limite, e ele já existe (`statement_timeout`, definido no startup da conexão).
type PoolComEspera struct {
	pool   *pgxpool.Pool
	espera time.Duration
}

// NovoPoolComEspera embrulha o pool com o teto de espera informado.
// Valor não positivo devolve o pool sem teto adicional.
func NovoPoolComEspera(pool *pgxpool.Pool, espera time.Duration) *PoolComEspera {
	return &PoolComEspera{pool: pool, espera: espera}
}

// adquirir pega uma conexão do pool respeitando o teto.
//
// O contexto derivado é cancelado ASSIM QUE a aquisição termina — e é por isso
// que o `cancel` é chamado antes de devolver, e não num defer do chamador.
// Deixá-lo vivo cancelaria a conexão recém-obtida junto com o timer.
func (p *PoolComEspera) adquirir(ctx context.Context) (*pgxpool.Conn, error) {
	if p.espera <= 0 {
		return p.pool.Acquire(ctx)
	}
	prazo, cancelar := context.WithTimeout(ctx, p.espera)
	defer cancelar()
	return p.pool.Acquire(prazo)
}

// Exec roda um comando que não devolve linhas.
func (p *PoolComEspera) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	conn, err := p.adquirir(ctx)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	defer conn.Release()
	return conn.Exec(ctx, sql, args...)
}

// Query devolve linhas cuja leitura ainda usa a conexão.
//
// A conexão só volta ao pool quando as linhas são fechadas — é o mesmo contrato
// de pgxpool.Pool.Query, e é o motivo de existir linhasQueLiberam: soltar a
// conexão aqui devolveria linhas apontando para uma conexão já reutilizada por
// outra requisição.
func (p *PoolComEspera) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	conn, err := p.adquirir(ctx)
	if err != nil {
		return nil, err
	}
	linhas, err := conn.Query(ctx, sql, args...)
	if err != nil {
		conn.Release()
		return nil, err
	}
	return &linhasQueLiberam{Rows: linhas, conn: conn}, nil
}

// QueryRow devolve uma linha só.
//
// pgx.Row não tem Close: quem o consome chama Scan uma vez e pronto. Por isso a
// conexão é liberada no Scan — ver linhaQueLibera.
func (p *PoolComEspera) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	conn, err := p.adquirir(ctx)
	if err != nil {
		return linhaComErro{err: err}
	}
	return &linhaQueLibera{Row: conn.QueryRow(ctx, sql, args...), conn: conn}
}

// Begin abre uma transação com o teto de espera aplicado à aquisição.
func (p *PoolComEspera) Begin(ctx context.Context) (pgx.Tx, error) {
	return p.BeginTx(ctx, pgx.TxOptions{})
}

// BeginTx abre uma transação com opções.
//
// A transação devolvida é a do pgxpool, que libera a conexão sozinha no
// Commit/Rollback — não há o que embrulhar aqui.
func (p *PoolComEspera) BeginTx(ctx context.Context, opcoes pgx.TxOptions) (pgx.Tx, error) {
	conn, err := p.adquirir(ctx)
	if err != nil {
		return nil, err
	}
	tx, err := conn.BeginTx(ctx, opcoes)
	if err != nil {
		conn.Release()
		return nil, err
	}
	return &transacaoQueLibera{Tx: tx, conn: conn}, nil
}

// Ping confere que o banco responde. Usado pelo readiness.
func (p *PoolComEspera) Ping(ctx context.Context) error {
	conn, err := p.adquirir(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	return conn.Ping(ctx)
}

// Pool devolve o pool embrulhado, para quem precisa dele cru.
func (p *PoolComEspera) Pool() *pgxpool.Pool { return p.pool }

// linhasQueLiberam devolve a conexão ao pool quando as linhas fecham.
type linhasQueLiberam struct {
	pgx.Rows
	conn *pgxpool.Conn
	// solta evita liberar duas vezes: Close é idempotente no pgx, e liberar
	// duas vezes a mesma conexão a devolveria ao pool enquanto outra requisição
	// já a estivesse usando.
	solta bool
}

func (l *linhasQueLiberam) Close() {
	l.Rows.Close()
	if !l.solta {
		l.solta = true
		l.conn.Release()
	}
}

// linhaQueLibera devolve a conexão depois do Scan.
type linhaQueLibera struct {
	pgx.Row
	conn  *pgxpool.Conn
	solta bool
}

func (l *linhaQueLibera) Scan(destinos ...any) error {
	err := l.Row.Scan(destinos...)
	if !l.solta {
		l.solta = true
		l.conn.Release()
	}
	return err
}

// linhaComErro representa a falha de AQUISIÇÃO numa interface que não a
// comporta: pgx.Row só sabe falhar no Scan.
type linhaComErro struct{ err error }

func (l linhaComErro) Scan(...any) error { return l.err }

// transacaoQueLibera devolve a conexão ao pool quando a transação termina.
type transacaoQueLibera struct {
	pgx.Tx
	conn  *pgxpool.Conn
	solta bool
}

func (t *transacaoQueLibera) Commit(ctx context.Context) error {
	err := t.Tx.Commit(ctx)
	t.liberar()
	return err
}

func (t *transacaoQueLibera) Rollback(ctx context.Context) error {
	err := t.Tx.Rollback(ctx)
	t.liberar()
	return err
}

func (t *transacaoQueLibera) liberar() {
	if !t.solta {
		t.solta = true
		t.conn.Release()
	}
}
