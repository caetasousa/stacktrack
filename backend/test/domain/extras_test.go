package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"kanbango/internal/domain/anexo"
	"kanbango/internal/domain/board"
	"kanbango/internal/domain/card"
	"kanbango/internal/domain/checklist"
	"kanbango/internal/domain/etiqueta"
)

// --- etiqueta -------------------------------------------------------------

func TestEtiquetaRecusaCorForaDaPaleta(t *testing.T) {
	if _, err := etiqueta.Nova("e-1", "b-1", "Urgente", etiqueta.Cor("neon"), 1); !errors.Is(err, etiqueta.ErrCorInvalida) {
		t.Errorf("erro = %v, esperado ErrCorInvalida", err)
	}
	if _, err := etiqueta.Nova("e-1", "b-1", "Urgente", etiqueta.CorVermelho, 1); err != nil {
		t.Errorf("cor da paleta devia ser aceita: %v", err)
	}
}

// O Trello aceita etiqueta só de cor. Aqui não: sem nome, quem lê o quadro
// precisa decorar o que cada cor significa.
func TestEtiquetaExigeNome(t *testing.T) {
	if _, err := etiqueta.Nova("e-1", "b-1", "   ", etiqueta.CorAzul, 1); !errors.Is(err, etiqueta.ErrNomeObrigatorio) {
		t.Errorf("erro = %v, esperado ErrNomeObrigatorio", err)
	}
	if _, err := etiqueta.Nova("e-1", "b-1", strings.Repeat("a", etiqueta.TamanhoMaximoNome+1), etiqueta.CorAzul, 1); !errors.Is(err, etiqueta.ErrNomeLongo) {
		t.Errorf("erro = %v, esperado ErrNomeLongo", err)
	}
}

func TestEditarEtiquetaMantemOEstadoQuandoAEntradaENaoValida(t *testing.T) {
	e, _ := etiqueta.Nova("e-1", "b-1", "Urgente", etiqueta.CorVermelho, 1)

	if err := e.Editar("Calma", etiqueta.Cor("neon")); !errors.Is(err, etiqueta.ErrCorInvalida) {
		t.Fatalf("erro = %v", err)
	}
	if e.Nome != "Urgente" || e.Cor != etiqueta.CorVermelho {
		t.Errorf("etiqueta = %+v, devia ter ficado como estava", e)
	}
}

// --- checklist ------------------------------------------------------------

func TestItemNasceDesmarcado(t *testing.T) {
	item, err := checklist.NovoItem("i-1", "c-1", "Escrever o teste", 1)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if item.Concluido {
		t.Error("item novo nasce desmarcado — é por isso que a coluna no banco não tem DEFAULT")
	}
}

func TestProgressoContaOsConcluidos(t *testing.T) {
	itens := []checklist.Item{
		{Concluido: true}, {Concluido: false}, {Concluido: true}, {Concluido: false}, {Concluido: false},
	}

	concluidos, total := checklist.Progresso(itens)

	if concluidos != 2 || total != 5 {
		t.Errorf("progresso = %d/%d, esperado 2/5", concluidos, total)
	}
}

func TestProgressoDeListaVazia(t *testing.T) {
	concluidos, total := checklist.Progresso(nil)
	if concluidos != 0 || total != 0 {
		t.Errorf("progresso = %d/%d, esperado 0/0", concluidos, total)
	}
}

func TestMarcarItemNaoMexeNoTexto(t *testing.T) {
	item, _ := checklist.NovoItem("i-1", "c-1", "Escrever o teste", 1)

	item.Marcar(true)

	if !item.Concluido || item.Texto != "Escrever o teste" {
		t.Errorf("item = %+v", item)
	}
}

// --- anexo ----------------------------------------------------------------

// javascript: e data: guardados como link virariam execução de script na tela
// de quem clicasse.
func TestAnexoDeLinkAceitaSomenteHTTP(t *testing.T) {
	recusados := []string{
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"file:///etc/passwd",
		"//exemplo.com/sem-esquema",
		"nao é url",
		"",
	}

	for _, endereco := range recusados {
		if _, err := anexo.NovoLink("a-1", "c-1", "", endereco, "u-1"); !errors.Is(err, anexo.ErrURLInvalida) {
			t.Errorf("%q: erro = %v, esperado ErrURLInvalida", endereco, err)
		}
	}

	for _, endereco := range []string{"http://exemplo.com/x", "https://github.com/org/repo/pull/12"} {
		if _, err := anexo.NovoLink("a-1", "c-1", "", endereco, "u-1"); err != nil {
			t.Errorf("%q devia ser aceito: %v", endereco, err)
		}
	}
}

