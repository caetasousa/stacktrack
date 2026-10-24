package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// TamanhoDaChave é o tamanho da chave da Cifra: 32 bytes, AES-256.
const TamanhoDaChave = 32

// ErrCifraInvalida é retornado quando o texto cifrado foi adulterado, veio de
// outra chave ou foi movido para outro registro.
var ErrCifraInvalida = errors.New("conteúdo cifrado inválido ou adulterado")

// Cifra protege em repouso o que precisa ser guardado por pouco tempo e não
// pode ficar legível no banco — hoje, a mensagem do outbox de email, que
// carrega o link do convite.
//
// AES-256-GCM: confidencialidade e integridade juntas. Cada mensagem leva um
// nonce aleatório próprio, e o `contexto` (o id do registro) entra como dado
// autenticado — um payload copiado para outra linha da tabela não decifra.
type Cifra struct {
	aead cipher.AEAD
}

// NovaCifra cria a cifra a partir da chave. Recusa chave de tamanho diferente
// de TamanhoDaChave: uma chave curta não falha, ela só protege menos — e é
// exatamente o tipo de erro que ninguém percebe.
func NovaCifra(chave []byte) (*Cifra, error) {
	if len(chave) != TamanhoDaChave {
		return nil, fmt.Errorf("a chave da cifra precisa ter %d bytes, veio com %d", TamanhoDaChave, len(chave))
	}
	bloco, err := aes.NewCipher(chave)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(bloco)
	if err != nil {
		return nil, err
	}
	return &Cifra{aead: aead}, nil
}

// ChaveDeTexto decodifica a chave do formato em que ela mora no vault e no
// .env: base64 padrão de 32 bytes (`openssl rand -base64 32`).
func ChaveDeTexto(texto string) ([]byte, error) {
	chave, err := base64.StdEncoding.DecodeString(strings.TrimSpace(texto))
	if err != nil {
		return nil, fmt.Errorf("a chave não é base64 válido: %w", err)
	}
	if len(chave) != TamanhoDaChave {
		return nil, fmt.Errorf("a chave precisa ter %d bytes depois de decodificada, tem %d", TamanhoDaChave, len(chave))
	}
	return chave, nil
}

// Cifrar devolve nonce || texto cifrado, amarrado ao contexto.
func (c *Cifra) Cifrar(claro []byte, contexto string) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return c.aead.Seal(nonce, nonce, claro, []byte(contexto)), nil
}

// Decifrar abre o que Cifrar produziu com o MESMO contexto. Devolve
// ErrCifraInvalida para qualquer adulteração, chave errada ou contexto trocado.
func (c *Cifra) Decifrar(cifrado []byte, contexto string) ([]byte, error) {
	tamanho := c.aead.NonceSize()
	if len(cifrado) < tamanho {
		return nil, ErrCifraInvalida
	}
	claro, err := c.aead.Open(nil, cifrado[:tamanho], cifrado[tamanho:], []byte(contexto))
	if err != nil {
		return nil, ErrCifraInvalida
	}
	return claro, nil
}
