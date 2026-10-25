package repository

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	demail "stacktrack/internal/domain/email"
	"stacktrack/internal/domain/notificacao"
	ucboard "stacktrack/internal/usecase/board"
	ucnotificacao "stacktrack/internal/usecase/notificacao"
)

// NotificacaoPostgres guarda os avisos pessoais. Serve a quatro pontas: aos
// casos de uso do quadro, que os criam dentro da transação do que os causou; ao
// sino de cada pessoa, que lista, conta e marca como lido; ao worker de email,
// que confere se o aviso ainda vale antes de mandar; e à faxina.
type NotificacaoPostgres struct {
	db consultante
}

// NovoNotificacaoPostgres cria o repositório ligado ao pool.
func NovoNotificacaoPostgres(pool Fonte) *NotificacaoPostgres {
	return &NotificacaoPostgres{db: pool}
}

// As duas perguntas que definem o que a pessoa vê, escritas UMA vez e usadas
// pela lista, pela contagem, pelo worker de email e pela faxina. Quatro cópias
// divergiriam no dia em que alguém mexesse numa só — e o sino contaria um aviso
// que a lista não mostra, ou o email sairia para algo que o sino já dá como
// encerrado.
var (
	// sqlVisivel: a pessoa ainda pode ver o aviso. O de convite exige a conta
	// com o email CONFIRMADO, e o mesmo email do convite — a regra do painel,
	// pela mesma razão: sem a prova, cadastrar o email de outra pessoa bastaria
	// para ver os convites dela. Os demais exigem que ela ainda participe do
	// quadro: quem saiu não vê mais nada dele.
	sqlVisivel = fmt.Sprintf(`(
	    (n.tipo = '%[1]s' AND EXISTS (
	        SELECT 1 FROM usuarios uv JOIN convites_board cvv ON cvv.id = n.convite_id
	         WHERE uv.id = n.usuario_id AND uv.email_confirmado_em IS NOT NULL AND cvv.email = uv.email))
	    OR (n.tipo <> '%[1]s' AND EXISTS (
	        SELECT 1 FROM board_membros mv WHERE mv.board_id = n.board_id AND mv.usuario_id = n.usuario_id)))`,
		notificacao.TipoConvite)

	// sqlVigente: o fato ainda vale, lido do estado de AGORA, que é sempre o $1
	// de quem usa o trecho. Calculado, e não gravado: uma marca de "não vale
	// mais" teria de ser escrita por cada caminho que desfaz alguma coisa — e
	// desatribuir, sair do quadro, trocar o prazo e recusar o convite já são
	// quatro.
	//
	// A menção não tem condição: retirá-la do comentário apaga o aviso, e
	// apagar o comentário o leva pelo CASCADE.
	sqlVigente = fmt.Sprintf(`(CASE n.tipo
	    WHEN '%[1]s' THEN EXISTS (
	        SELECT 1 FROM convites_board cvg WHERE cvg.id = n.convite_id
	           AND cvg.aceito_em IS NULL AND cvg.revogado_em IS NULL AND cvg.expira_em > $1)
	    WHEN '%[2]s' THEN EXISTS (
	        SELECT 1 FROM card_responsaveis rg WHERE rg.card_id = n.card_id AND rg.usuario_id = n.usuario_id)
	    WHEN '%[3]s' THEN EXISTS (
	        SELECT 1 FROM card_responsaveis rg JOIN cards kg ON kg.id = rg.card_id
	         WHERE rg.card_id = n.card_id AND rg.usuario_id = n.usuario_id AND kg.prazo = n.prazo)
	    WHEN '%[4]s' THEN TRUE
	    ELSE FALSE END)`,
		notificacao.TipoConvite, notificacao.TipoAtribuicao, notificacao.TipoPrazo, notificacao.TipoMencao)
)

// camposDoItem e origemDoItem montam o aviso como a lista o mostra. Os títulos
// e o trecho do comentário vêm do estado de agora — ver ucnotificacao.Item.
const (
	camposDoItem = `n.id, n.tipo, n.board_id, COALESCE(n.card_id::text, ''), COALESCE(n.convite_id::text, ''),
	    COALESCE(a.nome, ''), b.titulo, COALESCE(k.titulo, ''), COALESCE(left(cm.texto, 200), ''),
	    n.prazo, n.criada_em, n.lida_em,
	    cv.aceito_em, cv.recusado_em, cv.revogado_em, cv.expira_em`
	origemDoItem = `FROM notificacoes n
	    JOIN boards b ON b.id = n.board_id
	    LEFT JOIN usuarios a ON a.id = n.autor_id
	    LEFT JOIN cards k ON k.id = n.card_id
	    LEFT JOIN comentarios cm ON cm.id = n.comentario_id
	    LEFT JOIN convites_board cv ON cv.id = n.convite_id`
)

