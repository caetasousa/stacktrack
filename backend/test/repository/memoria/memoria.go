// Package memoria implementa em memória as portas que os usecases exigem.
//
// Existe para os testes de usecase rodarem sem Docker e em milissegundos: o
// que se quer provar ali é a REGRA (quem pode logar, o que acontece com email
// repetido), não o SQL. A fidelidade ao Postgres é assunto dos testes de
// repositório, que sobem um banco de verdade — e chegam na fase 8.
package memoria

import (
	"context"
	"errors"
	"time"

	"stacktrack/internal/domain/session"
	"stacktrack/internal/domain/usuario"
)

// Usuarios guarda contas em memória, indexadas por id.
type Usuarios struct {
	porID map[string]*usuario.Usuario
	// ErroForcado, quando definido, é devolvido por qualquer operação — serve
	// para exercitar o caminho de falha de infraestrutura, que é justamente o
	// que não pode ser confundido com "credencial inválida".
	ErroForcado error
}

// NovosUsuarios cria o repositório em memória vazio.
func NovosUsuarios() *Usuarios {
	return &Usuarios{porID: make(map[string]*usuario.Usuario)}
}

// Salvar grava a conta, devolvendo usuario.ErrEmailEmUso quando o email já
// existe — o mesmo comportamento do UNIQUE no Postgres.
func (r *Usuarios) Salvar(ctx context.Context, u *usuario.Usuario) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	for _, existente := range r.porID {
		if existente.Email == u.Email && existente.ID != u.ID {
			return usuario.ErrEmailEmUso
		}
	}
	copia := *u
	r.porID[u.ID] = &copia
	return nil
}

// BuscarPorID devolve (nil, nil) quando não encontra.
func (r *Usuarios) BuscarPorID(ctx context.Context, id string) (*usuario.Usuario, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	u, ok := r.porID[id]
	if !ok {
		return nil, nil
	}
	copia := *u
	return &copia, nil
}

// BuscarPorEmail devolve (nil, nil) quando não encontra. Normaliza o email
// antes de comparar, como o repositório de verdade faz.
func (r *Usuarios) BuscarPorEmail(ctx context.Context, email string) (*usuario.Usuario, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	alvo := usuario.NormalizarEmail(email)
	for _, u := range r.porID {
		if u.Email == alvo {
			copia := *u
			return &copia, nil
		}
	}
	return nil, nil
}

// Quantidade informa quantas contas existem — atalho para os testes.
func (r *Usuarios) Quantidade() int {
	return len(r.porID)
}

// Sessoes guarda sessões em memória, indexadas pelo hash do token.
type Sessoes struct {
	porHash map[string]*session.Session
	// ErroForcado, quando definido, é devolvido por Salvar e BuscarPorTokenHash.
	ErroForcado error
	// LimpezasChamadas conta as varreduras de sessões vencidas.
	LimpezasChamadas int
}

// NovasSessoes cria o repositório em memória vazio.
func NovasSessoes() *Sessoes {
	return &Sessoes{porHash: make(map[string]*session.Session)}
}

// Salvar grava a sessão.
func (r *Sessoes) Salvar(ctx context.Context, s *session.Session) error {
	if r.ErroForcado != nil {
		return r.ErroForcado
	}
	copia := *s
	r.porHash[s.TokenHash] = &copia
	return nil
}

// BuscarPorTokenHash devolve (nil, nil) quando não encontra.
func (r *Sessoes) BuscarPorTokenHash(ctx context.Context, hash string) (*session.Session, error) {
	if r.ErroForcado != nil {
		return nil, r.ErroForcado
	}
	s, ok := r.porHash[hash]
	if !ok {
		return nil, nil
	}
	copia := *s
	return &copia, nil
}

// Remover apaga a sessão. Remover o que não existe não é erro.
func (r *Sessoes) Remover(ctx context.Context, hash string) error {
	delete(r.porHash, hash)
	return nil
}

// RemoverExpiradas apaga as sessões vencidas e conta a chamada.
func (r *Sessoes) RemoverExpiradas(ctx context.Context) error {
	r.LimpezasChamadas++
	agora := time.Now()
	for hash, s := range r.porHash {
		if s.Expirada(agora) {
			delete(r.porHash, hash)
		}
	}
	return nil
}

// Quantidade informa quantas sessões existem — atalho para os testes.
func (r *Sessoes) Quantidade() int {
	return len(r.porHash)
}

// Hasher é um hasher falso e instantâneo. Argon2id de verdade custa ~50ms por
// chamada de propósito; num teste isso é só espera, e o que se quer verificar
// é que o usecase compara senha com hash, não que o Argon2 funciona.
type Hasher struct {
	// Chamadas conta as verificações feitas — é como o teste prova que o
	// login queima o mesmo custo quando o email não existe.
	Chamadas int
}

// ErrHashInvalido é devolvido quando o hash não veio deste Hasher.
var ErrHashInvalido = errors.New("hash em formato inválido")

// Gerar devolve um hash previsível a partir da senha.
func (h *Hasher) Gerar(senha string) (string, error) {
	return "hash(" + senha + ")", nil
}

// Verificar confere se o hash corresponde à senha informada.
func (h *Hasher) Verificar(senha, hash string) (bool, error) {
	h.Chamadas++
	if hash == "" {
		return false, ErrHashInvalido
	}
	return hash == "hash("+senha+")", nil
}
