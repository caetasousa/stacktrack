package repository

import (
	"context"
	"errors"
	"time"

	"stacktrack/internal/domain/board"
	"stacktrack/internal/domain/membro"
	ucboard "stacktrack/internal/usecase/board"

	"github.com/jackc/pgx/v5"
)

// BoardPostgres persiste quadros no PostgreSQL.
type BoardPostgres struct {
	db consultante
}

// NovoBoardPostgres cria o repositório de quadros sobre o pool informado.
func NovoBoardPostgres(pool Fonte) *BoardPostgres {
	return &BoardPostgres{db: pool}
}

// Salvar persiste um quadro novo.
func (r *BoardPostgres) Salvar(ctx context.Context, b *board.Board) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO boards (id, titulo, fundo, criado_em, atualizado_em)
		 VALUES ($1, $2, $3, $4, $5)`,
		b.ID, b.Titulo, vazioParaNulo(b.Fundo), b.CriadoEm, b.AtualizadoEm,
	)
	return err
}

// Renomear grava SÓ o título.
//
// Comando estreito, e não um `Atualizar` que grava o agregado inteiro. A
// diferença aparece na concorrência: com o UPDATE largo, quem renomeia o quadro
// grava também o fundo que leu há dez segundos, desfazendo em silêncio a troca
// de fundo que outra pessoa fez nesse meio-tempo. Nenhum erro, nenhum conflito
// — a mudança some.
//
// Escrever só a coluna que mudou faz duas edições de campos DIFERENTES
// conviverem sem se atropelar, que é o comportamento que a pessoa espera. Duas
// edições do MESMO campo continuam sendo "a última vence", e aí a última
// realmente é a última.
func (r *BoardPostgres) Renomear(ctx context.Context, id, titulo string, em time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE boards SET titulo = $2, atualizado_em = $3 WHERE id = $1`,
		id, titulo, em,
	)
	return err
}

// DefinirFundo grava SÓ o fundo. Ver Renomear para o porquê de ser estreito.
func (r *BoardPostgres) DefinirFundo(ctx context.Context, id, fundo string, em time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE boards SET fundo = $2, atualizado_em = $3 WHERE id = $1`,
		id, vazioParaNulo(fundo), em,
	)
	return err
}

// BuscarPorID retorna (quadro, nil) quando encontra e (nil, nil) quando não existe.
func (r *BoardPostgres) BuscarPorID(ctx context.Context, id string) (*board.Board, error) {
	var b board.Board
	var fundo *string
	var revisao *int64
	err := r.db.QueryRow(ctx,
		`SELECT id, titulo, fundo, criado_em, atualizado_em, revisao FROM boards WHERE id = $1`, id,
	).Scan(&b.ID, &b.Titulo, &fundo, &b.CriadoEm, &b.AtualizadoEm, &revisao)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.Fundo = valorOuVazio(fundo)
	// NULL é quadro anterior à migration da revisão, ou quadro sem mutação
	// nenhuma. Zero é a leitura certa dos dois — e é a mesma que o COALESCE do
	// incremento usa.
	if revisao != nil {
		b.Revisao = *revisao
	}
	return &b, nil
}

// Apagar remove o quadro. Colunas, cards e vínculos vão junto pelo
// ON DELETE CASCADE declarado no schema.
func (r *BoardPostgres) Apagar(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM boards WHERE id = $1`, id)
	return err
}