// tamanhoDoTrecho é até onde o começo do comentário aparece na lista.
const tamanhoDoTrecho = 140

// --- quem causa o aviso -------------------------------------------------------

// Criar grava o aviso, e devolve false quando a chave já existia.
//
// ON CONFLICT, e não o erro capturado: dentro da transação da atribuição, uma
// violação de unicidade abortaria a transação inteira — e a atribuição com ela.
func (r *NotificacaoPostgres) Criar(ctx context.Context, n *notificacao.Notificacao) (bool, error) {
	tag, err := r.db.Exec(ctx,
		`INSERT INTO notificacoes (id, usuario_id, tipo, board_id, card_id, comentario_id, convite_id,
		                           autor_id, prazo, chave, criada_em, lida_em)
		 VALUES ($1, $2, $3, $4, NULLIF($5, '')::uuid, NULLIF($6, '')::uuid, NULLIF($7, '')::uuid,
		         NULLIF($8, '')::uuid, $9, $10, $11, $12)
		 ON CONFLICT (chave) DO NOTHING`,
		n.ID, n.UsuarioID, string(n.Tipo), n.BoardID, n.CardID, n.ComentarioID, n.ConviteID,
		n.AutorID, n.Prazo, n.Chave, n.CriadaEm, n.LidaEm,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// ExisteNaoLida responde se a pessoa tem, sem ler, um aviso daquele tipo sobre
// o card.
func (r *NotificacaoPostgres) ExisteNaoLida(ctx context.Context, usuarioID string, tipo notificacao.Tipo, cardID string) (bool, error) {
	var existe bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM notificacoes
		                 WHERE usuario_id = $1 AND tipo = $2 AND card_id = $3 AND lida_em IS NULL)`,
		usuarioID, string(tipo), cardID,
	).Scan(&existe)
	return existe, err
}

// MencionadosNoComentario devolve quem tem aviso de menção no comentário.
func (r *NotificacaoPostgres) MencionadosNoComentario(ctx context.Context, comentarioID string) ([]string, error) {
	return r.usuarios(ctx,
		`SELECT usuario_id FROM notificacoes WHERE comentario_id = $1 AND tipo = $2 ORDER BY criada_em`,
		comentarioID, string(notificacao.TipoMencao))
}

// RemoverMencoes apaga os avisos de menção daquelas pessoas no comentário. O
// email que ainda não saiu vai junto, pelo CASCADE de email_outbox.
func (r *NotificacaoPostgres) RemoverMencoes(ctx context.Context, comentarioID string, usuarioIDs []string) error {
	if len(usuarioIDs) == 0 {
		return nil
	}
	_, err := r.db.Exec(ctx,
		`DELETE FROM notificacoes WHERE comentario_id = $1 AND tipo = $2 AND usuario_id = ANY($3::uuid[])`,
		comentarioID, string(notificacao.TipoMencao), usuarioIDs,
	)
	return err
}

// RemoverDoQuadro apaga os avisos de uma pessoa sobre o quadro.
func (r *NotificacaoPostgres) RemoverDoQuadro(ctx context.Context, boardID, usuarioID string) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM notificacoes WHERE board_id = $1 AND usuario_id = $2`, boardID, usuarioID,
	)
	return err
}

// DestinatariosDoQuadro devolve quem tem algum aviso sobre o quadro.
func (r *NotificacaoPostgres) DestinatariosDoQuadro(ctx context.Context, boardID string) ([]string, error) {
	return r.usuarios(ctx, `SELECT DISTINCT usuario_id FROM notificacoes WHERE board_id = $1`, boardID)
}

