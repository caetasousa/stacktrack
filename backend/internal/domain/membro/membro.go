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
	// ErrSemDono é retornado quando remover alguém, ou rebaixar seu papel,
	// deixaria o quadro sem dono nenhum.
	ErrSemDono = errors.New("o quadro precisa continuar com ao menos um dono")
	// ErrNaoEMembro é retornado quando a pessoa alvo da operação não participa
	// do quadro.
	ErrNaoEMembro = errors.New("esta pessoa não participa do quadro")
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

// PodeAdministrar informa se o membro pode renomear e apagar o quadro, além de
// convidar, remover e trocar o papel de quem participa. Só o dono.
func (m Membro) PodeAdministrar() bool {
	return m.Papel == PapelDono
}

// ValidarRemocao checa se tirar alvoID do quadro é permitido, dada a lista de
// quem participa hoje.
//
// Um quadro sem dono fica órfão: ninguém pode mais convidar, renomear nem
// apagá-lo, e nem o administrador do sistema — que não existe aqui — teria como
// consertar. É por isso que o último dono não sai; ele precisa promover outra
// pessoa antes.
func ValidarRemocao(todos []Membro, alvoID string) error {
	alvo, encontrado := buscar(todos, alvoID)
	if !encontrado {
		return ErrNaoEMembro
	}
	if alvo.Papel == PapelDono && contarDonos(todos) == 1 {
		return ErrSemDono
	}
	return nil
}

// ValidarTrocaDePapel checa se mudar o papel de alvoID é permitido. Rebaixar o
// último dono cai na mesma regra da remoção, pelo mesmo motivo.
func ValidarTrocaDePapel(todos []Membro, alvoID string, novo Papel) error {
	if !PapelValido(novo) {
		return ErrPapelInvalido
	}
	alvo, encontrado := buscar(todos, alvoID)
	if !encontrado {
		return ErrNaoEMembro
	}
	if alvo.Papel == PapelDono && novo != PapelDono && contarDonos(todos) == 1 {
		return ErrSemDono
	}
	return nil
}

func buscar(todos []Membro, usuarioID string) (Membro, bool) {
	for _, m := range todos {
		if m.UsuarioID == usuarioID {
			return m, true
		}
	}
	return Membro{}, false
}

func contarDonos(todos []Membro) int {
	total := 0
	for _, m := range todos {
		if m.Papel == PapelDono {
			total++
		}
	}
	return total
}

// DefinirPapel troca o papel do membro. A validação de que a troca é permitida
// é de ValidarTrocaDePapel, que precisa enxergar o quadro inteiro.
func (m *Membro) DefinirPapel(papel Papel) error {
	if !PapelValido(papel) {
		return ErrPapelInvalido
	}
	m.Papel = papel
	return nil
}
