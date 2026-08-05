package dto

import (
	"encoding/json"
	"strings"
)

// EmailEntrada é um email vindo do cliente, aparado no momento da
// decodificação.
//
// Existe porque a validação de formato roda ANTES de o domínio normalizar o
// email: sem aparar aqui, " ana@exemplo.com " — que teclado de celular e
// copiar-e-colar produzem o tempo todo — é recusado com "email inválido" no
// login e no cadastro, e a pessoa não tem como ver o espaço para corrigir.
//
// Aparar no UnmarshalJSON, e não em cada handler, faz a regra valer para todo
// DTO que use este tipo, inclusive os que ainda não existem.
type EmailEntrada string

// UnmarshalJSON decodifica a string e remove os espaços das pontas.
func (e *EmailEntrada) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*e = EmailEntrada(strings.TrimSpace(s))
	return nil
}

// String devolve o email como texto.
func (e EmailEntrada) String() string {
	return string(e)
}
