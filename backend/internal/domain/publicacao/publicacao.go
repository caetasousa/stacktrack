// Package publicacao modela o link público de acompanhamento de um quadro — o
// caminho de quem precisa ver o andamento sem ter conta e sem entrar na equipe.
//
// A publicação não é um papel a mais. Membro é vínculo entre uma PESSOA e um
// quadro, e toda regra de permissão parte dele; a publicação é o oposto: uma
// permissão sem pessoa nenhuma do outro lado, que vale para quem tiver o
// segredo e some quando ele for revogado. Misturar as duas transformaria "não
// participa" — a pergunta que autoriza a aplicação inteira — numa pergunta com
// exceção.
package publicacao

import (
	"errors"
	"time"
)

var (
	// ErrNaoEncontrada é retornado quando o token não corresponde a publicação
	// nenhuma. Token inventado, link revogado e quadro apagado respondem o
	// mesmo: distinguir diria a quem testa links qual das três hipóteses errou.
	ErrNaoEncontrada = errors.New("link público inválido")
	// ErrTokenObrigatorio é retornado quando se tenta criar a publicação sem
	// segredo nenhum — seria um quadro aberto a qualquer um que soubesse a rota.
	ErrTokenObrigatorio = errors.New("token da publicação é obrigatório")
)

// Publicacao é o link vivo de um quadro. Existir é estar publicado: não há
// campo "ativo", porque revogar apaga a linha — um sinalizador desligado é um
// segredo que continua no banco esperando ser religado por engano.
type Publicacao struct {
	BoardID string
	// Token é o segredo do link, em claro. Ver a migration V21 para por que
	// este não é guardado como hash, ao contrário do de sessão e do de convite.
	Token     string
	CriadoPor string
	CriadoEm  time.Time
}

// Nova cria a publicação de um quadro. Retorna ErrTokenObrigatorio se o token
// vier vazio.
func Nova(boardID, token, criadoPor string) (*Publicacao, error) {
	if token == "" {
		return nil, ErrTokenObrigatorio
	}
	return &Publicacao{
		BoardID:   boardID,
		Token:     token,
		CriadoPor: criadoPor,
		CriadoEm:  time.Now(),
	}, nil
}
