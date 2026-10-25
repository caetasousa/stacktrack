//go:build integracao

// Aplicar a etiqueta e atribuir o responsável são idempotentes — repetir não é
// erro, porque o resultado pretendido já vale. A promessa precisa valer DENTRO
// da unidade de trabalho, que é onde as duas rodam em produção.
//
// Como isto quebra:
//
//	D1 o repositório engole a violação da chave primária, mas o PostgreSQL já
//	   abortou a transação no INSERT recusado: o comando seguinte (o evento do
//	   quadro) falha com "current transaction is aborted", e o duplo clique, ou
//	   a segunda aba, recebe erro interno em vez de "já vale".
package repository_test

import (
	"context"
	"testing"
	"time"

	"stacktrack/internal/adapter/repository"
	"stacktrack/internal/domain/etiqueta"
	"stacktrack/internal/domain/membro"
	ucboard "stacktrack/internal/usecase/board"
)

func TestD1AtribuirDeNovoDentroDaTransacaoNaoEhErro(t *testing.T) {
	ctx := context.Background()
	boardID, colunaID, dona := cenario(t)
	bruno := usuarioDeTeste(t, "Bruno")
	m, _ := membro.Novo(boardID, bruno, membro.PapelEditor)
	if err := repository.NovoMembroPostgres(pool).Salvar(ctx, m); err != nil {
		t.Fatal(err)
	}
	cardID := cardDeTeste(t, colunaID, "Card")

	uc := ucboard.NovoResponsavelUseCase(repository.NovoMembroPostgres(pool), repository.NovoColunaPostgres(pool),
		repository.NovoCardPostgres(pool), repository.NovoResponsavelPostgres(pool))
	uc.ComEscritaAtomica(novaUnidade(3 * time.Second))
	uc.ComRegistro(repository.NovoEventoPostgres(pool))

	for i := 1; i <= 2; i++ {
		if err := uc.Atribuir(ctx, cardID, bruno, dona); err != nil {
			t.Fatalf("D1: atribuição %d: %v", i, err)
		}
	}
}

func TestD1AplicarAEtiquetaDeNovoDentroDaTransacaoNaoEhErro(t *testing.T) {
	ctx := context.Background()
	boardID, colunaID, dona := cenario(t)
	cardID := cardDeTeste(t, colunaID, "Card")

	etiquetas := repository.NovoEtiquetaPostgres(pool)
	uc := ucboard.NovoEtiquetaUseCase(repository.NovoMembroPostgres(pool), repository.NovoColunaPostgres(pool),
		repository.NovoCardPostgres(pool), etiquetas)
	uc.ComEscritaAtomica(novaUnidade(3 * time.Second))
	uc.ComRegistro(repository.NovoEventoPostgres(pool))

	e, err := uc.Criar(ctx, boardID, dona, "Urgente", etiqueta.CorVermelho)
	if err != nil {
		t.Fatalf("criar etiqueta: %v", err)
	}
	for i := 1; i <= 2; i++ {
		if err := uc.Aplicar(ctx, cardID, e.ID, dona); err != nil {
			t.Fatalf("D1: aplicação %d: %v", i, err)
		}
	}
}
