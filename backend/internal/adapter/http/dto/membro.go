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
// ele existe em texto puro só na resposta que o criou (ou renovou) e no email.
type ConvitePendenteResponse struct {
	ID       string    `json:"id"`
	Email    string    `json:"email"`
	Papel    string    `json:"papel"`
	ExpiraEm time.Time `json:"expiraEm"`
	Expirado bool      `json:"expirado"`
	// Entrega é o último email deste convite; nil quando o envio está
	// desligado ou o convite nunca foi mandado por email.
	Entrega *EntregaResponse `json:"entrega,omitempty"`
}

// EntregaResponse é o estado do email de um convite, como o dono precisa ver:
// saiu, está saindo, ou não vai sair. A causa técnica da falha fica no log e no
// outbox, para a operação — para o dono, o que importa é o que fazer.
type EntregaResponse struct {
	// Estado é pendente, enviado, falhou, cancelado ou expirado.
	Estado    string     `json:"estado"`
	EnviadoEm *time.Time `json:"enviadoEm,omitempty"`
}

// MembrosResponse é o que a tela de membros precisa. Convites só vêm
// preenchidos para o dono — quem não administra o quadro não vê para quem ele
// foi oferecido.
type MembrosResponse struct {
	Membros  []MembroResponse          `json:"membros"`
	Convites []ConvitePendenteResponse `json:"convites"`
}

// ConviteCriadoResponse é o resultado de convidar ou reenviar: o convite, o
// link e como ele chega à pessoa.
//
// O link vem SEMPRE, mesmo com o email na fila: é o plano B de quem convida
// quando o email demora ou cai no spam, e a única entrega quando o envio está
// desligado. Ele aparece nesta resposta e no email, e em nenhum outro lugar.
type ConviteCriadoResponse struct {
	Convite *ConvitePendenteResponse `json:"convite"`
	Link    string                   `json:"link"`
	// Entrega é "email" (na fila, sai em instantes), "manual" (envio desligado
	// neste ambiente) ou "cota" (teto de emails atingido: só o link vale).
	Entrega string `json:"entrega"`
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
	EmailMascarado string    `json:"emailMascarado"`
	Papel          string    `json:"papel"`
	ConvidadoPor   string    `json:"convidadoPor,omitempty"`
	ExpiraEm       time.Time `json:"expiraEm"`
}

// ConviteRecebidoResponse é um convite em aberto para o email da conta, em
// "Convites para você". Sem o id do quadro: ele só sai depois do aceite.
type ConviteRecebidoResponse struct {
	ID           string    `json:"id"`
	Quadro       string    `json:"quadro"`
	ConvidadoPor string    `json:"convidadoPor,omitempty"`
	Papel        string    `json:"papel"`
	ExpiraEm     time.Time `json:"expiraEm"`
}

// ConvitesRecebidosResponse é a lista de "Convites para você".
type ConvitesRecebidosResponse struct {
	Convites []ConviteRecebidoResponse `json:"convites"`
}
