package board

import (
	"context"
	"log/slog"
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
//  2. apagar;
//  3. só então descartar os arquivos.
//
// Descartar antes do DELETE deixaria o registro apontando para um arquivo que
// já não existe se a transação falhasse, o que é pior que lixo: é anexo quebrado
// numa tela que continua mostrando o link para baixá-lo.

// descartarArquivos apaga do volume os arquivos informados, sem interromper o
// fluxo. Se a limpeza falhar, sobra lixo no disco — chato, e melhor que derrubar
// uma operação que já deu certo do ponto de vista de quem pediu. O log é o que
// dá chance de alguém varrer isso um dia.
func descartarArquivos(ctx context.Context, armazem armazemDeArquivos, caminhos []string) {
	for _, caminho := range caminhos {
		if err := armazem.Remover(caminho); err != nil {
			slog.WarnContext(ctx, "anexo órfão no armazém",
				slog.String("caminho", caminho), slog.String("erro", err.Error()))
		}
	}
}
