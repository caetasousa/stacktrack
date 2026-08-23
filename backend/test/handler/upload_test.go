// O upload em STREAMING, medido.
//
// O critério de aceite de A4 é explícito: "upload de 10 MiB não causa alocação
// equivalente ao multipart completo". Não é uma afirmação sobre estilo de
// código — é um número, e por isso o teste mede.
//
// O que se media antes: `FormFile` chama `ParseMultipartForm`, que materializa
// o formulário inteiro (até 32 MiB em memória por padrão, o excedente em
// temporários do sistema) ANTES de qualquer decisão. Dez envios simultâneos de
// 10 MiB davam centenas de megabytes num container de 384 MiB.
package handler_test

import (
	"crypto/rand"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"stacktrack/internal/adapter/armazem"
)

// tamanhoDoEnvio é o teto do domínio: é o pior caso legítimo.
const tamanhoDoEnvio = 10 << 20

// cardParaAnexo monta quadro, coluna e card e devolve o id do card.
func (a *apiDeQuadro) cardParaAnexo(t *testing.T, cookie *http.Cookie) string {
	t.Helper()
	boardID := a.criarQuadro(t, cookie, "Anexos")
	colunaID := idDoCorpo(t, chamar(a, http.MethodPost, "/boards/"+boardID+"/colunas",
		`{"titulo":"A fazer"}`, cookie))
	return idDoCorpo(t, chamar(a, http.MethodPost, "/colunas/"+colunaID+"/cards",
		`{"titulo":"Com anexo"}`, cookie))
}

// corpoMultipart monta o corpo em STREAMING, por um pipe.
//
// Um `bytes.Buffer` aqui alocaria os 10 MiB no próprio teste e envenenaria a
// medição: o que se quer medir é o que o SERVIDOR aloca, não o cliente.
func corpoMultipart(t *testing.T, bytesDoArquivo int) (io.Reader, string) {
	t.Helper()
	leitura, escrita := io.Pipe()
	form := multipart.NewWriter(escrita)

	go func() {
		defer escrita.Close()
		parte, err := form.CreateFormFile("arquivo", "grande.bin")
		if err != nil {
			escrita.CloseWithError(err)
			return
		}
		// Conteúdo aleatório, em blocos: comprime mal e não deixa o sistema de
		// arquivos otimizar buracos, então o número medido é honesto.
		bloco := make([]byte, 64*1024)
		rand.Read(bloco)
		// Os primeiros bytes decidem o tipo detectado. `text/plain` está na
		// lista de permissão; aleatório viraria octet-stream e o envio seria
		// recusado antes de terminar, o que não é o caminho a medir.
		for i := range bloco[:512] {
			bloco[i] = 'a'
		}
		escritos := 0
		for escritos < bytesDoArquivo {
			fim := len(bloco)
			if bytesDoArquivo-escritos < fim {
				fim = bytesDoArquivo - escritos
			}
			n, err := parte.Write(bloco[:fim])
			if err != nil {
				escrita.CloseWithError(err)
				return
			}
			escritos += n
		}
		form.Close()
	}()

	return leitura, form.FormDataContentType()
}

// apiComArmazemReal monta a API com o armazém de DISCO, não com o fake: o
// assunto é o caminho dos bytes, e o fake em memória guardaria tudo por
// definição.
func apiComArmazemReal(t *testing.T) (*apiDeQuadro, string) {
	t.Helper()
	dir := t.TempDir()
	disco, err := armazem.NovoDisco(dir)
	if err != nil {
		t.Fatalf("armazém: %v", err)
	}
	return montarAPIDeQuadroCom(disco), dir
}

