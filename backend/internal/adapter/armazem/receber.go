package armazem

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	ucboard "stacktrack/internal/usecase/board"
)

// Recebido é um upload já gravado em disco, mas ainda NÃO publicado.
//
// A separação entre receber e publicar é o coração do upload streaming. O
// conteúdo chega por um socket, byte a byte, e não cabe decidir se ele é
// aceitável antes de tê-lo por inteiro: o tamanho real só se conhece no fim, e
// o tipo só depois de olhar o começo. Escrevendo primeiro num temporário, o
// processo nunca segura o arquivo em memória e a decisão acontece com os fatos
// na mão.
type Recebido struct {
	// temporario é o caminho absoluto do arquivo ainda não publicado.
	temporario string
	// Tamanho é quantos bytes chegaram, contados na leitura.
	Tamanho int64
	// Hash é o SHA-256 do conteúdo, calculado no mesmo passe da escrita.
	//
	// Serve à verificação da restauração de backup (A6): conferir que o arquivo
	// no disco é o que a linha do banco diz que é. Calculá-lo aqui é de graça —
	// os bytes já estão passando.
	Hash string
	// TipoDetectado é o MIME deduzido do CONTEÚDO, não do que o cliente
	// declarou.
	TipoDetectado string
}

// Receber grava o conteúdo num temporário do MESMO diretório dos anexos,
// medindo tamanho, hash e tipo no caminho.
//
// Mesmo diretório porque a publicação é um `rename`, e `rename` só é atômico
// dentro do mesmo filesystem. Gravar em /tmp e mover depois viraria uma cópia —
// que pode falhar no meio e deixar um arquivo parcial com nome definitivo.
//
// O limite é aplicado com um byte de folga: lendo `limite+1`, um arquivo
// exatamente no teto passa e o primeiro byte além dele é detectado sem precisar
// confiar em nenhum cabeçalho.
func (d *Disco) Receber(conteudo io.Reader, limite int64) (ucboard.ArquivoRecebido, error) {
	arquivo, err := os.CreateTemp(d.raiz, "recebendo-*.parcial")
	if err != nil {
		return nil, fmt.Errorf("criar temporário: %w", err)
	}
	temporario := arquivo.Name()

	// Qualquer saída que não seja sucesso não pode deixar o parcial para trás.
	sucesso := false
	defer func() {
		arquivo.Close()
		if !sucesso {
			os.Remove(temporario)
		}
	}()

	soma := sha256.New()
	// Os primeiros bytes são guardados para a detecção de tipo. 512 é o que o
	// net/http usa, e é o suficiente para todo formato da lista de permissão.
	comeco := make([]byte, 0, 512)

	limitado := io.LimitReader(conteudo, limite+1)
	buffer := make([]byte, 32*1024)
	var total int64
	for {
		lidos, err := limitado.Read(buffer)
		if lidos > 0 {
			total += int64(lidos)
			if total > limite {
				return nil, ucboard.ErrArquivoAcimaDoLimite
			}
			if len(comeco) < 512 {
				comeco = append(comeco, buffer[:min(lidos, 512-len(comeco))]...)
			}
			soma.Write(buffer[:lidos])
			if _, err := arquivo.Write(buffer[:lidos]); err != nil {
				return nil, fmt.Errorf("gravar temporário: %w", err)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("ler o envio: %w", err)
		}
	}

	// fsync antes de publicar: sem ele, um corte de energia depois do rename
	// pode deixar o nome definitivo apontando para um arquivo vazio — o
	// diretório é durável e o conteúdo não.
	if err := arquivo.Sync(); err != nil {
		return nil, fmt.Errorf("sincronizar temporário: %w", err)
	}

	sucesso = true
	return &Recebido{
		temporario:    temporario,
		Tamanho:       total,
		Hash:          hex.EncodeToString(soma.Sum(nil)),
		TipoDetectado: detectarTipo(comeco),
	}, nil
}

// Bytes, Tipo e Digest satisfazem ucboard.ArquivoRecebido — o usecase conhece o
// que precisa para decidir, e não onde o temporário está.
func (r *Recebido) Bytes() int64   { return r.Tamanho }
func (r *Recebido) Tipo() string   { return r.TipoDetectado }
func (r *Recebido) Digest() string { return r.Hash }

// Publicar promove o temporário ao nome definitivo, de forma ATÔMICA.
//
// `rename` dentro do mesmo filesystem ou acontece por inteiro ou não acontece:
// nenhum leitor jamais encontra um arquivo publicado pela metade. É a razão de
// o temporário nascer no mesmo diretório.
//
// O nome é sorteado, e não derivado do que veio de quem enviou: nome de arquivo
// é entrada do usuário, e entrada do usuário não vira caminho. Também evita que
// dois envios do mesmo "relatorio.pdf" se sobrescrevam.
func (d *Disco) Publicar(recebido ucboard.ArquivoRecebido, extensao string) (string, error) {
	interno, ok := recebido.(*Recebido)
	if !ok || interno == nil {
		return "", errors.New("nada a publicar")
	}
	nome, err := nomeSorteado(extensao)
	if err != nil {
		return "", err
	}
	destino, err := d.resolver(nome)
	if err != nil {
		return "", err
	}
	// Um destino já existente só aconteceria com o sorteio quebrado; renomear
	// por cima apagaria o anexo de outra pessoa em silêncio.
	if _, err := os.Stat(destino); err == nil {
		return "", fmt.Errorf("nome de anexo já em uso: %s", nome)
	}
	if err := os.Rename(interno.temporario, destino); err != nil {
		return "", fmt.Errorf("publicar anexo: %w", err)
	}
	return nome, nil
}

// Descartar apaga um temporário que não será publicado.
//
// Chamado quando o domínio recusa o arquivo — cota estourada, tipo não
// permitido — ou quando a transação falha. Sem ele, cada recusa deixaria lixo.
func (d *Disco) Descartar(recebido ucboard.ArquivoRecebido) error {
	interno, ok := recebido.(*Recebido)
	if !ok || interno == nil {
		return nil
	}
	err := os.Remove(interno.temporario)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// LimparParciais apaga temporários abandonados por um processo que morreu no
// meio de um upload.
//
// Devolve quantos apagou. É chamado pela faxina periódica: um `.parcial` só
// existe entre o começo e o fim de um envio, então qualquer um mais velho que a
// idade informada é resto de um processo que não terminou.
func (d *Disco) LimparParciais(idadeMinima int64) (int, error) {
	entradas, err := os.ReadDir(d.raiz)
	if err != nil {
		return 0, err
	}
	var apagados int
	for _, entrada := range entradas {
		if entrada.IsDir() || !strings.HasSuffix(entrada.Name(), ".parcial") {
			continue
		}
		info, err := entrada.Info()
		if err != nil {
			continue
		}
		if nowUnix()-info.ModTime().Unix() < idadeMinima {
			continue
		}
		if os.Remove(filepath.Join(d.raiz, entrada.Name())) == nil {
			apagados++
		}
	}
	return apagados, nil
}

// nomeSorteado devolve um nome de arquivo aleatório com a extensão informada.
func nomeSorteado(extensao string) (string, error) {
	sorteio := make([]byte, 16)
	if _, err := rand.Read(sorteio); err != nil {
		return "", err
	}
	return hex.EncodeToString(sorteio) + extensao, nil
}

// detectarTipo deduz o MIME pelo CONTEÚDO, não pelo que o cliente declarou.
//
// O Content-Type do multipart é escolhido por quem envia. Aceitá-lo é aceitar
// que um HTML seja gravado como "image/png" — e, embora o download force
// Content-Disposition: attachment e nosniff, a lista de permissão do domínio
// deixaria de significar o que ela diz significar.
//
// `http.DetectContentType` implementa o algoritmo de sniffing do WHATWG sobre
// os primeiros 512 bytes. Conteúdo vazio devolve o tipo genérico, e a lista de
// permissão do domínio decide o que fazer com ele.
func detectarTipo(comeco []byte) string {
	if len(comeco) == 0 {
		return "application/octet-stream"
	}
	return http.DetectContentType(comeco)
}

// nowUnix existe para o teste conseguir envelhecer um parcial sem esperar.
var nowUnix = func() int64 { return time.Now().Unix() }
