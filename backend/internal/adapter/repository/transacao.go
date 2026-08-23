package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	dboard "stacktrack/internal/domain/board"
	"stacktrack/internal/domain/evento"
	ucboard "stacktrack/internal/usecase/board"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	// codigoLockTimeout é o SQLSTATE 55P03 (lock_not_available), que o
	// PostgreSQL devolve quando lock_timeout estoura.
	codigoLockTimeout = "55P03"
	// codigoDeadlock é o SQLSTATE 40P01. Não deveria acontecer — a ordem de
	// aquisição é sempre a mesma —, e é justamente por isso que ele é
	// traduzido: se aparecer, é sinal de que alguma escrita nova furou a ordem,
	// e o erro precisa ser reconhecível no log em vez de virar um 500 anônimo.
	codigoDeadlock = "40P01"
)

// UnidadeDeTrabalho grava uma mudança e o evento que a descreve na MESMA
// transação, com a linha do quadro travada.
//
// # O outbox transacional
//
// Sem transação comum, o card muda numa transação e o evento é gravado noutra,
// logo depois. Um processo que morra entre as duas deixa a mudança gravada e o
// evento não — e esse buraco é INVISÍVEL. O cliente que reconecta pergunta "o
// que houve desde o 41?", recebe do 42 em diante, e nunca fica sabendo que
// existiu uma mudança sem evento. A tela dele passa a discordar do banco em
// silêncio, que é o pior defeito possível aqui.
//
// # O lock do quadro
//
// A transação sozinha não basta. READ COMMITTED — o padrão do PostgreSQL —
// garante que cada comando enxergue um instantâneo consistente, e não que duas
// transações concorrentes cheguem a um resultado que faça sentido JUNTAS.
//
// O exemplo que dói: dois donos, cada um removendo o outro. As duas transações
// leem "há dois donos", as duas concluem "posso remover", as duas comitam, e o
// quadro fica sem dono nenhum — órfão, sem ninguém que possa convidar ou
// apagá-lo. Nenhuma constraint pega isso, porque a regra é sobre o CONJUNTO de
// linhas, não sobre uma delas.
//
// `SELECT ... FOR UPDATE` na linha do quadro serializa toda mutação daquele
// quadro. É grosso de propósito: o quadro é a fronteira de consistência deste
// domínio, o perfil de carga é de dezenas de conexões (não milhares), e um lock
// por agregado é o desenho que se consegue provar correto com testes de
// interleaving. Quadros diferentes não se esperam — só as escritas do MESMO
// quadro serializam.
//
// # A ordem de aquisição
//
// Sempre board → convite/membro → coluna/card → agregados dependentes. Como o
// lock do quadro é SEMPRE o primeiro, e é o único explícito, não há ciclo
// possível entre duas transações: nenhuma segura um recurso que a outra queira
// antes de ter o quadro. É isso que torna o deadlock impossível por construção,
// e não por sorte.
type UnidadeDeTrabalho struct {
	pool    Fonte
	eventos *EventoPostgres
	// esperaPorLock é o teto de espera pelo lock do quadro (lock_timeout).
	esperaPorLock time.Duration
	// tempoDeComando é o teto de cada comando dentro da transação
	// (statement_timeout).
	tempoDeComando time.Duration
}

// NovaUnidadeDeTrabalho cria a unidade sobre o pool informado.
//
// esperaPorLock e tempoDeComando valem SÓ dentro da transação (SET LOCAL): o
// pool devolve a conexão ao final com os valores originais, e nenhuma consulta
// de leitura herda esses tetos.
//
// Valores não positivos caem em padrões conservadores. O teto existe porque a
// alternativa é espera ilimitada: sem lock_timeout, uma transação que trave
// segurando o quadro faz cada nova mutação daquele quadro esperar para sempre,
// uma conexão do pool por vez, até o pool acabar — e aí o quadro travado
// derruba a API inteira.
func NovaUnidadeDeTrabalho(pool Fonte, esperaPorLock, tempoDeComando time.Duration) *UnidadeDeTrabalho {
	if esperaPorLock <= 0 {
		esperaPorLock = 2 * time.Second
	}
	if tempoDeComando <= 0 {
		tempoDeComando = 5 * time.Second
	}
	return &UnidadeDeTrabalho{
		pool:           pool,
		eventos:        NovoEventoPostgres(pool),
		esperaPorLock:  esperaPorLock,
		tempoDeComando: tempoDeComando,
	}
}

