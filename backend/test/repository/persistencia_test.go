//go:build integracao

package repository_test

import (
	"context"
	"testing"
	"time"

	"stacktrack/internal/adapter/repository"
	"stacktrack/internal/domain/board"
	"stacktrack/internal/domain/card"
	"stacktrack/internal/domain/coluna"
	"stacktrack/internal/domain/cor"
	"stacktrack/internal/domain/evento"
	"stacktrack/internal/domain/membro"
	"stacktrack/internal/domain/ordem"

	"github.com/google/uuid"
)

// cenario cria um usuário, um quadro e uma coluna reais no banco.
func cenario(t *testing.T) (boardID, colunaID, usuarioID string) {
	t.Helper()
	ctx := context.Background()

	usuarioID = uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO usuarios (id, nome, email, senha_hash, criado_em, atualizado_em)
		 VALUES ($1, 'Ana', $2, 'hash', now(), now())`,
		usuarioID, usuarioID+"@teste.dev"); err != nil {
		t.Fatalf("usuário: %v", err)
	}

	b, err := board.Novo(uuid.NewString(), "Quadro de teste")
	if err != nil {
		t.Fatalf("board: %v", err)
	}
	if err := repository.NovoBoardPostgres(pool).Salvar(context.Background(), b); err != nil {
		t.Fatalf("salvar board: %v", err)
	}
	m, _ := membro.Novo(b.ID, usuarioID, membro.PapelDono)
	if err := repository.NovoMembroPostgres(pool).Salvar(context.Background(), m); err != nil {
		t.Fatalf("membro: %v", err)
	}

	c, err := coluna.Nova(uuid.NewString(), b.ID, "A fazer", cor.Verde, ordem.ChaveInicial)
	if err != nil {
		t.Fatalf("coluna: %v", err)
	}
	if err := repository.NovoColunaPostgres(pool).Salvar(context.Background(), c); err != nil {
		t.Fatalf("salvar coluna: %v", err)
	}
	return b.ID, c.ID, usuarioID
}

// O teste que os fakes nunca puderam fazer: escreve e LÊ DE VOLTA do banco.
//
// Se um campo faltar no INSERT, no UPDATE ou em qualquer SELECT, ele volta
// zerado — e é exatamente assim que cards.prazo e boards.fundo passaram meses
// existindo em todas as camadas sem nunca serem gravados.
func TestTodoCampoDoCardSobreviveAoBanco(t *testing.T) {
	_, colunaID, _ := cenario(t)
	repo := repository.NovoCardPostgres(pool)

	prazo := time.Date(2030, 3, 14, 15, 9, 0, 0, time.UTC)
	c, err := card.Novo(uuid.NewString(), colunaID, "Migração", "com **markdown**", cor.Roxo, ordem.ChaveInicial)
	if err != nil {
		t.Fatalf("card: %v", err)
	}
	c.DefinirPrazo(&prazo)

	if err := repo.Salvar(context.Background(), c); err != nil {
		t.Fatalf("salvar: %v", err)
	}

	lido, err := repo.BuscarPorID(context.Background(), c.ID)
	if err != nil || lido == nil {
		t.Fatalf("buscar: %v", err)
	}

	if lido.Titulo != c.Titulo {
		t.Errorf("titulo = %q", lido.Titulo)
	}
	if lido.Descricao != c.Descricao {
		t.Errorf("descricao = %q", lido.Descricao)
	}
	if lido.Cor != cor.Roxo {
		t.Errorf("cor = %q — o campo não sobreviveu ao SQL", lido.Cor)
	}
	if lido.Prazo == nil || !lido.Prazo.Equal(prazo) {
		t.Errorf("prazo = %v — o campo não sobreviveu ao SQL", lido.Prazo)
	}
	if lido.Chave != ordem.ChaveInicial {
		t.Errorf("chave = %q — o campo não sobreviveu ao SQL", lido.Chave)
	}
	if lido.Version != c.Version {
		t.Errorf("version = %d, esperado %d", lido.Version, c.Version)
	}
}

// O mesmo para o fundo do quadro, que já sumiu uma vez.
func TestFundoDoQuadroSobreviveAoBanco(t *testing.T) {
	boardID, _, _ := cenario(t)
	repo := repository.NovoBoardPostgres(pool)

	b, err := repo.BuscarPorID(context.Background(), boardID)
	if err != nil || b == nil {
		t.Fatalf("buscar: %v", err)
	}
	if err := b.DefinirFundo("oceano"); err != nil {
		t.Fatalf("definir fundo: %v", err)
	}
	if err := repo.DefinirFundo(context.Background(), b.ID, b.Fundo, b.AtualizadoEm); err != nil {
		t.Fatalf("definir fundo: %v", err)
	}

	relido, err := repo.BuscarPorID(context.Background(), boardID)
	if err != nil || relido == nil {
		t.Fatalf("reler: %v", err)
	}
	if relido.Fundo != "oceano" {
		t.Errorf("fundo = %q — o UPDATE ou o SELECT não levaram o campo", relido.Fundo)
	}
}

// O bloqueio otimista contra o banco de verdade: é aqui que o `WHERE version`
// prova que funciona. O fake repete a regra, mas quem escreve o SQL errado não
// é o fake.
func TestOBancoRecusaEscritaComVersaoDefasada(t *testing.T) {
	_, colunaID, _ := cenario(t)
	repo := repository.NovoCardPostgres(pool)

	c, _ := card.Novo(uuid.NewString(), colunaID, "Disputado", "", "", ordem.ChaveInicial)
	if err := repo.Salvar(context.Background(), c); err != nil {
		t.Fatalf("salvar: %v", err)
	}

	// Duas cópias da mesma linha, como duas requisições que leram juntas.
	copiaAna, _ := repo.BuscarPorID(context.Background(), c.ID)
	copiaBob, _ := repo.BuscarPorID(context.Background(), c.ID)

	_ = copiaAna.Editar("Da ana", "", "")
	if err := repo.Atualizar(context.Background(), copiaAna); err != nil {
		t.Fatalf("a primeira escrita devia passar: %v", err)
	}

	_ = copiaBob.Editar("Do bob", "", "")
	if err := repo.Atualizar(context.Background(), copiaBob); err != card.ErrConflito {
		t.Errorf("erro = %v, esperado ErrConflito", err)
	}

	final, _ := repo.BuscarPorID(context.Background(), c.ID)
	if final.Titulo != "Da ana" {
		t.Errorf("titulo = %q — a escrita defasada sobrescreveu", final.Titulo)
	}
}

// O log de eventos: o seq tem de crescer, e o replay por revisão tem de
// devolver só o que veio depois do cursor, em ordem.
func TestLogDeEventosOrdenaEDevolveApenasOIntervaloPedido(t *testing.T) {
	boardID, _, usuarioID := cenario(t)
	log := repository.NovoEventoPostgres(pool)

	var seqs []int64
	for i := 1; i <= 3; i++ {
		e := evento.Novo(evento.ColunaCriada, boardID, usuarioID, map[string]string{"titulo": "coluna"})
		e.Revisao, e.Quantidade = int64(i), 1
		seq, err := log.Registrar(context.Background(), e)
		if err != nil {
			t.Fatalf("registrar: %v", err)
		}
		seqs = append(seqs, seq)
	}

	for i := 1; i < len(seqs); i++ {
		if seqs[i] <= seqs[i-1] {
			t.Fatalf("o seq não cresceu: %v", seqs)
		}
	}

	// Depois da revisão 1, só as duas seguintes.
	perdidos, err := log.DesdeRevisao(context.Background(), boardID, 1, 100)
	if err != nil {
		t.Fatalf("desde a revisão: %v", err)
	}
	if len(perdidos) != 2 {
		t.Fatalf("depois da revisão 1 vieram %d eventos, esperado 2", len(perdidos))
	}
	if perdidos[0].Revisao != 2 || perdidos[1].Revisao != 3 {
		t.Errorf("ordem errada: %v", perdidos)
	}
	if perdidos[0].Tipo != evento.ColunaCriada {
		t.Errorf("tipo = %q", perdidos[0].Tipo)
	}

	// E o payload sobreviveu ao JSONB.
	if perdidos[0].Dados == nil {
		t.Error("o payload voltou vazio do log")
	}

}

// O log não vaza entre quadros — a mesma fronteira dos eventos ao vivo.
func TestLogNaoVazaEntreQuadros(t *testing.T) {
	boardA, _, usuarioID := cenario(t)
	boardB, _, _ := cenario(t)
	log := repository.NovoEventoPostgres(pool)

	e := evento.Novo(evento.CardCriado, boardA, usuarioID, nil)
	e.Revisao, e.Quantidade = 1, 1
	if _, err := log.Registrar(context.Background(), e); err != nil {
		t.Fatalf("registrar: %v", err)
	}

	deB, err := log.DesdeRevisao(context.Background(), boardB, 0, 100)
	if err != nil {
		t.Fatalf("desde a revisão: %v", err)
	}
	if len(deB) != 0 {
		t.Errorf("o quadro B recebeu %d eventos do A", len(deB))
	}
}

// Apagar o quadro leva o log junto, pelo ON DELETE CASCADE. Sem isso, a tabela
// guardaria para sempre a história de quadros que não existem mais.
func TestApagarQuadroLevaOLogJunto(t *testing.T) {
	boardID, _, usuarioID := cenario(t)
	log := repository.NovoEventoPostgres(pool)

	if _, err := log.Registrar(context.Background(), evento.Novo(evento.CardCriado, boardID, usuarioID, nil)); err != nil {
		t.Fatalf("registrar: %v", err)
	}
	if err := repository.NovoBoardPostgres(pool).Apagar(context.Background(), boardID); err != nil {
		t.Fatalf("apagar: %v", err)
	}

	var quantos int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM board_events WHERE board_id = $1`, boardID).Scan(&quantos); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if quantos != 0 {
		t.Errorf("sobraram %d eventos de um quadro apagado", quantos)
	}
}

