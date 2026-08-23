// A exclusão recuperável de arquivos: o outbox e a porta de cobertura.
//
// O problema: apagar um card apaga a linha do anexo (CASCADE) e o processo
// apaga o arquivo do disco em seguida. As duas coisas não são a mesma transação
// — o filesystem não participa do commit — e o segundo passo é IRREVERSÍVEL. Se
// o backup mais recente for anterior à exclusão, a restauração traz de volta uma
// linha cujo arquivo já não existe em lugar nenhum: o anexo aparece na tela e
// não abre, para sempre.
//
// A saída é registrar a exclusão na mesma transação do CASCADE e só remover os
// bytes quando um backup externo COMPROVAR que aquela exclusão está coberta.

package board

import (
	"context"
	"time"
)

// ExclusaoDeArquivo é uma remoção de domínio já confirmada cujo arquivo ainda
// está no disco.
type ExclusaoDeArquivo struct {
	ID      int64
	Caminho string
	BoardID string
	// ExcluidoEm é o instante que o backup precisa cobrir.
	ExcluidoEm time.Time
	Tentativas int
}

// RepositorioExclusaoDeArquivo é o outbox das exclusões.
//
// `Registrar` participa da transação da mutação — é por isso que ele está em
// Escrita. Os demais são do worker, que roda fora de qualquer requisição.
type RepositorioExclusaoDeArquivo interface {
	// Registrar grava as chaves físicas afetadas. Chamado ANTES do DELETE que
	// dispara o CASCADE, dentro da mesma transação: depois dele as linhas de
	// `anexos` já não existem e não há de onde tirar os caminhos.
	Registrar(ctx context.Context, boardID string, caminhos []string, em time.Time) error
	// Pendentes devolve exclusões COBERTAS, ainda no disco e na hora de tentar.
	Pendentes(ctx context.Context, agora time.Time, limite int) ([]ExclusaoDeArquivo, error)
	// MarcarRemovido registra que os bytes saíram do disco.
	MarcarRemovido(ctx context.Context, id int64, em time.Time) error
	// AdiarComErro registra a falha e agenda a próxima tentativa.
	AdiarComErro(ctx context.Context, id int64, erro string, proximaEm time.Time) error
	// MarcarCobertos marca como cobertos os IDs EXATOS comprovados por um
	// snapshot externo bem-sucedido.
	MarcarCobertos(ctx context.Context, ids []int64, em time.Time) error
	// SemCobertura devolve exclusões que ainda não têm prova — é o que a
	// métrica de A8 publica e o que o backup de A6 vai comprovar.
	SemCobertura(ctx context.Context, limite int) ([]ExclusaoDeArquivo, error)
}

// PortaDeCobertura responde se uma exclusão está comprovadamente num snapshot
// externo bem-sucedido.
//
// ⚠️ A pergunta é sobre o ID EXATO, e não sobre tempo. Timestamp, relógio do
// host ou `max(id)` NÃO servem de prova, e a razão é concreta: o dump do
// PostgreSQL e a cópia dos arquivos acontecem em instantes diferentes, o relógio
// do VPS pode andar para trás, e uma exclusão gravada depois do dump pode ter um
// id menor que outra gravada antes. Só a lista literal dos ids que o backup
// levou responde a pergunta sem margem.
//
// Quem a implementa de verdade é A6, lendo os manifests remotos validados. Até
// lá vale CoberturaNegada, e o worker não remove nada.
type PortaDeCobertura interface {
	// Cobertos devolve, dentre os ids informados, quais estão comprovadamente
	// num snapshot externo. Devolver a lista vazia é a resposta correta quando
	// não há prova — nunca um erro silencioso.
	Cobertos(ctx context.Context, ids []int64) ([]int64, error)
}

// CoberturaNegada é a porta de cobertura enquanto A6 não existe.
//
// Ela responde SEMPRE que nada está coberto, e é o que põe produção em
// fail-closed: o outbox acumula e nenhum byte sai do disco. É a decisão certa —
// disco é barato, e um anexo apagado cedo demais não volta.
//
// É um tipo com nome, e não um `nil` tratado como "desligado", porque o
// comportamento precisa ser uma escolha visível no wiring. Um nil silencioso
// seria indistinguível de esquecimento.
type CoberturaNegada struct{}

// Cobertos nunca cobre nada.
func (CoberturaNegada) Cobertos(context.Context, []int64) ([]int64, error) {
	return nil, nil
}
