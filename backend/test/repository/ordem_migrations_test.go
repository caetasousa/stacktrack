//go:build integracao

package repository_test

import (
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)

var numeroDaVersao = regexp.MustCompile(`^V(\d+)__`)

// ordenarPorVersao coloca as migrations na ordem em que o Flyway as aplicaria.
//
// A ordem alfabética NÃO serve: nela V10 vem antes de V2, e a tabela de cards
// seria criada depois da coluna que a referencia. É o mesmo motivo pelo qual o
// Flyway compara versões como números, e não como texto.
func ordenarPorVersao(caminhos []string) {
	sort.Slice(caminhos, func(i, j int) bool {
		return versao(caminhos[i]) < versao(caminhos[j])
	})
}

func versao(caminho string) int {
	achado := numeroDaVersao.FindStringSubmatch(filepath.Base(caminho))
	if len(achado) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(achado[1])
	return n
}
