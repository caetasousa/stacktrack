SHELL := /bin/bash

.PHONY: test test-fast

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
