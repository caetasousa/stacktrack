// Package security contém adapters de criptografia usados pela aplicação.
package security

import (
	"github.com/alexedwards/argon2id"
)

// HasherArgon2id gera e verifica hashes de senha usando Argon2id.
type HasherArgon2id struct {
	params *argon2id.Params
}

// NovoHasherArgon2id cria um HasherArgon2id com os parâmetros recomendados
// pela OWASP: 19 MiB de memória, 2 iterações, paralelismo 1, salt de 16 bytes
// e chave de 32 bytes.
//
// O custo é o ponto: ~50ms por verificação é irrelevante para quem faz login
// uma vez e proibitivo para quem tenta milhões de senhas de um vazamento. Por
// isso o login tem teto de tentativas — o mesmo custo que protege a senha é um
// vetor de negação de serviço se puder ser disparado à vontade.
func NovoHasherArgon2id() *HasherArgon2id {
	return &HasherArgon2id{
		params: &argon2id.Params{
			Memory:      19 * 1024,
			Iterations:  2,
			Parallelism: 1,
			SaltLength:  16,
			KeyLength:   32,
		},
	}
}

// Gerar devolve o hash da senha no formato PHC, com salt aleatório embutido.
func (h *HasherArgon2id) Gerar(senha string) (string, error) {
	return argon2id.CreateHash(senha, h.params)
}

// Verificar compara a senha em texto puro com o hash em tempo constante.
func (h *HasherArgon2id) Verificar(senha, hash string) (bool, error) {
	return argon2id.ComparePasswordAndHash(senha, hash)
}
