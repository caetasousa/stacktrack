// As cotas de anexo: por card e por quadro.
package usecase_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	danexo "stacktrack/internal/domain/anexo"
	ucboard "stacktrack/internal/usecase/board"
)

// cotasPequenas encolhe os tetos para o teste caber na memória.
//
// Com o teto real (1 GiB), provar a cota do quadro exigiria gravar cem arquivos
// de 10 MiB — um gigabyte em RAM para exercitar uma comparação. Os números
// mudam; a lógica exercitada é exatamente a mesma.
func cotasPequenas(e *extras, porCard int, bytesPorBoard int64) {
	e.anexoUC.ComCotas(ucboard.Cotas{PorCard: porCard, BytesPorBoard: bytesPorBoard})
}

// anexarArquivo envia um arquivo do tamanho pedido.
func anexarArquivo(t *testing.T, e *extras, cardID, dono, nome string, tamanho int64) error {
	t.Helper()
	conteudo := bytes.Repeat([]byte("x"), int(tamanho))
	_, err := e.anexoUC.AnexarArquivo(context.Background(), cardID, dono, nome, bytes.NewReader(conteudo))
	return err
}

// Os padrões do produto continuam sendo os do domínio: um usecase construído
// sem dizer nada já limita, em vez de aceitar tudo até alguém configurá-lo.
func TestCotasPadraoSaoAsDoDominio(t *testing.T) {
	padrao := ucboard.CotasPadrao()
	if padrao.PorCard != danexo.MaximoDeAnexosPorCard {
		t.Errorf("PorCard = %d, esperado %d", padrao.PorCard, danexo.MaximoDeAnexosPorCard)
	}
	if padrao.BytesPorBoard != danexo.MaximoDeBytesPorBoard {
		t.Errorf("BytesPorBoard = %d, esperado %d", padrao.BytesPorBoard, danexo.MaximoDeBytesPorBoard)
	}
}

// O teto de anexos POR CARD. Um card é uma unidade de trabalho, não um
// repositório de arquivos — e o modal carrega todos os anexos dele de uma vez.
func TestCardRecusaAnexoAlemDoTeto(t *testing.T) {
	e := novoExtras()
	_, cardID := e.montar(t, "ana")
	const teto = 3
	cotasPequenas(e, teto, 0)

	for i := 0; i < teto; i++ {
		if _, err := e.anexoUC.AnexarLink(context.Background(), cardID, "ana",
			fmt.Sprintf("link %d", i), "https://exemplo.com"); err != nil {
			t.Fatalf("anexo %d falhou: %v", i, err)
		}
	}

	_, err := e.anexoUC.AnexarLink(context.Background(), cardID, "ana", "o excedente", "https://exemplo.com")
	if !errors.Is(err, danexo.ErrAnexosDemaisNoCard) {
		t.Errorf("erro = %v, esperado ErrAnexosDemaisNoCard", err)
	}
}

// O teto de BYTES por quadro é o que de fato protege o disco: o limite por
// arquivo não protege nada sozinho, porque cem arquivos de 10 MiB cabem no
// limite individual e enchem um gigabyte.
func TestQuadroRecusaArquivoQueEstouraACota(t *testing.T) {
	e := novoExtras()
	_, cardID := e.montar(t, "ana")
	cotasPequenas(e, 0, 1000)

	// Um arquivo que sozinho não cabe no quadro.
	err := anexarArquivo(t, e, cardID, "ana", "gigante.txt", 1001)
	if !errors.Is(err, danexo.ErrCotaDoQuadroExcedida) {
		t.Errorf("erro = %v, esperado ErrCotaDoQuadroExcedida", err)
	}
}

// A cota do quadro soma o que JÁ existe: o segundo arquivo é recusado por causa
// do primeiro, e não pelo próprio tamanho.
func TestCotaDoQuadroSomaOQueJaExiste(t *testing.T) {
	e := novoExtras()
	_, cardID := e.montar(t, "ana")
	cotasPequenas(e, 0, 1000)

	const metade = int64(500)
	if err := anexarArquivo(t, e, cardID, "ana", "um.txt", metade); err != nil {
		t.Fatalf("primeiro arquivo: %v", err)
	}
	if err := anexarArquivo(t, e, cardID, "ana", "dois.txt", metade); err != nil {
		t.Fatalf("segundo arquivo (ainda cabe): %v", err)
	}
	// O terceiro não cabe, embora cada um caiba sozinho.
	err := anexarArquivo(t, e, cardID, "ana", "tres.txt", metade)
	if !errors.Is(err, danexo.ErrCotaDoQuadroExcedida) {
		t.Errorf("erro = %v, esperado ErrCotaDoQuadroExcedida", err)
	}
}

// LINK não consome cota de disco: ele não ocupa byte nenhum no volume.
func TestLinkNaoConsomeACotaDeBytesDoQuadro(t *testing.T) {
	e := novoExtras()
	_, cardID := e.montar(t, "ana")
	cotasPequenas(e, 0, 1000)

	if _, err := e.anexoUC.AnexarLink(context.Background(), cardID, "ana", "um link", "https://exemplo.com"); err != nil {
		t.Fatalf("link: %v", err)
	}
	// Um arquivo que ocupa o quadro inteiro ainda cabe: o link não tirou espaço.
	if err := anexarArquivo(t, e, cardID, "ana", "cheio.txt", 1000); err != nil {
		t.Errorf("o link consumiu cota de disco: %v", err)
	}
}

// O arquivo RECUSADO não fica no disco. Ele é gravado antes da transação (o
// caso de uso precisa do tamanho e do hash), e a recusa tem de desfazer isso —
// senão a cota estourada viraria uma fonte permanente de lixo.
func TestArquivoRecusadoPelaCotaNaoFicaNoDisco(t *testing.T) {
	e := novoExtras()
	_, cardID := e.montar(t, "ana")
	cotasPequenas(e, 0, 1000)

	antes := e.armazem.Quantidade()
	if err := anexarArquivo(t, e, cardID, "ana", "gigante.txt", 1001); err == nil {
		t.Fatal("o arquivo devia ter sido recusado")
	}
	if depois := e.armazem.Quantidade(); depois != antes {
		t.Errorf("arquivos no armazém: antes %d, depois %d — o recusado ficou no disco", antes, depois)
	}
}
