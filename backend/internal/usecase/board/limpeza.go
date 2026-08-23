package board

import (
	"context"
	"time"
)

// A limpeza do VOLUME quando algo é apagado.
//
// O `ON DELETE CASCADE` do schema limpa a tabela `anexos` e não toca no disco.
// O resultado era um vazamento silencioso: apagar um card removia as linhas dos
// anexos dele e deixava os arquivos no volume para sempre, sem nenhuma linha
// que os referenciasse — invisível para quem usa, e crescendo.
//
// A ordem importa e é sempre a mesma:
//
//  1. COLETAR os caminhos antes do DELETE — depois dele não há de onde tirá-los;
//  2. REGISTRAR a exclusão no outbox, na mesma transação;
//  3. apagar.
//
// O terceiro passo NÃO é mais "descartar os arquivos". Ver
// registrarExclusaoDeArquivos.
//

// registrarExclusaoDeArquivos grava no OUTBOX as chaves físicas que a mutação
// vai deixar órfãs, dentro da MESMA transação que as apaga do banco.
//
// Ele substituiu o descarte imediato, e a troca é a diferença entre uma
// exclusão recuperável e uma irreversível. Antes, os bytes saíam do disco logo
// depois do commit: se o backup mais recente fosse anterior à exclusão, a
// restauração trazia de volta uma linha cujo arquivo já não existia em lugar
// nenhum — anexo que aparece na tela e não abre, para sempre, sem conserto.
//
// Agora o arquivo fica onde está, e quem o remove é o worker — só depois de um
// backup externo comprovar que aquela exclusão está coberta. Ver
// usecase/board/exclusao.go.
//
// Chamado ANTES do DELETE que dispara o CASCADE: depois dele as linhas de
// `anexos` já foram, e não há de onde tirar os caminhos.
func registrarExclusaoDeArquivos(ctx context.Context, e Escrita, boardID string, caminhos []string) error {
	if len(caminhos) == 0 {
		return nil
	}
	if e.Exclusoes == nil {
		// Sem o outbox ligado — o caso dos testes de regra, que não têm banco —
		// não há onde registrar. O arquivo simplesmente fica no disco: é o
		// resultado seguro, e nunca a remoção silenciosa.
		return nil
	}
	return e.Exclusoes.Registrar(ctx, boardID, caminhos, time.Now())
}