// PrazosParaAvisar devolve quem responde por um card cujo prazo cai em
// (agora, ate] e ainda não foi avisado daquela data. Só quem participa do
// quadro: a responsabilidade de quem saiu já foi levada junto com ele, e o
// JOIN é a segunda rede.
func (r *NotificacaoPostgres) PrazosParaAvisar(ctx context.Context, agora, ate time.Time, limite int) ([]ucboard.PrazoParaAvisar, error) {
	linhas, err := r.db.Query(ctx,
		`SELECT k.id, col.board_id, rp.usuario_id, k.prazo
		   FROM cards k
		   JOIN colunas col ON col.id = k.coluna_id
		   JOIN card_responsaveis rp ON rp.card_id = k.id
		   JOIN board_membros m ON m.board_id = col.board_id AND m.usuario_id = rp.usuario_id
		  WHERE k.prazo > $1 AND k.prazo <= $2
		    AND NOT EXISTS (
		        SELECT 1 FROM notificacoes n
		         WHERE n.card_id = k.id AND n.usuario_id = rp.usuario_id AND n.tipo = $3 AND n.prazo = k.prazo)
		  ORDER BY k.prazo, k.id, rp.usuario_id
		  LIMIT $4`,
		agora, ate, string(notificacao.TipoPrazo), limite,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	prazos := make([]ucboard.PrazoParaAvisar, 0)
	for linhas.Next() {
		var p ucboard.PrazoParaAvisar
		if err := linhas.Scan(&p.CardID, &p.BoardID, &p.UsuarioID, &p.Prazo); err != nil {
			return nil, err
		}
		prazos = append(prazos, p)
	}
	return prazos, linhas.Err()
}

// --- o sino ---------------------------------------------------------------

// Listar devolve até `limite` avisos visíveis da pessoa, do mais novo para o
// mais antigo, começando depois do aviso `antesDe`.
//
// O cursor é o aviso, e não um número de página: entre uma página e outra
// chegam avisos novos, e com OFFSET a segunda página repetiria o fim da
// primeira.
func (r *NotificacaoPostgres) Listar(ctx context.Context, usuarioID, antesDe string, limite int, agora time.Time) ([]ucnotificacao.Item, error) {
	linhas, err := r.db.Query(ctx,
		`SELECT `+camposDoItem+`, `+sqlVigente+`
		 `+origemDoItem+`
		  WHERE n.usuario_id = $2 AND `+sqlVisivel+`
		    AND ($3 = '' OR (n.criada_em, n.id) < (
		        SELECT p.criada_em, p.id FROM notificacoes p WHERE p.id::text = $3 AND p.usuario_id = $2))
		  ORDER BY n.criada_em DESC, n.id DESC
		  LIMIT $4`,
		agora, usuarioID, antesDe, limite,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	itens := make([]ucnotificacao.Item, 0)
	for linhas.Next() {
		item, err := lerItem(linhas)
		if err != nil {
			return nil, err
		}
		itens = append(itens, item)
	}
	return itens, linhas.Err()
}

// Buscar devolve (nil, nil) quando o aviso não existe ou não é visível para a
// pessoa.
func (r *NotificacaoPostgres) Buscar(ctx context.Context, id, usuarioID string, agora time.Time) (*ucnotificacao.Item, error) {
	linhas, err := r.db.Query(ctx,
		`SELECT `+camposDoItem+`, `+sqlVigente+`
		 `+origemDoItem+`
		  WHERE n.id = $3 AND n.usuario_id = $2 AND `+sqlVisivel,
		agora, usuarioID, id,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	if !linhas.Next() {
		return nil, linhas.Err()
	}
	item, err := lerItem(linhas)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// ContarNaoLidas conta os avisos não lidos, visíveis e que ainda valem.
func (r *NotificacaoPostgres) ContarNaoLidas(ctx context.Context, usuarioID string, agora time.Time) (int, error) {
	var n int
	err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM notificacoes n
		  WHERE n.usuario_id = $2 AND n.lida_em IS NULL AND `+sqlVisivel+` AND `+sqlVigente,
		agora, usuarioID,
	).Scan(&n)
	return n, err
}

// MarcarLida marca o aviso e cancela, no MESMO comando, o email dele que ainda
// não saiu. Num comando só para não haver o estado intermediário em que o aviso
// está lido e o email continua na fila: quem já viu no sistema receberia o
// email do mesmo jeito.
//
// Marcar o que já foi lido mantém a hora da primeira leitura — e ainda devolve
// true: a pessoa pediu "lida", e lida ela está.
func (r *NotificacaoPostgres) MarcarLida(ctx context.Context, id, usuarioID string, em time.Time) (bool, error) {
	var marcadas int
	err := r.db.QueryRow(ctx,
		`WITH lida AS (
		     UPDATE notificacoes n SET lida_em = COALESCE(n.lida_em, $3)
		      WHERE n.id = $1 AND n.usuario_id = $2 AND `+sqlVisivel+`
		      RETURNING n.id
		 ), cancelado AS (
		     UPDATE email_outbox SET estado = $4, payload_cifrado = NULL
		      WHERE notificacao_id IN (SELECT id FROM lida) AND estado = $5
		 )
		 SELECT count(*) FROM lida`,
		id, usuarioID, em, string(demail.EstadoCancelado), string(demail.EstadoPendente),
	).Scan(&marcadas)
	return marcadas > 0, err
}

// MarcarTodasLidas marca todos os avisos não lidos da pessoa e cancela os
// emails deles que ainda não saíram.
func (r *NotificacaoPostgres) MarcarTodasLidas(ctx context.Context, usuarioID string, em time.Time) error {
	_, err := r.db.Exec(ctx,
		`WITH lidas AS (
		     UPDATE notificacoes SET lida_em = $2
		      WHERE usuario_id = $1 AND lida_em IS NULL
		      RETURNING id
		 )
		 UPDATE email_outbox SET estado = $3, payload_cifrado = NULL
		  WHERE notificacao_id IN (SELECT id FROM lidas) AND estado = $4`,
		usuarioID, em, string(demail.EstadoCancelado), string(demail.EstadoPendente),
	)
	return err
}

// --- o worker de email ------------------------------------------------------

// AindaVale responde, na hora de o email sair, se ainda há o que avisar: o
// aviso existe, não foi lido, a pessoa ainda o vê e o fato ainda vale.
func (r *NotificacaoPostgres) AindaVale(ctx context.Context, notificacaoID string, agora time.Time) (bool, error) {
	var vale bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM notificacoes n
		                 WHERE n.id = $2 AND n.lida_em IS NULL AND `+sqlVisivel+` AND `+sqlVigente+`)`,
		agora, notificacaoID,
	).Scan(&vale)
	return vale, err
}

// --- a faxina ---------------------------------------------------------------

// Nome identifica esta limpeza na faxina.
func (r *NotificacaoPostgres) Nome() string { return "notificacoes" }

// LimparLote apaga os avisos que já cumpriram o papel, respeitando a retenção
// que o dono de cada quadro escolheu:
//
//   - o LIDO há mais dias do que a retenção do quadro;
//   - o NÃO LIDO que deixou de valer (ou de ser visível) e é mais velho que a
//     retenção — ninguém vai ler um aviso de atribuição desfeita há meses;
//   - qualquer um mais velho que a idade máxima, lido ou não.
//
// O não lido que ainda vale fica até a idade máxima: ele ainda pede atenção.
// O quadro sem escolha usa o padrão do domínio, passado como parâmetro — o
// padrão não é do banco.
func (r *NotificacaoPostgres) LimparLote(ctx context.Context, limite int) (int64, error) {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM notificacoes
		  WHERE ctid IN (
		    SELECT n.ctid FROM notificacoes n JOIN boards b ON b.id = n.board_id
		     WHERE n.criada_em < $1::timestamptz - make_interval(days => $3)
		        OR (n.lida_em IS NOT NULL
		            AND n.lida_em < $1::timestamptz - make_interval(days => COALESCE(b.retencao_notificacoes_dias, $2)))
		        OR (n.lida_em IS NULL
		            AND n.criada_em < $1::timestamptz - make_interval(days => COALESCE(b.retencao_notificacoes_dias, $2))
		            AND NOT (`+sqlVisivel+` AND `+sqlVigente+`))
		     LIMIT $4)`,
		time.Now(), notificacao.RetencaoPadraoDias, notificacao.RetencaoMaximaDias, limite,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// --- auxiliares -------------------------------------------------------------

func (r *NotificacaoPostgres) usuarios(ctx context.Context, sql string, args ...any) ([]string, error) {
	linhas, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	ids := make([]string, 0)
	for linhas.Next() {
		var id string
		if err := linhas.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, linhas.Err()
}

type linhaDeItem interface {
	Scan(dest ...any) error
}

func lerItem(linha linhaDeItem) (ucnotificacao.Item, error) {
	var i ucnotificacao.Item
	var tipo, trecho string
	err := linha.Scan(&i.ID, &tipo, &i.BoardID, &i.CardID, &i.ConviteID,
		&i.AutorNome, &i.TituloQuadro, &i.TituloCard, &trecho,
		&i.Prazo, &i.CriadaEm, &i.LidaEm,
		&i.ConviteAceitoEm, &i.ConviteRecusadoEm, &i.ConviteRevogadoEm, &i.ConviteExpiraEm,
		&i.Vigente)
	i.Tipo = notificacao.Tipo(tipo)
	i.Trecho = encurtarTrecho(trecho)
	return i, err
}

// encurtarTrecho corta o começo do comentário numa fronteira de caractere e
// junta as quebras de linha: a lista mostra uma linha só.
func encurtarTrecho(texto string) string {
	texto = strings.Join(strings.Fields(texto), " ")
	if utf8.RuneCountInString(texto) <= tamanhoDoTrecho {
		return texto
	}
	return string([]rune(texto)[:tamanhoDoTrecho-1]) + "…"
}
