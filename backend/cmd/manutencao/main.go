// Package main é o comando de manutenção do banco.
//
// Hoje ele faz uma coisa só: reparar duplicidade histórica de chave de
// ordenação, que é a pré-condição do contract `UNIQUE (coluna_id, chave)` /
// `UNIQUE (board_id, chave)` descrito em migrations/README.md.
//
// É um BINÁRIO SEPARADO, e não uma rota escondida da API, por três razões:
//
//  1. ele reescreve dado em massa — não é operação que deva ficar alcançável
//     por HTTP, nem por acidente nem por credencial vazada;
//  2. ele precisa rodar como parte de um deploy, antes de uma migration, e
//     terminar com código de saída — um endpoint não se encaixa num pipeline;
//  3. o relatório é a evidência de que a pré-condição foi satisfeita, e
//     evidência se lê na saída de um comando, não num log de requisição.
//
// Uso:
//
//	manutencao reparar-ordenacao [--conferir]
//
// `--conferir` apenas RELATA, sem escrever nada. É o modo de rodar antes da
// janela de manutenção para saber se há trabalho — e para descobrir o tamanho
// dele sem ainda tomar lock de quadro nenhum.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"stacktrack/config"
	"stacktrack/internal/adapter/repository"
	ucboard "stacktrack/internal/usecase/board"
)

// origemDoReparo identifica este comando no payload do evento e na auditoria.
//
// Não é um usuário, e o autor do evento fica vazio: nenhuma pessoa arrastou
// nada, e `board_events.autor_id` é UUID de conta. Ver
// ucboard.DadosDaManutencao.
const origemDoReparo = "reparo-de-ordenacao"

func main() {
	if len(os.Args) < 2 {
		usar()
	}

	switch os.Args[1] {
	case "reparar-ordenacao":
		os.Exit(repararOrdenacao(os.Args[2:]))
	default:
		fmt.Fprintf(os.Stderr, "comando desconhecido: %s\n\n", os.Args[1])
		usar()
	}
}

func usar() {
	fmt.Fprint(os.Stderr, `uso: manutencao <comando>

comandos:
  reparar-ordenacao [--conferir]
        Repara duplicidade de chave de ordenação, quadro a quadro, sob o lock
        de cada um. Com --conferir apenas relata, sem escrever.

O comando lê a configuração do banco das mesmas variáveis DB_* da API.
`)
	os.Exit(2)
}

func repararOrdenacao(args []string) int {
	fs := flag.NewFlagSet("reparar-ordenacao", flag.ExitOnError)
	conferir := fs.Bool("conferir", false, "apenas relatar, sem escrever nada")
	_ = fs.Parse(args)

	// O sinal encerra o comando entre um quadro e outro: cada quadro é uma
	// transação própria, então interromper aqui não deixa reparo pela metade.
	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()

	pool, err := config.NovoPool(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro ao conectar no banco: %v\n", err)
		return 1
	}
	defer pool.Close()

	duplicidades := repository.NovoDuplicidadePostgres(pool)

	if *conferir {
		achadas, err := duplicidades.DuplicidadesDeChave(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "erro ao consultar duplicidades: %v\n", err)
			return 1
		}
		imprimirDuplicidades("encontradas", achadas)
		if len(achadas) == 0 {
			fmt.Println("\nnada a reparar: o contract de unicidade pode ser aplicado.")
			return 0
		}
		fmt.Printf("\n%d contêiner(es) precisam de reparo; rode sem --conferir.\n", len(achadas))
		// Código 1 no modo conferir significa "há trabalho", e é o que permite
		// um pipeline decidir sozinho se precisa rodar o reparo.
		return 1
	}

	// SEM publicador: este processo não tem hub, e nenhuma aba está ligada nele.
	//
	// A entrega ao vivo do reparo fica de fora por isso, e não por descuido —
	// quem reconecta recebe a mudança pelo replay por revisão, que é o caminho
	// correto e já existe. Publicar daqui exigiria um canal entre processos, que
	// é justamente o dispatcher que A3 ainda não tem.
	reparo := ucboard.NovoReparoDeOrdenacao(duplicidades, repository.NovaUnidadeDeTrabalho(
		pool, config.EsperaPorLockDeQuadro(), config.TempoMaximoDeComando(),
	))

	relatorio, err := reparo.Executar(ctx, origemDoReparo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro no reparo: %v\n", err)
		imprimirRelatorio(relatorio)
		return 1
	}

	imprimirRelatorio(relatorio)
	if !relatorio.Limpo() {
		fmt.Fprintln(os.Stderr, "\nAINDA HÁ DUPLICIDADE: o contract de unicidade NÃO pode ser aplicado.")
		return 1
	}
	fmt.Println("\nbanco limpo: o contract de unicidade pode ser aplicado.")
	return 0
}

func imprimirRelatorio(r ucboard.RelatorioDeReparo) {
	imprimirDuplicidades("encontradas antes do reparo", r.Encontradas)
	if len(r.QuadrosReparados) > 0 {
		fmt.Printf("\nquadros reparados (%d):\n", len(r.QuadrosReparados))
		for _, id := range r.QuadrosReparados {
			fmt.Printf("  %s\n", id)
		}
	}
	imprimirDuplicidades("restantes depois do reparo", r.Restantes)
}

func imprimirDuplicidades(titulo string, duplicidades []ucboard.Duplicidade) {
	fmt.Printf("\n%s (%d):\n", titulo, len(duplicidades))
	for _, d := range duplicidades {
		fmt.Printf("  %s — chave %q em %d linhas\n", d.Container(), d.Chave, d.Ocorrencias)
	}
}