// Escrever abre a transação, trava a linha do quadro, entrega à função os
// repositórios ligados a ela, grava o evento e comita. Devolve o seq atribuído
// ao evento.
//
// Nada é publicado aqui: publicar é entrega ao vivo, e anunciar antes do commit
// avisaria de uma mudança que ainda pode não acontecer. Quem publica é o
// usecase, depois desta função retornar sem erro.
func (u *UnidadeDeTrabalho) Escrever(
	ctx context.Context,
	e evento.Evento,
	mudanca func(ucboard.Escrita) error,
) (evento.Evento, error) {
	tx, err := u.pool.Begin(ctx)
	if err != nil {
		return e, err
	}
	// Rollback depois do commit não tem efeito, então o defer cobre só os
	// caminhos de erro — inclusive o panic, que sem isto deixaria a transação
	// aberta segurando conexão do pool.
	defer tx.Rollback(ctx) //nolint:errcheck // sem efeito depois do commit

	if err := u.aplicarTetos(ctx, tx); err != nil {
		return e, err
	}
	// Travar e numerar são o MESMO comando: o UPDATE ... RETURNING toma o lock
	// exclusivo da linha e devolve a revisão nova de uma vez. Fazer os dois em
	// dois comandos custaria um round-trip a mais em todo caminho de escrita,
	// sem ganhar nada — quem trava é justamente quem vai incrementar.
	revisao, err := u.travarENumerar(ctx, tx, e.BoardID)
	if err != nil {
		return e, err
	}
	e.Revisao = revisao

	if err := mudanca(repositoriosDe(tx)); err != nil {
		return e, traduzirErroDeConcorrencia(err)
	}
	// Na criação, a primeira tentativa de numerar não encontra a linha porque a
	// própria mudança é quem a insere. Agora ela já existe dentro desta transação:
	// numerar de novo produz a revisão 1 e mantém criação, dono e evento no mesmo
	// commit. Para quadros existentes este ramo nunca roda.
	if revisao == 0 && e.BoardID != "" {
		revisao, err = u.travarENumerar(ctx, tx, e.BoardID)
		if err != nil {
			return e, err
		}
		e.Revisao = revisao
	}

	seq, err := u.eventos.RegistrarNaTransacao(ctx, tx, e)
	if err != nil {
		return e, traduzirErroDeConcorrencia(err)
	}
	e.Seq = seq

	if err := tx.Commit(ctx); err != nil {
		return e, traduzirErroDeConcorrencia(err)
	}
	return e, nil
}

// ExcluirQuadro executa a exclusão terminal sob o lock da linha do quadro.
// Não registra evento porque o DELETE leva board_events por cascata; o objetivo
// desta transação é fazer a coleta dos caminhos físicos e a exclusão lógica
// enxergarem exatamente o mesmo estado.
func (u *UnidadeDeTrabalho) ExcluirQuadro(
	ctx context.Context,
	boardID string,
	mudanca func(ucboard.Escrita) error,
) error {
	tx, err := u.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // sem efeito depois do commit

	if err := u.aplicarTetos(ctx, tx); err != nil {
		return err
	}
	var encontrado string
	if err := tx.QueryRow(ctx,
		`SELECT id FROM boards WHERE id = $1 FOR UPDATE`, boardID,
	).Scan(&encontrado); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dboard.ErrNaoEncontrado
		}
		return traduzirErroDeConcorrencia(err)
	}
	if err := mudanca(repositoriosDe(tx)); err != nil {
		return traduzirErroDeConcorrencia(err)
	}
	return traduzirErroDeConcorrencia(tx.Commit(ctx))
}

// aplicarTetos define lock_timeout e statement_timeout SÓ para esta transação.
//
// SET LOCAL, e não SET: o pool reaproveita conexões, e um SET comum vazaria o
// teto para toda consulta seguinte daquela conexão — inclusive as leituras
// longas de auditoria, que passariam a ser cortadas por um limite pensado para
// escrita.
func (u *UnidadeDeTrabalho) aplicarTetos(ctx context.Context, tx pgx.Tx) error {
	// Os valores são interpolados porque SET LOCAL não aceita parâmetro
	// posicional ($1) — é uma limitação do comando, não uma escolha. A
	// interpolação é segura por construção: os dois valores são inteiros
	// derivados de time.Duration, e não há caminho por onde texto de fora
	// chegue aqui.
	_, err := tx.Exec(ctx, fmt.Sprintf(
		"SET LOCAL lock_timeout = %d; SET LOCAL statement_timeout = %d",
		u.esperaPorLock.Milliseconds(), u.tempoDeComando.Milliseconds(),
	))
	return err
}