// ListarDoUsuario devolve os quadros de que o usuário participa, com o papel
// dele em cada um, do mais recente para o mais antigo.
//
// A consulta parte de board_membros: assim é impossível uma listagem trazer
// quadro alheio, mesmo que alguém esqueça a checagem no usecase.
func (r *BoardPostgres) ListarDoUsuario(ctx context.Context, usuarioID string) ([]ucboard.Resumo, error) {
	linhas, err := r.db.Query(ctx,
		`SELECT b.id, b.titulo, b.fundo, b.criado_em, b.atualizado_em, m.papel
		 FROM board_membros m
		 JOIN boards b ON b.id = m.board_id
		 WHERE m.usuario_id = $1
		 ORDER BY b.criado_em DESC`, usuarioID,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	resumos := make([]ucboard.Resumo, 0)
	for linhas.Next() {
		var r ucboard.Resumo
		var papel string
		var fundo *string
		if err := linhas.Scan(&r.Board.ID, &r.Board.Titulo, &fundo, &r.Board.CriadoEm, &r.Board.AtualizadoEm, &papel); err != nil {
			return nil, err
		}
		r.Board.Fundo = valorOuVazio(fundo)
		r.Papel = membro.Papel(papel)
		resumos = append(resumos, r)
	}
	return resumos, linhas.Err()
}

// MembroPostgres persiste os vínculos entre pessoas e quadros.
type MembroPostgres struct {
	db consultante
}

// NovoMembroPostgres cria o repositório de vínculos sobre o pool informado.
func NovoMembroPostgres(pool Fonte) *MembroPostgres {
	return &MembroPostgres{db: pool}
}

// Salvar persiste um vínculo novo.
func (r *MembroPostgres) Salvar(ctx context.Context, m *membro.Membro) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO board_membros (board_id, usuario_id, papel, criado_em) VALUES ($1, $2, $3, $4)`,
		m.BoardID, m.UsuarioID, string(m.Papel), m.CriadoEm,
	)
	return err
}

// Buscar retorna (vínculo, nil) quando a pessoa participa do quadro e
// (nil, nil) quando não participa. É a consulta que responde a toda
// autorização da aplicação.
func (r *MembroPostgres) Buscar(ctx context.Context, boardID, usuarioID string) (*membro.Membro, error) {
	var m membro.Membro
	var papel string
	err := r.db.QueryRow(ctx,
		`SELECT board_id, usuario_id, papel, criado_em
		 FROM board_membros WHERE board_id = $1 AND usuario_id = $2`, boardID, usuarioID,
	).Scan(&m.BoardID, &m.UsuarioID, &papel, &m.CriadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.Papel = membro.Papel(papel)
	return &m, nil
}

// Atualizar grava o papel de um vínculo existente.
func (r *MembroPostgres) Atualizar(ctx context.Context, m *membro.Membro) error {
	_, err := r.db.Exec(ctx,
		`UPDATE board_membros SET papel = $3 WHERE board_id = $1 AND usuario_id = $2`,
		m.BoardID, m.UsuarioID, string(m.Papel),
	)
	return err
}

// Remover apaga o vínculo. Remover quem não participa não é erro.
func (r *MembroPostgres) Remover(ctx context.Context, boardID, usuarioID string) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM board_membros WHERE board_id = $1 AND usuario_id = $2`, boardID, usuarioID,
	)
	return err
}

// Todos devolve só os vínculos do quadro — é o que a regra do último dono
// precisa enxergar, sem pagar o JOIN com usuarios.
func (r *MembroPostgres) Todos(ctx context.Context, boardID string) ([]membro.Membro, error) {
	linhas, err := r.db.Query(ctx,
		`SELECT board_id, usuario_id, papel, criado_em FROM board_membros WHERE board_id = $1`,
		boardID,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	membros := make([]membro.Membro, 0)
	for linhas.Next() {
		var m membro.Membro
		var papel string
		if err := linhas.Scan(&m.BoardID, &m.UsuarioID, &papel, &m.CriadoEm); err != nil {
			return nil, err
		}
		m.Papel = membro.Papel(papel)
		membros = append(membros, m)
	}
	return membros, linhas.Err()
}

// Participantes devolve os vínculos já com nome e email, numa consulta só —
// buscar a pessoa de cada vínculo seria um N+1 na tela de membros.
// O dono vem primeiro, e depois por ordem de entrada.
func (r *MembroPostgres) Participantes(ctx context.Context, boardID string) ([]ucboard.Participante, error) {
	linhas, err := r.db.Query(ctx,
		`SELECT m.usuario_id, u.nome, u.email, m.papel, m.criado_em
		 FROM board_membros m
		 JOIN usuarios u ON u.id = m.usuario_id
		 WHERE m.board_id = $1
		 ORDER BY (m.papel <> 'dono'), m.criado_em`, boardID,
	)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()

	participantes := make([]ucboard.Participante, 0)
	for linhas.Next() {
		var p ucboard.Participante
		var papel string
		if err := linhas.Scan(&p.UsuarioID, &p.Nome, &p.Email, &papel, &p.CriadoEm); err != nil {
			return nil, err
		}
		p.Papel = membro.Papel(papel)
		participantes = append(participantes, p)
	}
	return participantes, linhas.Err()
}
