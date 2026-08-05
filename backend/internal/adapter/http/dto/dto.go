// Package dto contém os contratos de entrada e saída da API HTTP. Os tipos
// daqui são a fronteira: o domínio nunca é serializado direto, para que mudar
// um campo interno não quebre quem consome a API sem querer.
package dto

import "github.com/go-playground/validator/v10"

var validate = validator.New()