// travarENumerar serializa as mutações do quadro e devolve a revisão nova.
//
// O `UPDATE ... RETURNING` faz as duas coisas: um UPDATE toma o lock exclusivo
// da linha, exatamente como `SELECT ... FOR UPDATE` faria, e já devolve o valor
// incrementado. É por a numeração acontecer SOB esse lock que a revisão sai
// contígua e na ordem de commit — que é justamente o que o `seq`, sendo
// BIGSERIAL, não consegue garantir.
//
// `COALESCE(revisao, 0)` cobre as linhas anteriores à migration, que nasceram
// com NULL. A decisão de que "quadro sem revisão começa do zero" é do domínio, e
// é por isso que ela está aqui e não num UPDATE dentro da migration — migration
// não escreve dado.
//
// Quadro inexistente NÃO é erro, e o caso normal é a criação: a linha ainda não
// existe quando a transação que a insere começa. Não há invariante a proteger
// sobre uma linha que não existe, e a revisão provisória devolvida é zero. O
// chamador repete a numeração depois do INSERT para que o evento de criação já
// nasça na revisão 1.
func (u *UnidadeDeTrabalho) travarENumerar(ctx context.Context, tx pgx.Tx, boardID string) (int64, error) {
	if boardID == "" {
		return 0, nil
	}
	var revisao int64
	err := tx.QueryRow(ctx,
		`UPDATE boards SET revisao = COALESCE(revisao, 0) + 1 WHERE id = $1 RETURNING revisao`,
		boardID,
	).Scan(&revisao)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, traduzirErroDeConcorrencia(err)
	}
	return revisao, nil
}

// repositoriosDe monta TODOS os repositórios ligados à transação.
//
// Todos, e não só os que a mutação vai usar: construir um struct de ponteiros é
// grátis, e a alternativa — cada usecase declarar de quais precisa — reapareceria
// como campo nil em produção no dia em que alguém acrescentasse uma escrita e
// esquecesse da lista.
func repositoriosDe(tx pgx.Tx) ucboard.Escrita {
	return ucboard.Escrita{
		Cards:        &CardPostgres{db: tx},
		Colunas:      &ColunaPostgres{db: tx},
		Boards:       &BoardPostgres{db: tx},
		Usuarios:     &UsuarioPostgres{db: tx},
		Membros:      &MembroPostgres{db: tx},
		Convites:     &ConvitePostgres{db: tx},
		Etiquetas:    &EtiquetaPostgres{db: tx},
		Checklists:   &ChecklistPostgres{db: tx},
		Anexos:       &AnexoPostgres{db: tx},
		Responsaveis: &ResponsavelPostgres{db: tx},
		Comentarios:  &ComentarioPostgres{db: tx},
		Exclusoes:    &ExclusaoDeArquivoPostgres{db: tx},
		Publicacoes:  &PublicacaoPostgres{db: tx},
	}
}

// traduzirErroDeConcorrencia transforma o SQLSTATE em erro de domínio.
//
// Sem isto, estourar o lock_timeout chega ao handler como erro cru do driver e
// vira 500 — "erro interno" para uma situação que não tem nada de interna e que
// se resolve tentando de novo em um segundo.
func traduzirErroDeConcorrencia(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case codigoLockTimeout, codigoDeadlock:
			return fmt.Errorf("%w (SQLSTATE %s)", ucboard.ErrQuadroOcupado, pgErr.Code)
		}
	}
	return err
}

// Instantaneo executa leituras sobre um único snapshot do banco.
//
// REPEATABLE READ, READ ONLY, DEFERRABLE. O DEFERRABLE só tem efeito com READ
// ONLY e SERIALIZABLE, e é inofensivo aqui; fica declarado para deixar a
// intenção explícita caso o nível suba um dia.
//
// Ver ucboard.InstantaneoConsistente para o defeito que isto elimina.
type Instantaneo struct {
	pool Fonte
	// tempoDeComando é o teto de cada comando da leitura. Uma leitura longa
	// segura o snapshot aberto, e um snapshot aberto impede o VACUUM de limpar
	// as versões antigas das linhas.
	tempoDeComando time.Duration
}

// NovoInstantaneo cria o leitor consistente sobre o pool informado.
func NovoInstantaneo(pool Fonte, tempoDeComando time.Duration) *Instantaneo {
	if tempoDeComando <= 0 {
		tempoDeComando = 5 * time.Second
	}
	return &Instantaneo{pool: pool, tempoDeComando: tempoDeComando}
}

// Executar abre a transação de leitura, entrega os repositórios ligados a ela e
// encerra.
//
// O encerramento é sempre Rollback, e não Commit: não há nada a confirmar numa
// transação READ ONLY, e o Rollback é o caminho mais barato de liberar o
// snapshot. Um erro nele não muda o resultado da leitura, que já foi entregue.
func (i *Instantaneo) Executar(ctx context.Context, leitura func(ucboard.Leitura) error) error {
	tx, err := i.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:       pgx.RepeatableRead,
		AccessMode:     pgx.ReadOnly,
		DeferrableMode: pgx.Deferrable,
	})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // leitura: não há o que confirmar

	if _, err := tx.Exec(ctx, fmt.Sprintf(
		"SET LOCAL statement_timeout = %d", i.tempoDeComando.Milliseconds(),
	)); err != nil {
		return err
	}
	return leitura(repositoriosDe(tx))
}