// A chave de ordenação contra um Postgres DE VERDADE.
//
// O que só o banco responde: se a chave é gravada e relida, e — o que mais
// importa — se o ORDER BY com COLLATE "C" ordena do jeito que o domínio assume.
// A ordenação de texto no Postgres depende da collation, e uma que ignore caixa
// ou trate acentos ordenaria diferente. Nenhum fake pegaria isso.
func TestChaveDeOrdemPersisteEOrdena(t *testing.T) {
	ctx := context.Background()
	boardID, colunaID, _ := cenario(t)
	repo := repository.NovoCardPostgres(pool)

	// Gravados fora de ordem: quem manda na leitura é a chave.
	for _, caso := range []struct{ titulo, chave string }{
		{"terceiro", "t"}, {"primeiro", "b"}, {"segundo", "n"},
	} {
		c, err := card.Novo(uuid.NewString(), colunaID, caso.titulo, "", "", caso.chave)
		if err != nil {
			t.Fatalf("montar %s: %v", caso.titulo, err)
		}
		if err := repo.Salvar(ctx, c); err != nil {
			t.Fatalf("salvar %s: %v", caso.titulo, err)
		}
	}

	cards, err := repo.ListarDoBoard(ctx, boardID)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(cards) != 3 {
		t.Fatalf("cards = %d", len(cards))
	}
	for i, esperado := range []string{"primeiro", "segundo", "terceiro"} {
		if cards[i].Titulo != esperado {
			t.Errorf("posição %d = %q, esperado %q — o ORDER BY da chave não ordenou",
				i, cards[i].Titulo, esperado)
		}
		if cards[i].Chave == "" {
			t.Errorf("a chave de %q não voltou do banco", cards[i].Titulo)
		}
	}
}
