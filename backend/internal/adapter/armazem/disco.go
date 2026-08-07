// Package armazem guarda o conteúdo dos anexos. Hoje em disco; a porta do
// usecase é a mesma se um dia isso virar S3.
package armazem

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrForaDoDiretorio é retornado quando um caminho tenta sair da pasta de
// anexos.
var ErrForaDoDiretorio = errors.New("caminho de anexo inválido")

// Disco guarda os anexos em um diretório do sistema de arquivos.
type Disco struct {
	raiz string
}

// NovoDisco cria o armazém, garantindo que o diretório exista.
func NovoDisco(raiz string) (*Disco, error) {
	if err := os.MkdirAll(raiz, 0o750); err != nil {
		return nil, fmt.Errorf("criar diretório de anexos: %w", err)
	}
	absoluto, err := filepath.Abs(raiz)
	if err != nil {
		return nil, fmt.Errorf("resolver diretório de anexos: %w", err)
	}
	return &Disco{raiz: absoluto}, nil
}

// Guardar grava o conteúdo com um nome aleatório e devolve esse nome.
//
// O nome é sorteado, e não derivado do que veio de quem enviou: nome de arquivo
// é entrada do usuário, e entrada do usuário não vira caminho. Também evita que
// dois envios do mesmo "relatorio.pdf" se sobrescrevam.
func (d *Disco) Guardar(conteudo io.Reader, extensao string) (string, error) {
	sorteio := make([]byte, 16)
	if _, err := rand.Read(sorteio); err != nil {
		return "", err
	}
	nome := hex.EncodeToString(sorteio) + extensao

	destino, err := d.resolver(nome)
	if err != nil {
		return "", err
	}

	// O_EXCL: se o nome já existir — o que só aconteceria com o gerador
	// quebrado —, falha em vez de sobrescrever um anexo de outra pessoa.
	arquivo, err := os.OpenFile(destino, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return "", err
	}
	defer arquivo.Close()

	if _, err := io.Copy(arquivo, conteudo); err != nil {
		os.Remove(destino)
		return "", err
	}
	return nome, nil
}

// Abrir devolve o conteúdo de um anexo já gravado.
func (d *Disco) Abrir(caminho string) (io.ReadCloser, error) {
	destino, err := d.resolver(caminho)
	if err != nil {
		return nil, err
	}
	return os.Open(destino)
}

// Remover apaga o arquivo. Remover o que não existe não é erro: o resultado
// pretendido já vale.
func (d *Disco) Remover(caminho string) error {
	destino, err := d.resolver(caminho)
	if err != nil {
		return err
	}
	if err := os.Remove(destino); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// resolver monta o caminho absoluto e recusa qualquer coisa que escape da pasta
// de anexos.
//
// É a defesa contra path traversal. Mesmo que o nome no banco fosse adulterado
// para "../../etc/passwd", ele não vira leitura de arquivo do sistema: o
// caminho resolvido precisa continuar dentro da raiz.
func (d *Disco) resolver(nome string) (string, error) {
	if nome == "" || strings.ContainsRune(nome, os.PathSeparator) || nome == "." || nome == ".." {
		return "", ErrForaDoDiretorio
	}
	destino := filepath.Join(d.raiz, nome)
	if !strings.HasPrefix(destino, d.raiz+string(os.PathSeparator)) {
		return "", ErrForaDoDiretorio
	}
	return destino, nil
}
