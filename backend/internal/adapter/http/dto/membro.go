package dto

import "time"

// ConviteRequest é o corpo de convidar alguém para o quadro.
type ConviteRequest struct {
	Email EmailEntrada `json:"email" validate:"required,email,max=255"`
	Papel string       `json:"papel" validate:"required,oneof=dono editor leitor"`
}

// Validar checa o formato dos campos informados.
func (r ConviteRequest) Validar() error {
	return validate.Struct(r)
}

// PapelRequest é o corpo de trocar o papel de quem participa.
type PapelRequest struct {
	Papel string `json:"papel" validate:"required,oneof=dono editor leitor"`
}

// Validar checa o formato dos campos informados.
func (r PapelRequest) Validar() error {
	return validate.Struct(r)
}

// MembroResponse é alguém que participa do quadro.
type MembroResponse struct {
	UsuarioID string    `json:"usuarioId"`
	Nome      string    `json:"nome"`
	Email     string    `json:"email"`
	Papel     string    `json:"papel"`
	DesdeEm   time.Time `json:"desdeEm"`
}

// ConvitePendenteResponse é um convite ainda não aceito. Não devolve o token:
// ele existe em texto puro uma única vez, na resposta que o criou.
type ConvitePendenteResponse struct {
	ID       string    `json:"id"`
	Email    string    `json:"email"`
	Papel    string    `json:"papel"`
	ExpiraEm time.Time `json:"expiraEm"`
	Expirado bool      `json:"expirado"`
}

// MembrosResponse é o que a tela de membros precisa. Convites só vêm
// preenchidos para o dono — quem não administra o quadro não vê para quem ele
// foi oferecido.
type MembrosResponse struct {
	Membros  []MembroResponse          `json:"membros"`
	Convites []ConvitePendenteResponse `json:"convites"`
}

// ConviteCriadoResponse é o resultado de convidar: sempre um convite pendente
// e o link, que aparece NESTA resposta e em nenhuma outra.
//
// Adicionado e Membro são campos de TRANSIÇÃO. Existia um caminho em que a
// pessoa com conta virava membro na hora, e o cliente publicado ainda ramifica
// por esse campo. Ele foi removido do domínio, então `adicionado` agora é
// sempre false e `membro` nunca vem preenchido — o cliente antigo cai no ramo
// do link, que é o comportamento correto. Os dois campos saem quando não
// houver mais cliente antigo em campo.
type ConviteCriadoResponse struct {
	Adicionado bool                     `json:"adicionado"`
	Membro     *MembroResponse          `json:"membro,omitempty"`
	Convite    *ConvitePendenteResponse `json:"convite,omitempty"`
	Link       string                   `json:"link,omitempty"`
}

// ConviteDetalheResponse descreve um convite para quem tem o link, antes de
// aceitar — sem exigir sessão, porque quem foi convidado costuma ainda não ter
// conta.
type ConviteDetalheResponse struct {
	Quadro string `json:"quadro"`
	// EmailMascarado substituiu o campo `email`, que devolvia o endereço
	// inteiro numa rota pública — quem tivesse o link ficava sabendo o email de
	// quem foi convidado. O nome do campo mudou de propósito, em vez de só
	// mudar o valor: um cliente antigo lendo `email` mostra vazio, o que é
	// visivelmente errado e se conserta, enquanto um campo `email` com valor
	// mascarado passaria despercebido como se ainda fosse o endereço real.
	EmailMascarado string `json:"emailMascarado"`
	Papel          string `json:"papel"`
	ConvidadoPor   string `json:"convidadoPor,omitempty"`
}