// Um envio de 10 MiB não pode alocar 10 MiB.
func TestUploadDeDezMiBNaoAlocaOArquivoInteiro(t *testing.T) {
	api, dir := apiComArmazemReal(t)
	cookie, _ := api.conta(t, "Ana", "ana@exemplo.com")
	cardID := api.cardParaAnexo(t, cookie)

	corpo, tipo := corpoMultipart(t, tamanhoDoEnvio)
	req := httptest.NewRequest(http.MethodPost, "/cards/"+cardID+"/anexos/arquivo", corpo)
	req.Header.Set("Content-Type", tipo)
	req.AddCookie(cookie)

	// A medição é do total ALOCADO durante a requisição, e não do heap em uso:
	// o heap em uso depende de quando o GC roda, e isso não é o que interessa.
	// TotalAlloc só cresce, então a diferença é exatamente o que a requisição
	// pediu de memória.
	runtime.GC()
	var antes, depois runtime.MemStats
	runtime.ReadMemStats(&antes)

	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	runtime.ReadMemStats(&depois)
	alocado := depois.TotalAlloc - antes.TotalAlloc

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, esperado 201: %s", rec.Code, rec.Body)
	}

	// O teto é generoso de propósito — o teste não é sobre o número exato, e
	// sim sobre a ORDEM DE GRANDEZA. Materializando o multipart, a alocação
	// passa dos 10 MiB do arquivo; em streaming ela fica em alguns megabytes de
	// buffers reutilizados.
	const teto = tamanhoDoEnvio / 2
	if alocado > teto {
		t.Errorf("alocou %d bytes para um arquivo de %d: o multipart está sendo materializado",
			alocado, tamanhoDoEnvio)
	}
	t.Logf("alocado durante o upload: %d bytes (%.1f%% do arquivo)",
		alocado, float64(alocado)*100/float64(tamanhoDoEnvio))

	// E o arquivo chegou inteiro ao disco.
	entradas, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ler diretório: %v", err)
	}
	var gravados int
	for _, e := range entradas {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.Size() == tamanhoDoEnvio {
			gravados++
		}
	}
	if gravados != 1 {
		t.Errorf("arquivos de %d bytes no disco = %d, esperado 1", tamanhoDoEnvio, gravados)
	}
}

// Nenhum `.parcial` sobrevive a um envio bem-sucedido.
func TestUploadBemSucedidoNaoDeixaTemporario(t *testing.T) {
	api, dir := apiComArmazemReal(t)
	cookie, _ := api.conta(t, "Ana", "ana@exemplo.com")
	cardID := api.cardParaAnexo(t, cookie)

	corpo, tipo := corpoMultipart(t, 64*1024)
	req := httptest.NewRequest(http.MethodPost, "/cards/"+cardID+"/anexos/arquivo", corpo)
	req.Header.Set("Content-Type", tipo)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if parciais := len(parciaisEm(t, dir)); parciais != 0 {
		t.Errorf("sobraram %d arquivos .parcial depois de um envio bem-sucedido", parciais)
	}
}

// Envio RECUSADO — acima do limite — também não deixa temporário.
//
// É o caminho que mais produz lixo se ninguém cuidar: cada tentativa grande
// demais deixaria um arquivo abandonado no volume.
func TestUploadRecusadoNaoDeixaTemporario(t *testing.T) {
	api, dir := apiComArmazemReal(t)
	cookie, _ := api.conta(t, "Ana", "ana@exemplo.com")
	cardID := api.cardParaAnexo(t, cookie)

	// Acima do teto do domínio.
	corpo, tipo := corpoMultipart(t, tamanhoDoEnvio+1024)
	req := httptest.NewRequest(http.MethodPost, "/cards/"+cardID+"/anexos/arquivo", corpo)
	req.Header.Set("Content-Type", tipo)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, esperado 413: %s", rec.Code, rec.Body)
	}
	if parciais := len(parciaisEm(t, dir)); parciais != 0 {
		t.Errorf("sobraram %d arquivos .parcial depois de um envio recusado", parciais)
	}
}

func parciaisEm(t *testing.T, dir string) []string {
	t.Helper()
	achados, err := filepath.Glob(filepath.Join(dir, "*.parcial"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	return achados
}
