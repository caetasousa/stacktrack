// Package token gera e deriva tokens opacos usados nas sessões de autenticação.
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// tamanhoBytes é o tamanho do token aleatório antes da codificação (256 bits).
const tamanhoBytes = 32

// Gerar cria um token aleatório de 256 bits codificado em base64url.
//
// crypto/rand, e não math/rand: o token de sessão é a credencial inteira de
// quem o possui, e um gerador previsível permitiria adivinhar a sessão de
// outra pessoa.
func Gerar() (string, error) {
	b := make([]byte, tamanhoBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Hash devolve o SHA-256 do token em hexadecimal — é essa representação que
// fica persistida no banco, nunca o token puro.
//
// SHA-256 sem salt e sem custo é o certo AQUI, ao contrário do que vale para
// senha: o token tem 256 bits de entropia aleatória, então não existe
// dicionário nem rainbow table que o alcance, e a busca por hash precisa ser
// rápida — ela roda a cada requisição autenticada.
func Hash(t string) string {
	soma := sha256.Sum256([]byte(t))
	return hex.EncodeToString(soma[:])
}