func TestLinkSemNomeUsaODominioComoRotulo(t *testing.T) {
	a, err := anexo.NovoLink("a-1", "c-1", "  ", "https://github.com/org/repo", "u-1")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if a.Nome != "github.com" {
		t.Errorf("nome = %q, esperado o domínio", a.Nome)
	}
}

// Nome de arquivo vem de quem envia. Ele não pode sobreviver nem como rótulo
// com caminho dentro, muito menos virar caminho de verdade.
func TestNomeDeArquivoPerdeOCaminho(t *testing.T) {
	a, err := anexo.NovoArquivo("a-1", "c-1", "../../etc/passwd", "guardado.bin", "text/plain", 10, "u-1")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if strings.Contains(a.Nome, "/") || strings.Contains(a.Nome, "..") {
		t.Errorf("nome = %q, devia ter perdido o caminho", a.Nome)
	}
	if a.Nome != "passwd" {
		t.Errorf("nome = %q, esperado só o nome-base", a.Nome)
	}
	// E o caminho gravado é o que o armazém devolveu, nunca o que veio de fora.
	if a.Caminho != "guardado.bin" {
		t.Errorf("caminho = %q", a.Caminho)
	}
}

func TestArquivoRespeitaTamanhoETipo(t *testing.T) {
	casos := map[string]struct {
		mime     string
		tamanho  int64
		esperado error
	}{
		"vazio":             {"image/png", 0, anexo.ErrArquivoVazio},
		"grande demais":     {"image/png", anexo.TamanhoMaximoArquivo + 1, anexo.ErrArquivoGrande},
		"no limite":         {"image/png", anexo.TamanhoMaximoArquivo, nil},
		"tipo desconhecido": {"application/x-msdownload", 10, anexo.ErrTipoNaoPermitido},
		"png":               {"image/png", 10, nil},
	}

	for nome, caso := range casos {
		t.Run(nome, func(t *testing.T) {
			_, err := anexo.NovoArquivo("a-1", "c-1", "arquivo.png", "guardado", caso.mime, caso.tamanho, "u-1")
			if !errors.Is(err, caso.esperado) {
				t.Errorf("erro = %v, esperado %v", err, caso.esperado)
			}
		})
	}
}

// HTML e SVG servidos da NOSSA origem executariam script na nossa origem.
// A lista é de permissão justamente para formato novo não entrar sozinho.
func TestHTMLESVGNaoSaoPermitidos(t *testing.T) {
	for _, mime := range []string{"text/html", "image/svg+xml", "application/xhtml+xml"} {
		if anexo.TipoPermitido(mime) {
			t.Errorf("%q não podia ser permitido", mime)
		}
	}
}

func TestTipoPermitidoIgnoraOCharset(t *testing.T) {
	if !anexo.TipoPermitido("text/plain; charset=utf-8") {
		t.Error("o charset que o navegador manda junto não pode invalidar o tipo")
	}
}

// --- prazo e fundo --------------------------------------------------------

func TestCardNasceSemPrazoEAceitaDataNoPassado(t *testing.T) {
	c, _ := card.Novo("c-1", "col-1", "Tarefa", "", "", 1)
	if c.Prazo != nil {
		t.Error("card novo não tem prazo")
	}
	if c.Vencido(time.Now()) {
		t.Error("card sem prazo nunca vence")
	}

	ontem := time.Now().Add(-24 * time.Hour)
	c.DefinirPrazo(&ontem)

	// Registrar que algo venceu ontem é informação legítima; recusar obrigaria
	// a pessoa a mentir a data.
	if !c.Vencido(time.Now()) {
		t.Error("prazo de ontem devia estar vencido")
	}
	if c.Version != 2 {
		t.Errorf("Version = %d — mexer no prazo é edição e conta versão", c.Version)
	}

	c.DefinirPrazo(nil)
	if c.Prazo != nil || c.Vencido(time.Now()) {
		t.Error("passar nil devia limpar o prazo")
	}
}

func TestFundoCaiNoPadraoERecusaDesconhecido(t *testing.T) {
	b, _ := board.Novo("b-1", "Estudos")

	if b.FundoEfetivo() != board.FundoPadrao {
		t.Errorf("fundo = %q, esperado o padrão", b.FundoEfetivo())
	}
	if err := b.DefinirFundo("arco-iris"); !errors.Is(err, board.ErrFundoInvalido) {
		t.Errorf("erro = %v, esperado ErrFundoInvalido", err)
	}
	if err := b.DefinirFundo("oceano"); err != nil {
		t.Errorf("fundo da paleta devia ser aceito: %v", err)
	}
	if b.FundoEfetivo() != "oceano" {
		t.Errorf("fundo = %q", b.FundoEfetivo())
	}
}
