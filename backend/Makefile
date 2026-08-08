SHELL := /bin/bash

.PHONY: test test-fast test-tempo-real

# Roda todos os testes e mostra, de forma legível, cada caso que passou ou
# falhou. Falha o alvo (exit != 0) se qualquer teste quebrar, para não mascarar
# erros.
#
# `-race`: a API serve requisições concorrentes, e corrida de dados não quebra o
# teste sem o detector — só aparece em produção sob carga. Da fase 5 em diante,
# quando o hub de WebSocket entra, este alvo deixa de ser opcional.
test:
	@set -o pipefail; go test -race -v ./... 2>&1 \
		| grep -E -- '--- (PASS|FAIL)' \
		| sed -E 's/.*--- (PASS|FAIL): /\1 /; s/ \(.*\)//; s@/@ › @; s/_/ /g'

# Roda só os testes rápidos (em memória, sem Docker).
test-fast:
	@go test ./...

# O teste de duas abas da fase 5: exige a API NO AR, porque exercita o
# handshake de verdade — cookie de sessão real, checagem de origem, evento
# atravessando o hub. Fica atrás de build tag para não quebrar o `go test ./...`
# de quem não subiu a stack.
#
#   make run              # noutro terminal, na raiz do repositório
#   make -C backend test-tempo-real
test-tempo-real:
	@go test -race -tags tempo_real -v ./test/realtime/ 2>&1 \
		| grep -E -- '--- (PASS|FAIL)' \
		| sed -E 's/.*--- (PASS|FAIL): /\1 /; s/ \(.*\)//; s/_/ /g'
