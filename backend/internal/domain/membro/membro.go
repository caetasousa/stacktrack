// Package membro modela a participação de uma pessoa em um quadro, e o que
// cada papel pode fazer. É aqui que mora a resposta para "você pode?" — não no
// handler, que só traduz a resposta em código HTTP.
package membro

import (
	"errors"
	"time"
)

// Papel é o nível de participação de alguém em um quadro.
type Papel string

const (
	// PapelDono é quem criou o quadro: pode tudo, inclusive apagá-lo e
	// administrar quem participa.
	PapelDono Papel = "dono"
	// PapelEditor pode mexer no conteúdo do quadro (colunas e cards), mas não
	// administrar o quadro em si.
	PapelEditor Papel = "editor"
	// PapelLeitor só enxerga. É o papel que existe para o padrão ser negar:
	// sem ele, "membro" e "pode editar" seriam a mesma coisa, e a distinção
	// só apareceria no dia em que fosse tarde.
	PapelLeitor Papel = "leitor"
)

var (
	// ErrPapelInvalido é retornado quando o papel não é um dos três conhecidos.
	ErrPapelInvalido = errors.New("papel inválido")
	// ErrSemPermissao é retornado quando o papel do membro não autoriza a operação.
	ErrSemPermissao = errors.New("seu papel neste quadro não permite esta operação")
)

// Membro é o vínculo entre uma pessoa e um quadro.
type Membro struct {
	BoardID   string
	UsuarioID string
	Papel     Papel
	CriadoEm  time.Time
}

// Novo cria um vínculo. Retorna ErrPapelInvalido se o papel não existir.
func Novo(boardID, usuarioID string, papel Papel) (*Membro, error) {
	if !PapelValido(papel) {
		return nil, ErrPapelInvalido
	}
	return &Membro{
		BoardID:   boardID,
		UsuarioID: usuarioID,
		Papel:     papel,
		CriadoEm:  time.Now(),
	}, nil
}

// PapelValido informa se o papel é um dos conhecidos. Papel desconhecido é
// tratado como inválido em vez de como "sem permissão": um valor estranho no
// banco é defeito, não decisão de acesso.
func PapelValido(p Papel) bool {
	return p == PapelDono || p == PapelEditor || p == PapelLeitor
}

// PodeVer informa se o membro pode ler o quadro. Todo papel conhecido pode —
// participar já é a permissão de leitura.
func (m Membro) PodeVer() bool {
	return PapelValido(m.Papel)
}

// PodeEditar informa se o membro pode criar, alterar e apagar colunas e cards.
func (m Membro) PodeEditar() bool {
	return m.Papel == PapelDono || m.Papel == PapelEditor
}

// PodeAdministrar informa se o membro pode renomear e apagar o quadro, e — a
// partir da fase 3 — convidar e remover gente. Só o dono.
func (m Membro) PodeAdministrar() bool {
	return m.Papel == PapelDono
}
